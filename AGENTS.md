# GtPDF 项目指南

## 项目简介

GtPDF 是一个功能完整的跨平台 PDF 工具箱，采用 Go + Fyne 开发，集成了 PDFium 渲染引擎、pdfcpu 处理库和 Tesseract OCR。

## 技术栈

| 类别 | 技术 |
|------|------|
| GUI 框架 | fyne.io/fyne/v2 v2.7.3 |
| PDF 处理 | github.com/pdfcpu/pdfcpu v0.11.1 |
| PDF 渲染 | github.com/klippa-app/go-pdfium v1.19.2 |
| 底层引擎 | PDFium (Google) |
| OCR | Tesseract + Leptonica |
| 中文字体 | WenQuanYi Micro Hei |

## 主要功能

- **PDF 阅读**: 渲染、缩放 (0.1x–5x)、平移、夜间模式、文本选择、全文搜索、书签、链接、侧栏自适应
- **标注**: 高亮、下划线、波浪线、删除线、矩形（描边/填充）、线段、夹批、填充（白色矩形）、9 色调色板
- **笔记**: JSON 侧载文件存储，悬停工具提示
- **OCR**: 选区/全页文字识别（中英文）
- **合并**: 多文件合并，支持页码范围选择
- **拆分**: 按页数或指定页码拆分
- **重排**: 页面顺序调整
- **添加页码**: 多种格式/位置/颜色
- **旋转**: 90°/180° 旋转
- **加密/解密**: AES-256
- **展开**: 旋转扫描件展平，自动检测 DPI
- **导出**: 标注页导出 PNG，含进度提示
- **右键菜单**: 复制页成图、删除页面、夜间模式切换、笔记列表

## 目录结构

```
~/go/src/gtpdf/
├── main.go                     # 应用入口
├── go.mod                      # 依赖管理
│
├── pdfium_plus/                # PDFium 封装
│   ├── document.go             # 渲染/文本/搜索/书签/链接
│   ├── annot.go                # 标注 CRUD
│   └── init.go                  # 初始化
│
├── pdfembed/                   # PDFium 动态库嵌入
├── ocrembed/                   # OCR 嵌入
│
├── reader_pdf_plus.go          # PDF 渲染核心
├── reader_windows_plus.go     # 阅读器 UI
├── reader_annot.go             # 标注功能
├── reader_note.go             # 笔记功能
├── reader_plus_text_layer.go  # 文本选择层
├── crypt.go                    # 加密/解密
└── ocr.go                      # OCR 接口
```

## 核心模块

| 文件 | 功能 |
|------|------|
| `main.go` | 应用入口，创建 Fyne 窗口，各功能 Tab |
| `reader_pdf_plus.go` | PDFiumDoc/Page 封装，渲染/文本提取 |
| `reader_windows_plus.go` | PDFViewerPlus 阅读器 UI |
| `reader_annot.go` | AnnotLayer 标注渲染，工具栏 |
| `reader_note.go` | NoteStore/NoteLayer 笔记管理 |
| `reader_plus_text_layer.go` | TextSelectionLayer 文本选择 |
| `pdfium_plus/document.go` | 渲染/文本/搜索/书签/链接 API |

## 构建

```bash
# Linux AMD64
./build_linux_amd64.sh

# Linux ARM64 : 支持 glibc 2.28
./build_linux_arm64_quick.sh

# Linux ARM64 : 支持 glibc 2.23 (ocr估计需要本地安装，嵌入的不行)
./build_linux_arm64_glibc2.23.sh

# Windows
./build_windows.sh
```

## 编译要求
- Go 1.25+
- CGO 支持

## 技术要点
### 关于在fyne中使用goroutine
You should use fyne.Do or fyne.DoAndWait whenever you are invoking any Fyne APIs other than ones specifically marked as excluded (currently only data binding) from a goroutine that your code created. This includes calling Refresh() as well as setting properties on widgets, CanvasObjects, etc.
All callbacks from Fyne to your own code are now guaranteed to occur on the "app goroutine" - ie main - so it is never needed to use fyne.Do[AndWait] in your Fyne event handlers/ callbacks.
All functions passed to fyne.Do[AndWait] are executed sequentially in the order received.

### 关于webassembly
本项目不得使用webassembly

