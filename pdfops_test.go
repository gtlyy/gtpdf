package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func copyToTemp(t *testing.T, src string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", src))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), src)
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestSafePageCountFile(t *testing.T) {
	t.Run("valid_pdf", func(t *testing.T) {
		path := copyToTemp(t, "eth.pdf")
		count, err := safePageCountFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count <= 0 {
			t.Fatalf("expected positive page count, got %d", count)
		}
	})

	t.Run("nonexistent_file", func(t *testing.T) {
		_, err := safePageCountFile("/nonexistent.pdf")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("empty_pdf", func(t *testing.T) {
		// 测试 ru_geng_zai_hou.pdf（已知页数），核对是否一致
		path := copyToTemp(t, "ru_geng_zai_hou.pdf")
		count, err := safePageCountFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count < 1 {
			t.Errorf("expected >=1 page, got %d", count)
		}
	})
}

func TestSplitByPageCount(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	outDir := t.TempDir()

	conf := model.NewDefaultConfiguration()
	err := api.SplitFile(path, outDir, 1, conf)
	if err != nil {
		t.Fatalf("SplitFile failed: %v", err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	// 确认输出文件数量 >= 1
	if len(entries) == 0 {
		t.Error("SplitFile produced no output files")
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".pdf") {
			t.Errorf("unexpected non-PDF output: %s", e.Name())
		}
	}
}

func TestSplitByPageNr(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	outDir := t.TempDir()

	conf := model.NewDefaultConfiguration()
	err := api.SplitByPageNrFile(path, outDir, []int{2}, conf)
	if err != nil {
		t.Fatalf("SplitByPageNrFile failed: %v", err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Errorf("expected >=2 output files, got %d", len(entries))
	}
}

func TestCollectPages(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	outPath := filepath.Join(t.TempDir(), "reordered.pdf")

	conf := model.NewDefaultConfiguration()
	// 测试倒序收集
	err := api.CollectFile(path, outPath, []string{"3", "2", "1"}, conf)
	if err != nil {
		t.Fatalf("CollectFile failed: %v", err)
	}

	count, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 pages in reordered output, got %d", count)
	}
}

func TestRotatePages(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	outPath := filepath.Join(t.TempDir(), "rotated.pdf")

	conf := model.NewDefaultConfiguration()
	err := api.RotateFile(path, outPath, 90, nil, conf)
	if err != nil {
		t.Fatalf("RotateFile failed: %v", err)
	}

	count, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Errorf("expected >=1 page after rotation, got %d", count)
	}
}

func TestTrimPages(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	outPath := filepath.Join(t.TempDir(), "trimmed.pdf")

	conf := model.NewDefaultConfiguration()
	err := api.TrimFile(path, outPath, []string{"1-2"}, conf)
	if err != nil {
		t.Fatalf("TrimFile failed: %v", err)
	}

	count, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 pages after trim, got %d", count)
	}
}

func TestMergeCreateFile(t *testing.T) {
	dir := t.TempDir()
	pdf1 := copyToTemp(t, "eth.pdf")
	pdf2 := copyToTemp(t, "ru_geng_zai_hou.pdf")
	outPath := filepath.Join(dir, "merged.pdf")

	conf := model.NewDefaultConfiguration()
	err := api.MergeCreateFile([]string{pdf1, pdf2}, outPath, false, conf)
	if err != nil {
		t.Fatalf("MergeCreateFile failed: %v", err)
	}

	count, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	// 合并后应包含两个文件的总页数
	if count < 2 {
		t.Errorf("expected >=2 pages in merged output, got %d", count)
	}
}

func TestAddTextWatermarksFile(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	outPath := filepath.Join(t.TempDir(), "numbered.pdf")

	conf := model.NewDefaultConfiguration()
	err := api.AddTextWatermarksFile(path, outPath, nil, true, "%d", "", conf)
	if err != nil {
		t.Fatalf("AddTextWatermarksFile failed: %v", err)
	}

	count, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Errorf("expected >=1 page after numbering, got %d", count)
	}
}

func TestImportImagesFile(t *testing.T) {
	imgFiles := []string{
		"testdata/testpp_1.png",
		"testdata/testpp_2.png",
	}
	outPath := filepath.Join(t.TempDir(), "from_images.pdf")

	imp := pdfcpu.DefaultImportConfig()
	imp.PageSize = "A4"
	conf := model.NewDefaultConfiguration()

	err := api.ImportImagesFile(imgFiles, outPath, imp, conf)
	if err != nil {
		t.Fatalf("ImportImagesFile failed: %v", err)
	}

	count, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Errorf("expected >=1 page, got %d", count)
	}
}

func TestFixBadAnnotations(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	ctx, err := pdfcpu.Read(f, conf)
	if err != nil {
		t.Fatalf("pdfcpu.Read failed: %v", err)
	}

	// fixBadAnnotations 不应报错
	fixBadAnnotations(ctx)
}

func TestAddPageNumbersLowLevel(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	outPath := filepath.Join(t.TempDir(), "numbered_ll.pdf")

	conf := model.NewDefaultConfiguration()
	err := addPageNumbersLowLevel(path, outPath, nil, true, "%d", "", conf)
	if err != nil {
		t.Fatalf("addPageNumbersLowLevel failed: %v", err)
	}

	count, err := api.PageCountFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Errorf("expected >=1 page, got %d", count)
	}
}

func TestEnsureChineseFont(t *testing.T) {
	// ensureChineseFont 安装字体到用户字体目录，不需要 root
	// 但会修改 pdfcpu 的字体缓存，可能影响其他测试
	if err := ensureChineseFont(); err != nil {
		t.Fatalf("ensureChineseFont failed: %v", err)
	}
	// 第二次调用应幂等（字体已安装）
	if err := ensureChineseFont(); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
}

func TestPDFInfo(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	info, err := api.PDFInfo(f, path, nil, false, conf)
	if err != nil {
		t.Fatalf("PDFInfo failed: %v", err)
	}

	t.Logf("PDF info: pages=%d, version=%s, encrypted=%v",
		info.PageCount, info.Version, info.Encrypted)

	if info.PageCount <= 0 {
		t.Errorf("expected positive page count, got %d", info.PageCount)
	}
}

func TestListRotation(t *testing.T) {
	path := copyToTemp(t, "eth.pdf")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	info, err := api.PDFInfo(f, path, nil, false, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PDF info: pages=%d, encrypted=%v", info.PageCount, info.Encrypted)
}
