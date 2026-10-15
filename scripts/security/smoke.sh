#!/usr/bin/env bash
set -euo pipefail
image=${1:?image reference required}
docker run --rm "$image" version
docker run --rm "$image" diff --help >/dev/null
docker run --rm --entrypoint sh "$image" -ec 'command -v git && command -v grep && command -v mobius && command -v møbius'
# Verify explicit non-root execution, a read-only rootfs, and writable caches.
docker run --rm --user 65532:65532 --read-only --tmpfs /tmp:rw,mode=1777 \
  --cap-drop ALL --security-opt no-new-privileges \
  --entrypoint sh "$image" -ec 'test "$(id -u)" = 65532; mkdir -p "$HOME"; git init /tmp/repo; mobius version; møbius diff --help >/dev/null'
