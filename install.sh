#!/bin/bash
set -e

# nethelper 安装脚本
# 用法: ./install.sh

# ============================================================================
# 颜色定义
# ============================================================================
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

info()  { echo -e "${BLUE}[INFO]${NC} $1"; }
ok()    { echo -e "${GREEN}[OK]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ============================================================================
# 检查前置依赖
# ============================================================================
echo ""
echo "=========================================="
echo "  nethelper 安装程序 v0.1.0"
echo "=========================================="
echo ""

# 检查 Go
if ! command -v go &> /dev/null; then
    error "Go 未安装。请先安装 Go 1.22+："
    echo "  macOS:  brew install go"
    echo "  Linux:  https://go.dev/dl/"
    exit 1
fi

GO_VERSION=$(go version | sed 's/.*go\([0-9]*\.[0-9]*\).*/\1/')
info "Go 版本: $GO_VERSION"

# ============================================================================
# 确定安装路径
# ============================================================================
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
DATA_DIR="$HOME/.nethelper"
CONFIG_FILE="$DATA_DIR/config.yaml"

# 如果 /usr/local/bin 需要 sudo，尝试 ~/go/bin 或 ~/.local/bin
if [ ! -w "$INSTALL_DIR" ] 2>/dev/null; then
    if [ -d "$HOME/go/bin" ]; then
        INSTALL_DIR="$HOME/go/bin"
    elif [ -d "$HOME/.local/bin" ]; then
        INSTALL_DIR="$HOME/.local/bin"
    else
        mkdir -p "$HOME/.local/bin"
        INSTALL_DIR="$HOME/.local/bin"
    fi
    warn "使用用户目录: $INSTALL_DIR"
fi

info "项目目录:   $PROJECT_DIR"
info "安装目录:   $INSTALL_DIR"
info "数据目录:   $DATA_DIR"
echo ""

# ============================================================================
# Step 1: 编译
# ============================================================================
info "Step 1/4: 编译 nethelper..."

cd "$PROJECT_DIR"
go build -o nethelper ./cmd/nethelper

if [ ! -f "$PROJECT_DIR/nethelper" ]; then
    error "编译失败"
    exit 1
fi

ok "编译成功 ($(du -h nethelper | cut -f1 | tr -d ' '))"

# ============================================================================
# Step 2: 安装到 PATH
# ============================================================================
info "Step 2/4: 安装到 $INSTALL_DIR..."

cp "$PROJECT_DIR/nethelper" "$INSTALL_DIR/nethelper"
chmod +x "$INSTALL_DIR/nethelper"

ok "已安装到 $INSTALL_DIR/nethelper"

# 检查是否在 PATH 中
if ! command -v nethelper &> /dev/null; then
    warn "nethelper 不在 PATH 中。请添加到 shell 配置："
    echo ""
    SHELL_NAME=$(basename "$SHELL")
    case "$SHELL_NAME" in
        zsh)
            echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc"
            echo "  source ~/.zshrc"
            ;;
        bash)
            echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.bashrc"
            echo "  source ~/.bashrc"
            ;;
        *)
            echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
            ;;
    esac
    echo ""
else
    ok "nethelper 已在 PATH 中"
fi

# ============================================================================
# Step 3: 初始化数据目录
# ============================================================================
info "Step 3/4: 初始化数据目录..."

mkdir -p "$DATA_DIR"
mkdir -p "$DATA_DIR/logs"

if [ ! -f "$CONFIG_FILE" ]; then
    cp "$PROJECT_DIR/configs/config.example.yaml" "$CONFIG_FILE"
    ok "配置模板已复制到 $CONFIG_FILE"
    warn "请编辑配置文件设置日志监控目录和 LLM provider"
else
    ok "配置文件已存在: $CONFIG_FILE"
fi

# ============================================================================
# Step 4: 验证
# ============================================================================
info "Step 4/4: 验证安装..."
echo ""

NETHELPER_BIN="$INSTALL_DIR/nethelper"
$NETHELPER_BIN version
echo ""
$NETHELPER_BIN config llm 2>/dev/null || true

# ============================================================================
# 完成
# ============================================================================
echo ""
echo "=========================================="
echo -e "  ${GREEN}安装完成！${NC}"
echo "=========================================="
echo ""
echo "  快速开始:"
echo ""
echo "  1. 编辑配置文件:"
echo "     vim $CONFIG_FILE"
echo ""
echo "  2. 导入一个日志文件试试:"
echo "     nethelper watch ingest <your-log-file>"
echo ""
echo "  3. 查看解析到的设备:"
echo "     nethelper show device"
echo ""
echo "  4. 启动自动监控:"
echo "     nethelper watch start"
echo ""
echo "  5. 查看所有命令:"
echo "     nethelper --help"
echo ""

# 清理编译产物
rm -f "$PROJECT_DIR/nethelper"
