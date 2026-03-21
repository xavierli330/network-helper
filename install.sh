#!/bin/bash
set -e

# nethelper 安装脚本
# 用法: ./install.sh

# ============================================================================
# 颜色与工具函数
# ============================================================================
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
ok()      { echo -e "${GREEN}  ✓${NC} $1"; }
warn()    { echo -e "${YELLOW}  ⚠${NC} $1"; }
error()   { echo -e "${RED}  ✗${NC} $1"; }
prompt()  { echo -en "${CYAN}  ?${NC} $1"; }
section() { echo -e "\n${BOLD}$1${NC}\n"; }

# 读取用户输入，带默认值
ask() {
    local var_name=$1 prompt_text=$2 default=$3
    prompt "$prompt_text [${default}]: "
    read -r input
    eval "$var_name=\"${input:-$default}\""
}

# ============================================================================
# 欢迎
# ============================================================================
clear 2>/dev/null || true
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║       nethelper 安装向导 v0.1.0          ║${NC}"
echo -e "${BOLD}║  网络工程师的排障效率工具                ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════╝${NC}"
echo ""
echo "  支持厂商: 华为 VRP | 思科 IOS | 华三 Comware | Juniper JUNOS"
echo "  核心能力: 日志解析 · 拓扑分析 · 配置追踪 · FTS5搜索 · LLM增强"
echo ""

# ============================================================================
# Step 1: 检查前置依赖
# ============================================================================
section "Step 1/5: 检查环境"

# Go
if ! command -v go &> /dev/null; then
    error "Go 未安装"
    echo ""
    echo "    请先安装 Go 1.22+："
    echo "    macOS:   brew install go"
    echo "    Linux:   https://go.dev/dl/"
    echo ""
    exit 1
fi

GO_VERSION=$(go version | sed 's/.*go\([0-9]*\.[0-9]*\).*/\1/')
ok "Go $GO_VERSION"

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
ok "项目目录: $PROJECT_DIR"

# ============================================================================
# Step 2: 交互式配置
# ============================================================================
section "Step 2/5: 配置选项"

echo "  以下选项按回车使用默认值，也可以输入自定义路径。"
echo ""

# 数据目录
ask DATA_DIR "数据存储目录（数据库、日志、PID 文件）" "$HOME/.nethelper"

# 安装目录
DEFAULT_INSTALL="/usr/local/bin"
if [ ! -w "$DEFAULT_INSTALL" ] 2>/dev/null; then
    if [ -d "$HOME/go/bin" ]; then
        DEFAULT_INSTALL="$HOME/go/bin"
    else
        DEFAULT_INSTALL="$HOME/.local/bin"
    fi
fi
ask INSTALL_DIR "可执行文件安装目录" "$DEFAULT_INSTALL"

# 日志监控目录
ask WATCH_DIR "网络设备日志目录（终端回显保存的位置）" "$HOME/network-logs"

# LLM 配置
echo ""
echo "  LLM 大语言模型是可选的增强功能（排障建议、配置解读）。"
echo "  不配置也不影响核心功能。"
echo ""
prompt "是否现在配置 LLM？(y/N): "
read -r SETUP_LLM
SETUP_LLM=${SETUP_LLM:-n}

LLM_PROVIDER=""
LLM_API_KEY=""
LLM_MODEL=""
LLM_BASE_URL=""

if [[ "$SETUP_LLM" =~ ^[Yy] ]]; then
    echo ""
    echo "  可选 LLM provider:"
    echo "    1) Ollama（本地运行，免费，推荐）"
    echo "    2) OpenAI"
    echo "    3) DeepSeek"
    echo "    4) 通义千问（阿里云）"
    echo "    5) 其他 OpenAI 兼容服务"
    echo ""
    prompt "选择 (1-5): "
    read -r LLM_CHOICE
    LLM_CHOICE=${LLM_CHOICE:-1}

    case "$LLM_CHOICE" in
        1)
            LLM_PROVIDER="ollama"
            LLM_MODEL="qwen2.5:14b"
            LLM_BASE_URL="http://localhost:11434"
            ask LLM_MODEL "Ollama 模型名" "$LLM_MODEL"
            ask LLM_BASE_URL "Ollama 地址" "$LLM_BASE_URL"
            ;;
        2)
            LLM_PROVIDER="openai"
            ask LLM_API_KEY "OpenAI API Key" ""
            ask LLM_MODEL "模型" "gpt-4o-mini"
            LLM_BASE_URL=""
            ;;
        3)
            LLM_PROVIDER="deepseek"
            ask LLM_API_KEY "DeepSeek API Key" ""
            LLM_MODEL="deepseek-chat"
            LLM_BASE_URL="https://api.deepseek.com/v1"
            ;;
        4)
            LLM_PROVIDER="qwen"
            ask LLM_API_KEY "通义千问 API Key" ""
            LLM_MODEL="qwen-plus"
            LLM_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode/v1"
            ;;
        5)
            ask LLM_PROVIDER "Provider 名称" "custom"
            ask LLM_API_KEY "API Key" ""
            ask LLM_MODEL "模型名" ""
            ask LLM_BASE_URL "API Base URL" ""
            ;;
    esac
fi

# 确认
section "确认配置"
echo "  数据目录:    $DATA_DIR"
echo "  安装目录:    $INSTALL_DIR"
echo "  监控目录:    $WATCH_DIR"
if [ -n "$LLM_PROVIDER" ]; then
    echo "  LLM:         $LLM_PROVIDER ($LLM_MODEL)"
else
    echo "  LLM:         未配置（后续可编辑 config.yaml 添加）"
fi
echo ""
prompt "继续安装？(Y/n): "
read -r CONFIRM
if [[ "$CONFIRM" =~ ^[Nn] ]]; then
    echo "  已取消。"
    exit 0
fi

# ============================================================================
# Step 3: 编译
# ============================================================================
section "Step 3/5: 编译"

cd "$PROJECT_DIR"
info "正在编译..."
go build -o nethelper ./cmd/nethelper

if [ ! -f "$PROJECT_DIR/nethelper" ]; then
    error "编译失败"
    exit 1
fi

ok "编译成功 ($(du -h nethelper | cut -f1 | tr -d ' '))"

# ============================================================================
# Step 4: 安装
# ============================================================================
section "Step 4/5: 安装"

# 安装二进制
mkdir -p "$INSTALL_DIR"
cp "$PROJECT_DIR/nethelper" "$INSTALL_DIR/nethelper"
chmod +x "$INSTALL_DIR/nethelper"
ok "已安装到 $INSTALL_DIR/nethelper"

# PATH 检查
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    SHELL_NAME=$(basename "$SHELL")
    case "$SHELL_NAME" in
        zsh)  SHELL_RC="$HOME/.zshrc" ;;
        bash) SHELL_RC="$HOME/.bashrc" ;;
        *)    SHELL_RC="" ;;
    esac

    if [ -n "$SHELL_RC" ]; then
        if ! grep -q "$INSTALL_DIR" "$SHELL_RC" 2>/dev/null; then
            echo "export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$SHELL_RC"
            ok "已添加 $INSTALL_DIR 到 $SHELL_RC"
            warn "运行 source $SHELL_RC 或打开新终端使其生效"
        fi
    else
        warn "请手动将 $INSTALL_DIR 添加到 PATH"
    fi
else
    ok "nethelper 已在 PATH 中"
fi

# 初始化数据目录
mkdir -p "$DATA_DIR"
mkdir -p "$DATA_DIR/logs"
ok "数据目录: $DATA_DIR"

# 创建日志监控目录
mkdir -p "$WATCH_DIR"
ok "监控目录: $WATCH_DIR"

# 生成配置文件
CONFIG_FILE="$DATA_DIR/config.yaml"
if [ -f "$CONFIG_FILE" ]; then
    warn "配置文件已存在: $CONFIG_FILE（跳过覆盖）"
else
    cat > "$CONFIG_FILE" << YAML
# nethelper 配置文件
# 由 install.sh 生成于 $(date '+%Y-%m-%d %H:%M:%S')

# 数据存储
data_dir: $DATA_DIR
db_path: $DATA_DIR/nethelper.db

# 日志监控目录
watch_dirs:
  - $WATCH_DIR

YAML

    # LLM 配置
    if [ -n "$LLM_PROVIDER" ]; then
        cat >> "$CONFIG_FILE" << YAML
# LLM 配置
llm:
  default: $LLM_PROVIDER
  providers:
    $LLM_PROVIDER:
YAML
        [ -n "$LLM_BASE_URL" ] && echo "      base_url: $LLM_BASE_URL" >> "$CONFIG_FILE"
        [ -n "$LLM_MODEL" ]    && echo "      model: $LLM_MODEL" >> "$CONFIG_FILE"
        [ -n "$LLM_API_KEY" ]  && echo "      api_key: $LLM_API_KEY" >> "$CONFIG_FILE"
    else
        cat >> "$CONFIG_FILE" << 'YAML'
# LLM 配置（未配置，不影响核心功能）
# 编辑此处添加 LLM 支持，参考 docs/configuration.md
# llm:
#   default: ollama
#   providers:
#     ollama:
#       base_url: http://localhost:11434
#       model: qwen2.5:14b
YAML
    fi

    ok "配置文件: $CONFIG_FILE"
fi

# ============================================================================
# Step 5: 验证
# ============================================================================
section "Step 5/5: 验证"

export PATH="$INSTALL_DIR:$PATH"
nethelper version
echo ""
nethelper config llm 2>/dev/null || true

# ============================================================================
# 完成
# ============================================================================
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║  ${GREEN}安装完成！${NC}${BOLD}                              ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════╝${NC}"
echo ""
echo "  📁 配置文件:  $CONFIG_FILE"
echo "  📁 数据库:    $DATA_DIR/nethelper.db"
echo "  📁 日志目录:  $WATCH_DIR"
echo ""
echo -e "  ${BOLD}快速开始:${NC}"
echo ""
echo "  1. 导入设备日志:"
echo "     nethelper watch ingest <log-file>"
echo ""
echo "  2. 查看解析结果:"
echo "     nethelper show device"
echo "     nethelper show route --device <id>"
echo ""
echo "  3. 启动实时监控:"
echo "     nethelper watch start"
echo ""
echo "  4. 更多命令:"
echo "     nethelper --help"
echo ""

# 清理
rm -f "$PROJECT_DIR/nethelper"
