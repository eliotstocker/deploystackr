# DeployStackr

> **ArgoCD is for Kubernetes. DeployStackr is for humans.**  
> The lightweight GitOps promotion dashboard that kills the 6-tab release ritual.

*(If you've had the misfortune of working with me, you will see this as the spiritual successor to Dolphin or Slipstream).*

---

## 🤦‍♂️ The Problem (Why This Exists)

DevOps is great. GitOps is great. But spending your afternoon wrestling a bunch of manual process is **not** what anyone gets out of bed for.

Before DeployStackr, checking on or shipping a release meant the infamous **6-tab browser dance**:
1. Open GitHub releases to find the latest tag.
2. Open the commit log to see what actually changed since the last release.
3. Open ArgoCD/Flux to check what version is currently running in staging.
4. Open your GitOps manifests repo, find line 348 of a 500-line Helm `values.yaml` or Kubernetes JSON manifest.
5. Hand-craft a YAML diff, push a branch, open a PR just to bump a 6-character tag.
6. Open Slack and ask: *"Hey, who deployed to staging, and can someone review this PR?"*

**Git should remain the single source of truth—but humans shouldn't have to hand-craft YAML diffs just to bump an image tag.**

DeployStackr gives you:
- An easy way to deploy a new version of any app to any environment in 2 clicks.
- A dead-simple version history for each environment so you never have to ask *"Wait, what's running in staging?"* again.
- Instant, 1-click diffs showing the exact commits between what's live and what you're about to ship.
- Instant rollbacks to any previous deployment with zero guesswork.
- Full visibility into who deployed what, where, when, how long it ran, and what PRs are still pending.

---

## 🧭 Core Philosophy

DeployStackr is designed around three strict principles:

1. **Do One Thing Well (The Precision Version Bumper)**  
   Its sole job is to edit a file in Git to bump a version or image tag (via direct commit or Pull Request). It intentionally avoids deployment orchestration, rollout logic, and cluster state management. That execution belongs downstream to GitOps engines (**ArgoCD**, **Flux**) or CI/CD (**GitHub Actions**). DeployStackr is the human control switch that triggers the GitOps loop.

2. **Index Upstream, Don't Host (Catalog, Not a Registry)**  
   DeployStackr queries known upstream sources (OCI registries, GitHub Releases, or Git tags) to present available candidate versions. It does **not** act as an artifact registry or binary storage layer. It holds no container images or artifacts—it simply discovers what versions are available to deploy.

3. **Git History as the Immutable Audit Log**  
   DeployStackr requires no external database. Your Git repository **is** the audit log. By parsing commit metadata, author signatures, and PR merge history directly from your Git tree, it tracks exactly who changed a version, when, and how long each version was active in an environment. Your audit trail can never drift because it is identical to your infrastructure repository.

---

## ✨ What It Does

* 🚀 **1-Click Promotions (Without YAML Gymnastics)**: Deploy directly or open an automated Pull Request. DeployStackr uses an AST-preserving parser that updates your image tags or version fields in Helm `values.yaml` or JSON deployment manifests without obliterating your comments, formatting, or indentation.
* 🔍 **Instant Changelog & Commit Diffs**: Click any version to immediately see the exact Git commit log and file diff between what's live in that environment and the target version. No more guessing what went into a release.
* 🕰️ **The Deployment Status Dots**:
  * **Colored Dots**: Active deployments in each environment.
  * **Hollow Ring with Pulse**: Pending deployments (e.g. an open promotion PR awaiting merge).
  * **Grey Dots**: Previous releases. Hover over any dot to see who deployed it, when it went live, when it was retired, and how long it ran.
* ⏪ **Instant Rollback Menu**: Need to revert staging at 2 AM? Click the three-dot menu on any environment, inspect the last 5 deployments, and trigger an immediate rollback.
* 🔒 **Role-Based Access Control (RBAC)**: Lock down production to platform teams or senior devs via GitHub Teams or Google Groups, while letting everyone deploy freely to dev/staging.
* 🪝 **Inbound & Outgoing Webhooks**:
  * **Accepts**: Incoming webhooks from CI/CD pipelines, GitHub releases, or Docker registries when a new version is published or a deploy PR is merged.
  * **Sends**: Customizable webhook alerts (Slack, Teams, PagerDuty, CI) on `deployment_requested`, `deployment_completed`, or `new_version_available` with templated payloads.
* 📦 **Zero Database Required**: DeployStackr is a single, self-contained Go binary with an embedded Svelte SPA. Your Git repository **is** the database. DeployStackr reconstructs history and audit trails directly from your Git commit tree.

---

## 🏛️ How It Works

```
                                +-------------------+
                                | Upstream Registry |
                                | (OCI/GitHub/Tags) |
                                +---------+---------+
                                          |
                                          v
+-------------------------------------------------------------------------------+
|                                 DeployStackr                                  |
|                                                                               |
|   +-----------------------+                    +--------------------------+   |
|   |   Svelte 5 Web UI     | <--(REST API)----> |        Go Engine         |   |
|   |   (Embedded in binary)|                    |                          |   |
|   +-----------------------+                    |  * In-Memory Git Indexer |   |
|                                                |  * AST YAML/JSON Mutator |   |
|                                                |  * Webhook Dispatcher    |   |
|                                                +------------+-------------+   |
+-------------------------------------------------------------|-----------------+
                                                              |
                                      +-----------------------+-----------------------+
                                      | Direct Commit Push    | Open Pull Request     |
                                      v                       v                       v
                              +---------------------------------------------------------------+
                              |                 GitOps Repository (ArgoCD / Flux)             |
                              |              apps/staging/... values.yaml                     |
                              |              apps/prod/...    deployment.json                 |
                              +---------------------------------------------------------------+
```

---

## ⚡ Quick Start

### 1. Run with Docker or Local Binary

```bash
# Pull and run the multi-arch container (supports linux/arm64 for Raspberry Pis and linux/amd64)
docker run -p 8080:8080 \
  -e GITOPS_REPO="git@github.com:my-org/gitops-manifests.git" \
  -v ~/.ssh/id_rsa:/etc/deploystackr/ssh/id_rsa:ro \
  -v deploystackr_scratch:/data/scratch \
  eliotstocker/deploystackr:latest

# Or build locally:
npm --prefix web run build
CGO_ENABLED=0 go build -o deploystackr ./cmd/server

# Option A: In-repo GitOps discovery (reads .deploystackr.yaml directly from the GitOps repo)
./deploystackr -gitops https://github.com/my-org/gitops-manifests.git

# Option B: Local config file
./deploystackr -config config.yaml

# Option C: Remote config via HTTP(S) / GitHub raw URL
./deploystackr -config https://raw.githubusercontent.com/my-org/gitops/main/.deploystackr.yaml
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

---

## ☸️ Kubernetes & Raspberry Pi Deployment

DeployStackr is packaged as a lightweight, static multi-arch container image (`<20MB`, Alpine 3.20 runtime with `git`, `openssh-client`, and `ca-certificates`) ready for bare-metal Kubernetes clusters (k3s, MicroK8s, talos) running on Raspberry Pis (`linux/arm64`) as well as cloud servers (`linux/amd64`).

Ready-to-use manifests are located in `deploy/kubernetes/`:
* [deployment.yaml](file:///Users/eliotstocker/VersionControl/deploystackr/deploy/kubernetes/deployment.yaml) — Non-root Deployment (`appuser: 10001`), probes, volume mounts, and environment configuration.
* [service.yaml](file:///Users/eliotstocker/VersionControl/deploystackr/deploy/kubernetes/service.yaml) — ClusterIP Service exposing port 8080.
* [pvc.yaml](file:///Users/eliotstocker/VersionControl/deploystackr/deploy/kubernetes/pvc.yaml) — PersistentVolumeClaim for bare repos and transient checkouts (`/data/scratch`).
* [secret-ssh.example.yaml](file:///Users/eliotstocker/VersionControl/deploystackr/deploy/kubernetes/secret-ssh.example.yaml) — SSH deploy key mount example.
* [secret-token.example.yaml](file:///Users/eliotstocker/VersionControl/deploystackr/deploy/kubernetes/secret-token.example.yaml) — GitHub Personal Access Token secret example.
* [configmap.example.yaml](file:///Users/eliotstocker/VersionControl/deploystackr/deploy/kubernetes/configmap.example.yaml) — Standalone config mount example.

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listening port for the Web UI and API. |
| `SCRATCH_DIR` | `/data/scratch` | Directory used for bare clones, cache, and transient checkout trees. |
| `CONFIG_PATH` | `/etc/deploystackr/config.yaml` | Local path or HTTP(S) URL to the configuration file. |
| `POLL_INTERVAL` | `30m` | Periodic GitOps polling and reconciliation interval (e.g. `30m`, `1h`, or `0` to disable). |
| `GITOPS_REPO` | `""` | GitOps repository URL (e.g., `git@github.com:org/gitops.git`). Bootstraps directly when using in-repo `.deploystackr.yaml`. |
| `GITOPS_BRANCH` | `main` | Default target branch for GitOps repository operations. |
| `SSH_KEY_PATH` | Auto-detected | Path to the private SSH key. Auto-detects `/etc/deploystackr/ssh/id_rsa`, `id_ed25519`, `ssh-privatekey`, or `~/.ssh/`. |
| `GIT_SSH_COMMAND` | `ssh -o StrictHostKeyChecking=accept-new` | SSH invocation command. Auto-configures `accept-new` to ensure non-interactive headless operation. |
| `GITHUB_TOKEN` | `""` | GitHub Personal Access Token for Git operations, automated PR creation, and user auth. |
| `GITHUB_TOKEN_FILE` | Auto-detected | Path to file containing GitHub token (or mounts in `/etc/deploystackr/secrets/`). |
| `GPG_KEY_FILE` | Auto-detected | Path to GPG private key for commit signing. Auto-detects `/etc/deploystackr/gpg/signing.key`. |
| `SIGN_COMMITS_WITH_SSH` | `false` | Enable commit signing using the mounted SSH key (`gpg.format=ssh`). |
| `DOCKER_CONFIG_FILE` | Auto-detected | Path to Docker config / `.dockerconfigjson` for private OCI registries (ECR, GHCR, Harbor). |
| `GIT_ASKPASS` | `/usr/local/bin/git-askpass.sh` | Non-interactive credential helper script for HTTPS git remotes. |

### Git & Credential Authentication in Headless Kubernetes

#### 1. SSH Deploy Keys
Mount your private SSH deploy key to `/etc/deploystackr/ssh` with read permissions (`defaultMode: 0400`). DeployStackr automatically scans this directory, associates the key, and invokes SSH with `-o StrictHostKeyChecking=accept-new` so connections never block on unknown host prompts:

```bash
kubectl create secret generic deploystackr-ssh-key \
  --namespace=deploystackr \
  --from-file=id_rsa=~/.ssh/id_rsa
```

#### 2. HTTPS / GitHub Personal Access Token (or File / Secret Mount)
Set `GITHUB_TOKEN` as an environment variable or mount it from a Kubernetes Secret to `/etc/deploystackr/secrets/github-token` or `/etc/deploystackr/secrets/token`. DeployStackr automatically reads tokens from env vars or mounted files:

```bash
kubectl create secret generic deploystackr-github-token \
  --namespace=deploystackr \
  --from-literal=token=ghp_yourTokenHere
```

#### 3. GPG / SSH Commit Signing
For GitOps repositories enforcing signed commits via branch protection, mount your GPG key to `/etc/deploystackr/gpg/signing.key` or set `GPG_KEY_FILE`. Alternatively, set `SIGN_COMMITS_WITH_SSH=true` to sign commits directly using your mounted SSH deploy key:

```bash
kubectl create secret generic deploystackr-gpg-key \
  --namespace=deploystackr \
  --from-file=signing.key=/path/to/signing.key
```

#### 4. Private OCI Registries (Docker / ECR / GHCR / Harbor)
Mount a standard `kubernetes.io/dockerconfigjson` Secret to `/etc/deploystackr/registry/.dockerconfigjson` or set `DOCKER_CONFIG_FILE`. DeployStackr will automatically authenticate against private container registries during candidate version discovery:

```bash
kubectl create secret docker-registry deploystackr-registry-auth \
  --namespace=deploystackr \
  --docker-server=ghcr.io \
  --docker-username=your-user \
  --docker-password=your-token
```

### In-Repo GitOps Config (`.deploystackr.yaml`)

Targeting Kubernetes manifests or Kustomize using **RFC 9535 JSONPath** with filter expressions:

```yaml
version: "1"
gitops:
  repo: "git@github.com:eliotstocker/gitops-cluster.git"
  branch: "main"

services:
  api-service:
    display_name: "Core API"
    icon: "server"
    source:
      type: "oci"
      repository: "ghcr.io/eliotstocker/api-service"
      filter: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
    context:
      repo: "eliotstocker/api-service"
      changelog_strategy: "git_diff"
    environments:
      staging:
        branch: "main"
        target:
          # RFC 9535 filter targeting the specific container by name (immune to sidecar insertion!)
          file: "apps/api-service/staging/deployment.yaml"
          path: "$.spec.template.spec.containers[?@.name == 'api'].image"
        write_strategy:
          mode: "direct"
          commit_message: "chore(staging): bump api-service to {{ .TargetVersion }}"
      production:
        branch: "main"
        target:
          # RFC 9535 filter targeting Kustomize kustomization.yaml image tag
          file: "apps/api-service/production/kustomization.yaml"
          path: "$.images[?@.name == 'ghcr.io/eliotstocker/api-service'].newTag"
        write_strategy:
          mode: "pull_request"
          title: "Promote api-service to {{ .TargetVersion }}"
          body: "Promoted via DeployStackr by {{ .User }}"
```

---

## ⚙️ Configuration

DeployStackr supports two clean ways to configure applications:

### 1. In-Repo GitOps Config (`.deploystackr.yaml`) — *Recommended*
Place a `.deploystackr.yaml` directly in the root of your GitOps repository. DeployStackr automatically discovers it, indexes the services, and reloads them whenever changes are pushed to your GitOps repository (zero server restarts needed!).

### 2. Standalone Config File (Local or Remote URL)
Pass a file path or remote raw YAML URL via `-config`:

```yaml
version: "1"

# Default GitOps repository (can be overridden per environment)
gitops:
  repo: "git@github.com:my-org/gitops-manifests.git"
  branch: "main"

# Optional: Outgoing webhook alerts (Slack, Teams, Discord, etc.)
webhooks:
  - event: "deployment_requested"
    endpoint: "https://hooks.slack.com/services/T00/B00/XXXX"
    headers:
      Content-Type: "application/json"
    payload: '{"text":"🚀 *{{ .User }}* requested deployment of *{{ .Service }}* ({{ .TargetVersion }}) to *{{ .Environment }}*"}'

# Define your applications
services:
  billing-service:
    display_name: "Billing Service"
    icon: "payments" # Material symbol name

    # Where do new candidate versions come from?
    source:
      type: "oci" # "oci" | "github_releases" | "git_tags"
      repository: "ghcr.io/my-org/billing-service"
      filter: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"

    # Source code repository for changelog diffs
    context:
      repo: "https://github.com/my-org/billing-service.git"
      changelog_strategy: "git_diff"

    # Optional: Who is allowed to deploy this service?
    permissions:
      github_teams:
        - "my-org/billing-team"
        - "my-org/platform-team"

    # Target environments
    environments:
      staging:
        branch: "main"
        target:
          file: "apps/staging/billing/values.yaml"
          path: "$.image.tag"
        write_strategy:
          mode: "direct" # Direct commit & push for instant dev/staging deploys
          commit_message: "chore(staging): promote billing-service to {{ .TargetVersion }}"

      production:
        branch: "main"
        target:
          file: "apps/production/billing/values.yaml"
          path: "$.image.tag"
        write_strategy:
          mode: "pull_request" # Generates a branch and opens a PR for prod safety
          title: "Promote billing-service to {{ .TargetVersion }}"
          body: "Promoted via DeployStackr by {{ .User }}"
```

---

## 🔌 API & Webhooks

### Inbound Webhooks
Connect your CI/CD pipelines to notify DeployStackr of new builds or merged PRs:
* `POST /api/webhooks/release`: Notifies DeployStackr of a newly published tag/container image. Accepts GitHub release webhooks or generic JSON (`{"service": "...", "version": "..."}`).
* `POST /api/webhooks/pr`: Notifies DeployStackr when a GitOps deployment PR has merged. Immediately updates environment status and audit logs.
* `POST /api/webhook`: Smart multiplexer endpoint that auto-detects GitHub event headers (`release`, `pull_request`, or `push`).

### Outgoing Webhook Events
Subscribe to events globally or per-service:
* `deployment_requested`: Promotion initiated.
* `deployment_completed`: Manifest pushed or PR opened.
* `new_version_available`: New release discovered or received via webhook.
* `pr_merged`: Deployment PR merged into GitOps.

### Core REST Endpoints
* `GET /api/services`: List all services, active versions, and open PR status.
* `GET /api/services/:service/versions`: Available upstream releases/tags.
* `GET /api/services/:service/diff?from=v1.2.0&to=v1.3.0`: Git diff and commit logs between versions.
* `POST /api/promote`: Trigger a deployment (`{ service, environment, target_version, mode }`).
* `GET /api/webhooks`: View configured webhooks and recent delivery history.

### MCP Server (AI Agent Integration)
DeployStackr embeds a built-in Model Context Protocol (MCP) server so AI coding agents can inspect GitOps promotion state and trigger version promotions directly:
* **Streamable HTTP Transport**: `POST /mcp` (also mounted at `/api/mcp`) on the running server. Pass `Authorization: Bearer <GITHUB_TOKEN>` for RBAC evaluation and Git author attribution on promotions.
* **Service or Context Repo Selection**: Every tool accepts either `service` (slug or display name) or `context_repo` (`owner/repo`, HTTPS URL, or SSH `git@...` remote).
* **Tools**:
  * `list_services`: List configured services with their slug, display name, context repository, upstream source, environments, and promotion permissions.
  * `get_deployed`: Inspect active GitOps-declared versions, promotion metadata, open promotion PRs, and optional live cluster rollout status.
  * `get_deploy_queue`: Inspect the promotion queue — open promotion PRs awaiting merge, in-flight cluster rollouts, and unpromoted upstream candidate versions ahead of each environment.
  * `get_version_diff`: Preview Git commit logs and diff stats between two versions or between an environment's active version and a target version.
  * `get_history`: Inspect paginated Git-derived promotion audit history for a service with optional environment, `limit`/`offset`, and `since`/`until` filters.
  * `promote`: Trigger a GitOps version promotion (direct commit or Pull Request) for a specific tag or `"latest"`, enforcing RBAC permissions and dispatching configured webhooks.

---

## 🧪 Testing

```bash
# Run backend test suite
CGO_ENABLED=0 go test ./...

# Run frontend typechecks and build
npm --prefix web run check
npm --prefix web run build
```

---

## 🌐 The Stac.kr Developer Toolchain

DeployStackr is part of the `stac.kr` developer ecosystem built for engineers and autonomous AI coding agents:
* 💻 **[vibestac.kr](https://vibestac.kr)** — Portable local dev stack TUI, task runner, and MCP server.
* ⚡ **[vibebac.kr](https://vibebac.kr)** — Instant cloud persistence, collections, and authentication backend for AI coding assistants.
* 📦 **[metastac.kr](https://metastac.kr)** — Event-driven multi-repo Git submodule orchestration platform.
* 🐬 **[deploystac.kr](https://deploystac.kr)** — Lightweight GitOps promotion dashboard and version bumper.

---

## 📄 License

MIT. Built for developers who love GitOps, but hate manual YAML busywork.
