package ast

import (
	"path/filepath"
	"strings"
)

// ExtractValue inspects YAML or JSON content and extracts the scalar at path.
func ExtractValue(content []byte, filename, path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".json" {
		return ExtractJSONValue(content, path)
	}
	if ext == ".yaml" || ext == ".yml" {
		return ExtractYAMLValue(content, path)
	}

	trimmed := strings.TrimSpace(string(content))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return ExtractJSONValue(content, path)
	}
	return ExtractYAMLValue(content, path)
}

// MutateContent modifies the scalar at path in YAML or JSON, preserving formatting and comments.
func MutateContent(content []byte, filename, path, newValue string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".json" {
		return MutateJSON(content, path, newValue)
	}
	if ext == ".yaml" || ext == ".yml" {
		return MutateYAML(content, path, newValue)
	}

	trimmed := strings.TrimSpace(string(content))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return MutateJSON(content, path, newValue)
	}
	return MutateYAML(content, path, newValue)
}
