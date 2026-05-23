#!/bin/bash
set -e

export GOPROXY=https://goproxy.cn,direct

PDFIUM_VERSION=${PDFIUM_VERSION:-7811}
OUT_DIR=gtpdf_linux_amd64
PDFIUM_TAR=pdfium-linux-x64.tgz

echo "编译 Linux AMD64 版本（PDFium: $PDFIUM_VERSION）..."
mkdir -pv "$OUT_DIR"

echo "获取 PDFium SDK..."
if [ -f ~/download/"$PDFIUM_TAR" ]; then
  cp ~/download/"$PDFIUM_TAR" /tmp/pdfium-sdk.tgz
elif [ -f "$PDFIUM_TAR" ]; then
  cp "$PDFIUM_TAR" /tmp/pdfium-sdk.tgz
else
  curl -fkSL "https://github.com/bblanchon/pdfium-binaries/releases/download/chromium/${PDFIUM_VERSION}/pdfium-linux-x64.tgz" \
    -o /tmp/pdfium-sdk.tgz
    cp /tmp/pdfium-sdk.tgz ~/download/"$PDFIUM_TAR" 
fi

mkdir -p /tmp/pdfium-sdk
tar -xzf /tmp/pdfium-sdk.tgz -C /tmp/pdfium-sdk && rm /tmp/pdfium-sdk.tgz

echo "复制 .so 到 embed 目录..."
cp /tmp/pdfium-sdk/lib/libpdfium.so pdfembed/libpdfium_linux_amd64.so

echo "准备 OCR 嵌入资源..."
rm -f ocrembed/libtesseract.so.5.5 ocrembed/libleptonica.so.6

PKG_INSTALL=""
if command -v apt &>/dev/null; then
  PKG_INSTALL="sudo apt install -y"
  TESS_PKGS="libtesseract5 libleptonica-dev"
  TESSDATA_PKGS="tesseract-ocr-chi-sim tesseract-ocr-eng"
elif command -v dnf &>/dev/null; then
  PKG_INSTALL="sudo dnf install -y"
  TESS_PKGS="tesseract leptonica"
  TESSDATA_PKGS="tesseract-tessdata-chi-sim tesseract-tessdata-eng"
fi

TESLIB=""
for libdir in /usr/lib64 /usr/lib /usr/lib/x86_64-linux-gnu; do
  [ -f "$libdir/libtesseract.so.5.5.2" ] || [ -f "$libdir/libtesseract.so.5.5" ] || [ -f "$libdir/libtesseract.so" ] && TESLIB="$libdir" && break
done

if [ -z "$TESLIB" ] && [ -n "$PKG_INSTALL" ]; then
  echo "  安装 OCR 库..."
  $PKG_INSTALL $TESS_PKGS
  for libdir in /usr/lib64 /usr/lib /usr/lib/x86_64-linux-gnu; do
    [ -f "$libdir/libtesseract.so.5.5.2" ] || [ -f "$libdir/libtesseract.so.5.5" ] || [ -f "$libdir/libtesseract.so" ] && TESLIB="$libdir" && break
  done
fi

if [ -n "$TESLIB" ]; then
  TESS=$(ls "$TESLIB"/libtesseract.so* 2>/dev/null | head -1)
  LEP=$(ls "$TESLIB"/libleptonica.so* 2>/dev/null | head -1)
  [ -n "$TESS" ] && cp "$TESS" ocrembed/libtesseract.so.5.5
  [ -n "$LEP" ]  && cp "$LEP"  ocrembed/libleptonica.so.6
  echo "  已复制: tesseract -> libtesseract.so.5.5"
  echo "  已复制: leptonica -> libleptonica.so.6"
else
  echo "  警告: 未找到 tesseract 库文件，OCR 功能不可用"
fi

if [ ! -f ocrembed/tessdata/chi_sim.traineddata ]; then
  echo "  复制 tessdata..."
  mkdir -p ocrembed/tessdata
  TESSDIR=""
  for d in /usr/share/tesseract/tessdata /usr/share/tesseract-ocr/tessdata /usr/share/tesseract-ocr/4.00/tessdata; do
    [ -d "$d" ] && TESSDIR="$d" && break
  done
  if [ -z "$TESSDIR" ] && [ -n "$PKG_INSTALL" ]; then
    echo "  安装 tessdata 包..."
    $PKG_INSTALL $TESSDATA_PKGS
    for d in /usr/share/tesseract/tessdata /usr/share/tesseract-ocr/tessdata /usr/share/tesseract-ocr/4.00/tessdata; do
      [ -d "$d" ] && TESSDIR="$d" && break
    done
  fi
  if [ -n "$TESSDIR" ]; then
    for lang in chi_sim chi_tra eng; do
      cp "$TESSDIR/${lang}.traineddata" ocrembed/tessdata/ 2>/dev/null || true
    done
    echo "  tessdata 已就绪"
  else
    echo "  警告: 未找到 tessdata，OCR 功能不可用"
  fi
fi

echo "配置 pkg-config..."
cat > /tmp/pdfium-sdk/pdfium.pc << EOF
prefix=/tmp/pdfium-sdk
libdir=/tmp/pdfium-sdk/lib
includedir=/tmp/pdfium-sdk/include
Name: PDFium
Description: PDFium
Version: 7802
Requires:
Libs: -L\${libdir} -lpdfium
Cflags: -I\${includedir}
EOF

export PKG_CONFIG_PATH=/tmp/pdfium-sdk

echo "编译..."
CGO_ENABLED=1 go build -tags "pdfium_cgo,pdfium_experimental" -o "$OUT_DIR/gtpdf" .

echo "移除 DT_NEEDED 依赖..."
patchelf --remove-needed libpdfium.so "$OUT_DIR/gtpdf"

rm -rf /tmp/pdfium-sdk

echo "复制 LICENSE..."
cp LICENSE "$OUT_DIR/"

echo "编译完成："
ls -lh "$OUT_DIR"/
