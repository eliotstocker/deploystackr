package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"deploystackr/pkg/api"
	"deploystackr/pkg/config"
	"deploystackr/pkg/deployer"
	"deploystackr/pkg/git"
	"deploystackr/pkg/indexer"
	"deploystackr/pkg/store"
	"deploystackr/web"
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func main() {
	defaultPort := 8080
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", &defaultPort)
	}

	configPath := flag.String("config", getEnv("CONFIG_PATH", "config.yaml"), "Path or HTTP(S) URL to deploystackr config file")
	gitopsRepo := flag.String("gitops", getEnv("GITOPS_REPO", ""), "Optional GitOps repository URL to bootstrap without a local config file")
	gitopsBranch := flag.String("branch", getEnv("GITOPS_BRANCH", "main"), "Default GitOps branch when using -gitops")
	port := flag.Int("port", defaultPort, "HTTP server port")
	scratchDir := flag.String("scratch", getEnv("SCRATCH_DIR", filepath.Join(os.TempDir(), "deploystackr")), "Directory for bare clones and promotion checkouts")
	pollInterval := flag.Duration("poll-interval", getDurationEnv("POLL_INTERVAL", 30*time.Minute), "Interval for periodic GitOps polling and reconciliation (e.g. 30m, 1h, 0 to disable)")
	flag.Parse()

	// Configure headless Git authentication for SSH and HTTPS
	git.SetupGitAuth(*scratchDir)

	var cfg *config.Config
	var err error

	isRemoteConfig := strings.HasPrefix(*configPath, "http://") || strings.HasPrefix(*configPath, "https://")

	if *gitopsRepo != "" && (*configPath == "config.yaml" || *configPath == "") {
		log.Printf("Bootstrapping from GitOps repository: %s (branch: %s)", *gitopsRepo, *gitopsBranch)
		cfg = &config.Config{
			Version: "1",
			GitOps: config.GitOpsConfig{
				Repo:   *gitopsRepo,
				Branch: *gitopsBranch,
			},
			Services: make(map[string]*config.ServiceConfig),
			Auth: &config.AuthConfig{
				DevMode: true,
			},
		}
	} else if isRemoteConfig {
		log.Printf("Loading remote config from: %s", *configPath)
		cfg, err = config.Load(*configPath)
		if err != nil {
			log.Fatalf("Failed to load remote config from %s: %v", *configPath, err)
		}
	} else {
		// Ensure local config file exists; if not create default sample config
		if _, err := os.Stat(*configPath); os.IsNotExist(err) {
			log.Printf("Config file '%s' not found. Creating sample configuration...", *configPath)
			if err := createSampleConfig(*configPath); err != nil {
				log.Fatalf("Failed to create sample config: %v", err)
			}
		}

		cfg, err = config.Load(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config from %s: %v", *configPath, err)
		}
		if *gitopsRepo != "" && cfg.GitOps.Repo == "" {
			cfg.GitOps.Repo = *gitopsRepo
			if *gitopsBranch != "" {
				cfg.GitOps.Branch = *gitopsBranch
			}
		}
	}

	memStore := store.NewMemoryStore()

	idx, err := indexer.NewIndexer(cfg, memStore, filepath.Join(*scratchDir, "clones"))
	if err != nil {
		log.Fatalf("Failed to initialize indexer: %v", err)
	}

	deployerMgr := deployer.NewManager(cfg, memStore, idx, filepath.Join(*scratchDir, "promotions"))

	// Perform initial history indexing in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		log.Println("[indexer] Starting initial GitOps history indexing...")
		if err := idx.ReindexAll(ctx); err != nil {
			log.Printf("[indexer] Initial indexing completed with notice: %v", err)
		} else {
			log.Println("[indexer] Initial history indexing complete.")
		}
	}()

	// Periodic GitOps polling and reconciliation
	donePoll := make(chan struct{})
	if *pollInterval > 0 {
		log.Printf("[indexer] Periodic GitOps polling enabled (interval: %s)", *pollInterval)
		ticker := time.NewTicker(*pollInterval)
		go func() {
			for {
				select {
				case <-donePoll:
					ticker.Stop()
					return
				case <-ticker.C:
					log.Println("[indexer] Periodic GitOps polling: checking for repository changes...")
					pollCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
					if err := idx.ReindexAll(pollCtx); err != nil {
						log.Printf("[indexer] Periodic reconciliation notice: %v", err)
					} else {
						log.Println("[indexer] Periodic reconciliation complete.")
					}
					cancel()
				}
			}
		}()
	}

	distFS := web.Dist()
	srv := api.NewServer(cfg, memStore, idx, deployerMgr, distFS)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      srv,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("🚀 DeployStackr GitOps Engine listening on http://localhost:%d", *port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failure: %v", err)
		}
	}()

	<-stopChan
	log.Println("Shutting down DeployStackr server...")
	close(donePoll)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server exited cleanly.")
}

func createSampleConfig(path string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	sample := `version: "1"
gitops:
  repo: "git@github.com:org/gitops-manifests.git"
  branch: "main"

services:
  billing-service:
    source:
      type: "oci"
      repository: "ghcr.io/org/billing-service"
      filter: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
    context:
      repo: "org/billing-service"
      changelog_strategy: "git_diff"
    environments:
      staging:
        branch: "main"
        target:
          file: "apps/staging/billing/values.yaml"
          path: "$.image.tag"
        write_strategy:
          mode: "direct"
          commit_message: "chore(staging): promote billing-service to {{ .TargetVersion }}"
      production:
        repo: "git@github.com:org/prod-gitops.git"
        branch: "main"
        target:
          file: "apps/production/billing/deployment.json"
          path: "$.spec.template.spec.containers[0].image"
        write_strategy:
          mode: "pull_request"
          title: "Promote billing-service to {{ .TargetVersion }}"
          body: "Promoted via DeployStackr by {{ .User }}"

  auth-service:
    source:
      type: "github_releases"
      repository: "org/auth-service"
      filter: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
    context:
      repo: "org/auth-service"
      changelog_strategy: "git_diff"
    environments:
      staging:
        branch: "main"
        target:
          file: "apps/staging/auth/values.yaml"
          path: "$.image.tag"
        write_strategy:
          mode: "direct"
          commit_message: "chore(staging): promote auth-service to {{ .TargetVersion }}"
      production:
        branch: "main"
        target:
          file: "apps/production/auth/values.yaml"
          path: "$.image.tag"
        write_strategy:
          mode: "pull_request"
          title: "Promote auth-service to {{ .TargetVersion }}"
          body: "Promoted via DeployStackr by {{ .User }}"

  notification-service:
    source:
      type: "git_tags"
      repository: "https://github.com/org/notification-service.git"
      filter: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
    context:
      repo: "org/notification-service"
      changelog_strategy: "git_diff"
    environments:
      staging:
        branch: "main"
        target:
          file: "apps/staging/notification/values.yaml"
          path: "$.image.tag"
        write_strategy:
          mode: "direct"
      production:
        branch: "main"
        target:
          file: "apps/production/notification/values.yaml"
          path: "$.image.tag"
        write_strategy:
          mode: "pull_request"
`
	return os.WriteFile(path, []byte(sample), 0644)
}
