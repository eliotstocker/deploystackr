package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSecretOrEnv(t *testing.T) {
	// 1. Env var direct
	t.Setenv("TEST_SECRET", "super-secret-123")
	val := ReadSecretOrEnv("TEST_SECRET", "TEST_SECRET_FILE")
	if val != "super-secret-123" {
		t.Fatalf("expected super-secret-123, got %s", val)
	}

	// 2. Secret file via fileEnvVar
	tmpDir := t.TempDir()
	secretFile := filepath.Join(tmpDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("file-secret-456\n"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SECRET", "")
	t.Setenv("TEST_SECRET_FILE", secretFile)
	valFromFile := ReadSecretOrEnv("TEST_SECRET", "TEST_SECRET_FILE")
	if valFromFile != "file-secret-456" {
		t.Fatalf("expected file-secret-456, got %s", valFromFile)
	}

	// 3. Fallback candidate path
	t.Setenv("TEST_SECRET_FILE", "")
	valFromCandidate := ReadSecretOrEnv("TEST_SECRET", "TEST_SECRET_FILE", secretFile)
	if valFromCandidate != "file-secret-456" {
		t.Fatalf("expected file-secret-456 from candidate, got %s", valFromCandidate)
	}
}

func TestGetRegistryAuth(t *testing.T) {
	tmpDir := t.TempDir()
	dockerConfig := filepath.Join(tmpDir, "config.json")
	content := `{
  "auths": {
    "ghcr.io": {
      "auth": "dXNlcjpwYXNz"
    },
    "registry.internal.corp": {
      "username": "robot",
      "password": "token-xyz"
    }
  }
}`
	if err := os.WriteFile(dockerConfig, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DOCKER_CONFIG_FILE", dockerConfig)

	// Test base64 auth entry
	user, pass, authHdr, ok := GetRegistryAuth("ghcr.io")
	if !ok {
		t.Fatalf("expected auth for ghcr.io")
	}
	if user != "user" || pass != "pass" {
		t.Errorf("expected user:pass, got %s:%s", user, pass)
	}
	if authHdr != "Basic dXNlcjpwYXNz" {
		t.Errorf("unexpected auth header: %s", authHdr)
	}

	// Test username/password entry
	user2, pass2, authHdr2, ok2 := GetRegistryAuth("registry.internal.corp")
	if !ok2 {
		t.Fatalf("expected auth for registry.internal.corp")
	}
	if user2 != "robot" || pass2 != "token-xyz" {
		t.Errorf("expected robot:token-xyz, got %s:%s", user2, pass2)
	}
	if authHdr2 == "" {
		t.Errorf("expected authHeader for robot:token-xyz")
	}

	// Test absent host
	_, _, _, ok3 := GetRegistryAuth("unknown.registry.io")
	if ok3 {
		t.Errorf("expected no auth for unknown.registry.io")
	}
}
