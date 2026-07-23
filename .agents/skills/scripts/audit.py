#!/usr/bin/env python3
"""Audit Alga project skills for common maintenance issues."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
REPO = ROOT.parents[1]
REQUIRED_FRONTMATTER = {"name", "description"}
REQUIRED_INTERFACE = {"display_name", "short_description"}
MAX_SKILL_LINES = 180
FORBIDDEN_PHRASES = {
    "implement all required CRUD/list behavior": "prefer task-scoped behavior, not blanket CRUD/list guidance",
}


def parse_frontmatter(text: str) -> tuple[dict[str, str], str | None]:
    if not text.startswith("---\n"):
        return {}, "missing frontmatter"
    end = text.find("\n---\n", 4)
    if end == -1:
        return {}, "unterminated frontmatter"
    fields: dict[str, str] = {}
    for raw in text[4:end].splitlines():
        if ":" not in raw:
            continue
        key, value = raw.split(":", 1)
        fields[key.strip()] = value.strip()
    missing = REQUIRED_FRONTMATTER - fields.keys()
    if missing:
        return fields, "missing frontmatter fields: " + ", ".join(sorted(missing))
    return fields, None


def skill_files() -> list[Path]:
    return sorted(p for p in ROOT.glob("*/SKILL.md") if p.is_file())


def validation_files(skill_dir: Path) -> list[Path]:
    validation_dir = skill_dir / "validation"
    if not validation_dir.exists():
        return []
    return sorted(p for p in validation_dir.iterdir() if p.is_file())


def referenced_paths(text: str) -> set[str]:
    candidates = set(re.findall(r"`((?:apps|integrations|packages|\.agents)/[^`]+)`", text))
    candidates.update(re.findall(r"\((\.agents/skills/[^)]+)\)", text))
    return candidates


def main() -> int:
    errors: list[str] = []
    warnings: list[str] = []

    skills = skill_files()
    if not skills:
        errors.append("no SKILL.md files found")

    names: dict[str, Path] = {}
    for path in skills:
        rel = path.relative_to(REPO)
        text = path.read_text(encoding="utf-8")
        fields, err = parse_frontmatter(text)
        if err:
            errors.append(f"{rel}: {err}")

        name = fields.get("name")
        if name:
            expected_dir = path.parent.name
            if name != expected_dir:
                errors.append(f"{rel}: name {name!r} does not match directory {expected_dir!r}")
            if name in names:
                errors.append(f"{rel}: duplicate skill name also in {names[name].relative_to(REPO)}")
            names[name] = path

        line_count = len(text.splitlines())
        if line_count > MAX_SKILL_LINES:
            warnings.append(f"{rel}: {line_count} lines exceeds {MAX_SKILL_LINES}")

        for phrase, reason in FORBIDDEN_PHRASES.items():
            if phrase in text:
                errors.append(f"{rel}: forbidden phrase {phrase!r}: {reason}")

        if str(REPO) in text:
            errors.append(f"{rel}: hardcoded repository path {REPO}")

        if not validation_files(path.parent):
            errors.append(f"{rel}: missing validation artifact under validation/")

        metadata = path.parent / "agents" / "openai.yaml"
        if not metadata.exists():
            warnings.append(f"{rel}: missing optional agents/openai.yaml")
        else:
            meta_text = metadata.read_text(encoding="utf-8")
            if "interface:" not in meta_text:
                errors.append(f"{metadata.relative_to(REPO)}: missing interface section")
            for field in REQUIRED_INTERFACE:
                if not re.search(rf"^\s+{field}:\s+", meta_text, re.MULTILINE):
                    errors.append(f"{metadata.relative_to(REPO)}: missing interface.{field}")

        for ref in referenced_paths(text):
            clean = ref.split(":", 1)[0]
            if any(ch in clean for ch in "*{} <>"):
                continue
            target = REPO / clean
            if not target.exists():
                warnings.append(f"{rel}: referenced path does not exist: {clean}")

    for message in warnings:
        print(f"WARN {message}")
    for message in errors:
        print(f"ERROR {message}", file=sys.stderr)

    print(f"Audited {len(skills)} skills: {len(errors)} errors, {len(warnings)} warnings")
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
