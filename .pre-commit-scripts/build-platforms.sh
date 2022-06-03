#!/bin/bash
set -e
cleanup() {
  ARG=$?
  rm -f testbin
  exit $ARG
}
trap cleanup EXIT
GOOS=linux   go build -o testbin cmd/connectopus.go
GOOS=windows go build -o testbin cmd/connectopus.go
GOOS=darwin  go build -o testbin cmd/connectopus.go
