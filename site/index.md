# ArgoCD is for Kubernetes. DeployStackr is for Humans.

**The lightweight GitOps promotion dashboard that kills the 6-tab release ritual.**

Safely bump image tags in Git with 1 click, preview exact commit diffs, and inspect audit tenure with zero database overhead.

:::cta
- `curl -fsSL https://deploystac.kr/install.sh | bash`
- [View on GitHub](https://github.com/eliotstocker/deploystackr){.btn-primary}
:::

## Core Philosophy {#philosophy}

Three strict principles that keep promotions predictable, lightweight, and auditable.

:::flow
### Do One Thing Well
DeployStackr's sole job is to safely edit a file in Git to bump a version or image tag (via direct commit or Pull Request). It does not manage clusters or orchestrate rollouts—that execution belongs strictly to GitOps engines (**ArgoCD**, **Flux**) or CI/CD pipelines (**GitHub Actions**).

### Index Upstream, Don't Host
DeployStackr queries known upstream sources (OCI registries, GitHub Releases, or Git tags) to present candidate deployable versions. It is not an artifact registry and stores no binaries or image blobs—it simply answers: *"What versions are available to deploy?"*

### Git History as Audit Log
Zero external database required. Your Git repository is the single source of truth. By parsing commit metadata, author signatures, and PR merge history directly from your Git tree, DeployStackr tracks exactly who deployed what, when, and how long it ran without data drift.
:::

## The Control Switch For GitOps {#preview}

Clean environment status, instant diffs, and zero YAML gymnastics.

{{include:snippets/preview.html}}

## Engineered For Humans, Agents & GitOps {#features}

Everything you need to ship safely without touching raw manifest YAML.

:::features
### AST-Preserving Mutator
Modifies target image tags in Helm `values.yaml` or JSON manifests without wiping comments, indentation, or key ordering.

### Instant Commit & File Diffs
Click any release to immediately inspect the exact Git commits and line-by-line diff between what is running live and what you are about to ship.

### The Deployment Dots
Colored dots indicate live environments, pulsing rings show pending PRs awaiting review, and grey dots let you inspect previous release lifespans.

### 1-Click Instant Rollbacks
Need to revert staging or production in an emergency? Open the rollback history, review the last five deployments, and restore with one click.

### Single Self-Contained Binary
Compiled in Go with an embedded Svelte 5 frontend. Zero external database, zero Redis cache, zero runtime dependencies.

### In-Repo GitOps Config
Place a `.deploystackr.yaml` in your GitOps repo root. DeployStackr auto-discovers services and reloads automatically on git push.

### Built-in MCP Server
Connect AI coding agents directly via Streamable HTTP (`/mcp`) to inspect active versions, promotion queues, commit diffs, audit history, and trigger promotions with full GitHub RBAC.
:::

## Built-in MCP Server {#mcp}

Let AI agents inspect GitOps promotion state and trigger version promotions directly from your IDE or CLI using Streamable HTTP at `/mcp` (or `/api/mcp`). Authenticate requests with a GitHub token (`Authorization: Bearer <GITHUB_TOKEN>` or `X-GitHub-Token`) so DeployStackr enforces your repository RBAC and attributes every commit and PR to you.

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

- **`list_services`**: Discover configured services, context repositories, environments, and promotion permissions.
- **`get_deployed`**: Inspect active GitOps-declared versions, pending promotion PRs, and live cluster rollout status by `service` or `context_repo`.
- **`get_deploy_queue`**: View open promotion PRs, in-flight rollouts, and unpromoted upstream candidate versions.
- **`get_version_diff`**: Preview Git commit logs and diff stats between any two versions or an environment's active version.
- **`get_history`**: Page through Git-derived promotion audit history with `limit`, `offset`, and `since`/`until` filters.
- **`promote`**: Trigger a GitOps version promotion (direct commit or Pull Request) for a specific tag or `"latest"`.
