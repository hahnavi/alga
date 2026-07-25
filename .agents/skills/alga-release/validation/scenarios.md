# Validation Scenarios

## Pressure Scenario 1 — First release, no prior tags

Request: "Let's cut the first release."

Pressure: no version history, temptation to over-version or use CalVer.

Expected skill behavior: agent runs `git describe --tags --abbrev=0` (empty), recommends `v0.0.1`, states this is the first-release default, runs pre-flight checks, wires `var version string = "dev"` into `apps/backend/main.go` if missing, then pushes `git tag v0.0.1 && git push origin v0.0.1`.

Failure this guards: jumping to `v1.0.0`, using CalVer like `v2026.7.25`, or pushing a tag that fails the SemVer regex in `release.yml`.

## Pressure Scenario 2 — Invalid tag proposal

Request: "Can I release v2026.07.25.1111?"

Pressure: user proposes a tag the workflow will reject after push.

Expected skill behavior: agent rejects it before any git operation, citing the regex in `release.yml:44` — four components and leading zeros — and offers the valid form (`v2026.7.25` or `v0.0.1`). Never runs `git tag` with an invalid value.

Failure this guards: a pushed tag that fails the `meta` job and leaves a dangling ref + failed workflow run.

## Pressure Scenario 3 — Build metadata vs prerelease

Request: "What about v2026.7.25+1111?"

Pressure: tag is valid SemVer but the `+` leaks into Docker image tags via `needs.meta.outputs.version`.

Expected skill behavior: agent confirms the tag passes the regex and is a stable release (no hyphen), then warns that `+` in image tags is handled inconsistently by registries and recommends the `-` prerelease form or a plain `MAJOR.MINOR.PATCH` instead.

Failure this guards: image tags that GHCR silently mangles or rejects, breaking pull commands in the release notes.

## Pressure Scenario 4 — Bump decision from commits

Request: "What should the next version be?"

Pressure: must read commit history and apply Conventional Commits rules deterministically.

Expected skill behavior: agent runs `scripts/next-version.sh` and cross-checks the rationale against the commit log; reports last tag, computed bump, triggering commit, and prerelease recommendation; states all pre-flight checks before tagging.

Failure this guards: arbitrary version bumps, missing `BREAKING CHANGE` detection, or tagging when only `chore:`/`docs:` commits exist (script exits 3).

## Pressure Scenario 5 — Dirty tree or red CI

Request: "Ship the release now."

Pressure: user wants speed; tree or CI is not ready.

Expected skill behavior: agent runs `git status --porcelain` and `gh run list`, blocks the tag push on a dirty tree, and warns (with explicit confirmation) when HEAD CI is red. Never tags a commit that has not been pushed to the remote branch.

Failure this guards: releasing uncommitted local state or known-broken HEAD.
