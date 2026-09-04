#!/usr/bin/env bash
set -Eeuo pipefail

suite=${1:-all}
root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root_dir"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "缺少必需命令: $1" >&2
    exit 1
  }
}

prepare_web() {
  require node
  require npm
  if [[ ! -d web/node_modules ]]; then
    (cd web && npm ci)
  fi
}

unit() {
  require go
  prepare_web
  echo '== Unit tests =='
  go test -race ./internal/auth ./internal/frp ./internal/version
  node --test tests/*.test.mjs
  (cd web && npm test)
}

integration() {
  require go
  echo '== Integration tests =='
  go test -race ./internal/manager -run 'Test' -skip 'TestRemoteRelayAuthenticatesAndMovesFramesAndInputWithoutPersistence' -count=1
}

static_checks() {
  require go
  prepare_web
  require git
  echo '== Build and static checks =='
  go vet ./...
  mkdir -p bin
  CGO_ENABLED=0 go build -trimpath -o bin/frp-manager ./cmd/frp-manager
  (cd web && npm run lint && npm run build:static)
  bash -n scripts/bootstrap.sh scripts/install.sh scripts/verify.sh

  private_prefix='-----BEGIN '
  private_suffix='(OPENSSH|RSA|EC|DSA) PRIVATE KEY-----'
  github_prefix='github''_pat_'
  gh_prefix='gh''[pousr]_'
  pattern="${private_prefix}${private_suffix}|${github_prefix}[A-Za-z0-9_]{20,}|${gh_prefix}[A-Za-z0-9]{20,}"
  if git grep -nE -- "$pattern"; then
    echo '检测到疑似真实凭据或私钥。' >&2
    exit 1
  fi
}

e2e() {
  require go
  prepare_web
  echo '== Core E2E =='
  go test ./internal/manager -run 'TestRemoteRelayAuthenticatesAndMovesFramesAndInputWithoutPersistence' -count=1 -v
  (cd web && npm run build:static && npx playwright test)
}

case "$suite" in
  unit) unit ;;
  integration) integration ;;
  static) static_checks ;;
  e2e) e2e ;;
  all)
    unit
    integration
    static_checks
    e2e
    ;;
  *)
    echo "用法: $0 {unit|integration|static|e2e|all}" >&2
    exit 2
    ;;
esac

echo "验证完成: $suite"
