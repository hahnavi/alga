---
name: alga-chart-release
description: Use when cutting an Alga Helm chart release — bumping the chart version in Chart.yaml, running pre-flight checks, and pushing a chart-v* tag to trigger chart-release.yml, which publishes the chart to GHCR as an OCI artifact.
priority: P1
tags: [release, helm, chart, semver, ci, github, ghcr]
---

# Alga Helm Chart Release

Cut a chart release by pushing a `chart-v*` SemVer tag. The pipeline lives in `.github/workflows/chart-release.yml`: it validates the tag against `Chart.yaml`, lints, packages, pushes the chart to `oci://ghcr.io/<owner>/charts/alga` (`<owner>` is the lowercase GitHub owner — OCI refs reject uppercase), and opens a GitHub Release. This skill decides **which chart version to tag** and confirms the tree is ready.

The chart is versioned **independently** of the application. App releases use `v*` tags (`alga-release` skill); chart releases use `chart-v*` tags. Never bump the chart version just because the app released.

Before tagging, state: the last chart tag (or "first chart release"), the computed next version, the bump rationale, and which pre-flight checks passed.

## Check First

- Chart workflow: `.github/workflows/chart-release.yml` (tag trigger, SemVer regex, tag↔Chart.yaml match check).
- Chart source: `deploy/charts/alga/` — `version` and `appVersion` live in `Chart.yaml`.
- Last chart tag: `git tag --list 'chart-v*' --sort=-v:refname | head -1` (empty = first chart release).
- Chart changes since last tag: `git log --oneline <last-chart-tag>..HEAD -- deploy/charts/alga`.
- Docs that reference the chart: `grep -rn 'charts/alga\|helm install' docs/` — currently `docs/getting-started/installation.md` pins the chart version in its `helm install` command and documents values keys.
- Branch protection on `main`: version bumps land via PR merge if PRs are required — never a direct push.

## Version Decision

Bump `version` in `Chart.yaml` based on what changed under `deploy/charts/alga/` since the last chart tag:

| Change | Bump |
|--------|------|
| Removed/renamed values, changed defaults that break existing `values.yaml` overrides, removed resources | **major** |
| New values, new optional resources, new features | **minor** |
| Template fixes, docs in chart, `appVersion`-only bump | **patch** |
| No chart changes | **no release** — do not tag |

### appVersion

`appVersion` must always equal the latest app release tag **including the `v` prefix** (e.g. `"v0.0.2"`). Determine it with:

```bash
git tag --list 'v*' --sort=-v:refname | head -1
```

Every chart release PR must set `appVersion` in `Chart.yaml` to this value, even if the app version hasn't changed since the last chart release — treat it as a mandatory sync step, not an optional update. Do not override `appVersion` at package time — `Chart.yaml` is the source of truth.

### Image Tags

`backend.image.tag` and `frontend.image.tag` in `values.yaml` must be pinned to the latest app release tag (the same value as `appVersion`, including the `v` prefix). Never use `latest` — it makes installs non-reproducible and silently drifts from the chart's declared `appVersion`.

Every chart release PR must verify and, if stale, update both tags:

```bash
latest_app_tag=$(git tag --list 'v*' --sort=-v:refname | head -1)
grep -n 'tag:' deploy/charts/alga/values.yaml | while IFS= read -r line; do
  tag=$(echo "$line" | sed 's/.*tag: *"\{0,1\}\([^"]*\)"\{0,1\}/\1/')
  if [ "$tag" = "latest" ] || [ "$tag" != "$latest_app_tag" ]; then
    echo "MISMATCH: $line (expected $latest_app_tag, got $tag)" >&2
    exit 1
  fi
done
```

### Tag Format Constraints

The tag is `chart-v<MAJOR>.<MINOR>.<PATCH>[-<prerelease>]` and the workflow **fails if the tag version does not equal `version` in Chart.yaml**. Order of operations is therefore fixed: bump `Chart.yaml`, merge to `main`, then tag the merge commit.

Same SemVer rules as app tags: exactly three numeric components, no leading zeros, optional `-rc.1`-style prerelease (marks the GitHub Release as prerelease). Avoid `+` build metadata — it survives into the OCI tag and registries handle it inconsistently.

## Docs Sync

Docs must land in the same version-bump PR, before tagging:

- **Version pins** — update any chart version pinned in docs (e.g., the `--version <X.Y.Z>` in the `helm install` command in `docs/getting-started/installation.md`) to the new version.
- **Values changes** — if values were added, removed, renamed, or had defaults changed, update every doc that references them: install commands (`--set` flags), required-values lists, ingress/external-service guidance. Removed or renamed values left in docs produce broken installs for users.
- **Behavior changes** — new required values, new fail-closed checks, or changed bundled-service defaults must be reflected in the docs' prerequisites and install steps.

No doc changes needed only when the release touches templates without changing any user-facing values or install flow.

## Pre-Flight Checks

Run all of these before tagging. Stop and report if any fail.

1. **Clean working tree** — `git status --porcelain` is empty.
2. **On `main` and synced** — `git fetch origin && git status -uno` shows nothing to pull/push.
3. **`Chart.yaml` version matches the tag you intend to push** — the workflow hard-fails on mismatch, leaving a dangling tag.
4. **`appVersion` equals the latest app tag** — `git tag --list 'v*' --sort=-v:refname | head -1` must match `appVersion` in `Chart.yaml` (including the `v` prefix).
5. **Image tags are pinned** — `backend.image.tag` and `frontend.image.tag` in `values.yaml` equal the latest app tag (same value as `appVersion`). Neither may be `latest`.
6. **Chart lints and packages locally**:
   ```bash
   helm lint deploy/charts/alga
   helm package deploy/charts/alga -d /tmp && rm /tmp/alga-*.tgz
   ```
   The "missing required values" warnings for `postgresql`/`valkey`/`rabbitmq` passwords are expected.
7. **Tag does not already exist** — `git tag --list 'chart-v*'`.
8. **Docs are synced** — `grep -rn 'charts/alga\|helm install' docs/` shows no stale version pin or values reference (see Docs Sync).
9. **CI is green on the commit you'll tag** — `gh run list --branch main --limit 3`.

## Execute the Release

```bash
git tag chart-v<X.Y.Z>
git push origin chart-v<X.Y.Z>
```

The tag push triggers `chart-release.yml`. Monitor:

```bash
gh run watch                        # follow the Chart Release workflow
gh release view chart-v<X.Y.Z>     # confirm the GitHub Release landed
```

Never force-push or delete a chart tag that has been released. If the workflow fails, fix the cause, bump to the next patch, and tag again.

## Post-Release Sanity

- GitHub Release "Helm Chart <X.Y.Z>" exists at the tag.
- Chart is pullable and installable (`<owner>` lowercase):
  ```bash
  helm pull oci://ghcr.io/<owner>/charts/alga --version <X.Y.Z>
  helm show chart oci://ghcr.io/<owner>/charts/alga --version <X.Y.Z>
  ```
- On first publish, set the `charts/alga` package visibility in GitHub Packages (private by default).
