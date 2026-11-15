package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"deploystackr/pkg/auth"
	"deploystackr/pkg/cluster"
	"deploystackr/pkg/config"
	"deploystackr/pkg/deployer"
	"deploystackr/pkg/indexer"
	"deploystackr/pkg/provider"
	"deploystackr/pkg/store"
	"deploystackr/pkg/webhook"
)

const (
	defaultProtocolVersion = "2025-03-26"
	maxBodyBytes           = 4 << 20
)

var supportedProtocolVersions = map[string]bool{
	"2025-06-18": true,
	"2025-03-26": true,
	"2024-11-05": true,
}

type Deps struct {
	Config         *config.Config
	Store          *store.MemoryStore
	Indexer        *indexer.Indexer
	Deployer       *deployer.Manager
	Dispatcher     *webhook.Dispatcher
	Auth           *auth.AuthManager
	ClusterChecker func() *cluster.Checker
}

type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Annotations *ToolAnnotations       `json:"annotations,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type toolHandler func(context.Context, json.RawMessage) CallToolResult

type Server struct {
	deps     Deps
	tools    []Tool
	handlers map[string]toolHandler

	contextVersionsMu    sync.Mutex
	contextVersionsCache map[string][]provider.VersionInfo
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type authContextKey struct{}

type requestAuth struct {
	user          *auth.User
	forwardedUser string
}

func withRequestAuth(ctx context.Context, ra requestAuth) context.Context {
	return context.WithValue(ctx, authContextKey{}, ra)
}

func authFromContext(ctx context.Context) requestAuth {
	ra, _ := ctx.Value(authContextKey{}).(requestAuth)
	return ra
}

func NewServer(deps Deps) *Server {
	s := &Server{
		deps:                 deps,
		handlers:             make(map[string]toolHandler),
		contextVersionsCache: make(map[string][]provider.VersionInfo),
	}
	s.registerTools()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Allow", "POST, DELETE, OPTIONS")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute))

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if len(body) > maxBodyBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		var user *auth.User
		if s.deps.Auth != nil {
			user = s.deps.Auth.GetUserFromDeploymentRequest(r)
		}

		ctx := withRequestAuth(r.Context(), requestAuth{
			user:          user,
			forwardedUser: r.Header.Get("X-Forwarded-User"),
		})

		respBytes, hasResp := s.HandleMessage(ctx, body)
		if !hasResp {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	default:
		w.Header().Set("Allow", "POST, DELETE, OPTIONS")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) HandleMessage(ctx context.Context, payload []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) < 1 {
		b, _ := json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &rpcError{Code: -32700, Message: "Parse error: empty message"},
		})
		return b, true
	}

	if trimmed[0] == '[' {
		var rawBatch []json.RawMessage
		if err := json.Unmarshal(trimmed, &rawBatch); err != nil || len(rawBatch) < 1 {
			b, _ := json.Marshal(rpcResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage("null"),
				Error:   &rpcError{Code: -32600, Message: "Invalid Request"},
			})
			return b, true
		}

		responses := make([]*rpcResponse, 0, len(rawBatch))
		for _, rawItem := range rawBatch {
			if resp, hasResp := s.handleSingle(ctx, rawItem); hasResp && resp != nil {
				responses = append(responses, resp)
			}
		}
		if len(responses) < 1 {
			return nil, false
		}
		b, _ := json.Marshal(responses)
		return b, true
	}

	resp, hasResp := s.handleSingle(ctx, trimmed)
	if !hasResp || resp == nil {
		return nil, false
	}
	b, _ := json.Marshal(resp)
	return b, true
}

func (s *Server) handleSingle(ctx context.Context, raw []byte) (*rpcResponse, bool) {
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &rpcError{Code: -32700, Message: "Parse error: " + err.Error()},
		}, true
	}

	id := bytes.TrimSpace(req.ID)
	isNotification := len(id) < 1

	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		respID := id
		if len(respID) < 1 {
			respID = json.RawMessage("null")
		}
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      respID,
			Error:   &rpcError{Code: -32600, Message: "Invalid Request"},
		}, true
	}

	if isNotification {
		return nil, false
	}

	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(bytes.TrimSpace(req.Params)) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}
		protoVer := defaultProtocolVersion
		if supportedProtocolVersions[params.ProtocolVersion] {
			protoVer = params.ProtocolVersion
		}
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"protocolVersion": protoVer,
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{
						"listChanged": false,
					},
				},
				"serverInfo": map[string]interface{}{
					"name":    "deploystackr",
					"version": "1.0.0",
				},
				"instructions": "DeployStackr is a GitOps promotion dashboard and version promotion trigger. Use list_services for lightweight discovery of configured services, get_deployed to inspect active GitOps-declared versions, get_history for paginated promotion audit history, get_deploy_queue to inspect open promotion PRs and unpromoted upstream candidate versions, get_version_diff to compare commits between versions, and promote to trigger a GitOps promotion via direct commit or Pull Request. Tools allow choosing a target by service name ('service') or context repository ('context_repo').",
			},
		}, true

	case "ping":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result:  map[string]interface{}{},
		}, true

	case "resources/list":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"resources": []interface{}{},
			},
		}, true

	case "prompts/list":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"prompts": []interface{}{},
			},
		}, true

	case "tools/list":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"tools": s.tools,
			},
		}, true

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
			return &rpcResponse{
				JSONRPC: "2.0",
				ID:      id,
				Error:   &rpcError{Code: -32602, Message: "Invalid params: tool name is required"},
			}, true
		}

		handler, ok := s.handlers[params.Name]
		if !ok {
			return &rpcResponse{
				JSONRPC: "2.0",
				ID:      id,
				Error:   &rpcError{Code: -32602, Message: fmt.Sprintf("unknown tool: %s", params.Name)},
			}, true
		}

		args := bytes.TrimSpace(params.Arguments)
		if len(args) < 1 || bytes.Equal(args, []byte("null")) {
			args = []byte("{}")
		}

		result := handler(ctx, args)
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result:  result,
		}, true

	default:
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &rpcError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)},
		}, true
	}
}

func (s *Server) getClusterChecker() *cluster.Checker {
	if s.deps.ClusterChecker == nil {
		return nil
	}
	return s.deps.ClusterChecker()
}
