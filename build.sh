#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-dev}"
OUTPUT="${2:-dist}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST="$ROOT/$OUTPUT"

command -v go >/dev/null || { echo "未找到 Go，请先安装 Go 1.27 或更高版本。" >&2; exit 1; }
command -v tar >/dev/null || { echo "未找到 tar。" >&2; exit 1; }

rm -rf "$DIST"
mkdir -p "$DIST"
export CGO_ENABLED=0
export GOCACHE="$ROOT/.gocache-build"

build_one() {
  local name="$1" goos="$2" goarch="$3" goarm="${4:-}" ext="$5"
  local package="web2api-$VERSION-$name"
  local stage="$DIST/$package"
  mkdir -p "$stage"
  if [[ -n "$goarm" ]]; then export GOARM="$goarm"; else unset GOARM || true; fi
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags='-s -w' -o "$stage/web2api$ext" .
  cp "$ROOT/config.example.yaml" "$stage/config.example.yaml"
  cp "$ROOT/README.md" "$stage/README.md"
  if [[ "$goos" == windows ]]; then
    cp "$ROOT/start.bat" "$stage/start.bat"
  elif [[ "$goos" == linux ]]; then
    cp "$ROOT/deploy/README.md" "$stage/DEPLOY.md"
    cp "$ROOT/deploy/install.sh" "$stage/install.sh"
    cp "$ROOT/deploy/web2api.service" "$stage/web2api.service"
    cp "$ROOT/deploy/config.yaml" "$stage/config.yaml"
  fi
  tar -czf "$DIST/$package.tar.gz" -C "$DIST" "$package"
  rm -rf "$stage"
  echo "已生成 $DIST/$package.tar.gz"
}

build_one linux-amd64 linux amd64 '' ''
build_one linux-arm64 linux arm64 '' ''
build_one linux-armv7 linux arm 7 ''
build_one darwin-amd64 darwin amd64 '' ''
build_one darwin-arm64 darwin arm64 '' ''

# Windows 包在 Unix 构建机上也可以交叉编译，统一使用 tar.gz，便于在 CI 中上传。
build_one windows-amd64 windows amd64 '' .exe
build_one windows-arm64 windows arm64 '' .exe

if command -v sha256sum >/dev/null; then
  (cd "$DIST" && sha256sum web2api-*.tar.gz > SHA256SUMS)
elif command -v shasum >/dev/null; then
  (cd "$DIST" && shasum -a 256 web2api-*.tar.gz > SHA256SUMS)
else
  echo "未找到 sha256sum 或 shasum，无法生成 SHA256SUMS。" >&2
  exit 1
fi
echo "完成。发布包和 SHA256SUMS 位于 $DIST"
