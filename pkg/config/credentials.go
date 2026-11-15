package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ReadSecretOrEnv resolves a secret string by checking envVar, fileEnvVar, and candidate file paths.
func ReadSecretOrEnv(envVar, fileEnvVar string, candidatePaths ...string) string {
	if(envVar != "") {
		if val := strings.TrimSpace(os.Getenv(envVar)); val != "" {
			return val
		}
	}

	if(fileEnvVar != "") {
		if path := strings.TrimSpace(os.Getenv(fileEnvVar)); path != "" {
			if data, err := os.ReadFile(path); err == nil {
				if val := strings.TrimSpace(string(data)); val != "" {
					return val
				}
			}
		}
	}

	for _, path := range candidatePaths {
		if(path == "") {
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			if val := strings.TrimSpace(string(data)); val != "" {
				return val
			}
		}
	}

	return ""
}

// GetGitHubToken resolves the GitHub token from env vars or mounted files.
func GetGitHubToken() string {
	return ReadSecretOrEnv("GITHUB_TOKEN", "GITHUB_TOKEN_FILE",
		"/etc/deploystackr/secrets/github-token",
		"/etc/deploystackr/secrets/token",
		"/etc/deploystackr/github/token",
		"/var/run/secrets/deploystackr/github-token",
	)
}

// DockerConfig holds registry credentials parsed from .dockerconfigjson or config.json.
type DockerConfig struct {
	Auths map[string]DockerAuth `json:"auths"`
}

type DockerAuth struct {
	Auth          string `json:"auth,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
}

// GetRegistryAuth finds credentials for a given registry host from mounted docker configs.
func GetRegistryAuth(host string) (username, password, authHeader string, ok bool) {
	configPaths := []string{}
	if p := os.Getenv("REGISTRY_AUTH_FILE"); p != "" {
		configPaths = append(configPaths, p)
	}
	if p := os.Getenv("DOCKER_CONFIG_FILE"); p != "" {
		configPaths = append(configPaths, p)
	}
	if d := os.Getenv("DOCKER_CONFIG"); d != "" {
		configPaths = append(configPaths, filepath.Join(d, "config.json"))
	}
	configPaths = append(configPaths,
		"/etc/deploystackr/registry/.dockerconfigjson",
		"/etc/deploystackr/docker/config.json",
		"/etc/deploystackr/secrets/.dockerconfigjson",
		"/etc/deploystackr/secrets/config.json",
	)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		configPaths = append(configPaths, filepath.Join(home, ".docker", "config.json"))
	}

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if(err != nil || len(data) < 1) {
			continue
		}

		var cfg DockerConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}

		// Normalize host lookup
		lookupKeys := []string{
			host,
			"https://" + host,
			"https://" + host + "/v1/",
			"https://" + host + "/v2/",
		}
		if(host == "docker.io" || host == "registry-1.docker.io") {
			lookupKeys = append(lookupKeys, "https://index.docker.io/v1/", "index.docker.io/v1/")
		}

		for _, key := range lookupKeys {
			if entry, exists := cfg.Auths[key]; exists {
				if(entry.IdentityToken != "") {
					return "", "", "Bearer " + entry.IdentityToken, true
				}
				if(entry.Username != "" && entry.Password != "") {
					encoded := base64.StdEncoding.EncodeToString([]byte(entry.Username + ":" + entry.Password))
					return entry.Username, entry.Password, "Basic " + encoded, true
				}
				if(entry.Auth != "") {
					decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
					if(err == nil) {
						parts := strings.SplitN(string(decoded), ":", 2)
						if(len(parts) == 2) {
							return parts[0], parts[1], "Basic " + entry.Auth, true
						}
					}
					return "", "", "Basic " + entry.Auth, true
				}
			}
		}
	}

	return "", "", "", false
}
