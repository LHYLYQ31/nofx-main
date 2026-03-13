#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   GHCR_TOKEN=xxx ./docker/push-images.sh backend
#   GHCR_TOKEN=xxx ./docker/push-images.sh frontend
#   GHCR_TOKEN=xxx ./docker/push-images.sh all

REGISTRY="ghcr.io"
OWNER="lhylyq31"
REPO="newmoneyclub"
TAG="latest"
GHCR_USER="LHYLYQ31"

if [[ -z "${GHCR_TOKEN:-}" ]]; then
  echo "Error: GHCR_TOKEN is not set"
  exit 1
fi

TARGET="${1:-all}"

login() {
  echo "$GHCR_TOKEN" | docker login "$REGISTRY" -u "$GHCR_USER" --password-stdin
}

build_backend() {
  docker build -f ./docker/Dockerfile.backend -t backend-image .
  docker tag backend-image "$REGISTRY/$OWNER/$REPO/backend:$TAG"
  docker push "$REGISTRY/$OWNER/$REPO/backend:$TAG"
}

build_frontend() {
  docker build -f ./docker/Dockerfile.frontend -t frontend-image .
  docker tag frontend-image "$REGISTRY/$OWNER/$REPO/frontend:$TAG"
  docker push "$REGISTRY/$OWNER/$REPO/frontend:$TAG"
}

case "$TARGET" in
  backend)
    login
    build_backend
    ;;
  frontend)
    login
    build_frontend
    ;;
  all)
    login
    build_backend
    build_frontend
    ;;
  *)
    echo "Usage: $0 [backend|frontend|all]"
    exit 1
    ;;
esac