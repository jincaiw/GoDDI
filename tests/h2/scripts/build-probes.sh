#!/usr/bin/env bash
# Build the real DNS protocol probes and install them into the client
# container. They are Go programs from this repo (not mocks) because the client
# image has no dig; rebuilding them here keeps `make h2-up` reproducible after
# `make h2-down` removes the containers.
set -uo pipefail
cd "$(dirname "$0")/.."
ROOT="$(cd ../.. && pwd)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
for tool in axfrprobe notifyprobe; do
  if ! (cd "$ROOT" && CGO_ENABLED=0 GOOS=linux go build -mod=readonly -o "$TMP/$tool" "./tests/h2/tools/$tool"); then
    echo "FAIL | could not build $tool"; exit 1
  fi
  docker cp "$TMP/$tool" "client:/usr/local/bin/$tool" >/dev/null 2>&1 || { echo "FAIL | could not install $tool into client"; exit 1; }
done
echo "PASS | DNS probes installed in the client container (axfrprobe, notifyprobe)"
