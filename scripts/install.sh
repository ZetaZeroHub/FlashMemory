#!/bin/sh
# FlashMemory 一键安装脚本
# 用法: curl -fsSL https://fm.zzh.app/install.sh | sh
#
# 环境变量 (可选):
#   FLASHMEMORY_VERSION  - 指定版本号 (默认: latest)
#   FLASHMEMORY_INSTALL_DIR - 安装目录 (默认: ~/.flashmemory)

set -e

# ─── 配色 ────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

info()  { printf "${CYAN}[INFO]${NC}  %s\n" "$1"; }
ok()    { printf "${GREEN}[OK]${NC}    %s\n" "$1"; }
warn()  { printf "${YELLOW}[WARN]${NC}  %s\n" "$1"; }
error() { printf "${RED}[ERROR]${NC} %s\n" "$1"; exit 1; }

# ─── 常量 ────────────────────────────────────────────────
REPO="ZetaZeroHub/FlashMemory"
INSTALL_DIR="${FLASHMEMORY_INSTALL_DIR:-$HOME/.flashmemory}"
BIN_DIR="${INSTALL_DIR}/bin"

# ─── 检测操作系统 ────────────────────────────────────────
detect_os() {
    case "$(uname -s)" in
        Darwin)  echo "darwin"  ;;
        Linux)   echo "linux"   ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *)       error "不支持的操作系统: $(uname -s)" ;;
    esac
}

# ─── 检测架构 ────────────────────────────────────────────
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        arm64|aarch64)  echo "arm64" ;;
        *)              error "不支持的 CPU 架构: $(uname -m)" ;;
    esac
}

# ─── 获取最新版本 ────────────────────────────────────────
get_latest_version() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | head -1 | sed 's/.*"v\([^"]*\)".*/\1/'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | head -1 | sed 's/.*"v\([^"]*\)".*/\1/'
    else
        error "请安装 curl 或 wget"
    fi
}

# ─── 下载文件 ────────────────────────────────────────────
download() {
    local url="$1" dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --progress-bar "$url" -o "$dest"
    elif command -v wget >/dev/null 2>&1; then
        wget -q --show-progress "$url" -O "$dest"
    fi
}

# ─── 主流程 ──────────────────────────────────────────────
main() {
    printf "\n${BOLD}╔══════════════════════════════════════════╗${NC}\n"
    printf "${BOLD}║    FlashMemory Installer                 ║${NC}\n"
    printf "${BOLD}║    跨语言代码分析与语义搜索系统            ║${NC}\n"
    printf "${BOLD}╚══════════════════════════════════════════╝${NC}\n\n"

    # 1. 检测环境
    OS=$(detect_os)
    ARCH=$(detect_arch)
    info "检测到平台: ${OS}/${ARCH}"

    # 2. 获取版本
    VERSION="${FLASHMEMORY_VERSION:-}"
    if [ -z "$VERSION" ]; then
        info "正在获取最新版本..."
        VERSION=$(get_latest_version)
        if [ -z "$VERSION" ]; then
            error "无法获取最新版本，请检查网络或手动指定: FLASHMEMORY_VERSION=1.0.0"
        fi
    fi
    info "目标版本: v${VERSION}"

    # 3. 构造下载 URL
    EXT="tar.gz"
    if [ "$OS" = "windows" ]; then
        EXT="zip"
    fi
    ARCHIVE_NAME="flashmemory_${VERSION}_${OS}_${ARCH}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE_NAME}.${EXT}"
    info "下载地址: ${DOWNLOAD_URL}"

    # 4. 创建临时目录
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    # 5. 下载
    info "正在下载..."
    download "$DOWNLOAD_URL" "${TMP_DIR}/flashmemory.${EXT}"
    if [ ! -f "${TMP_DIR}/flashmemory.${EXT}" ]; then
        error "下载失败，请检查版本号或网络连接"
    fi
    ok "下载完成"

    # 6. 解压
    info "正在解压..."
    if [ "$EXT" = "zip" ]; then
        if command -v unzip >/dev/null 2>&1; then
            unzip -qo "${TMP_DIR}/flashmemory.${EXT}" -d "${TMP_DIR}"
        else
            error "请安装 unzip: sudo apt install unzip"
        fi
    else
        tar xzf "${TMP_DIR}/flashmemory.${EXT}" -C "${TMP_DIR}"
    fi
    ok "解压完成"

    # 7. 安装
    info "正在安装到 ${INSTALL_DIR}..."
    mkdir -p "${BIN_DIR}"

    # 找到解压后的目录并复制文件
    SRC_DIR="${TMP_DIR}/${ARCHIVE_NAME}"
    if [ ! -d "$SRC_DIR" ]; then
        # 有时 tar 可能平铺文件
        SRC_DIR=$(find "${TMP_DIR}" -maxdepth 1 -type d -name "flashmemory_*" | head -1)
    fi

    if [ -z "$SRC_DIR" ] || [ ! -d "$SRC_DIR" ]; then
        error "解压后未找到预期目录"
    fi

    # 复制二进制文件
    if [ "$OS" = "windows" ]; then
        cp -f "${SRC_DIR}/fm_http.exe" "${BIN_DIR}/"
        cp -f "${SRC_DIR}/fm.exe" "${BIN_DIR}/"
    else
        cp -f "${SRC_DIR}/fm_http" "${BIN_DIR}/"
        cp -f "${SRC_DIR}/fm" "${BIN_DIR}/"
        chmod +x "${BIN_DIR}/fm_http" "${BIN_DIR}/fm"
    fi

    # 复制 FAISSService
    if [ -d "${SRC_DIR}/FAISSService" ]; then
        rm -rf "${BIN_DIR}/FAISSService"
        cp -r "${SRC_DIR}/FAISSService" "${BIN_DIR}/FAISSService"
    fi

    # 复制配置模板
    if [ -f "${SRC_DIR}/fm.yaml.example" ] && [ ! -f "${INSTALL_DIR}/fm.yaml" ]; then
        cp "${SRC_DIR}/fm.yaml.example" "${INSTALL_DIR}/fm.yaml"
        info "已创建默认配置文件: ${INSTALL_DIR}/fm.yaml"
    fi

    ok "安装完成"

    # 8. 配置 PATH
    info "配置 PATH 环境变量..."
    PATH_LINE="export PATH=\"${BIN_DIR}:\$PATH\""
    FAISS_LINE="export FAISS_SERVICE_PATH=\"${BIN_DIR}/FAISSService\""

    add_to_profile() {
        local profile="$1"
        if [ -f "$profile" ]; then
            if ! grep -q "flashmemory" "$profile" 2>/dev/null; then
                printf "\n# FlashMemory\n%s\n%s\n" "$PATH_LINE" "$FAISS_LINE" >> "$profile"
                ok "已添加到 ${profile}"
            else
                info "${profile} 中已存在 FlashMemory 配置"
            fi
        fi
    }

    case "$OS" in
        darwin)
            add_to_profile "$HOME/.zshrc"
            add_to_profile "$HOME/.bash_profile"
            ;;
        linux)
            add_to_profile "$HOME/.bashrc"
            add_to_profile "$HOME/.profile"
            if [ -f "$HOME/.zshrc" ]; then
                add_to_profile "$HOME/.zshrc"
            fi
            ;;
        windows)
            warn "Windows 用户请手动将 ${BIN_DIR} 添加到系统 PATH"
            warn "  或运行: setx PATH \"%PATH%;${BIN_DIR}\""
            ;;
    esac

    # 9. 完成
    printf "\n${GREEN}${BOLD}✅ FlashMemory v${VERSION} 安装成功！${NC}\n\n"
    printf "  安装位置: ${BIN_DIR}\n"
    printf "  二进制:   ${BIN_DIR}/fm, ${BIN_DIR}/fm_http\n"
    printf "  FAISS:    ${BIN_DIR}/FAISSService\n\n"

    if [ "$OS" != "windows" ]; then
        printf "  ${YELLOW}请运行以下命令使 PATH 生效（或重新打开终端）：${NC}\n"
        if [ -f "$HOME/.zshrc" ]; then
            printf "    source ~/.zshrc\n"
        else
            printf "    source ~/.bashrc\n"
        fi
    fi

    printf "\n  快速开始:\n"
    printf "    fm --help          # CLI 工具帮助\n"
    printf "    fm_http            # 启动 HTTP 服务\n\n"
}

main "$@"
