---
name: alga-release
description: Use when cutting an Alga release — deciding the SemVer tag from Conventional Commits, running pre-flight checks, generating release notes, and pushing a tag to trigger release.yml.
priority: P1
tags: [release, semver, ci, github, docker]
---

# Alga Release

Cut a release by pushing a `v*` SemVer tag. The full pipeline lives in `.github/workflows/release.yml`: it builds the backend binary, pushes backend+frontend Docker images to GHCR, attaches SHA256 checksums, and opens a GitHub Release. This skill decides **which tag to push** and confirms the tree is ready.

Before tagging, state: the last tag (or "first release"), the computed next version, the bump rationale, the prerelease decision, and which pre-flight checks passed.

## Check First

- Release workflow: `.github/workflows/release.yml` (tag trigger, SemVer regex, image tagging).
- Last tag: `git describe --tags --abbrev=0 2>/dev/null` (empty = first release → `v0.0.1`).
- Commits since last tag: `git log --oneline <last-tag>..HEAD`.
- Version wiring target: `apps/backend/main.go` (`var version string`).
- Existing tags: `git tag --list 'v*'`.

## Version Decision (Conventional Commits)

The repo uses Conventional Commits (`feat:`, `fix(scope):`, `deps(go):`, `chore(ci):`). Compute the bump from commit subjects since the last tag. Prefer `scripts/next-version.sh` for the deterministic computation; verify its output against the rules below before tagging.

| Signal in `<last-tag>..HEAD` | Bump |
|------------------------------|------|
| `BREAKING CHANGE:` footer, or `type!:` (e.g. `feat!:`) | **major** |
| `feat:` / `feat(scope):` | **minor** |
| `fix:`, `perf:`, `refactor:` | **patch** |
| `deps:` / `deps(scope):` (Renovate) | **patch** (minor dep updates still patch the product) |
| `chore:`, `docs:`, `test:`, `style:`, `ci:`, `build:`, `revert:` alone | **no release** — do not tag |
| Non-conventional subjects mixed with conventional | follow the highest conventional signal |

### First Release

No prior tags exist. Recommend `v0.0.1` (SemVer "initial development" — signals not yet API-stable). Do not jump to `v1.0.0` unless the user explicitly says the product is production-stable.

### Prereleases

If the user wants validation before a stable tag, cut a prerelease: `v0.0.1-rc.1`, `v0.0.1-beta.1`, or `v0.0.1-alpha.1`. The workflow detects the hyphen and marks `prerelease=true` on the GitHub Release. Promote by tagging the same commit without the suffix later.

### Tag Format Constraints

The workflow's regex (`release.yml:44`) rejects invalid tags **after** the push, leaving a failed run and a dangling tag. Validate locally first:

- Exactly three numeric components: `MAJOR.MINOR.PATCH` (no more, no less).
- No leading zeros: `v0.0.1` ok, `v01.0.1` rejected, `v0.01.0` rejected.
- Optional prerelease: `-<ident>` (e.g. `-rc.1`, `-beta.1`, `-1111`).
- Optional build metadata: `+<ident>` (e.g. `+1111`). **Avoid** — the `+` survives into Docker image tags via `needs.meta.outputs.version`, and registries handle it inconsistently. Prefer the `-` prerelease form if you need a suffix.

Rejected examples: `v2026.07.25.1111` (4 components, leading zeros), `v1.2` (2 components), `v1.2.3.4`.

## Pre-Flight Checks

Run all of these before tagging. Stop and report if any fail.

1. **Clean working tree** — `git status --porcelain` must be empty. Stash or commit first.
2. **On the release branch** — warn (do not block) if not on `main`; confirm the user intends to release from the current branch.
3. **Synced with remote** — `git fetch origin && git status -uno` shows nothing to pull/push on the release branch.
4. **Head commit is the one to tag** — confirm `git rev-parse HEAD` matches what CI last ran.
5. **CI is green on HEAD** — `gh run list --branch main --limit 3`. If red, fix before tagging; a tag push triggers a fresh workflow but inherits broken code.
6. **Version is wired into the binary** — see below.

### Wire `var version string` (one-time setup)

`release.yml:89` passes `-X main.version=<version>` at build time. Until a `var version string` exists in `apps/backend/main.go`, that flag is a harmless no-op and the binary cannot report its own version. On the first release, add:

```go
// in apps/backend/main.go
var version = "dev"
```

Then expose it. Add a `version` subcommand to `rootCmd` (mirrors the existing cobra pattern in `main.go`):

```go
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Alga version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
```

The workflow's `-X main.version` overrides `"dev"` in release builds. Keep the default `"dev"` so local `go run .` and `go build` still work. Do not hardcode a version string in source — the tag is the source of truth.

## Generate Release Notes

The workflow's `generate_release_notes: true` already produces an auto-generated GitHub changelog. For the tag-push decision and any manual body, group commits since the last tag:

```
### Breaking Changes
- <subject> (<short-sha>)

### Features
- feat(scope): <subject> (<short-sha>)

### Fixes
- fix(scope): <subject> (<short-sha>)

### Dependencies
- deps(go): <subject> (<short-sha>)
- deps(npm): <subject> (<short-sha>)
```

Commands to gather the sections:

```bash
git log --pretty=format:'%s (%h)' <last-tag>..HEAD
```

Skip `chore:`, `style:`, and `Merge pull request` lines from the notes unless they carry user-visible impact. Renovate dependency bumps go under **Dependencies** regardless of scope.

## Execute the Release

After pre-flight passes and the version is confirmed:

```bash
git tag v<X.Y.Z>
git push origin v<X.Y.Z>
```

The tag push triggers `release.yml`. Monitor:

```bash
gh run watch     # follow the Release workflow
gh release view v<X.Y.Z>   # confirm the GitHub Release landed
```

Never force-push or delete a tag that has already been released. If a release workflow fails, fix the cause and cut the next patch (`-patch` bump) rather than rewriting history.

### Post-Release Sanity

- GitHub Release exists at the tag with the binary and `checksums-<version>.txt`.
- Docker images appear at `ghcr.io/<owner>/alga-backend:<version>` and `.../alga-frontend:<version>`.
- `docker run --rm ghcr.io/<owner>/alga-backend:<version> version` prints `<version>` (confirms the `-X main.version` wiring).

## Verify

This skill produces git operations and (once) a source edit, not a build. Before tagging:

- `git status --porcelain` is empty.
- `apps/backend && go build .` succeeds after wiring `var version`.
- The proposed tag matches the SemVer regex (run `scripts/next-version.sh` and eyeball the output).

For the full verification ladder after code changes from wiring, use `alga-dev-environment`.
