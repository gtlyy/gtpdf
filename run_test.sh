#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# 确保 PDFium 动态库可用（测试时 CGO 需要链接 libpdfium.so）
ln -sf libpdfium_linux_amd64.so pdfembed/libpdfium.so
trap 'rm -f "$SCRIPT_DIR/pdfembed/libpdfium.so"' EXIT

# 运行全部测试（含 pdfium_plus 包）
LD_LIBRARY_PATH="$SCRIPT_DIR/pdfembed:${LD_LIBRARY_PATH:-}" go test -v -count=1 ./...
