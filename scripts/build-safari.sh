#!/bin/bash
# Safari Web Extension 构建脚本
# 将 Chrome 扩展转换为 Safari Web Extension Xcode 项目
#
# 前置条件:
#   - macOS 12.0+
#   - Xcode 15+ (需安装 Command Line Tools)
#   - Safari 16.4+ (支持 Manifest V3 Service Worker)
#
# 使用方法:
#   npm run build:safari
#   或直接: bash scripts/build-safari.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EXTENSION_DIR="$PROJECT_ROOT/extension"
DIST_DIR="$EXTENSION_DIR/dist"
SAFARI_OUTPUT_DIR="$PROJECT_ROOT/safari-extension"

echo "=== Gemini Web Proxy - Safari Extension Builder ==="
echo ""

# 检查是否在 macOS 上
if [[ "$(uname)" != "Darwin" ]]; then
    echo "错误: 此脚本只能在 macOS 上运行"
    exit 1
fi

# 检查 Xcode Command Line Tools
if ! command -v xcrun &> /dev/null; then
    echo "错误: 未找到 xcrun，请安装 Xcode Command Line Tools:"
    echo "  xcode-select --install"
    exit 1
fi

# 检查 safari-web-extension-converter 是否可用
if ! xcrun --find safari-web-extension-converter &> /dev/null; then
    echo "错误: 未找到 safari-web-extension-converter"
    echo "请确保已安装 Xcode 15+ 并且包含 Safari Web Extension 支持"
    exit 1
fi

# 第一步：构建 Chrome 扩展
echo "[1/3] 构建 Chrome 扩展..."
cd "$EXTENSION_DIR"
npm run build
echo "  ✓ Chrome 扩展构建完成: $DIST_DIR"
echo ""

# 清理旧的 Safari 项目
if [ -d "$SAFARI_OUTPUT_DIR" ]; then
    echo "[2/3] 清理旧的 Safari 项目..."
    rm -rf "$SAFARI_OUTPUT_DIR"
    echo "  ✓ 已清理: $SAFARI_OUTPUT_DIR"
else
    echo "[2/3] 无需清理旧项目"
fi
echo ""

# 第二步：转换为 Safari Web Extension
echo "[3/3] 转换为 Safari Web Extension..."
xcrun safari-web-extension-converter "$DIST_DIR" \
    --project-location "$SAFARI_OUTPUT_DIR" \
    --app-name "Gemini Web Proxy" \
    --bundle-identifier "com.gemini-web-proxy.extension" \
    --macos-only \
    --no-open \
    --force

echo ""
echo "=== 构建完成 ==="
echo ""
echo "Safari 扩展 Xcode 项目位置: $SAFARI_OUTPUT_DIR"
echo ""
echo "下一步操作:"
echo "  1. 用 Xcode 打开项目:"
echo "     open $SAFARI_OUTPUT_DIR/Gemini\\ Web\\ Proxy/Gemini\\ Web\\ Proxy.xcodeproj"
echo ""
echo "  2. 在 Xcode 中按 Cmd+R 构建并运行"
echo ""
echo "  3. 在 Safari 中启用扩展:"
echo "     Safari → 设置 → 高级 → 勾选\"在菜单栏中显示开发菜单\""
echo "     开发 → 勾选\"允许未签名的扩展\""
echo "     Safari → 设置 → 扩展 → 启用 Gemini Web Proxy"
echo ""
echo "  提示: 无需 Apple Developer 账号，使用开发者模式即可运行未签名扩展"
