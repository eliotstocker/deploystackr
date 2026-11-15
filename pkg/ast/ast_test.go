package ast

import (
	"strings"
	"testing"
)

func TestYAMLMutationPreservesCommentsAndFormatting(t *testing.T) {
	origYAML := `# Service deployment configuration
image:
  repository: ghcr.io/org/billing-service
  # Active tag deployed via GitOps
  tag: v1.0.0 # inline comment
replicaCount: 3
`
	val, err := ExtractYAMLValue([]byte(origYAML), "$.image.tag")
	if err != nil {
		t.Fatalf("ExtractYAMLValue failed: %v", err)
	}
	if val != "v1.0.0" {
		t.Fatalf("expected v1.0.0, got %s", val)
	}

	mutated, err := MutateYAML([]byte(origYAML), "$.image.tag", "v1.2.0")
	if err != nil {
		t.Fatalf("MutateYAML failed: %v", err)
	}

	mutatedStr := string(mutated)
	if !strings.Contains(mutatedStr, "# Service deployment configuration") {
		t.Errorf("lost header comment")
	}
	if !strings.Contains(mutatedStr, "# Active tag deployed via GitOps") {
		t.Errorf("lost middle comment")
	}
	if !strings.Contains(mutatedStr, "tag: v1.2.0") {
		t.Errorf("expected tag: v1.2.0, got:\n%s", mutatedStr)
	}
	if !strings.Contains(mutatedStr, "replicaCount: 3") {
		t.Errorf("lost replicaCount")
	}
}

func TestJSONMutationPreservesFormatting(t *testing.T) {
	origJSON := `{
  "apiVersion": "apps/v1",
  "kind": "Deployment",
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "billing",
            "image": "ghcr.io/org/billing-service:v1.0.0"
          }
        ]
      }
    }
  }
}`

	path := "$.spec.template.spec.containers[0].image"
	val, err := ExtractJSONValue([]byte(origJSON), path)
	if err != nil {
		t.Fatalf("ExtractJSONValue failed: %v", err)
	}
	if val != "ghcr.io/org/billing-service:v1.0.0" {
		t.Fatalf("expected image string, got %s", val)
	}

	newImage := "ghcr.io/org/billing-service:v1.3.5"
	mutated, err := MutateJSON([]byte(origJSON), path, newImage)
	if err != nil {
		t.Fatalf("MutateJSON failed: %v", err)
	}

	mutatedStr := string(mutated)
	if !strings.Contains(mutatedStr, `"image": "ghcr.io/org/billing-service:v1.3.5"`) {
		t.Errorf("expected new image in mutated JSON, got:\n%s", mutatedStr)
	}
	// Verify indentation was not re-formatted or messed up
	if !strings.Contains(mutatedStr, `  "apiVersion": "apps/v1",`) {
		t.Errorf("indentation damaged")
	}
}

func TestMultiDocYAMLMutation(t *testing.T) {
	orig := `---
apiVersion: v1
kind: Namespace
metadata:
  name: ingress-nginx
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: ingress-nginx
spec:
  chart:
    spec:
      version: 4.10.0
`

	val, err := ExtractYAMLValue([]byte(orig), "$.spec.chart.spec.version")
	if err != nil {
		t.Fatalf("ExtractYAMLValue failed: %v", err)
	}
	if val != "4.10.0" {
		t.Fatalf("expected 4.10.0, got %s", val)
	}

	mutated, err := MutateYAML([]byte(orig), "$.spec.chart.spec.version", "4.11.3")
	if err != nil {
		t.Fatalf("MutateYAML failed: %v", err)
	}

	mutatedStr := string(mutated)
	if !strings.Contains(mutatedStr, "version: 4.11.3") {
		t.Errorf("expected updated version, got:\n%s", mutatedStr)
	}
	if !strings.Contains(mutatedStr, "name: ingress-nginx") {
		t.Errorf("lost namespace document")
	}
}

func TestRFC9535YAMLFilterExpression(t *testing.T) {
	manifest := `# Kustomize image configuration
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
images:
  - name: ghcr.io/org/auth-service
    newTag: v1.1.0 # auth tag
  - name: ghcr.io/org/api-service
    # Main API deployment version
    newTag: v1.4.0
  - name: ghcr.io/org/worker-service
    newTag: v0.9.5
`

	// Test extraction using RFC 9535 query
	path := "$.images[?@.name == 'ghcr.io/org/api-service'].newTag"
	val, err := ExtractYAMLValue([]byte(manifest), path)
	if err != nil {
		t.Fatalf("ExtractYAMLValue failed: %v", err)
	}
	if val != "v1.4.0" {
		t.Fatalf("expected v1.4.0, got %s", val)
	}

	// Test mutation with comments preserved
	mutated, err := MutateYAML([]byte(manifest), path, "v1.5.2")
	if err != nil {
		t.Fatalf("MutateYAML failed: %v", err)
	}

	mutatedStr := string(mutated)
	if !strings.Contains(mutatedStr, "newTag: v1.5.2") {
		t.Errorf("expected newTag: v1.5.2 in mutated YAML, got:\n%s", mutatedStr)
	}
	if !strings.Contains(mutatedStr, "# Main API deployment version") {
		t.Errorf("lost inline comment")
	}
	if !strings.Contains(mutatedStr, "newTag: v1.1.0") {
		t.Errorf("accidentally changed auth-service")
	}

	// Test loose shorthand syntax [name=...]
	shorthandPath := "images[name=ghcr.io/org/worker-service].newTag"
	valWorker, err := ExtractYAMLValue([]byte(manifest), shorthandPath)
	if err != nil {
		t.Fatalf("ExtractYAMLValue with shorthand failed: %v", err)
	}
	if valWorker != "v0.9.5" {
		t.Fatalf("expected v0.9.5, got %s", valWorker)
	}
}

func TestRFC9535JSONFilterExpression(t *testing.T) {
	doc := `{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "istio-proxy",
            "image": "docker.io/istio/proxyv2:1.18.0"
          },
          {
            "name": "billing-api",
            "image": "ghcr.io/org/billing:v2.0.0"
          }
        ]
      }
    }
  }
}`

	path := "$.spec.template.spec.containers[?@.name == 'billing-api'].image"
	val, err := ExtractJSONValue([]byte(doc), path)
	if err != nil {
		t.Fatalf("ExtractJSONValue with filter failed: %v", err)
	}
	if val != "ghcr.io/org/billing:v2.0.0" {
		t.Fatalf("expected billing:v2.0.0, got %s", val)
	}

	mutated, err := MutateJSON([]byte(doc), path, "ghcr.io/org/billing:v2.1.0")
	if err != nil {
		t.Fatalf("MutateJSON failed: %v", err)
	}

	mutatedStr := string(mutated)
	if !strings.Contains(mutatedStr, `"image": "ghcr.io/org/billing:v2.1.0"`) {
		t.Errorf("expected updated image in JSON, got:\n%s", mutatedStr)
	}
	if !strings.Contains(mutatedStr, `"image": "docker.io/istio/proxyv2:1.18.0"`) {
		t.Errorf("accidentally changed sidecar container image")
	}
}

func TestNegativeIndexSupport(t *testing.T) {
	yamlDoc := `
items:
  - first
  - middle
  - last
`
	val, err := ExtractYAMLValue([]byte(yamlDoc), "$.items[-1]")
	if err != nil {
		t.Fatalf("Extract negative index failed: %v", err)
	}
	if val != "last" {
		t.Fatalf("expected 'last', got %s", val)
	}

	jsonDoc := `{"items": ["apple", "banana", "cherry"]}`
	valJSON, err := ExtractJSONValue([]byte(jsonDoc), "$.items[-1]")
	if err != nil {
		t.Fatalf("Extract JSON negative index failed: %v", err)
	}
	if valJSON != "cherry" {
		t.Fatalf("expected 'cherry', got %s", valJSON)
	}
}
