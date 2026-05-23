package pdfium_plus

import (
	"testing"

	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/responses"
)

func TestBuildLineText(t *testing.T) {
	tests := []struct {
		name  string
		chars []TextChar
		want  string
	}{
		{"empty", nil, ""},
		{"single_char", []TextChar{{Text: "A"}}, "A"},
		{"multi_char", []TextChar{{Text: "H"}, {Text: "e"}, {Text: "l"}, {Text: "l"}, {Text: "o"}}, "Hello"},
		{"chinese", []TextChar{{Text: "你"}, {Text: "好"}}, "你好"},
		{"mixed", []TextChar{{Text: "A"}, {Text: "B"}, {Text: "C"}}, "ABC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLineText(tt.chars); got != tt.want {
				t.Errorf("buildLineText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanTextForSearchPlus(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"empty", "", ""},
		{"no_change", "Hello World", "Hello World"},
		{"newline_to_space", "Hello\nWorld", "Hello World"},
		{"tab_to_space", "Hello\tWorld", "Hello World"},
		{"carriage_return", "Hello\rWorld", "Hello World"},
		{"control_chars", "Hello\x00World", "HelloWorld"},
		{"mixed", "Line1\nLine2\tLine3\rLine4", "Line1 Line2 Line3 Line4"},
		{"all_control", "\x00\x01\x02", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanTextForSearchPlus(tt.s); got != tt.want {
				t.Errorf("cleanTextForSearchPlus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseDASize(t *testing.T) {
	tests := []struct {
		name string
		da   string
		want float32
	}{
		{"empty", "", 0},
		{"simple", "/Helv 12 Tf 0 0 0 rg", 12},
		{"decimal", "/F1 10.5 Tf 0 g", 10.5},
		{"no_Tf", "/Helv 12 Tj", 0},
		{"Tf_at_start", "12 Tf", 12},
		{"multiple_operators", "/F1 8 Tf 0 0 0 rg /F2 14 Tf", 8},
		{"no_number", "/Helv Tf", 0},
		{"zero", "/F1 0 Tf", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDASize(tt.da); got != tt.want {
				t.Errorf("parseDASize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseDAColor(t *testing.T) {
	tests := []struct {
		name     string
		da       string
		wantR    uint
		wantG    uint
		wantB    uint
	}{
		{"empty", "", 0, 0, 0},
		{"rg_red", "/Helv 12 Tf 1 0 0 rg", 255, 0, 0},
		{"rg_green", "0 1 0 rg", 0, 255, 0},
		{"rg_blue", "0 0 1 rg", 0, 0, 255},
		{"rg_white", "1 1 1 rg", 255, 255, 255},
		{"rg_gray", "0.5 0.5 0.5 rg", 127, 127, 127},
		{"g_gray", "0.5 g", 127, 127, 127},
		{"g_black", "0 g", 0, 0, 0},
		{"g_white", "1 g", 255, 255, 255},
		{"no_color", "/Helv 12 Tf", 0, 0, 0},
		{"rg_then_g", "1 0 0 rg 0.5 g", 255, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := parseDAColor(tt.da)
			if gotR != tt.wantR || gotG != tt.wantG || gotB != tt.wantB {
				t.Errorf("parseDAColor() = (%d,%d,%d), want (%d,%d,%d)", gotR, gotG, gotB, tt.wantR, tt.wantG, tt.wantB)
			}
		})
	}
}

func TestAnnotSubtypeToString(t *testing.T) {
	tests := []struct {
		name    string
		subtype enums.FPDF_ANNOTATION_SUBTYPE
		want    string
	}{
		{"text", enums.FPDF_ANNOT_SUBTYPE_TEXT, "Text"},
		{"link", enums.FPDF_ANNOT_SUBTYPE_LINK, "Link"},
		{"freetext", enums.FPDF_ANNOT_SUBTYPE_FREETEXT, "FreeText"},
		{"line", enums.FPDF_ANNOT_SUBTYPE_LINE, "Line"},
		{"square", enums.FPDF_ANNOT_SUBTYPE_SQUARE, "Square"},
		{"circle", enums.FPDF_ANNOT_SUBTYPE_CIRCLE, "Circle"},
		{"polygon", enums.FPDF_ANNOT_SUBTYPE_POLYGON, "Polygon"},
		{"polyline", enums.FPDF_ANNOT_SUBTYPE_POLYLINE, "PolyLine"},
		{"highlight", enums.FPDF_ANNOT_SUBTYPE_HIGHLIGHT, "Highlight"},
		{"underline", enums.FPDF_ANNOT_SUBTYPE_UNDERLINE, "Underline"},
		{"squiggly", enums.FPDF_ANNOT_SUBTYPE_SQUIGGLY, "Squiggly"},
		{"strikeout", enums.FPDF_ANNOT_SUBTYPE_STRIKEOUT, "StrikeOut"},
		{"stamp", enums.FPDF_ANNOT_SUBTYPE_STAMP, "Stamp"},
		{"caret", enums.FPDF_ANNOT_SUBTYPE_CARET, "Caret"},
		{"ink", enums.FPDF_ANNOT_SUBTYPE_INK, "Ink"},
		{"popup", enums.FPDF_ANNOT_SUBTYPE_POPUP, "Popup"},
		{"fileattachment", enums.FPDF_ANNOT_SUBTYPE_FILEATTACHMENT, "FileAttachment"},
		{"widget", enums.FPDF_ANNOT_SUBTYPE_WIDGET, "Widget"},
		{"unknown", enums.FPDF_ANNOTATION_SUBTYPE(999), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := annotSubtypeToString(tt.subtype); got != tt.want {
				t.Errorf("annotSubtypeToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertBookmarks(t *testing.T) {
	tests := []struct {
		name      string
		bookmarks []responses.GetBookmarksBookmark
		want      []BookmarkItem
	}{
		{"empty", nil, nil},
		{"single_no_dest", []responses.GetBookmarksBookmark{
			{Title: "Chapter 1"},
		}, []BookmarkItem{
			{Title: "Chapter 1", Page: 0},
		}},
		{"single_with_dest", []responses.GetBookmarksBookmark{
			{Title: "Chapter 1", DestInfo: &responses.DestInfo{PageIndex: 5}},
		}, []BookmarkItem{
			{Title: "Chapter 1", Page: 5},
		}},
		{"single_with_action", []responses.GetBookmarksBookmark{
			{Title: "Chapter 1", ActionInfo: &responses.ActionInfo{
				DestInfo: &responses.DestInfo{PageIndex: 10}},
			},
		}, []BookmarkItem{
			{Title: "Chapter 1", Page: 10},
		}},
		{"nested", []responses.GetBookmarksBookmark{
			{
				Title: "Chapter 1", DestInfo: &responses.DestInfo{PageIndex: 1},
				Children: []responses.GetBookmarksBookmark{
					{Title: "Section 1.1", DestInfo: &responses.DestInfo{PageIndex: 2}},
				},
			},
		}, []BookmarkItem{
			{Title: "Chapter 1", Page: 1, Children: []BookmarkItem{
				{Title: "Section 1.1", Page: 2},
			}},
		}},
		{"multiple_top_level", []responses.GetBookmarksBookmark{
			{Title: "Ch1", DestInfo: &responses.DestInfo{PageIndex: 1}},
			{Title: "Ch2", DestInfo: &responses.DestInfo{PageIndex: 10}},
		}, []BookmarkItem{
			{Title: "Ch1", Page: 1},
			{Title: "Ch2", Page: 10},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertBookmarks(tt.bookmarks, nil)
			if len(got) != len(tt.want) {
				t.Fatalf("convertBookmarks() length = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Title != tt.want[i].Title || got[i].Page != tt.want[i].Page {
					t.Errorf("convertBookmarks()[%d] = {%q,%d}, want {%q,%d}", i, got[i].Title, got[i].Page, tt.want[i].Title, tt.want[i].Page)
				}
				if len(got[i].Children) != len(tt.want[i].Children) {
					t.Errorf("convertBookmarks()[%d] children length = %d, want %d", i, len(got[i].Children), len(tt.want[i].Children))
				}
			}
		})
	}
}

func TestAdjustFreeTextAP(t *testing.T) {
	tests := []struct {
		name string
		ap   string
		dx   float32
		dy   float32
		want string
	}{
		{"empty", "", 0, 0, ""},
		{"no_Tm", "1 0 0 1 0 0 cm", 10, 20, "1 0 0 1 0 0 cm"},
		{"single_Tm", "1 0 0 1 100 200 Tm", 10, 20, "1 0 0 1 110.00 220.00 Tm"},
		{"negative_delta", "1 0 0 1 100 200 Tm", -10, -20, "1 0 0 1 90.00 180.00 Tm"},
		{"multiple_Tm", "1 0 0 1 100 200 Tm BT /F1 12 Tf ET 1 0 0 1 300 400 Tm", 5, 10,
			"1 0 0 1 105.00 210.00 Tm BT /F1 12 Tf ET 1 0 0 1 305.00 410.00 Tm"},
		{"zero_delta", "1 0 0 1 50 60 Tm", 0, 0, "1 0 0 1 50.00 60.00 Tm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AdjustFreeTextAP(tt.ap, tt.dx, tt.dy); got != tt.want {
				t.Errorf("AdjustFreeTextAP() = %q, want %q", got, tt.want)
			}
		})
	}
}
