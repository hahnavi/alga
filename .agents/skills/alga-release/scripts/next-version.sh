#!/usr/bin/env bash
# Compute the next Alga SemVer from Conventional Commits since the last tag.
#
# Usage:
#   .agents/skills/alga-release/scripts/next-version.sh            # print next version (no 'v')
#   .agents/skills/alga-release/scripts/next-version.sh --tag      # print next tag (with 'v')
#   .agents/skills/alga-release/scripts/next-version.sh --rationale # print bump reason
#
# Rules mirror SKILL.md:
#   - No prior tag  -> 0.0.1 (first release)
#   - BREAKING CHANGE footer or type!: -> major
#   - feat:         -> minor
#   - fix/perf/refactor/deps: -> patch
#   - chore/docs/test/style/ci/build/revert only -> no release (exit 2)
#
# Run from anywhere in the repo. Requires git.

set -euo pipefail

print_tag=0
print_rationale=0
for arg in "$@"; do
  case "$arg" in
    --tag) print_tag=1 ;;
    --rationale) print_rationale=1 ;;
    -h|--help)
      sed -n '2,16p' "$0"
      exit 0
      ;;
    *) echo "error: unknown argument '$arg'" >&2; exit 2 ;;
  esac
done

# Ensure we are inside a git repo.
if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "error: not inside a git repository" >&2
  exit 2
fi

# Locate the most recent v* tag. Empty if first release.
last_tag=$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || echo "")

if [[ -z "$last_tag" ]]; then
  version="0.0.1"
  rationale="first release (no prior v* tag)"
else
  base="${last_tag#v}"
  if [[ ! "$base" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
    echo "error: last tag '$last_tag' is not MAJOR.MINOR.PATCH" >&2
    exit 2
  fi
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"

  range="${last_tag}..HEAD"
  bump=""
  rationale=""
  # Store regexes in variables: unquoted parens inside [[ =~ ]] confuse bash.
  merge_re='^Merge[[:space:]]'
  breaking_re='^[a-z]+(\([^)]+\))?!:'

  # Scan commit subjects. Highest signal wins: major > minor > patch.
  while IFS= read -r subj; do
    [[ -z "$subj" ]] && continue
    # Skip merge commits — they carry no conventional signal.
    [[ "$subj" =~ $merge_re ]] && continue
    # Breaking change via type!: (e.g. feat!:, fix(scope)!:)
    if [[ "$subj" =~ $breaking_re ]]; then
      bump="major"
      rationale="breaking change in subject: $subj"
      break
    fi
    case "$subj" in
      feat:*|feat\(*:*)
        if [[ "$bump" != "major" ]]; then bump="minor"; rationale="feat: $subj"; fi ;;
      fix:*|fix\(*:*|perf:*|perf\(*:*|refactor:*|refactor\(*:*)
        if [[ -z "$bump" ]]; then bump="patch"; rationale="fix/perf/refactor: $subj"; fi ;;
      deps:*|deps\(*:*)
        if [[ -z "$bump" ]]; then bump="patch"; rationale="deps: $subj"; fi ;;
      revert:*)
        if [[ -z "$bump" ]]; then bump="patch"; rationale="revert: $subj"; fi ;;
      chore:*|docs:*|test:*|style:*|ci:*|build:*)
        ;;  # no bump on its own
      *)
        # Non-conventional subject: treat as patch if nothing stronger found.
        if [[ -z "$bump" ]]; then bump="patch"; rationale="non-conventional: $subj"; fi ;;
    esac
  done < <(git log --pretty=format:'%s' "$range")

  # Scan full commit bodies for a "BREAKING CHANGE:" footer.
  if [[ "$bump" != "major" ]]; then
    breaking_subj=$(git log --pretty=format:'%B' "$range" \
      | grep -iE '^BREAKING[[:space:]]-?CHANGE:' | head -1 || true)
    if [[ -n "$breaking_subj" ]]; then
      bump="major"
      rationale="BREAKING CHANGE footer: ${breaking_subj:0:80}"
    fi
  fi

  if [[ -z "$bump" ]]; then
    echo "no release needed: only chore/docs/test/style/ci/build commits since $last_tag" >&2
    exit 3
  fi

  case "$bump" in
    major) version="$((major+1)).0.0" ;;
    minor) version="${major}.$((minor+1)).0" ;;
    patch) version="${major}.${minor}.$((patch+1))" ;;
  esac
fi

if [[ "$print_rationale" -eq 1 ]]; then
  echo "$rationale"
  exit 0
fi

if [[ "$print_tag" -eq 1 ]]; then
  echo "v${version}"
else
  echo "$version"
fi
