package ast

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExtractYAMLValue reads a scalar value at the specified path from YAML content.
func ExtractYAMLValue(content []byte, path string) (string, error) {
	node, err := findYAMLNode(content, path)
	if err != nil {
		return "", err
	}
	return node.Value, nil
}

// MutateYAML updates a scalar value at the specified path, preserving comments and formatting.
func MutateYAML(content []byte, path string, newValue string) ([]byte, error) {
	docs, err := decodeYAMLDocs(content)
	if err != nil {
		return nil, err
	}

	segments, err := ParsePath(path)
	if err != nil {
		return nil, err
	}

	var targetNode *yaml.Node
	var lastErr error
	for _, doc := range docs {
		node, err := resolveNode(doc, segments)
		if err == nil {
			targetNode = node
			break
		}
		lastErr = err
	}

	if targetNode == nil {
		return nil, fmt.Errorf("resolve path %s: %w", path, lastErr)
	}

	if targetNode.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("target at %s is not a scalar node (kind=%d)", path, targetNode.Kind)
	}

	// In-place mutate the scalar value while preserving styles and comments
	targetNode.Value = newValue

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	// Detect indentation: default to 2
	indent := detectYAMLIndent(content)
	enc.SetIndent(indent)

	for _, doc := range docs {
		if err := enc.Encode(doc); err != nil {
			return nil, fmt.Errorf("encode modified yaml: %w", err)
		}
	}
	_ = enc.Close()

	return buf.Bytes(), nil
}

func decodeYAMLDocs(content []byte) ([]*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
		docs = append(docs, &doc)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("empty yaml document")
	}
	return docs, nil
}

func findYAMLNode(content []byte, path string) (*yaml.Node, error) {
	docs, err := decodeYAMLDocs(content)
	if err != nil {
		return nil, err
	}

	segments, err := ParsePath(path)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, doc := range docs {
		node, err := resolveNode(doc, segments)
		if err == nil {
			return node, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("resolve path %s: %w", path, lastErr)
}

func resolveNode(current *yaml.Node, segments []PathSegment) (*yaml.Node, error) {
	if current.Kind == yaml.DocumentNode {
		if len(current.Content) == 0 {
			return nil, fmt.Errorf("empty yaml document")
		}
		return resolveNode(current.Content[0], segments)
	}

	if len(segments) == 0 {
		return current, nil
	}

	seg := segments[0]
	rest := segments[1:]

	switch seg.Type {
	case SegmentField:
		if current.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("expected mapping node, got kind %d for key %s", current.Kind, seg.Field)
		}
		// In mapping nodes, Content is [key1, val1, key2, val2, ...]
		for i := 0; i < len(current.Content); i += 2 {
			keyNode := current.Content[i]
			valNode := current.Content[i+1]
			if keyNode.Value == seg.Field {
				return resolveNode(valNode, rest)
			}
		}
		return nil, fmt.Errorf("key not found: %s", seg.Field)

	case SegmentIndex:
		if current.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("expected sequence node, got kind %d for index %d", current.Kind, seg.Index)
		}
		idx := seg.Index
		if(idx < 0) {
			idx = len(current.Content) + idx
		}
		if idx < 0 || idx >= len(current.Content) {
			return nil, fmt.Errorf("index out of bounds: %d (len=%d)", seg.Index, len(current.Content))
		}
		return resolveNode(current.Content[idx], rest)

	case SegmentFilter:
		if current.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("expected sequence node for filter, got kind %d", current.Kind)
		}
		for _, item := range current.Content {
			if matchYAMLFilter(item, seg.Filter) {
				return resolveNode(item, rest)
			}
		}
		return nil, fmt.Errorf("no item matching filter expression")

	case SegmentWildcard:
		if current.Kind == yaml.SequenceNode {
			for _, item := range current.Content {
				if node, err := resolveNode(item, rest); err == nil {
					return node, nil
				}
			}
		} else if current.Kind == yaml.MappingNode {
			for i := 1; i < len(current.Content); i += 2 {
				if node, err := resolveNode(current.Content[i], rest); err == nil {
					return node, nil
				}
			}
		}
		return nil, fmt.Errorf("wildcard matched no elements")
	}

	return nil, fmt.Errorf("unsupported segment type")
}

func matchYAMLFilter(node *yaml.Node, expr *FilterExpr) bool {
	if(node == nil || expr == nil || len(expr.Predicates) < 1) {
		return false
	}

	for _, pred := range expr.Predicates {
		matched := evalYAMLPredicate(node, pred)
		if(expr.IsOr && matched) {
			return true
		}
		if(!expr.IsOr && !matched) {
			return false
		}
	}
	return !expr.IsOr
}

func evalYAMLPredicate(node *yaml.Node, pred FilterPredicate) bool {
	val, exists := extractYAMLSubpathValue(node, pred.SubPath)
	if(pred.Op == OpExists) {
		if(pred.Negated) {
			return !exists || val == "" || val == "null" || val == "false"
		}
		return exists && val != "" && val != "null" && val != "false"
	}
	if(!exists) {
		return false
	}

	switch pred.Op {
	case OpEqual:
		return val == pred.Value
	case OpNotEqual:
		return val != pred.Value
	case OpLess:
		num1, err1 := strconv.ParseFloat(val, 64)
		num2, err2 := strconv.ParseFloat(pred.Value, 64)
		if(err1 == nil && err2 == nil) {
			return num1 < num2
		}
		return val < pred.Value
	case OpLessEqual:
		num1, err1 := strconv.ParseFloat(val, 64)
		num2, err2 := strconv.ParseFloat(pred.Value, 64)
		if(err1 == nil && err2 == nil) {
			return num1 <= num2
		}
		return val <= pred.Value
	case OpGreater:
		num1, err1 := strconv.ParseFloat(val, 64)
		num2, err2 := strconv.ParseFloat(pred.Value, 64)
		if(err1 == nil && err2 == nil) {
			return num1 > num2
		}
		return val > pred.Value
	case OpGreaterEqual:
		num1, err1 := strconv.ParseFloat(val, 64)
		num2, err2 := strconv.ParseFloat(pred.Value, 64)
		if(err1 == nil && err2 == nil) {
			return num1 >= num2
		}
		return val >= pred.Value
	default:
		return val == pred.Value
	}
}

func extractYAMLSubpathValue(node *yaml.Node, subpath []string) (string, bool) {
	if(node == nil) {
		return "", false
	}
	curr := node
	for _, p := range subpath {
		if(curr.Kind != yaml.MappingNode) {
			return "", false
		}
		found := false
		for i := 0; i < len(curr.Content); i += 2 {
			if(curr.Content[i].Value == p) {
				curr = curr.Content[i+1]
				found = true
				break
			}
		}
		if(!found) {
			return "", false
		}
	}
	if(curr.Kind == yaml.ScalarNode) {
		return curr.Value, true
	}
	return "", true
}

// detectYAMLIndent determines whether 2, 4 or custom spaces are primarily used.
func detectYAMLIndent(content []byte) int {
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     ") {
			return 4
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			return 2
		}
	}
	return 2
}
