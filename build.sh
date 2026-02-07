#!/bin/bash
set -e

# ============================================================
# Gemini Web Proxy - 交叉编译脚本
# 支持平台: linux/amd64, windows/amd64, darwin/arm64, darwin/amd64
# 需要 CGO (SQLite 依赖)
# ============================================================

APP_NAME="gemini-web-proxy"
SERVER_DIR="server"
VERSION=${1:-"dev"}

# 获取项目根目录（脚本所在目录）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/release"
EXTENSION_DIR="${SCRIPT_DIR}/extension"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC} $1"; }
ok()    { echo -e "${GREEN}[OK]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# ============================================================
# 检查并安装交叉编译工具链
# ============================================================
check_toolchain() {
    info "检查交叉编译工具链..."

    # Linux: 需要 musl-cross (x86_64-linux-musl-gcc)
    if ! command -v x86_64-linux-musl-gcc &> /dev/null; then
        warn "未找到 x86_64-linux-musl-gcc, 尝试安装..."
        if command -v brew &> /dev/null; then
            brew install filosottile/musl-cross/musl-cross
        else
            error "请手动安装 musl-cross: brew install filosottile/musl-cross/musl-cross"
        fi
    fi
    ok "Linux 工具链就绪 (x86_64-linux-musl-gcc)"

    # Windows: 需要 mingw-w64 (x86_64-w64-mingw32-gcc)
    if ! command -v x86_64-w64-mingw32-gcc &> /dev/null; then
        warn "未找到 x86_64-w64-mingw32-gcc, 尝试安装..."
        if command -v brew &> /dev/null; then
            brew install mingw-w64
        else
            error "请手动安装 mingw-w64: brew install mingw-w64"
        fi
    fi
    ok "Windows 工具链就绪 (x86_64-w64-mingw32-gcc)"

    ok "所有工具链就绪!"
}

# ============================================================
# 编译函数
# ============================================================
build_target() {
    local os=$1
    local arch=$2
    local cc=$3
    local ext=$4

    local output="${OUTPUT_DIR}/${APP_NAME}-${os}-${arch}${ext}"
    info "编译 ${os}/${arch} -> $(basename ${output})"

    cd "${SCRIPT_DIR}/${SERVER_DIR}"

    CGO_ENABLED=1 \
    GOOS=${os} \
    GOARCH=${arch} \
    CC=${cc} \
    go build \
        -ldflags "-s -w -X main.Version=${VERSION}" \
        -o "${output}" \
        .

    cd "${SCRIPT_DIR}"

    if [ -f "${output}" ]; then
        local size=$(du -h "${output}" | cut -f1 | tr -d ' ')
        ok "  ${os}/${arch} 编译完成 (${size})"
    else
        error "  ${os}/${arch} 编译失败!"
    fi
}

build_extension() {
  cd "${EXTENSION_DIR}"
  npm run build || error "编译前端工程失败"
  cp -r dist gemini-web-proxy-extension
  tar -zcvf ../release/gemini-web-proxy-extension.tar.gz gemini-web-proxy-extension
  rm -rf gemini-web-proxy-extension
}

# ============================================================
# 主流程
# ============================================================
main() {
    echo ""
    echo "========================================"
    echo "  Gemini Web Proxy 交叉编译"
    echo "  Version: ${VERSION}"
    echo "========================================"
    echo ""

    # 检查 Go
    if ! command -v go &> /dev/null; then
        error "未找到 Go, 请先安装 Go 1.22+"
    fi
    ok "Go $(go version | awk '{print $3}')"

    # 检查工具链
    check_toolchain

    # 清理并创建输出目录
    rm -rf "${OUTPUT_DIR}"
    mkdir -p "${OUTPUT_DIR}"

    echo ""
    info "开始编译..."
    echo ""

    # macOS arm64 (Apple Silicon)
    build_target "darwin" "arm64" "clang" ""

    # macOS amd64 (Intel)
    build_target "darwin" "amd64" "clang" ""

    # Linux amd64
    build_target "linux" "amd64" "x86_64-linux-musl-gcc" ""

    # Windows amd64
    build_target "windows" "amd64" "x86_64-w64-mingw32-gcc" ".exe"

    echo ""
    echo "========================================"
    info "编译完成! 产物列表:"
    echo "========================================"
    echo ""
    ls -lh "${OUTPUT_DIR}/"
    echo ""

    # 复制 config.yaml 到 release 目录
    cp "${SCRIPT_DIR}/config.yaml" "${OUTPUT_DIR}/config.yaml"
    ok "已复制 config.yaml 到 release/"

    echo ""
    ok "所有平台编译完成! 文件在 release/ 目录"

    build_extension
    ok "插件端编译完成"
}

main "$@"
