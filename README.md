# DeployStackr

**ArgoCD is for Kubernetes. DeployStackr is for Humans.**

The lightweight GitOps promotion dashboard that kills the 6-tab release ritual. Safely bump image tags in Git with 1 click, preview exact commit diffs, and inspect audit tenure with zero database overhead.

[Live Demo](https://deploystac.kr/demo/) | [Website](https://deploystac.kr)

---

## Core Philosophy

1. **Do One Thing Well:** DeployStackr's sole job is to safely edit a file in Git to bump a version or image tag (via direct commit or Pull Request). It does not manage clusters or orchestrate rollouts. Execution belongs to **ArgoCD**, **Flux**, or **GitHub Actions**.
2. **Index Upstream, Don't Host:** Queries OCI registries, GitHub Releases, or Git tags for candidate versions. Stores no binaries or container images.
3. **Git History as Audit Log:** Zero external database required. Parses commit metadata, author signatures, and PR history directly from Git to track who promoted what, when, and how long it ran.

---

## Features

* **AST-Preserving Mutator:** Updates target image tags in Helm `values.yaml` or JSON manifests without wiping comments, indentation, or key ordering.
* **Instant Commit & File Diffs:** Inspect exact Git commits and line-by-line diffs between what is running live and what you are about to promote.
* **Deployment Status Dots:** Colored dots show active environments, pulsing rings show open promotion PRs, and grey dots let you inspect previous release lifespans.
* **1-Click Rollbacks:** Review the last 5 versions on any environment and revert with one click.
* **Embeddable Widget:** Drop a compact promotion switch into Backstage, Notion, Confluence, or internal dashboards via `/embed/<service>`.
* **Built-in MCP Server:** Streamable HTTP at `/mcp` (or `/api/mcp`) so AI agents can inspect promotion state and trigger promotions using your GitHub token RBAC.
* **Single Self-Contained Binary:** Go binary with an embedded Svelte 5 UI. Zero Redis, zero database, zero runtime dependencies.

---

## Quick Setup

### 1. Install & Run

**Binary (macOS / Linux):**

```bash
curl -fsSL https://deploystac.kr/install.sh | bash
deploystackr -gitops git@github.com:my-org/gitops-manifests.git
```

**Docker:**

```bash
docker run -p 8080:8080 \
  -e GITOPS_REPO="git@github.com:my-org/gitops-manifests.git" \
  -v ~/.ssh/id_rsa:/etc/deploystackr/ssh/id_rsa:ro \
  -v deploystackr_scratch:/data/scratch \
  eliotstocker/deploystackr:latest
```

**From Source:**

```bash
npm --prefix web run build
CGO_ENABLED=0 go build -o deploystackr ./cmd/server
./deploystackr -gitops git@github.com:my-org/gitops-manifests.git
```

Open `http://localhost:8080`. For Kubernetes manifests, see [`deploy/kubernetes/`](deploy/kubernetes/).

### 2. Configure `.deploystackr.yaml`

Drop a `.deploystackr.yaml` in the root of your GitOps repository (auto-discovered and reloaded on push), or pass a local/remote file with `-config`:

```yaml
version: "1"

gitops:
  repo: "git@github.com:my-org/gitops-manifests.git"
  branch: "main"

services:
  api-service:
    display_name: "Core API"
    source:
      type: "oci" # oci | github_releases | git_tags
      repository: "ghcr.io/my-org/api-service"
      filter: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
    context:
      repo: "my-org/api-service"
      changelog_strategy: "git_diff"
    environments:
      staging:
        branch: "main"
        target:
          file: "apps/api-service/staging/values.yaml"
          path: "$.image.tag"
        write_strategy:
          mode: "direct"
          commit_message: "chore(staging): bump api-service to {{ .TargetVersion }}"
      production:
        branch: "main"
        target:
          file: "apps/api-service/production/deployment.yaml"
          path: "$.spec.template.spec.containers[?@.name == 'api'].image"
        write_strategy:
          mode: "pull_request"
          title: "Promote api-service to {{ .TargetVersion }}"
          body: "Promoted via DeployStackr by {{ .User }}"
```

### 3. Optional: Connect an AI Agent (MCP)

```json
{
  "mcpServers": {
    "deploystackr": {
      "url": "https://your-deploystackr-host/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_GITHUB_TOKEN"
      }
    }
  }
}
```

---

## License

MIT
