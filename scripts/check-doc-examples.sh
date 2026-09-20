#!/usr/bin/env bash
#
# check-doc-examples.sh: type-check every complete Go program in the docs.
#
# The documentation accumulated examples calling functions that never existed
# (NewRemoteConfigWithFallback, InitiateShutdown, a Stats struct, an
# internal/cli import path) and examples that simply did not compile — the
# first example in the quick-start guide called a pointer method on a composite
# literal. Nothing checked them, so nothing noticed.
#
# Blocks that do not start with "package main" are fragments by design and are
# skipped; so are the provider-tutorial blocks that build one file up across
# several sections.
#
# Copyright (c) 2025 AGILira - A. Giordano
# SPDX-License-Identifier: MPL-2.0

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cat > "$work/go.mod" <<EOF
module docexamples

go 1.25.9

require github.com/agilira/argus v0.0.0

replace github.com/agilira/argus => $repo_root

replace github.com/agilira/argus/cmd/cli => $repo_root/cmd/cli
EOF

# Tutorial sections that deliberately build one file up across several blocks.
SKIP_PREFIXES="provider_tutorial"

python3 - "$repo_root" "$work" <<'PY'
import pathlib, re, sys

repo, work = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
sources = [repo / "README.md"] + sorted((repo / "docs").glob("*.md"))

for doc in sources:
    for index, block in enumerate(re.findall(r"```go\n(.*?)```", doc.read_text(), re.S)):
        if not block.lstrip().startswith("package main"):
            continue
        name = f"{doc.stem.replace('-', '_')}_{index}"
        target = work / name
        target.mkdir(exist_ok=True)
        (target / "main.go").write_text(block)
PY

cd "$work"

# Resolve the examples' imports. This must not be allowed to fail quietly: a
# tidy that silently does nothing reports every example as broken for the wrong
# reason, which is how a check stops meaning anything.
if ! tidy_output="$(go mod tidy 2>&1)"; then
    printf 'go mod tidy failed; cannot check documentation examples\n' >&2
    printf '%s\n' "$tidy_output" >&2
    exit 1
fi

status=0
found=0
for dir in */; do
    name="${dir%/}"
    skip=0
    for prefix in $SKIP_PREFIXES; do
        case "$name" in "$prefix"*) skip=1 ;; esac
    done
    [ "$skip" -eq 1 ] && continue

    found=$((found + 1))
    if output="$(go vet "./$name" 2>&1)"; then
        printf '  ok    %s\n' "$name"
    else
        printf '  FAIL  %s\n' "$name"
        printf '%s\n' "$output" | sed 's/^/        /'
        status=1
    fi
done

printf '\n%d documentation examples checked\n' "$found"
exit $status
