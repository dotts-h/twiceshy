#!/usr/bin/env bash
# codemap.sh — generate docs/CODEMAP.md, a per-directory source index.
#
# A session should read CODEMAP.md to learn the layout (which file holds which
# top-level declaration) instead of opening source files to find out. Regenerate
# after adding/moving/renaming top-level declarations. Generated — never hand-edit.
#
# Language-agnostic: pure text extraction per file extension; no build, no network.
set -euo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
out="docs/CODEMAP.md"
mkdir -p docs

decl_pattern() {
  case "$1" in
    go)        echo '^(type|func) ';;
    py)        echo '^(class|def|async def) ';;
    js|jsx|ts|tsx|mjs) echo '^(export |function |class |const [A-Z])';;
    rs)        echo '^(pub )?(fn|struct|enum|trait|impl|mod) ';;
    rb)        echo '^(class|module|def) ';;
    java|kt)   echo '^(public|protected|private|class|interface|enum)';;
    sh)        echo '^[A-Za-z_][A-Za-z0-9_]*\(\)';;
    *)         echo '';;
  esac
}

# Source files: tracked-if-possible, common code extensions, no vendored trees.
list_files() {
  { git ls-files 2>/dev/null || find . -type f | sed 's|^\./||'; } \
    | grep -E '\.(go|py|js|jsx|ts|tsx|mjs|rs|rb|java|kt|sh)$' \
    | grep -vE '(^|/)(vendor|node_modules|dist|build|target|\.git)/' \
    | grep -vE '(_test\.go|\.test\.[jt]sx?|_spec\.rb)$' \
    | sort
}

{
  echo "# CODEMAP — generated, do not edit by hand"
  echo
  echo "> Regenerate with \`scripts/codemap.sh\`. A per-directory index of source"
  echo "> files and their top-level declarations, so a session learns the layout"
  echo "> from this one file instead of opening source to find a symbol. The source"
  echo "> is the source of truth — if this looks stale, re-run the script."
  echo
  echo "_Last generated: $(date -u +%Y-%m-%d) (UTC)._"
  echo
  prev_dir=""
  while IFS= read -r f; do
    [ -f "$f" ] || continue
    dir=$(dirname "$f")
    if [ "$dir" != "$prev_dir" ]; then
      echo "## $dir"; echo
      prev_dir="$dir"
    fi
    loc=$(wc -l <"$f" | tr -d ' ')
    ext="${f##*.}"
    pat=$(decl_pattern "$ext")
    echo "### $(basename "$f") ($loc LOC)"
    if [ -n "$pat" ]; then
      decls=$(grep -nE "$pat" "$f" 2>/dev/null | sed -E 's/[[:space:]]*\{[[:space:]]*$//' | head -40 || true)
      if [ -n "$decls" ]; then
        echo "$decls" | sed -E 's/^([0-9]+):(.*)$/- L\1: `\2`/'
      else
        echo "- _(no top-level declarations matched)_"
      fi
    else
      echo "- _(unindexed file type)_"
    fi
    echo
  done < <(list_files)
} >"$out"

echo "wrote $out"
