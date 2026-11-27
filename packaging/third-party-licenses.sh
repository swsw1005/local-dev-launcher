#!/usr/bin/env bash
set -euo pipefail

printf 'Go standard library\n\n'
cat "$(go env GOROOT)/LICENSE"
for task_arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$task_arch" go list -deps -f '{{if and .Module (not .Module.Main)}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' ./cmd/ldr
done | sort -u | while IFS='|' read -r module version directory; do
  [ -n "$module" ] || continue
  found=0
  for file in "$directory"/LICENSE "$directory"/LICENSE.txt "$directory"/LICENCE "$directory"/COPYING; do
    if [ -f "$file" ]; then
      printf '\n\n===== %s %s =====\n\n' "$module" "$version"
      cat "$file"
      found=1
      break
    fi
  done
  [ "$found" = 1 ] || { echo "License file missing for $module" >&2; exit 1; }
  if [ -f "$directory/NOTICE" ]; then
    printf '\n\n===== %s NOTICE =====\n\n' "$module"
    cat "$directory/NOTICE"
  fi
done
