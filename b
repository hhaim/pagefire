#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat <<'EOF'
Usage: ./b --run
       ./b push [tag]

  --run       Build and run PageFire with Docker Compose.
  push [tag]  Build for linux/amd64 and push to frigate:5001.

Push settings: DOCKER_REGISTRY, PAGEFIRE_REPO, PAGEFIRE_TAG,
               BUILDX_BUILDER, DOCKER_REGISTRY_INSECURE.
EOF
}

ensure_insecure_buildx_builder() {
  local builder="$1"
  local registry="$2"

  if docker buildx inspect "$builder" >/dev/null 2>&1; then
    return
  fi
  case "$registry" in
    *$'\n'*|*\"*)
      echo "Invalid DOCKER_REGISTRY for BuildKit config: $registry" >&2
      return 2
      ;;
  esac

  local config
  config="$(mktemp)"
  cat >"$config" <<EOF
[registry."$registry"]
  http = true
  insecure = true
EOF
  if ! docker buildx create \
    --name "$builder" \
    --driver docker-container \
    --config "$config" \
    >/dev/null; then
    rm -f "$config"
    return 1
  fi
  rm -f "$config"
  docker buildx inspect "$builder" --bootstrap >/dev/null
}

case "${1:-}" in
  --run)
    if (($# != 1)); then
      usage >&2
      exit 2
    fi
    cd "$ROOT"
    docker compose up -d --build pagefire
    ;;
  push)
    if (($# > 2)); then
      usage >&2
      exit 2
    fi
    registry="${DOCKER_REGISTRY:-frigate:5001}"
    repo="${PAGEFIRE_REPO:-${registry}/pagefire/pagefire}"
    tag="${2:-${PAGEFIRE_TAG:-latest}}"
    image="${repo}:${tag}"
    buildx_args=()

    case "${DOCKER_REGISTRY_INSECURE:-true}" in
      false|FALSE|0|no|NO)
        ;;
      *)
        safe_registry="${registry//[^A-Za-z0-9_.-]/-}"
        builder="${BUILDX_BUILDER:-pagefire-${safe_registry}}"
        ensure_insecure_buildx_builder "$builder" "$registry"
        buildx_args+=(--builder "$builder")
        ;;
    esac

    echo "Building and pushing ${image} for linux/amd64"
    docker buildx build \
      "${buildx_args[@]}" \
      --platform linux/amd64 \
      -f "$ROOT/Dockerfile" \
      -t "$image" \
      --push \
      "$ROOT"
    ;;
  -h|--help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
