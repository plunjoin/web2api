#!/bin/bash
# ============================================================
# web2api 一键安装脚本 —— 在 VPS 上执行
# 用法：bash install.sh
# ============================================================
set -e

INSTALL_DIR="/opt/web2api"
SERVICE_NAME="web2api"

# 自动检测架构。发布包内使用统一的 web2api 文件名；兼容旧版目录中的
# web2api-linux-* 文件，方便直接把 install.sh 和多个二进制放在同一目录。
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    LEGACY_BIN="web2api-linux-amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    LEGACY_BIN="web2api-linux-arm64"
elif [ "$ARCH" = "armv7l" ] || [ "$ARCH" = "armv7" ]; then
    LEGACY_BIN="web2api-linux-armv7"
else
    echo "[错误] 不支持的架构: $ARCH"
    exit 1
fi

if [ -f "web2api" ]; then
    BIN_FILE="web2api"
elif [ -f "$LEGACY_BIN" ]; then
    BIN_FILE="$LEGACY_BIN"
else
    BIN_FILE="$LEGACY_BIN"
fi

echo "检测到架构: $ARCH → 使用 $BIN_FILE"

# 1. 创建目录
echo "→ 创建部署目录 $INSTALL_DIR"
sudo mkdir -p "$INSTALL_DIR"/{data,auth}

# 2. 复制二进制
if [ ! -f "$BIN_FILE" ]; then
    echo "[错误] 当前目录找不到 $BIN_FILE"
    echo "  请确保 install.sh 和二进制文件在同一目录"
    exit 1
fi
echo "→ 安装二进制"
sudo cp "$BIN_FILE" "$INSTALL_DIR/web2api"
sudo chmod +x "$INSTALL_DIR/web2api"

# 3. 复制配置（已有则不覆盖）
if [ ! -f "$INSTALL_DIR/config.yaml" ]; then
    echo "→ 安装默认配置"
    sudo cp config.yaml "$INSTALL_DIR/config.yaml"
else
    echo "→ 配置已存在，跳过（如需更新请手动编辑 $INSTALL_DIR/config.yaml）"
fi

# 4. 安装 systemd 服务
echo "→ 安装 systemd 服务"
sudo cp web2api.service /etc/systemd/system/web2api.service
sudo systemctl daemon-reload
sudo systemctl enable web2api

# 5. 启动
echo "→ 启动服务"
sudo systemctl restart web2api
sleep 2

# 6. 检查状态
if sudo systemctl is-active --quiet web2api; then
    echo ""
    echo "=============================================="
    echo "  web2api 启动成功！"
    echo "  管理台: http://$(hostname -I | awk '{print $1}'):8800/admin"
    echo "  API地址: http://$(hostname -I | awk '{print $1}'):8800/v1"
    echo "=============================================="
    echo ""
    echo "常用命令:"
    echo "  查看日志:   sudo journalctl -u web2api -f"
    echo "  停止:       sudo systemctl stop web2api"
    echo "  重启:       sudo systemctl restart web2api"
    echo "  查看状态:   sudo systemctl status web2api"
    echo ""
    echo "记得放行 8800 端口（防火墙 + 云厂商安全组）"
else
    echo "[错误] 服务启动失败，查看日志:"
    sudo journalctl -u web2api --no-pager -n 20
    exit 1
fi
