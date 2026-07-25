---
name: alga-agent-release
description: Use when cutting an Alga Agent release — deciding the SemVer version from changes under apps/alga-agent, running pre-flight checks, and pushing an agent-v* tag to trigger agent-release.yml, which publishes the Docker image to GHCR and attaches cross-compiled binaries to a GitHub Release.
priority: P1
tags: [release, agent, semver, ci, github, docker, ghcr]
---

# Alga Agent Release

Cut an agent release by pushing an `agent-v*` SemVer tag. The pipeline lives in `.github/workflows/agent-release.yml`: it validates the tag, builds and pushes `ghcr.io/<owner>/alga-agent` (tags: `latest`, `<version>`, `<commit-sha>`), cross-compiles binaries for linux/darwin (amd64/arm64) with SHA256 checksums, and opens a GitHub Release. This skill decides **which version to tag** and confirms the tree is ready.

The agent is versioned **independently** of the application (`v*` tags, `alga-release` skill) and the Helm chart (`chart-v*` tags, `alga-chart-release` skill). Never bump the agent version just because the app released.

Before tagging, state: the last agent tag (or "first agent release"), the computed next version, the bump rationale, the prerelease decision, and which pre-flight checks passed.

## Check First

- Agent workflow: `.github/workflows/agent-release.yml` (tag trigger, SemVer regex, image tagging, binary matrix).
- Agent source: `apps/alga-agent/` — version wiring is `var version = "dev"` in `apps/alga-agent/main.go`, injected via `-X main.version` (already wired; no one-time setup needed).
- Last agent tag: `git tag --list 'agent-v*' --sort=-v:refname | head -1` (empty = first agent release → `agent-v0.0.1`).
- Agent changes since last tag: `git log --oneline <last-agent-tag>..HEAD -- apps/alga-agent integrations/alga-agent-sdk-go` (the Docker image builds the local SDK too — SDK changes ship in agent releases).
- Existing tags: `git tag --list 'agent-v*'`.
- Branch protection on `main`: fixes land via PR merge if PRs are required — never a direct push.

## Version Decision (Conventional Commits)

Compute the bump from commit subjects since the last agent tag that touch `apps/alga-agent/` or `integrations/alga-agent-sdk-go/`:

| Signal in `<last-agent-tag>..HEAD` | Bump |
|------------------------------------|------|
| `BREAKING CHANGE:` footer, or `type!:` (e.g. `feat!:`) | **major** |
| `feat:` / `feat(scope):` | **minor** |
| `fix:`, `perf:`, `refactor:` | **patch** |
| `deps:` / `deps(scope):` (Renovate) | **patch** |
| `chore:`, `docs:`, `test:`, `style:`, `ci:`, `build:`, `revert:` alone | **no release** — do not tag |
| No agent-touching commits | **no release** — do not tag |

### First Release

No prior `agent-v*` tags exist. Recommend `agent-v0.0.1` (SemVer "initial development"). Do not jump to `agent-v1.0.0` unless the user explicitly says the agent is production-stable.

### Prereleases

For validation before a stable tag: `agent-v0.0.1-rc.1`, `-beta.1`, or `-alpha.1`. The workflow detects the hyphen and marks `prerelease=true` on the GitHub Release. Promote by tagging the same commit without the suffix later.

### Tag Format Constraints

The workflow's regex rejects invalid tags **after** the push, leaving a failed run and a dangling tag. Validate locally first:

- Tag is `agent-v<MAJOR>.<MINOR>.<PATCH>[-<prerelease>]` — exactly three numeric components.
- No leading zeros: `agent-v0.0.1` ok, `agent-v01.0.1` rejected.
- Avoid `+` build metadata — it survives into the Docker image tag and registries handle it inconsistently. Prefer the `-` prerelease form.

## Pre-Flight Checks

Run all of these before tagging. Stop and report if any fail.

1. **Clean working tree** — `git status --porcelain` is empty.
2. **On `main` and synced** — `git fetch origin && git status -uno` shows nothing to pull/push.
3. **Tag does not already exist** — `git tag --list 'agent-v*'`.
4. **Agent builds locally**:
   ```bash
   cd apps/alga-agent && CGO_ENABLED=0 go build -ldflags="-X main.version=test" -o /tmp/alga-agent . && rm /tmp/alga-agent
   ```
5. **Agent tests pass** — `cd apps/alga-agent && go test ./...`.
6. **Docker image builds from the repo root** (context must include `go.work` and `integrations/alga-agent-sdk-go/`):
   ```bash
   docker build -f apps/alga-agent/Dockerfile --build-arg VERSION=test .
   ```
   Skip if Docker is unavailable locally; note it in the pre-flight report.
7. **CI is green on the exact commit you'll tag** — `gh run list --branch main --limit 3`; confirm the head SHA matches `git rev-parse HEAD`. Distinguish `startup_failure` (transient → `gh run rerun`) from a real `failure` (diagnose and fix via PR first).

## Execute the Release

After pre-flight passes and the version is confirmed:

```bash
git tag agent-v<X.Y.Z>
git push origin agent-v<X.Y.Z>
```

The tag push triggers `agent-release.yml`. Monitor:

```bash
gh run watch                         # follow the Agent Release workflow
gh release view agent-v<X.Y.Z>      # confirm the GitHub Release landed
```

Never force-push or delete an agent tag that has been released. If the workflow fails, fix the cause via PR and tag the next patch — do not rewrite history.

## Post-Release Sanity

- GitHub Release "Agent <X.Y.Z>" exists at the tag with four binaries and `checksums-agent-<version>.txt`.
- Docker image appears at `ghcr.io/<owner>/alga-agent:<version>` (`<owner>` lowercase).
- On first publish, set the `alga-agent` package visibility in GitHub Packages (private by default).
- Version wiring check: the container logs `starting alga-agent` with `"version": "<version>"` (the agent logs its version at startup; there is no `version` subcommand).
