package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func createAboutTab(win fyne.Window) *container.TabItem {
	content := container.NewVBox()
	content.Add(widget.NewLabelWithStyle("GtPDF V1.10.2", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))

	authorLabel := widget.NewLabel("作者: gtlyy")
	content.Add(authorLabel)

	content.Add(widget.NewSeparator())

	descLabel := widget.NewLabel("一款简洁的 PDF 工具箱，支持以下功能：")
	content.Add(descLabel)

	features := []string{
		"• 阅读 - 渲染、缩放 (0.1x–5x)、平移、文本选择、全文搜索、书签、夜间模式",
		"• 标注 - 高亮/下划线/波浪线/删除线、矩形/填充、自由文本、9 色调色板",
		"• 笔记 - JSON 侧载文件存储，悬停工具提示，多种颜色",
		"• OCR - 选区或全页文字识别（中英文）",
		"• 导出 - 标注页导出 PNG/JPEG，含进度提示，支持合并为一张图片",
		"• 扣图 - 提取 PDF 内嵌图片",
		"• 展开 - 旋转扫描件展平，自动检测 DPI",
		"• 打印 - 系统打印支持（双面、逐份、灰度、多页合一）",
		"• 右键菜单 - 复制成图、删除页面、夜间模式、笔记列表",
		"• 合并 - 多文件合并，支持页码范围选择",
		"• 拆分 - 按页数或指定页拆分",
		"• 重排 - 页面顺序调整（置顶/置底/倒序/奇偶分离）",
		"• 页码 - 添加页码水印（多种格式/位置/颜色）",
		"• 旋转 - 90°/180° 旋转，可指定页范围",
		"• 加密/解密 - AES-256 加密与解密",
		"• 图片转 PDF - JPG/PNG/BMP/TIFF 转换为 PDF",
	}
	for _, f := range features {
		content.Add(widget.NewLabel(f))
	}

	content.Add(widget.NewSeparator())

	libsLabel := widget.NewLabel("使用的开源库：")
	content.Add(libsLabel)

	libItems := []string{
		"• pdfcpu - PDF 处理库 (Apache License 2.0)",
		"• go-pdfium - PDFium Go 绑定 (Apache License 2.0)",
		"• Fyne - Go GUI 框架 (MIT License)",
		"• PDFium - PDF 渲染引擎 (Apache License 2.0)",
		"• Tesseract OCR - 文字识别 (Apache License 2.0)",
		"• Leptonica - 图像处理 (BSD 2-Clause)",
	}
	for _, l := range libItems {
		content.Add(widget.NewLabel(l))
	}

	content.Add(widget.NewSeparator())

	fontLabel := widget.NewLabel("字体：")
	content.Add(fontLabel)
	fontItems := []string{
		"• 文泉驿微米黑 (GPLv3 + Font Exception, © WenQuanYi)",
		"• 思源黑体 Source Han Sans SC (SIL Open Font License 1.1, © Adobe & Google)",
	}
	for _, fi := range fontItems {
		content.Add(widget.NewLabel(fi))
	}

	content.Add(widget.NewSeparator())

	licenseLabel := widget.NewLabel("本程序基于 GPLv3 许可证开源, 详见 LICENSE 文件")
	content.Add(licenseLabel)

	content.Add(widget.NewSeparator())

	copyrightLabel := widget.NewLabel("Copyright © 2026 gtlyy. This program is free software under GPLv3 license.")
	content.Add(copyrightLabel)

	return container.NewTabItem("关于", container.NewVScroll(content))
}
