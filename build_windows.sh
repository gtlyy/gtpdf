#!/bin/bash
set -e

PDFIUM_VERSION=${PDFIUM_VERSION:-7811}
OUT_DIR=gtpdf_windows
PDFIUM_TAR=pdfium-win-x64.tgz

echo "交叉编译 Windows AMD64 版本（PDFium: $PDFIUM_VERSION）..."
mkdir -pv "$OUT_DIR"

echo "获取 PDFium Windows SDK..."
if [ -f ~/download/"$PDFIUM_TAR" ]; then
  cp ~/download/"$PDFIUM_TAR" /tmp/pdfium-sdk-win.tgz
elif [ -f "$PDFIUM_TAR" ]; then
  cp "$PDFIUM_TAR" /tmp/pdfium-sdk-win.tgz
else
  curl -fkSL "https://github.com/bblanchon/pdfium-binaries/releases/download/chromium/${PDFIUM_VERSION}/${PDFIUM_TAR}" \
    -o /tmp/pdfium-sdk-win.tgz
    cp /tmp/pdfium-sdk-win.tgz  ~/download/"$PDFIUM_TAR"
fi

rm -rf /tmp/pdfium-sdk-win
mkdir -p /tmp/pdfium-sdk-win
tar -xzf /tmp/pdfium-sdk-win.tgz -C /tmp/pdfium-sdk-win && rm /tmp/pdfium-sdk-win.tgz

echo "配置 pkg-config..."
cat > /tmp/pdfium-sdk-win/pdfium.pc << EOF
prefix=/tmp/pdfium-sdk-win
libdir=/tmp/pdfium-sdk-win/lib
includedir=/tmp/pdfium-sdk-win/include
Name: PDFium
Description: PDFium
Version: 7802
Requires:
Libs: -L\${libdir} -lpdfium
Cflags: -I\${includedir}
EOF

export PKG_CONFIG_PATH=/tmp/pdfium-sdk-win

echo "创建 MinGW 导入库..."
ln -sf pdfium.dll.lib /tmp/pdfium-sdk-win/lib/libpdfium.dll.a

echo "编译..."
GOOS=windows GOARCH=amd64 \
  CGO_ENABLED=1 \
  CC=x86_64-w64-mingw32-gcc \
  go build -tags "pdfium_cgo,pdfium_experimental" -ldflags "-H windowsgui" -o "$OUT_DIR/gtpdf.exe" .

echo "复制 pdfium.dll..."
cp /tmp/pdfium-sdk-win/bin/pdfium.dll "$OUT_DIR/"

echo "复制 Tesseract OCR DLLs..."
TESSERACT_DIR="/winc/Program Files/Tesseract-OCR"
if [ -d "$TESSERACT_DIR" ]; then
  cp "$TESSERACT_DIR"/*.dll "$OUT_DIR/" 2>/dev/null || true
  echo "  DLLs 已复制到 $OUT_DIR/"
else
  echo "  警告: Tesseract-OCR 目录不存在: $TESSERACT_DIR"
fi

echo "复制 tessdata..."
if [ -d ocrembed/tessdata ]; then
  mkdir -p "$OUT_DIR/tessdata"
  cp ocrembed/tessdata/*.traineddata "$OUT_DIR/tessdata/" 2>/dev/null || true
  echo "  tessdata 已复制到 $OUT_DIR/tessdata/"
fi

rm -rf /tmp/pdfium-sdk-win

echo "复制 LICENSE..."
cp LICENSE "$OUT_DIR/"

echo "编译完成："
ls -lh "$OUT_DIR"/
