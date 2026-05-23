package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
)

// --- main.go ---

func TestGetPDFCPUFontDir(t *testing.T) {
	dir, err := getPDFCPUFontDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dir, "pdfcpu") || !strings.Contains(dir, "fonts") {
		t.Errorf("getPDFCPUFontDir() = %q, want .../pdfcpu/fonts", dir)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
}

func TestInitReorderPages(t *testing.T) {
	// 保存全局状态
	oldCount := reorderPageCount
	oldPages := reorderPages
	oldSelected := reorderSelected
	oldSecond := reorderSecondSelected
	oldWidgets := reorderPageWidgets
	defer func() {
		reorderPageCount = oldCount
		reorderPages = oldPages
		reorderSelected = oldSelected
		reorderSecondSelected = oldSecond
		reorderPageWidgets = oldWidgets
	}()

	reorderPageCount = 5
	initReorderPages()

	if len(reorderPages) != 5 {
		t.Fatalf("expected 5 pages, got %d", len(reorderPages))
	}
	for i, p := range reorderPages {
		if p != i+1 {
			t.Errorf("reorderPages[%d] = %d, want %d", i, p, i+1)
		}
	}
	if reorderSelected != -1 {
		t.Errorf("reorderSelected = %d, want -1", reorderSelected)
	}
}

func TestInitReorderPages_Zero(t *testing.T) {
	oldCount := reorderPageCount
	oldPages := reorderPages
	oldSelected := reorderSelected
	oldSecond := reorderSecondSelected
	oldWidgets := reorderPageWidgets
	defer func() {
		reorderPageCount = oldCount
		reorderPages = oldPages
		reorderSelected = oldSelected
		reorderSecondSelected = oldSecond
		reorderPageWidgets = oldWidgets
	}()

	reorderPageCount = 0
	initReorderPages()
	if len(reorderPages) != 0 {
		t.Errorf("expected empty pages, got %d", len(reorderPages))
	}
}

// --- log.go ---

func TestLogLevels(t *testing.T) {
	defer func(l LogLevel) { currentLevel = l }(currentLevel)

	// 设置为较高等级然后验证低等级不输出
	// 我们无法轻松捕获 log.Printf 输出，但可以验证不 panic
	currentLevel = LevelError
	logD("should not print: %d", 1)
	logI("should not print: %d", 2)
	logW("should not print: %d", 3)
	logE("should print: %d", 4)

	currentLevel = LevelDebug
	logD("debug msg")
	logI("info msg")
}

// --- reader_annot.go: AnnotLayer.pdfToScreen / screenToPdf ---

func TestAnnotLayerPDFToScreen(t *testing.T) {
	al := &AnnotLayer{
		imgOffX: 100,
		imgOffY: 50,
	}
	// pdfToScreen 需要 al.viewer 非空（读取 zoom）
	// 设置一个最小 viewer
	al.viewer = &PDFViewerPlus{zoom: 2.0}

	sx, sy := al.pdfToScreen(50, 100)
	// sx = 100 + 50*2 = 200
	// sy = 50 + 100*2 = 250
	if sx != 200 || sy != 250 {
		t.Errorf("pdfToScreen(50,100) = (%v,%v), want (200,250)", sx, sy)
	}

	sx2, sy2 := al.pdfToScreen(0, 0)
	if sx2 != 100 || sy2 != 50 {
		t.Errorf("pdfToScreen(0,0) = (%v,%v), want (100,50)", sx2, sy2)
	}
}

func TestAnnotLayerScreenToPdf(t *testing.T) {
	al := &AnnotLayer{
		pageWidth:  800,
		pageHeight: 600,
		imgW:       400,
		imgH:       300,
		imgOffX:    100,
		imgOffY:    50,
	}
	al.viewer = &PDFViewerPlus{zoom: 0.5}

	// screenToPdf 当 pageWidth/Height/imgW/imgH 都 >0 时走精确路径
	px, py := al.screenToPdf(300, 200)
	// ix = 300-100=200, iy = 200-50=150
	// px = 200/400*800 = 400
	// py = 150/300*600 = 300
	if px != 400 || py != 300 {
		t.Errorf("screenToPdf(300,200) = (%v,%v), want (400,300)", px, py)
	}

	// 当 pageWidth=0 时走回退路径
	al2 := &AnnotLayer{}
	al2.viewer = &PDFViewerPlus{zoom: 2.0}
	px2, py2 := al2.screenToPdf(100, 200)
	// 回退: px = 100/2 = 50, py = 200/2 = 100
	if px2 != 50 || py2 != 100 {
		t.Errorf("screenToPdf fallback(100,200) = (%v,%v), want (50,100)", px2, py2)
	}
}

// --- reader_plus_text_layer.go ---

func TestTextSelectionLayerGetBounds(t *testing.T) {
	tl := &TextSelectionLayer{}
	tl.dragStart = fyne.Position{X: 100, Y: 200}
	tl.dragCurrent = fyne.Position{X: 50, Y: 300}

	minX, minY, maxX, maxY := tl.getBounds()
	if minX != 50 || minY != 200 || maxX != 100 || maxY != 300 {
		t.Errorf("getBounds() = (%v,%v,%v,%v), want (50,200,100,300)", minX, minY, maxX, maxY)
	}
}

func TestTextSelectionLayerScreenToPdf(t *testing.T) {
	tl := &TextSelectionLayer{
		pageWidth:  800,
		pageHeight: 600,
		imgW:       400,
		imgH:       300,
		imgOffX:    100,
		imgOffY:    50,
	}
	tl.pdfViewer = &PDFViewerPlus{zoom: 0.5}

	px, py := tl.screenToPdf(300, 200)
	// ix = 300-100 = 200, iy = 200-50 = 150
	// px = 200/400*800 = 400
	// py = (1 - 150/300)*600 = (1-0.5)*600 = 300
	if px != 400 || py != 300 {
		t.Errorf("screenToPdf(300,200) = (%v,%v), want (400,300)", px, py)
	}

	// 回退路径
	tl2 := &TextSelectionLayer{}
	tl2.pdfViewer = &PDFViewerPlus{zoom: 2.0}
	px2, py2 := tl2.screenToPdf(100, 200)
	if px2 != 50 || py2 != 100 {
		t.Errorf("screenToPdf fallback(100,200) = (%v,%v), want (50,100)", px2, py2)
	}
}

func TestTextSelectionLayerGetSelectedText(t *testing.T) {
	tl := &TextSelectionLayer{selectedText: "hello world"}
	if got := tl.GetSelectedText(); got != "hello world" {
		t.Errorf("GetSelectedText() = %q, want %q", got, "hello world")
	}
}

// --- reader_windows_plus.go ---

func TestForceRenderPlus(t *testing.T) {
	v := &PDFViewerPlus{lastRenderPage: 5, lastRenderZoom: 150}
	v.forceRenderPlus()

	if v.lastRenderPage != -1 {
		t.Errorf("lastRenderPage = %d, want -1", v.lastRenderPage)
	}
	if v.lastRenderZoom != -1 {
		t.Errorf("lastRenderZoom = %v, want -1", v.lastRenderZoom)
	}
	if atomic.LoadInt32(&v.renderVer) != 1 {
		t.Errorf("renderVer = %d, want 1", atomic.LoadInt32(&v.renderVer))
	}
}

func TestCleanupTempFilePlus(t *testing.T) {
	tmpDir := t.TempDir()

	// 文件路径和原始路径相同 → 不删除
	v1 := &PDFViewerPlus{
		filePath:         filepath.Join(tmpDir, "doc.pdf"),
		originalFilePath: filepath.Join(tmpDir, "doc.pdf"),
	}
	v1.cleanupTempFilePlus() // 不应报错

	// 文件路径不同且文件存在 → 应删除
	tmpFile := filepath.Join(tmpDir, "temp.pdf")
	os.WriteFile(tmpFile, []byte("test"), 0644)
	v2 := &PDFViewerPlus{
		filePath:         tmpFile,
		originalFilePath: filepath.Join(tmpDir, "original.pdf"),
	}
	v2.cleanupTempFilePlus()
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("expected temp file to be removed")
	}

	// filePath 为空 → 不删除
	v3 := &PDFViewerPlus{filePath: "", originalFilePath: ""}
	v3.cleanupTempFilePlus() // 不应 panic
}

func TestExportImagePlus(t *testing.T) {
	tmpDir := t.TempDir()

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	c := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, c)
		}
	}

	// 导出 PNG
	pngPath := filepath.Join(tmpDir, "test.png")
	exportImagePlus(img, pngPath, "PNG")
	if _, err := os.Stat(pngPath); os.IsNotExist(err) {
		t.Fatal("PNG file was not created")
	}
	f, _ := os.Open(pngPath)
	_, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Errorf("decoding exported PNG failed: %v", err)
	}

	// 导出 JPEG
	jpgPath := filepath.Join(tmpDir, "test.jpg")
	exportImagePlus(img, jpgPath, "JPEG")
	if _, err := os.Stat(jpgPath); os.IsNotExist(err) {
		t.Fatal("JPEG file was not created")
	}
}

// --- reader_note.go: loadNotesForFile ---

func TestLoadNotesForFile(t *testing.T) {
	// 无 sidecar 文件 → 应返回空 store（不报错）
	tmpFile := filepath.Join(t.TempDir(), "test.pdf")
	store := loadNotesForFile(tmpFile)
	if store == nil {
		t.Fatal("loadNotesForFile returned nil")
	}
	if len(store.GetAll()) != 0 {
		t.Errorf("expected empty store, got %d notes", len(store.GetAll()))
	}

	// 有 sidecar 文件 → 应加载笔记
	sidecar := tmpFile + ".gtpdf.json"
	jsonContent := `[{"id":"n1","page":0,"text":"hello","color":"#FFD600"}]`
	if err := os.WriteFile(sidecar, []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	store2 := loadNotesForFile(tmpFile)
	if store2 == nil {
		t.Fatal("loadNotesForFile returned nil")
	}
	all := store2.GetAll()
	if len(all) != 1 || all[0].ID != "n1" || all[0].Text != "hello" {
		t.Errorf("loaded notes = %+v", all)
	}
}
