#!/usr/bin/env bash
# Generate docs/issues/INDEX.md from canonical issue-file frontmatter (#0141).
# Usage: generate-issues-index.sh [--check] [--issues-dir DIR] [--output FILE]
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

exec python3 - "$@" <<'PY'
import json
import os
import pathlib
import re
import sys
import tempfile


def usage(message=""):
    if message:
        print(message, file=sys.stderr)
    print("usage: generate-issues-index.sh [--check] [--issues-dir DIR] [--output FILE]", file=sys.stderr)
    raise SystemExit(2)


check = False
issues_dir = pathlib.Path("docs/issues")
output = None
args = iter(sys.argv[1:])
for arg in args:
    if arg == "--check":
        check = True
    elif arg == "--issues-dir":
        try:
            issues_dir = pathlib.Path(next(args))
        except StopIteration:
            usage("--issues-dir requires a value")
    elif arg == "--output":
        try:
            output = pathlib.Path(next(args))
        except StopIteration:
            usage("--output requires a value")
    else:
        usage(f"unknown argument: {arg}")
output = output or issues_dir / "INDEX.md"


def unquote(value):
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] == '"':
        try:
            return json.loads(value)
        except json.JSONDecodeError:
            return value[1:-1]
    if len(value) >= 2 and value[0] == value[-1] == "'":
        return value[1:-1].replace("''", "'")
    if value.lower() in ("null", "~"):
        return ""
    return value


def frontmatter(path):
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0].strip() != "---":
        raise ValueError("missing opening frontmatter marker")
    try:
        end = next(i for i, line in enumerate(lines[1:], 1) if line.strip() == "---")
    except StopIteration as exc:
        raise ValueError("missing closing frontmatter marker") from exc

    fields = {}
    link_fields = {}
    in_links = False
    for line in lines[1:end]:
        top = re.match(r"^([A-Za-z_][A-Za-z0-9_]*):(?:[ \t]*(.*))?$", line)
        if top:
            key, value = top.group(1), top.group(2) or ""
            fields[key] = unquote(value)
            in_links = key == "links"
            continue
        if in_links:
            nested = re.match(r"^[ \t]+([A-Za-z_][A-Za-z0-9_]*):(?:[ \t]*(.*))?$", line)
            if nested:
                link_fields[nested.group(1)] = unquote(nested.group(2) or "")
    fields["_links"] = link_fields
    return fields


def cell(value):
    return str(value).replace("|", "&#124;").replace("\n", " ").strip()


def links_cell(links):
    parts = []
    adr = links.get("adr", "").strip()
    if adr:
        match = re.search(r"(ADR-\d+)", adr, re.IGNORECASE)
        parts.append(match.group(1).upper() if match else adr)
    prs = re.findall(r"\d+", links.get("prs", ""))
    if prs:
        parts.append("PR#" + ",".join(str(int(pr)) for pr in prs))
    return ", ".join(parts)


records = []
seen_ids = set()
for path in issues_dir.glob("[0-9][0-9][0-9][0-9]-*.md"):
    try:
        fields = frontmatter(path)
    except (OSError, ValueError) as exc:
        print(f"{path}: {exc}", file=sys.stderr)
        raise SystemExit(1) from None
    issue_id = fields.get("id", "")
    required = ("id", "title", "status", "severity")
    missing = [key for key in required if not fields.get(key, "")]
    if missing:
        print(f"{path}: missing frontmatter field(s): {', '.join(missing)}", file=sys.stderr)
        raise SystemExit(1)
    if not re.fullmatch(r"\d{4}", issue_id) or issue_id != path.name[:4]:
        print(f"{path}: frontmatter id must match the four-digit filename prefix", file=sys.stderr)
        raise SystemExit(1)
    if issue_id in seen_ids:
        print(f"duplicate issue id: {issue_id}", file=sys.stderr)
        raise SystemExit(1)
    seen_ids.add(issue_id)
    records.append({
        "id": issue_id,
        "filename": path.name,
        "title": fields["title"],
        "status": fields["status"],
        "severity": fields["severity"],
        "group": fields.get("group", ""),
        "links": links_cell(fields["_links"]),
    })

records.sort(key=lambda record: int(record["id"]))
epics = [record for record in records if record["title"].lstrip().lower().startswith("epic:")]
issues = [record for record in records if record not in epics]
children = {}
for record in records:
    if record["group"]:
        children.setdefault(record["group"], []).append(record["id"])

lines = [
    "# Issues index — twiceshy",
    "",
    "Generated from issue-file frontmatter by `scripts/generate-issues-index.sh`.",
    "The files are canonical; do not edit this index by hand. Run the generator",
    "after changing issue lifecycle metadata, or use `--check` to detect drift.",
    "New issues: `scripts/new-issue.sh \"<title>\" [--epic] [--group <id>]`,",
    "then mirror them with `scripts/sync-forgejo.sh`.",
    "",
]

if epics:
    lines.extend([
        "## Epics",
        "",
        "| id | title | status | children |",
        "|----|-------|--------|----------|",
    ])
    for record in epics:
        kids = ", ".join(children.get(record["id"], [])) or "—"
        lines.append(
            f'| [{record["id"]}]({record["filename"]}) | {cell(record["title"])} | '
            f'{cell(record["status"])} | {kids} |'
        )
    lines.append("")

lines.extend([
    "## Issues",
    "",
    "| id | title | status | severity | group | links |",
    "|----|-------|--------|----------|-------|-------|",
])
for record in issues:
    group = cell(record["group"]) or "—"
    row = (
        f'| [{record["id"]}]({record["filename"]}) | {cell(record["title"])} | '
        f'{cell(record["status"])} | {cell(record["severity"])} | {group} | '
    )
    links = cell(record["links"])
    lines.append(row + (f'{links} |' if links else '|'))
lines.append("")
generated = "\n".join(lines)

try:
    current = output.read_text(encoding="utf-8")
except FileNotFoundError:
    current = None

if check:
    if current != generated:
        print(f"{output} is out of date; run scripts/generate-issues-index.sh", file=sys.stderr)
        raise SystemExit(1)
    raise SystemExit(0)

if current == generated:
    raise SystemExit(0)
output.parent.mkdir(parents=True, exist_ok=True)
fd, temp_name = tempfile.mkstemp(prefix=output.name + ".", dir=output.parent)
try:
    os.fchmod(fd, 0o644)
    with os.fdopen(fd, "w", encoding="utf-8") as handle:
        handle.write(generated)
    os.replace(temp_name, output)
finally:
    try:
        os.unlink(temp_name)
    except FileNotFoundError:
        pass
PY
