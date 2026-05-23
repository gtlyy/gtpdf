package main

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
)

func TestAddRecentFileAndLoad(t *testing.T) {
	dir := t.TempDir()
	recentFile = filepath.Join(dir, "recent.json")
	os.Remove(recentFile)

	got := loadRecent()
	if len(got) != 0 {
		t.Fatalf("expected empty recent, got %v", got)
	}

	addRecentFile("/path/to/a.pdf")
	got = loadRecent()
	if len(got) != 1 || got[0] != "/path/to/a.pdf" {
		t.Errorf("after add a.pdf: %v", got)
	}

	addRecentFile("/path/to/b.pdf")
	got = loadRecent()
	if len(got) != 2 || got[0] != "/path/to/b.pdf" || got[1] != "/path/to/a.pdf" {
		t.Errorf("after add b.pdf: %v", got)
	}
}

func TestAddRecentFileDedup(t *testing.T) {
	dir := t.TempDir()
	recentFile = filepath.Join(dir, "recent.json")

	addRecentFile("/path/to/a.pdf")
	addRecentFile("/path/to/a.pdf")
	got := loadRecent()
	if len(got) != 1 {
		t.Errorf("expected 1 after dedup, got %d: %v", len(got), got)
	}

	addRecentFile("/path/to/b.pdf")
	addRecentFile("/path/to/a.pdf")
	got = loadRecent()
	if len(got) != 2 || got[0] != "/path/to/a.pdf" || got[1] != "/path/to/b.pdf" {
		t.Errorf("after re-add a.pdf: %v", got)
	}
}

func TestAddRecentFileMaxTen(t *testing.T) {
	dir := t.TempDir()
	recentFile = filepath.Join(dir, "recent.json")

	for i := 0; i < 15; i++ {
		addRecentFile(filepath.Join("/path", string(rune(65+i))+".pdf"))
	}
	got := loadRecent()
	if len(got) > 10 {
		t.Errorf("expected at most 10 entries, got %d", len(got))
	}
}

func TestAddRecentFileEmpty(t *testing.T) {
	dir := t.TempDir()
	recentFile = filepath.Join(dir, "recent.json")

	addRecentFile("")
	got := loadRecent()
	if len(got) != 0 {
		t.Errorf("expected empty after adding empty path, got %v", got)
	}
}

func TestSaveRecentMaxTen(t *testing.T) {
	dir := t.TempDir()
	recentFile = filepath.Join(dir, "recent.json")

	var paths []string
	for i := 0; i < 20; i++ {
		paths = append(paths, filepath.Join("/path", string(rune(65+i))+".pdf"))
	}
	saveRecent(paths)
	got := loadRecent()
	if len(got) != 10 {
		t.Errorf("expected 10 entries after save, got %d", len(got))
	}
}

func TestRecentFilePersistence(t *testing.T) {
	dir := t.TempDir()
	recentFile = filepath.Join(dir, "recent.json")

	addRecentFile("/doc1.pdf")
	addRecentFile("/doc2.pdf")

	recentFile = filepath.Join(dir, "recent.json")
	got := loadRecent()
	if len(got) != 2 || got[0] != "/doc2.pdf" || got[1] != "/doc1.pdf" {
		t.Errorf("after re-load: %v", got)
	}
}

func TestLogInit(t *testing.T) {
	tests := []struct {
		envVal string
		want   LogLevel
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"", LevelInfo},
		{"invalid", LevelInfo},
	}
	for _, tt := range tests {
		t.Run("GTPDF_LOG="+tt.envVal, func(t *testing.T) {
			var level LogLevel
			func() {
				defer func(old string, oldLevel LogLevel) {
					os.Setenv("GTPDF_LOG", old)
					currentLevel = oldLevel
				}(os.Getenv("GTPDF_LOG"), currentLevel)

				os.Setenv("GTPDF_LOG", tt.envVal)
				switch tt.envVal {
				case "debug":
					currentLevel = LevelDebug
				case "info":
					currentLevel = LevelInfo
				case "warn":
					currentLevel = LevelWarn
				case "error":
					currentLevel = LevelError
				default:
					currentLevel = LevelInfo
				}
				level = currentLevel
			}()
			if level != tt.want {
				t.Errorf("got level %d, want %d", level, tt.want)
			}
		})
	}
}

func TestSelectReorderPage(t *testing.T) {
	oldPages := reorderPages
	oldSel := reorderSelected
	oldSecond := reorderSecondSelected
	oldContainer := reorderPagesContainer
	defer func() {
		reorderPages = oldPages
		reorderSelected = oldSel
		reorderSecondSelected = oldSecond
		reorderPagesContainer = oldContainer
	}()

	reorderPages = []int{3, 1, 4, 2, 5}
	reorderSelected = -1
	reorderSecondSelected = -1
	reorderPagesContainer = nil // refreshReorderUI 会检查这个，nil 则 no-op

	selectReorderPage(1) // 第一次选中索引 1
	if reorderSelected != 1 {
		t.Errorf("after first select, reorderSelected = %d, want 1", reorderSelected)
	}

	selectReorderPage(1) // 再次点击同一个 → 取消选中
	if reorderSelected != -1 {
		t.Errorf("after deselect, reorderSelected = %d, want -1", reorderSelected)
	}

	selectReorderPage(0) // 选中索引 0
	selectReorderPage(2) // 选中索引 2 → 交换 0 和 2
	if reorderPages[0] != 4 || reorderPages[2] != 3 {
		t.Errorf("after swap, pages = %v, want [4,1,3,2,5]", reorderPages)
	}
	if reorderSelected != -1 || reorderSecondSelected != -1 {
		t.Errorf("after swap, should be reset: sel=%d second=%d", reorderSelected, reorderSecondSelected)
	}
}

func TestCalcImageRectCommon(t *testing.T) {
	tests := []struct {
		name       string
		sizeW, sizeH float32
		pageW, pageH float64
		zoom       float32
		wOffX, wOffY, wW, wH float32
	}{
		{"zero_page", 800, 600, 0, 0, 1.0, 0, 0, 0, 0},
		{"fit_small", 800, 600, 100, 200, 2.0, 300, 100, 200, 400},
		{"zero_size", 0, 0, 100, 200, 1.0, 0, 0, 100, 200},
		{"zoom_one", 400, 400, 200, 300, 1.0, 100, 50, 200, 300},
		{"zoom_half", 400, 400, 200, 300, 0.5, 150, 125, 100, 150},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := fyne.NewSize(tt.sizeW, tt.sizeH)
			offX, offY, w, h := calcImageRectCommon(size, tt.pageW, tt.pageH, tt.zoom)
			if offX != tt.wOffX || offY != tt.wOffY || w != tt.wW || h != tt.wH {
				t.Errorf("calcImageRectCommon() = (%v,%v,%v,%v), want (%v,%v,%v,%v)",
					offX, offY, w, h, tt.wOffX, tt.wOffY, tt.wW, tt.wH)
			}
		})
	}
}
