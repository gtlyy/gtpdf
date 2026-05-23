#!/bin/bash
# 安装 GtPDF 桌面集成（右键 → 打开方式）
# 用法: ./install_desktop.sh [/path/to/gtpdf/binary]

set -e

BIN="${1:-$PWD/gtpdf_linux_amd64/gtpdf}"
if [ ! -f "$BIN" ]; then
  echo "错误: 找不到 gtpdf 二进制文件: $BIN"
  echo "用法: $0 [/path/to/gtpdf]"
  exit 1
fi

BIN="$(realpath "$BIN")"
BINDIR="$(dirname "$BIN")"
ICON="$BINDIR/../Icon.png"
if [ ! -f "$ICON" ]; then
  ICON="$PWD/Icon.png"
fi
if [ ! -f "$ICON" ]; then
  echo "警告: 未找到 Icon.png，跳过图标安装"
  ICON=""
fi

DESKTOP_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
ICON_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/256x256/apps"

mkdir -p "$DESKTOP_DIR"
if [ -n "$ICON" ]; then
  mkdir -p "$ICON_DIR"
  cp "$ICON" "$ICON_DIR/gtpdf.png"
fi

sed -e "s|__GTPDF_BIN__|$BIN|g" -e "s|__GTPDF_ICON__|gtpdf|g" "$PWD/gtpdf.desktop" > "$DESKTOP_DIR/gtpdf.desktop"

echo "已安装: $DESKTOP_DIR/gtpdf.desktop"
echo "  Exec=$BIN %f"
echo ""
echo "现在可以在文件管理器中右键 PDF → 打开方式 → GtPDF"
echo "或设为默认: xdg-mime default gtpdf.desktop application/pdf"
