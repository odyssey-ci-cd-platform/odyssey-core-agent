#!/bin/sh
# Push a branch, open its PR, watch CI until done.
# Usage: scripts/ship-pr.sh BRANCH TITLE [BODY-FILE]
# Body defaults to "Closes #N" (first number in BRANCH); falls back to a
# one-commit-task note when BRANCH carries no number.
set -eu

branch=${1:?branch required}
title=${2:?title required}
body_file=${3:-}

if [ -z "$body_file" ]; then
  n=$(printf '%s' "$branch" | grep -oE '[0-9]+' | head -1 || true)
  body_file=$(mktemp)
  if [ -n "$n" ]; then
    printf 'Closes #%s.\n' "$n" > "$body_file"
  else
    printf 'One-commit task (no issue).\n' > "$body_file"
  fi
fi

git push -u origin "$branch"
gh pr create --base main --head "$branch" --title "$title" --body-file "$body_file"
sleep 12
gh pr checks "$branch" --watch --interval 10
