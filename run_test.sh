#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

PDFIUM_VERSION=${PDFIUM_VERSION:-7811}

# 准备 PDFium SDK 头文件（CGo 编译需要）
if [ ! -d /tmp/pdfium-sdk/include ]; then
	echo "获取 PDFium SDK..."
	SDK_TAR=/tmp/pdfium-sdk.tgz
	if [ -f ~/download/pdfium-linux-x64.tgz ]; then
		cp ~/download/pdfium-linux-x64.tgz "$SDK_TAR"
	elif [ -f pdfium-linux-x64.tgz ]; then
		cp pdfium-linux-x64.tgz "$SDK_TAR"
	else
		curl -fkSL "https://github.com/bblanchon/pdfium-binaries/releases/download/chromium/${PDFIUM_VERSION}/pdfium-linux-x64.tgz" \
			-o "$SDK_TAR"
	fi
	mkdir -p /tmp/pdfium-sdk
	tar -xzf "$SDK_TAR" -C /tmp/pdfium-sdk && rm -f "$SDK_TAR"
fi

export PKG_CONFIG_PATH=/tmp/pdfium-sdk

# 确保 pdfium.pc 存在（pkg-config 需要）
if [ ! -f /tmp/pdfium-sdk/pdfium.pc ]; then
	cat > /tmp/pdfium-sdk/pdfium.pc << EOF
prefix=/tmp/pdfium-sdk
libdir=/tmp/pdfium-sdk/lib
includedir=/tmp/pdfium-sdk/include
Name: PDFium
Description: PDFium
Version: ${PDFIUM_VERSION}
Requires:
Libs: -L\${libdir} -lpdfium
Cflags: -I\${includedir}
EOF
fi

# 确保 PDFium 动态库可用（测试时 CGO 需要链接 libpdfium.so）
ln -sf libpdfium_linux_amd64.so pdfembed/libpdfium.so
trap 'rm -f "$SCRIPT_DIR/pdfembed/libpdfium.so"' EXIT

# 运行全部测试（含 pdfium_plus 包）
LD_LIBRARY_PATH="$SCRIPT_DIR/pdfembed:${LD_LIBRARY_PATH:-}" go test -v -count=1 ./...
