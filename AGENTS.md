# DeployStackr: Core Philosophy & Communications Standard

This document defines the core product philosophy, boundary constraints, and communication standards for **DeployStackr**. All documentation, feature proposals, UI copy, pull requests, commit messages, and AI agent communications must strictly adhere to these three tenets.

---

## 🧭 The Three Core Tenets

### 1. Do One Thing Well (The Precision Version Bumper)
* **What it is:** DeployStackr has a single, well-defined operational responsibility: safely editing a file in a GitOps repository to bump a version/tag string (via direct commit or Pull Request).
* **What it is NOT:** DeployStackr is **not** a deployment orchestrator, pipeline engine, or cluster manager.
* **The Boundary:** It does **not** execute deployments, monitor pod health, coordinate canary rollouts, or manage cluster state. All downstream execution belongs strictly to GitOps engines (**ArgoCD**, **Flux**) or CI/CD pipelines (**GitHub Actions**). DeployStackr is solely the human control switch that cleanly initiates the GitOps loop.

### 2. Catalog Upstream, Don't Host (Index, Not an Artifact Registry)
* **What it is:** DeployStackr queries known upstream sources (OCI registries like GHCR/DockerHub, GitHub Releases, or Git tags) to list candidate deployable versions and display changelogs/diffs.
* **What it is NOT:** DeployStackr is **not** an artifact registry, storage engine, or package mirror.
* **The Boundary:** It stores no binary images, layers, or release packages. It simply asks the upstream source: *"What versions are available to deploy?"* and provides instant comparison against active environments.

### 3. Git History as the Immutable Audit Log (Who Changed What & When)
* **What it is:** The Git repository itself is the single source of truth and complete audit trail.
* **What it is NOT:** DeployStackr does not require or maintain an external audit database (PostgreSQL, DynamoDB, etc.).
* **The Boundary:** By traversing commit metadata, author signatures, and PR merge history directly from the Git tree, DeployStackr reconstructs:
  * Exactly **who** triggered the promotion.
  * **When** it was deployed and retired.
  * How long each version remained active.
  * Which commit and PR brought it into the environment.
  
  Because Git is the audit log, the audit trail can never drift or desynchronize from actual infrastructure state.

---

## 📢 Communications & Messaging Guidelines

Whenever discussing, documenting, or designing features for DeployStackr:
1. **Never describe DeployStackr as a deployment tool or orchestrator.** Always describe it as a *GitOps promotion dashboard* or *version promotion trigger*.
2. **Never suggest adding artifact storage or package hosting.** Always reinforce querying upstream registries.
3. **Never propose adding an external database for deployment tracking.** History and audit logs must remain native to Git.
4. **Keep PRs and Commits attributed clearly:** Ensure commit messages and PR bodies clearly identify who requested the promotion and that it was executed via DeployStackr.
