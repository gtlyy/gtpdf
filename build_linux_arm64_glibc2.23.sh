#!/bin/bash
set -e

NAME=gtpdf-builder-arm64-glibc2.23
IMAGE=${IMAGE:-arm64v8/ubuntu:16.04}
OUT_DIR=gtpdf_linux_arm64_glibc2.23
GO_TAR=go1.25.0.linux-arm64.tar.gz
PDFIUM_TAR=pdfium-linux-arm64.tgz
INIT_FLAG=/opt/.gtpdf-initialized-glibc2.23

mkdir -pv "$OUT_DIR"

if docker ps -a --format '{{.Names}}' | grep -qx "$NAME"; then
  if ! docker ps --format '{{.Names}}' | grep -qx "$NAME"; then
    echo "启动已存在的容器 $NAME..."
    docker start "$NAME"
  fi
else
  docker run -d --name "$NAME" --platform linux/arm64 \
    -v "$PWD":/build \
    "$IMAGE" sleep infinity
fi

if ! docker exec "$NAME" test -f "$INIT_FLAG" 2>/dev/null; then
  echo "初始化构建环境..."
  docker exec "$NAME" mkdir -p /root/tmp

  # ---- Go ----
  echo "安装 Go..."
  if docker exec "$NAME" test -x /usr/local/go/bin/go 2>/dev/null; then
    echo "  Go 已安装，跳过"
  else
    # 优先从宿主 ~/download 取，再找本地，最后下载
    if [ -f ~/download/"$GO_TAR" ]; then
      echo "  从 ~/download 复制 Go"
      docker cp ~/download/"$GO_TAR" "$NAME":/root/tmp/go.tar.gz
    elif [ -f "$GO_TAR" ]; then
      echo "  从本地复制 Go"
      docker cp "$GO_TAR" "$NAME":/root/tmp/go.tar.gz
    else
      echo "  下载 Go"
      docker exec "$NAME" curl -fkSL https://go.dev/dl/"$GO_TAR" -o /root/tmp/go.tar.gz
    fi
    docker exec "$NAME" bash -c '
      rm -rf /usr/local/go
      tar -C /usr/local -xzf /root/tmp/go.tar.gz && rm -f /root/tmp/go.tar.gz
    '
    docker exec "$NAME" /usr/local/go/bin/go version
  fi

  # ---- 编译依赖 ----
  echo "安装编译依赖..."
  docker exec "$NAME" bash -c '
    apt-get update && apt-get install -y --allow-unauthenticated \
    gcc patchelf libx11-dev libxcursor-dev libxrandr-dev \
    libxfixes-dev libxi-dev libxinerama-dev \
    mesa-common-dev libgl1-mesa-dev libegl1-mesa-dev \
    pkg-config ca-certificates 2>&1 | tail -5
  '

  # ---- Tesseract OCR ----
  echo "安装 Tesseract OCR..."
  docker exec "$NAME" bash -c '
    apt-get install -y --allow-unauthenticated tesseract-ocr libtesseract-dev libleptonica-dev 2>&1 | tail -5
    ln -sf /usr/lib/liblept.so /usr/lib/libleptonica.so.6
  ' || echo "  tesseract 安装失败，OCR 不可用"

  # ---- PDFium ----
  echo "获取 PDFium SDK..."
  if docker exec "$NAME" test -f /opt/pdfium/include/fpdfview.h 2>/dev/null; then
    echo "  PDFium SDK 已存在，跳过"
  else
    # 优先从宿主 ~/download 取，再找本地
    if [ -f ~/download/"$PDFIUM_TAR" ]; then
      echo "  从 ~/download 复制 PDFium"
      docker cp ~/download/"$PDFIUM_TAR" "$NAME":/root/tmp/pdfium.tgz
    elif [ -f "$PDFIUM_TAR" ]; then
      echo "  从本地复制 PDFium"
      docker cp "$PDFIUM_TAR" "$NAME":/root/tmp/pdfium.tgz
    fi
    PDFIUM_VERSION="${PDFIUM_VERSION:-7811}"
    docker exec -e PDFIUM_VERSION="$PDFIUM_VERSION" "$NAME" bash -c '
      if [ ! -f /root/tmp/pdfium.tgz ]; then
        echo "  下载 PDFium"
        curl -fkSL "https://github.com/bblanchon/pdfium-binaries/releases/download/chromium/${PDFIUM_VERSION}/pdfium-linux-arm64.tgz" \
          -o /root/tmp/pdfium.tgz
      fi
      rm -rf /opt/pdfium
      mkdir -p /opt/pdfium
      if ! tar -xzf /root/tmp/pdfium.tgz -C /opt/pdfium; then
        rm -f /root/tmp/pdfium.tgz
        echo "  PDFium 解压失败，已清除缓存"
        exit 1
      fi
      rm -f /root/tmp/pdfium.tgz
    '
  fi

  # ---- pkg-config ----
  echo "配置 pkg-config..."
  docker exec "$NAME" bash -c '
    mkdir -p /usr/lib/pkgconfig
    cat > /usr/lib/pkgconfig/pdfium.pc << '\''EOF'\''
prefix=/opt/pdfium
libdir=/opt/pdfium/lib
includedir=/opt/pdfium/include
Name: PDFium
Description: PDFium
Version: 7802
Requires:
Libs: -L${libdir} -lpdfium
Cflags: -I${includedir}
EOF
  '

  docker exec "$NAME" touch "$INIT_FLAG"
  echo "初始化完成"
fi

# ---- 编译 ----
echo "编译..."
docker exec "$NAME" bash -c '
  export PATH=/usr/local/go/bin:$PATH
  export CGO_ENABLED=1
  export GOPROXY=https://goproxy.cn,direct
  cd /build

  # 确保 embed .so 存在（从容器内 PDFium SDK 复制，不走宿主文件）
  cp /opt/pdfium/lib/libpdfium.so pdfembed/libpdfium_linux_arm64.so

  # 确保 OCR 嵌入资源存在（始终覆盖，防止跨架构残留旧文件）
  rm -f ocrembed/libtesseract.so.5.5 ocrembed/libleptonica.so.6
  TESS=$(find /usr/lib /usr/local/lib -name "libtesseract.so*" 2>/dev/null | head -1)
  LEP=$(find /usr/lib /usr/local/lib -name "libleptonica.so*" 2>/dev/null | head -1)
  [ -n "$TESS" ] && cp "$TESS" ocrembed/libtesseract.so.5.5
  [ -n "$LEP" ]  && cp "$LEP"  ocrembed/libleptonica.so.6
  if [ ! -f ocrembed/tessdata/eng.traineddata ]; then
    mkdir -p ocrembed/tessdata
    for d in /usr/share/tesseract/tessdata /usr/share/tesseract-ocr/tessdata /usr/share/tesseract-ocr/4.00/tessdata; do
      [ -d "$d" ] && { for lang in chi_sim chi_tra eng; do cp "$d/${lang}.traineddata" ocrembed/tessdata/ 2>/dev/null || true; done; break; }
    done
  fi

  go build -tags "pdfium_cgo,pdfium_experimental" -o gtpdf_linux_arm64_glibc2.23/gtpdf .
  patchelf --remove-needed libpdfium.so gtpdf_linux_arm64_glibc2.23/gtpdf
  cp /build/LICENSE /build/gtpdf_linux_arm64_glibc2.23/
'

echo "编译完成："
ls -lh "$OUT_DIR"/gtpdf

git restore ocrembed/
