#!/bin/sh
set -eu

say() { printf '%s\n' "$*" >&2; }
TTY=/dev/tty
if [ ! -r "$TTY" ]; then TTY=; fi
prompt() {
  label=$1
  printf '%s' "$label" >&2
  if [ -n "$TTY" ]; then IFS= read -r value < "$TTY"; else IFS= read -r value; fi
  printf '%s' "$value"
}

command -v node >/dev/null 2>&1 || { say "需要先安装 Node.js 18+：https://nodejs.org/"; exit 1; }
command -v npm >/dev/null 2>&1 || { say "需要先安装 npm。"; exit 1; }
command -v curl >/dev/null 2>&1 || { say "需要先安装 curl。"; exit 1; }

WEB2API_URL=${WEB2API_URL:-${1:-}}
if [ -z "$WEB2API_URL" ]; then
  WEB2API_URL=$(prompt "后端地址（例如 https://api.example.com:8800）：")
fi
WEB2API_URL=${WEB2API_URL%/}
WEB2API_ADMIN_JWT=${WEB2API_ADMIN_JWT:-}
if [ -z "$WEB2API_ADMIN_JWT" ]; then
  printf '%s' "管理员 JWT（输入时不显示）：" >&2
  stty -echo 2>/dev/null || true
  if [ -n "$TTY" ]; then IFS= read -r WEB2API_ADMIN_JWT < "$TTY"; else IFS= read -r WEB2API_ADMIN_JWT; fi
  stty echo 2>/dev/null || true
  printf '\n' >&2
fi
WEB2API_EMAIL=${WEB2API_EMAIL:-}
if [ -z "$WEB2API_EMAIL" ]; then WEB2API_EMAIL=$(prompt "AI Studio 登录邮箱："); fi
WEB2API_LABEL=${WEB2API_LABEL:-$WEB2API_EMAIL}

tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t web2api)
cleanup() { rm -rf "$tmp_dir"; }
trap cleanup EXIT INT TERM

say "正在准备本机 Playwright（首次运行会下载浏览器，可能需要几分钟）..."
cd "$tmp_dir"
npm init -y >/dev/null 2>&1
npm pkg set type=module >/dev/null 2>&1
npm install --no-audit --no-fund --silent "playwright@1.55.0"
npx playwright install chromium >/dev/null

# 远程脚本只负责启动，真正的浏览器逻辑随同版本一起从后端取回。
curl -fsSL "$WEB2API_URL/tools/export-storage.js" -o export-storage.js
export WEB2API_URL WEB2API_ADMIN_JWT WEB2API_EMAIL WEB2API_LABEL WEB2API_TTY="$TTY"
node export-storage.js
