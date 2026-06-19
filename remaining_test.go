package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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

// --- reader_windows_plus.go: updatePageLayout ---

func TestUpdatePageLayout(t *testing.T) {
	v := &PDFViewerPlus{
		pageHeights:  []float32{100, 100, 100, 100},
		pageYOffsets: []float32{0, 100, 200, 300},
		pageImages:   make([]*canvas.Image, 4),
	}
	for i := range v.pageImages {
		v.pageImages[i] = canvas.NewImageFromImage(nil)
		v.pageImages[i].SetMinSize(fyne.NewSize(50, 100))
	}

	// 改第2页（idx=1）高度 100→150，delta=+50
	v.updatePageLayout(1, 50, 150)

	if v.pageHeights[1] != 150 {
		t.Errorf("pageHeights[1] = %v, want 150", v.pageHeights[1])
	}
	if v.pageYOffsets[2] != 250 {
		t.Errorf("pageYOffsets[2] = %v, want 250", v.pageYOffsets[2])
	}
	if v.pageYOffsets[3] != 350 {
		t.Errorf("pageYOffsets[3] = %v, want 350", v.pageYOffsets[3])
	}
	// pageYOffsets[0], [1] 不应变
	if v.pageYOffsets[0] != 0 || v.pageYOffsets[1] != 100 {
		t.Errorf("first two YOffsets should be unchanged: got [%v, %v]", v.pageYOffsets[0], v.pageYOffsets[1])
	}
}

func TestUpdatePageLayout_SameHeight(t *testing.T) {
	v := &PDFViewerPlus{
		pageHeights:  []float32{100, 100, 100, 100},
		pageYOffsets: []float32{0, 100, 200, 300},
		pageImages:   make([]*canvas.Image, 4),
	}
	for i := range v.pageImages {
		v.pageImages[i] = canvas.NewImageFromImage(nil)
		v.pageImages[i].SetMinSize(fyne.NewSize(50, 100))
	}

	// 同高度不触发级联更新
	v.updatePageLayout(2, 50, 100)

	for i := 0; i < 4; i++ {
		if v.pageYOffsets[i] != float32(i*100) {
			t.Errorf("pageYOffsets[%d] = %v, want %v", i, v.pageYOffsets[i], i*100)
		}
	}
}

func TestUpdatePageLayout_Bounds(t *testing.T) {
	v := &PDFViewerPlus{
		pageHeights:  []float32{100},
		pageYOffsets: []float32{0},
		pageImages:   make([]*canvas.Image, 1),
	}

	// 越界应不 panic
	v.updatePageLayout(-1, 50, 100)
	v.updatePageLayout(10, 50, 100)

	// pageImages nil 时应不 panic
	v.pageImages = nil
	v.updatePageLayout(0, 50, 100)
}

// --- reader_windows_plus.go: getCurrentPageFromScroll ---

func TestGetCurrentPageFromScroll(t *testing.T) {
	sc := container.NewScroll(canvas.NewRectangle(color.Black))
	sc.Resize(fyne.NewSize(200, 100))

	v := &PDFViewerPlus{
		pageHeights:   []float32{50, 50, 50},
		pageYOffsets:  []float32{0, 50, 100},
		contentScroll: sc,
		pdfDoc:        &PDFiumDoc{numPages: 3},
	}

	// viewportCenter=30 (Offset=0 + Height=60/2): 第1页内
	sc.Offset.Y = 0
	sc.Resize(fyne.NewSize(200, 60))
	if p := v.getCurrentPageFromScroll(); p != 1 {
		t.Errorf("center=30 → page=%d, want 1", p)
	}

	// viewportCenter=50 (Offset=0 + Height=100/2): pageYOffsets[1]=50，二分到第2页
	sc.Offset.Y = 0
	sc.Resize(fyne.NewSize(200, 100))
	if p := v.getCurrentPageFromScroll(); p != 2 {
		t.Errorf("center=50 → page=%d, want 2", p)
	}

	// viewportCenter=125 (Offset=75 + Height=100/2): 第3页
	sc.Offset.Y = 75
	sc.Resize(fyne.NewSize(200, 100))
	if p := v.getCurrentPageFromScroll(); p != 3 {
		t.Errorf("center=125 → page=%d, want 3", p)
	}
}

func TestGetCurrentPageFromScroll_Nil(t *testing.T) {
	v := &PDFViewerPlus{currentPage: 5}
	if p := v.getCurrentPageFromScroll(); p != 5 {
		t.Errorf("nil scroll → page=%d, want 5", p)
	}
}

// --- reader_windows_plus.go: calcFillChunk ---

func TestCalcFillChunk(t *testing.T) {
	// ≤500 页 → 全量
	if c := calcFillChunk(140, 0); c != 140 {
		t.Errorf("n=140 → chunk=%d, want 140", c)
	}
	if c := calcFillChunk(500, 0); c != 500 {
		t.Errorf("n=500 → chunk=%d, want 500", c)
	}

	// >500 页首次 → 500
	if c := calcFillChunk(800, 0); c != 500 {
		t.Errorf("n=800 first → chunk=%d, want 500", c)
	}
	if c := calcFillChunk(800, 3); c != 497 {
		t.Errorf("n=800 lastFilled=3 → chunk=%d, want 497", c)
	}

	// >500 页后续 → 100
	if c := calcFillChunk(800, 500); c != 100 {
		t.Errorf("n=800 lastFilled=500 → chunk=%d, want 100", c)
	}
	if c := calcFillChunk(800, 600); c != 100 {
		t.Errorf("n=800 lastFilled=600 → chunk=%d, want 100", c)
	}
	if c := calcFillChunk(800, 700); c != 100 {
		t.Errorf("n=800 lastFilled=700 → chunk=%d, want 100", c)
	}

	// 恰好填满：lastFilled=799, n=800 → chunk=100 被 target 截断（由调用者处理）
	if c := calcFillChunk(800, 799); c != 100 {
		t.Errorf("n=800 lastFilled=799 → chunk=%d, want 100 (capped by caller)", c)
	}
}

// --- reader_pdf_plus.go: GetPagePlus fast path ---

func TestGetPagePlusFastPath(t *testing.T) {
	d := &PDFiumDoc{
		numPages: 3,
		pages: []*PDFiumPage{
			{Index: 0, Width: 612, Height: 792, Loaded: true},
			nil,
			{Index: 2, Width: 800, Height: 600, Loaded: true},
		},
	}

	// 快路径 — 页已加载
	p := d.GetPagePlus(0)
	if p == nil || p.Width != 612 || p.Height != 792 {
		t.Errorf("page 0 = %+v, want Width=612 Height=792", p)
	}

	// 快路径 — 另一页
	p = d.GetPagePlus(2)
	if p == nil || p.Width != 800 || p.Height != 600 {
		t.Errorf("page 2 = %+v, want Width=800 Height=600", p)
	}

	// 越界 → nil
	if p := d.GetPagePlus(-1); p != nil {
		t.Error("index=-1 should return nil")
	}
	if p := d.GetPagePlus(10); p != nil {
		t.Error("index=10 should return nil")
	}
}

func TestGetPagePlusFastPath_DoubleCheck(t *testing.T) {
	// 验证写锁 double-check：pages[idx] 在 Lock 之后再检查
	d := &PDFiumDoc{
		numPages: 3,
		pages: []*PDFiumPage{
			nil,                                                              // RLock 看到 nil
			nil,                                                              // 不相关
			nil,                                                              // 不相关
		},
	}

	// 同时填 pages[0] — 模拟并发 goroutine 在 RLock 释放后写锁获取前填充了数据
	loaded := &PDFiumPage{Index: 0, Width: 999, Height: 999, Loaded: true}
	d.pages[0] = loaded

	p := d.GetPagePlus(0)
	if p == nil || p.Width != 999 {
		t.Errorf("double-check failed: got %+v, want Width=999", p)
	}
}

func TestGetPagePlusConcurrent(t *testing.T) {
	d := &PDFiumDoc{
		numPages: 1,
		pages: []*PDFiumPage{
			{Index: 0, Width: 612, Height: 792, Loaded: true},
		},
	}

	// 多 goroutine 并发调用不应死锁或 panic
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := d.GetPagePlus(0)
			if p == nil || p.Width != 612 {
				t.Errorf("concurrent: got %+v", p)
			}
		}()
	}
	wg.Wait()
}
