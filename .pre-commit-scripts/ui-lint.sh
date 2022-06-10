#!/bin/bash -e
files=""
for f in "$@"; do
  uf=$(echo $f | sed 's/^ui\///')
  files+="$uf "
done
cd ui
npx prettier --check $files --loglevel warn
npx eslint $files --max-warnings 0
