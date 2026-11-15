package webhook

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"deploystackr/pkg/config"
)

// Standard Event names for webhook triggers.
const (
	EventDeploymentRequested = "deployment_requested"
	EventDeploymentCompleted = "deployment_completed"
	EventNewVersionAvailable = "new_version_available"
	EventPRMerged            = "pr_merged"
	EventAll                 = "*"
)

// DeploymentRequestedEvent payload sent when a user triggers a promotion.
type DeploymentRequestedEvent struct {
	Event         string    `json:"event"`
	Service       string    `json:"service"`
	Environment   string    `json:"environment"`
	TargetVersion string    `json:"target_version"`
	OldVersion    string    `json:"old_version,omitempty"`
	User          string    `json:"user"`
	Email         string    `json:"email,omitempty"`
	Mode          string    `json:"mode"`
	Comment       string    `json:"comment,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// DeploymentCompletedEvent payload sent after a promotion succeeds.
type DeploymentCompletedEvent struct {
	Event         string    `json:"event"`
	Service       string    `json:"service"`
	Environment   string    `json:"environment"`
	TargetVersion string    `json:"target_version"`
	OldVersion    string    `json:"old_version,omitempty"`
	CommitSHA     string    `json:"commit_sha,omitempty"`
	Branch        string    `json:"branch,omitempty"`
	PRNumber      int       `json:"pr_number,omitempty"`
	PRURL         string    `json:"pr_url,omitempty"`
	User          string    `json:"user"`
	Success       bool      `json:"success"`
	Message       string    `json:"message"`
	Timestamp     time.Time `json:"timestamp"`
}

// NewVersionAvailableEvent payload sent when an upstream release or tag is published.
type NewVersionAvailableEvent struct {
	Event       string    `json:"event"`
	Service     string    `json:"service"`
	Version     string    `json:"version"`
	Description string    `json:"description,omitempty"`
	Changelog   string    `json:"changelog,omitempty"`
	ReleasedAt  string    `json:"released_at,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// PRMergedEvent payload sent when a deployment pull request is merged.
type PRMergedEvent struct {
	Event       string    `json:"event"`
	Service     string    `json:"service"`
	Environment string    `json:"environment,omitempty"`
	PRNumber    int       `json:"pr_number"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	MergedBy    string    `json:"merged_by,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// DeliveryRecord captures outgoing webhook execution details for observability and diagnostics.
type DeliveryRecord struct {
	ID         string            `json:"id"`
	Event      string            `json:"event"`
	Service    string            `json:"service,omitempty"`
	Endpoint   string            `json:"endpoint"`
	Method     string            `json:"method"`
	StatusCode int               `json:"status_code"`
	Success    bool              `json:"success"`
	Error      string            `json:"error,omitempty"`
	Payload    string            `json:"payload"`
	DurationMs int64             `json:"duration_ms"`
	Timestamp  time.Time         `json:"timestamp"`
}

// Dispatcher manages sending outgoing webhooks configured globally or per-service.
type Dispatcher struct {
	cfg        *config.Config
	httpClient *http.Client
	mu         sync.RWMutex
	history    []DeliveryRecord
}

// NewDispatcher creates a new webhook dispatcher instance.
func NewDispatcher(cfg *config.Config) *Dispatcher {
	return &Dispatcher{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		history: make([]DeliveryRecord, 0, 50),
	}
}

// UpdateConfig updates the active configuration reference.
func (d *Dispatcher) UpdateConfig(cfg *config.Config) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cfg = cfg
}

// GetHistory returns a copy of recent webhook delivery records (up to 50).
func (d *Dispatcher) GetHistory() []DeliveryRecord {
	d.mu.RLock()
	defer d.mu.RUnlock()
	res := make([]DeliveryRecord, len(d.history))
	copy(res, d.history)
	return res
}

func (d *Dispatcher) recordDelivery(rec DeliveryRecord) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.history) >= 50 {
		d.history = d.history[1:]
	}
	d.history = append(d.history, rec)
}

// Dispatch triggers matching webhooks asynchronously.
func (d *Dispatcher) Dispatch(event string, data interface{}, serviceName string) {
	d.mu.RLock()
	cfg := d.cfg
	d.mu.RUnlock()

	if cfg == nil {
		return
	}

	var targets []config.WebhookConfig

	// 1. Global webhooks
	for _, hook := range cfg.Webhooks {
		if hook.Event == event || hook.Event == EventAll {
			targets = append(targets, hook)
		}
	}

	// 2. Service-specific webhooks
	if serviceName != "" && cfg.Services != nil {
		if svc, ok := cfg.Services[serviceName]; ok {
			for _, hook := range svc.Webhooks {
				if hook.Event == event || hook.Event == EventAll {
					targets = append(targets, hook)
				}
			}
		}
	}

	if len(targets) == 0 {
		return
	}

	// Dispatch each webhook concurrently without blocking caller
	for _, target := range targets {
		go d.sendWebhook(target, event, data, serviceName)
	}
}

// SendDirect synchronously executes a webhook configuration (useful for testing).
func (d *Dispatcher) SendDirect(ctx context.Context, hook config.WebhookConfig, event string, data interface{}, serviceName string) DeliveryRecord {
	return d.executeWebhook(ctx, hook, event, data, serviceName)
}

func (d *Dispatcher) sendWebhook(hook config.WebhookConfig, event string, data interface{}, serviceName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	d.executeWebhook(ctx, hook, event, data, serviceName)
}

func (d *Dispatcher) executeWebhook(ctx context.Context, hook config.WebhookConfig, event string, data interface{}, serviceName string) DeliveryRecord {
	start := time.Now()
	rec := DeliveryRecord{
		ID:        randomID(),
		Event:     event,
		Service:   serviceName,
		Endpoint:  hook.Endpoint,
		Method:    hook.Method,
		Timestamp: start,
	}
	if rec.Method == "" {
		rec.Method = "POST"
	}

	// 1. Render endpoint if templated
	endpoint := hook.Endpoint
	if strings.Contains(endpoint, "{{") {
		tmpl, err := template.New("endpoint").Parse(endpoint)
		if err == nil {
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err == nil {
				endpoint = buf.String()
				rec.Endpoint = endpoint
			}
		}
	}

	if endpoint == "" {
		rec.Error = "empty endpoint url"
		d.recordDelivery(rec)
		return rec
	}

	// 2. Render payload
	var bodyBytes []byte
	if hook.Payload != "" {
		tmpl, err := template.New("payload").Parse(hook.Payload)
		if err != nil {
			rec.Error = fmt.Sprintf("parse payload template: %v", err)
			d.recordDelivery(rec)
			return rec
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			rec.Error = fmt.Sprintf("execute payload template: %v", err)
			d.recordDelivery(rec)
			return rec
		}
		bodyBytes = buf.Bytes()
	} else {
		// Default to JSON encoding of the data object
		var err error
		bodyBytes, err = json.Marshal(data)
		if err != nil {
			rec.Error = fmt.Sprintf("marshal default payload: %v", err)
			d.recordDelivery(rec)
			return rec
		}
	}
	rec.Payload = string(bodyBytes)

	// 3. Build HTTP request
	req, err := http.NewRequestWithContext(ctx, rec.Method, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		rec.Error = fmt.Sprintf("create request: %v", err)
		d.recordDelivery(rec)
		return rec
	}

	// Default Content-Type to application/json
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DeployStackr-Webhook-Dispatcher/1.0")

	// Custom headers
	for k, v := range hook.Headers {
		req.Header.Set(k, v)
	}

	// 4. Execute HTTP call
	resp, err := d.httpClient.Do(req)
	duration := time.Since(start).Milliseconds()
	rec.DurationMs = duration

	if err != nil {
		rec.Error = err.Error()
		log.Printf("[webhook] Error dispatching %s to %s: %v", event, endpoint, err)
		d.recordDelivery(rec)
		return rec
	}
	defer resp.Body.Close()

	rec.StatusCode = resp.StatusCode
	rec.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !rec.Success {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		rec.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))
		log.Printf("[webhook] Failed delivery %s to %s (HTTP %d)", event, endpoint, resp.StatusCode)
	} else {
		log.Printf("[webhook] Successfully sent %s to %s (HTTP %d, %dms)", event, endpoint, resp.StatusCode, duration)
	}

	d.recordDelivery(rec)
	return rec
}

func randomID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
