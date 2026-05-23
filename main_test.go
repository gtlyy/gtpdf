package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"

	"gtpdf/pdfium_plus"
)

// --- main.go: page parsing ---

func TestParsePageNrs(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"", nil},
		{"1", []int{1}},
		{"1,3,5", []int{1, 3, 5}},
		{"1-3", []int{1, 2, 3}},
		{"1-3,5,7-9", []int{1, 2, 3, 5, 7, 8, 9}},
		{"1,abc,3", []int{1, 3}},
		{"2-1", nil},
		{"0", nil},
		{"1-1", []int{1}},
		{"3-5,7", []int{3, 4, 5, 7}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parsePageNrs(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parsePageNrs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSplitPoints(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"", nil},
		{"3", []int{3}},
		{"3,7,12", []int{3, 7, 12}},
		{"3-5", nil},
		{"abc", nil},
		{"3,abc,7", []int{3, 7}},
		{"0,3", []int{3}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSplitPoints(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSplitPoints(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCustomOrder(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"", nil},
		{"3,1,2", []int{3, 1, 2}},
		{"3,1,abc,2", []int{3, 1, 2}},
		{"0,3", []int{3}},
		{"3,3,1", []int{3, 3, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseCustomOrder(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCustomOrder(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePageInput(t *testing.T) {
	tests := []struct {
		input    string
		want     []string
		wantErr  bool
		errMsg   string
	}{
		{"1,3,5", []string{"1", "3", "5"}, false, ""},
		{"1-5", []string{"1-5"}, false, ""},
		{"1-3,5,7-9", []string{"1-3", "5", "7-9"}, false, ""},
		{"", nil, true, "请输入页码"},
		{"abc", nil, true, "无效的页码"},
		{"1-abc", nil, true, "无效的结束页码"},
		{"abc-5", nil, true, "无效的起始页码"},
		{"1-2-3", nil, true, "无效的页码格式"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			got, err := parsePageInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parsePageInput(%q) expected error", tt.input)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("parsePageInput(%q) error = %q, want contains %q", tt.input, err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("parsePageInput(%q) unexpected error: %v", tt.input, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parsePageInput(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePageSelection(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"1,3,5", []string{"1", "3", "5"}},
		{"1, ,3", []string{"1", "3"}},
		{"1-3", []string{"1-3"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parsePageSelection(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parsePageSelection(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCountPageRanges(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"1", 1},
		{"1,3,5", 3},
		{"1-3", 3},
		{"1-3,5,7-9", 7},
		{"1-5", 5},
		{"abc", 0},
		{"1,abc,3", 2},
		{"1-1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := countPageRanges(tt.input); got != tt.want {
				t.Errorf("countPageRanges(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- main.go: utilities ---

func TestGetFileName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/doc.pdf", "doc.pdf"},
		{"doc.pdf", "doc.pdf"},
		{"/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/very_long_filename.pdf", "very_long_filename.pdf"},
		{"a/b/c/very_long_filename_123456789_123456789_123456789_.pdf", "very_long_filename_12345678..."},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getFileName(tt.path); got != tt.want {
				t.Errorf("getFileName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestGetOddPages(t *testing.T) {
	tests := []struct {
		pageCount int
		want      []string
	}{
		{0, nil},
		{1, []string{"1"}},
		{5, []string{"1", "3", "5"}},
		{6, []string{"1", "3", "5"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d", tt.pageCount), func(t *testing.T) {
			got := getOddPages(tt.pageCount)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getOddPages(%d) = %v, want %v", tt.pageCount, got, tt.want)
			}
		})
	}
}

func TestGetEvenPages(t *testing.T) {
	tests := []struct {
		pageCount int
		want      []string
	}{
		{0, nil},
		{1, nil},
		{5, []string{"2", "4"}},
		{6, []string{"2", "4", "6"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d", tt.pageCount), func(t *testing.T) {
			got := getEvenPages(tt.pageCount)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getEvenPages(%d) = %v, want %v", tt.pageCount, got, tt.want)
			}
		})
	}
}

func TestGetOppositePosition(t *testing.T) {
	tests := []struct {
		pos  string
		want string
	}{
		{"bl", "br"},
		{"br", "bl"},
		{"tl", "tr"},
		{"tr", "tl"},
		{"tc", "tc"},
		{"bc", "bc"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.pos, func(t *testing.T) {
			if got := getOppositePosition(tt.pos); got != tt.want {
				t.Errorf("getOppositePosition(%q) = %q, want %q", tt.pos, got, tt.want)
			}
		})
	}
}

func TestGenerateNPageSplitPreview(t *testing.T) {
	tests := []struct {
		name       string
		baseName   string
		n          int
		totalPages int
		want       string
	}{
		{"single_page", "doc", 1, 1, "doc_1-1.pdf"},
		{"two_pages_one_part", "doc", 5, 3, "doc_1-3.pdf"},
		{"multi_part", "doc", 3, 7, "doc_1-3.pdf\ndoc_4-6.pdf\ndoc_7-7.pdf"},
		{"exact", "doc", 3, 6, "doc_1-3.pdf\ndoc_4-6.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateNPageSplitPreview(tt.baseName, tt.n, tt.totalPages); got != tt.want {
				t.Errorf("generateNPageSplitPreview() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateSplitPreview(t *testing.T) {
	tests := []struct {
		name       string
		baseName   string
		points     []int
		totalPages int
		want       string
	}{
		{"no_points", "doc", nil, 5, "doc_1-5.pdf"},
		{"empty_points", "doc", []int{}, 5, "doc_1-5.pdf"},
		{"single_split", "doc", []int{3}, 5, "doc_1-2.pdf\ndoc_3-5.pdf"},
		{"multi_split", "doc", []int{3, 7}, 10, "doc_1-2.pdf\ndoc_3-6.pdf\ndoc_7-10.pdf"},
		{"split_at_1", "doc", []int{1}, 5, "doc_1-5.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateSplitPreview(tt.baseName, tt.points, tt.totalPages); got != tt.want {
				t.Errorf("generateSplitPreview() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- reader_annot.go ---

func TestIsTextMarkup(t *testing.T) {
	tests := []struct {
		tool AnnotTool
		want bool
	}{
		{AnnotToolHighlight, true},
		{AnnotToolUnderline, true},
		{AnnotToolSquiggly, true},
		{AnnotToolStrikeOut, true},
		{AnnotToolRectangle, false},
		{AnnotToolLine, false},
		{AnnotToolFreeText, false},
		{AnnotToolMove, false},
		{AnnotToolEraser, false},
		{AnnotTool("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.tool), func(t *testing.T) {
			if got := isTextMarkup(tt.tool); got != tt.want {
				t.Errorf("isTextMarkup(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func TestAbs32(t *testing.T) {
	tests := []struct {
		x    float32
		want float32
	}{
		{5.0, 5.0},
		{-5.0, 5.0},
		{0, 0},
		{-0.001, 0.001},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.x), func(t *testing.T) {
			if got := abs32(tt.x); got != tt.want {
				t.Errorf("abs32(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

func TestStrconvToFloat(t *testing.T) {
	tests := []struct {
		s       string
		wantVal float64
		wantOk  bool
	}{
		{"3.14", 3.14, true},
		{"0", 0, true},
		{"-1.5", -1.5, true},
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			gotVal, gotOk := strconvToFloat(tt.s)
			if gotOk != tt.wantOk || (tt.wantOk && math.Abs(gotVal-tt.wantVal) > 1e-9) {
				t.Errorf("strconvToFloat(%q) = (%v,%v), want (%v,%v)", tt.s, gotVal, gotOk, tt.wantVal, tt.wantOk)
			}
		})
	}
}

// --- reader_pdf_plus.go ---

func TestMathAbs(t *testing.T) {
	tests := []struct {
		x    float64
		want float64
	}{
		{5.0, 5.0},
		{-5.0, 5.0},
		{0, 0},
		{-0.001, 0.001},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.x), func(t *testing.T) {
			if got := mathAbs(tt.x); got != tt.want {
				t.Errorf("mathAbs(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

func makePageText() *pdfium_plus.PageText {
	return &pdfium_plus.PageText{
		PageWidth:  100,
		PageHeight: 200,
		Blocks: []pdfium_plus.TextBlock{
			{
				Lines: []pdfium_plus.TextLine{
					{
						Y: 190, Height: 10,
						Chars: []pdfium_plus.TextChar{
							{Text: "H", X: 10, Y: 190, Width: 8, Height: 10},
							{Text: "e", X: 18, Y: 190, Width: 6, Height: 10},
							{Text: "l", X: 24, Y: 190, Width: 4, Height: 10},
							{Text: "l", X: 28, Y: 190, Width: 4, Height: 10},
							{Text: "o", X: 32, Y: 190, Width: 8, Height: 10},
						},
					},
					{
						Y: 175, Height: 10,
						Chars: []pdfium_plus.TextChar{
							{Text: "W", X: 10, Y: 175, Width: 10, Height: 10},
							{Text: "o", X: 20, Y: 175, Width: 8, Height: 10},
							{Text: "r", X: 28, Y: 175, Width: 6, Height: 10},
							{Text: "l", X: 34, Y: 175, Width: 4, Height: 10},
							{Text: "d", X: 38, Y: 175, Width: 8, Height: 10},
						},
					},
				},
			},
		},
	}
}

func TestSelectTextFromPage(t *testing.T) {
	pt := makePageText()
	tests := []struct {
		name            string
		selStartX, selStartY, selEndX, selEndY, zoom float64
		want            string
	}{
		{"select_both", 0, 0, 100, 200, 1.0, "World\nHello"},
		{"select_world_only", 10, 170, 50, 185, 1.0, "World"},
		{"no_overlap", 200, 200, 300, 300, 1.0, ""},
		{"with_zoom", 20, 350, 100, 400, 2.0, "World\nHello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectTextFromPage(pt, tt.selStartX, tt.selStartY, tt.selEndX, tt.selEndY, tt.zoom)
			if got != tt.want {
				t.Errorf("SelectTextFromPage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortSelectedChars(t *testing.T) {
	chars := []selCharPlus{
		{text: "B", x: 50, y: 10},
		{text: "A", x: 10, y: 10},
		{text: "D", x: 10, y: 30},
		{text: "C", x: 10, y: 20},
	}
	SortSelectedChars(chars)
	expected := []string{"A", "B", "C", "D"}
	var got []string
	for _, c := range chars {
		got = append(got, c.text)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("SortSelectedChars() order = %v, want %v", got, expected)
	}
}

func TestFormatSelectedChars(t *testing.T) {
	tests := []struct {
		name     string
		selected []selCharPlus
		want     string
	}{
		{"empty", nil, ""},
		{"single_line", []selCharPlus{
			{text: "H", x: 10, y: 10},
			{text: "i", x: 18, y: 10},
		}, "Hi"},
		{"multi_line", []selCharPlus{
			{text: "A", x: 10, y: 10},
			{text: "B", x: 10, y: 30},
		}, "A\nB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatSelectedChars(tt.selected); got != tt.want {
				t.Errorf("FormatSelectedChars() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractTextFromPage(t *testing.T) {
	pt := makePageText()
	tests := []struct {
		name                     string
		selMinX, selMinY, selMaxX, selMaxY, zoom float64
		want                     string
	}{
		{"full_page", 0, 0, 100, 200, 1.0, "Hello\nWorld"},
		{"partial", 10, 175, 50, 200, 1.0, "Hello\nWorld"},
		{"only_world", 10, 170, 50, 185, 1.0, "World"},
		{"no_match", 200, 200, 300, 300, 1.0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTextFromPage(pt, tt.selMinX, tt.selMinY, tt.selMaxX, tt.selMaxY, tt.zoom)
			if got != tt.want {
				t.Errorf("ExtractTextFromPage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindCharsInRect(t *testing.T) {
	pt := makePageText()
	chars := FindCharsInRect(pt, 10, 175, 50, 200)
	if len(chars) == 0 {
		t.Fatal("FindCharsInRect() returned no chars")
	}
	// Should find all chars in both lines
	if len(chars) != 10 {
		t.Errorf("FindCharsInRect() returned %d chars, want 10", len(chars))
	}
}

// --- reader_pdf_plus.go: image ---

func TestCreateWhiteImagePDFiumPlus(t *testing.T) {
	img := createWhiteImagePDFiumPlus(10, 20)
	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 20 {
		t.Errorf("image size = %dx%d, want 10x20", img.Bounds().Dx(), img.Bounds().Dy())
	}
	r, g, b, a := img.At(5, 10).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff || a != 0xffff {
		t.Errorf("pixel color = (%d,%d,%d,%d), want (65535,65535,65535,65535)", r, g, b, a)
	}
}

func TestImageToRGBAPDFiumPlus(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 5, 5))
	dst := imageToRGBAPDFiumPlus(src)
	if dst.Bounds().Dx() != 5 || dst.Bounds().Dy() != 5 {
		t.Errorf("output size = %dx%d, want 5x5", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
}

func TestInvertImageToRGBAPDFiumPlus(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	dst := invertImageToRGBAPDFiumPlus(src)
	r, _, _, _ := dst.At(0, 0).RGBA()
	if r != 0 {
		t.Errorf("inverted red = %d, want 0", r)
	}
}

// --- reader_plus_text_layer.go ---

func TestRectsOverlap(t *testing.T) {
	tests := []struct {
		name string
		ax1, ay1, ax2, ay2 float32
		bx1, by1, bx2, by2 float32
		want bool
	}{
		{"overlapping", 0, 0, 10, 10, 5, 5, 15, 15, true},
		{"no_overlap", 0, 0, 10, 10, 20, 20, 30, 30, false},
		{"touching_edge", 0, 0, 10, 10, 10, 0, 20, 10, false},
		{"contained", 0, 0, 20, 20, 5, 5, 10, 10, true},
		{"same_rect", 0, 0, 10, 10, 0, 0, 10, 10, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rectsOverlap(tt.ax1, tt.ay1, tt.ax2, tt.ay2, tt.bx1, tt.by1, tt.bx2, tt.by2); got != tt.want {
				t.Errorf("rectsOverlap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildHitBoxesFromPageText(t *testing.T) {
	pt := makePageText()
	boxes := BuildHitBoxesFromPageText(pt, 2.0)
	if len(boxes) != 10 {
		t.Errorf("BuildHitBoxesFromPageText() returned %d boxes, want 10", len(boxes))
	}
	if boxes[0].Text != "H" || boxes[0].ScreenX != 20 || boxes[0].ScreenY != 380 {
		t.Errorf("first box = %+v, want Text=H ScreenX=20 ScreenY=380", boxes[0])
	}
}

func TestBuildHitBoxesFromPageTextNil(t *testing.T) {
	if got := BuildHitBoxesFromPageText(nil, 1.0); got != nil {
		t.Errorf("BuildHitBoxesFromPageText(nil) = %v, want nil", got)
	}
}

func TestMatchHitBoxes(t *testing.T) {
	boxes := []CharHitBox{
		{ScreenX: 0, ScreenY: 0, ScreenW: 10, ScreenH: 10},
		{ScreenX: 20, ScreenY: 20, ScreenW: 10, ScreenH: 10},
		{ScreenX: 50, ScreenY: 50, ScreenW: 10, ScreenH: 10},
	}
	matched := MatchHitBoxes(boxes, 5, 5, 25, 25)
	if !reflect.DeepEqual(matched, []int{0, 1}) {
		t.Errorf("MatchHitBoxes() = %v, want [0,1]", matched)
	}
}

func TestExtractTextFromHitBoxes(t *testing.T) {
	boxes := []CharHitBox{
		{Text: "H", PdfX: 10, PdfY: 190},
		{Text: "i", PdfX: 18, PdfY: 190},
		{Text: "!", PdfX: 10, PdfY: 175},
	}
	tests := []struct {
		name    string
		matched []int
		want    string
	}{
		{"empty", nil, ""},
		{"single_line", []int{0, 1}, "Hi"},
		{"multi_line", []int{0, 1, 2}, "!\nHi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractTextFromHitBoxes(boxes, tt.matched); got != tt.want {
				t.Errorf("ExtractTextFromHitBoxes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCharHitBoxString(t *testing.T) {
	box := &CharHitBox{Text: "A", PdfX: 10.5, PdfY: 20.5, PdfW: 5, PdfH: 8}
	s := box.String()
	if !strings.Contains(s, "A") || !strings.Contains(s, "10.5") {
		t.Errorf("CharHitBox.String() = %q, want to contain 'A' and '10.5'", s)
	}
}

// --- reader_note.go ---

func TestParseNoteColor(t *testing.T) {
	tests := []struct {
		colorStr string
		want     color.Color
	}{
		{"#FFD600", color.NRGBA{R: 255, G: 214, B: 0, A: 200}},
		{"#FF5722", color.NRGBA{R: 255, G: 87, B: 34, A: 200}},
		{"#4CAF50", color.NRGBA{R: 76, G: 175, B: 80, A: 200}},
		{"#2196F3", color.NRGBA{R: 33, G: 150, B: 243, A: 200}},
		{"#9C27B0", color.NRGBA{R: 156, G: 39, B: 176, A: 200}},
		{"#FF9800", color.NRGBA{R: 255, G: 152, B: 0, A: 200}},
		{"#000000", color.NRGBA{R: 255, G: 214, B: 0, A: 200}},
		{"unknown", color.NRGBA{R: 255, G: 214, B: 0, A: 200}},
	}
	for _, tt := range tests {
		t.Run(tt.colorStr, func(t *testing.T) {
			got := parseNoteColor(tt.colorStr)
			r1, g1, b1, a1 := got.RGBA()
			r2, g2, b2, a2 := tt.want.RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				t.Errorf("parseNoteColor(%q) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.colorStr, r1, g1, b1, a1, r2, g2, b2, a2)
			}
		})
	}
}

// --- reader_windows_plus.go ---

func TestBuildPageSelectionWithoutPagePlus(t *testing.T) {
	tests := []struct {
		name       string
		totalPages int
		removePage int
		want       []string
	}{
		{"remove_middle", 5, 3, []string{"1-2", "4-5"}},
		{"remove_first", 5, 1, []string{"2-5"}},
		{"remove_last", 5, 5, []string{"1-4"}},
		{"remove_second", 5, 2, []string{"1", "3-5"}},
		{"remove_second_last", 5, 4, []string{"1-3", "5"}},
		{"single_page", 1, 1, nil},
		{"invalid_remove", 5, 0, nil},
		{"remove_out_of_range", 5, 6, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPageSelectionWithoutPagePlus(tt.totalPages, tt.removePage)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildPageSelectionWithoutPagePlus(%d,%d) = %v, want %v",
					tt.totalPages, tt.removePage, got, tt.want)
			}
		})
	}
}
