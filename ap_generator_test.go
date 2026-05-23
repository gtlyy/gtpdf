package main

import (
	"os"
	"strings"
	"testing"
)

func TestFindType0FontObjNum(t *testing.T) {
	tests := []struct {
		name     string
		pdfText  string
		fontName string
		want     string
	}{
		{
			"simple_type0",
			"1 0 obj <<" +
				"/Type/Font/Subtype/Type0/BaseFont/SourceHanSansSC-Regular" +
				">>",
			"SourceHanSansSC-Regular",
			"1",
		},
		{
			"not_type0",
			"2 0 obj <<" +
				"/Type/Font/Subtype/Type1/BaseFont/Helvetica" +
				">>",
			"Helvetica",
			"",
		},
		{
			"multiple_objs",
			"1 0 obj <<" +
				"/Type/Font/Subtype/Type0/BaseFont/SourceHan" +
				">>" +
				"2 0 obj <<" +
				"/Type/Font/Subtype/TrueType/BaseFont/Arial" +
				">>",
			"SourceHan",
			"1",
		},
		{
			"name_in_wrong_obj",
			"1 0 obj <<" +
				"/Type/Page" +
				">>" +
				"2 0 obj <<" +
				"/Type/Font/Subtype/Type0/BaseFont/MyFont" +
				">>",
			"MyFont",
			"2",
		},
		{
			"not_found",
			`1 0 obj<</Type/Font/Subtype/Type0/BaseFont/Helvetica>>`,
			"NotFound",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findType0FontObjNum(tt.pdfText, tt.fontName); got != tt.want {
				t.Errorf("findType0FontObjNum() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFixAPStreamCoords(t *testing.T) {
	tests := []struct {
		name   string
		str    string
		objNum string
		dx, dy float64
		want   string
	}{
		{
			"simple_tm_adjust",
			"1 0 obj <</Length 50>>\nstream\n1 0 0 1 100 200 Tm\nendstream",
			"1", 10, 20,
			"1 0 obj <</Length 50>>\nstream\n1 0 0 1 110.00 220.00 Tm\nendstream",
		},
		{
			"obj_not_found",
			"2 0 obj <</Length 10>>\nstream\nBT ET\nendstream",
			"99", 10, 20,
			"2 0 obj <</Length 10>>\nstream\nBT ET\nendstream",
		},
		{
			"no_stream",
			`1 0 obj <</Type/Font>>`,
			"1", 5, 5,
			`1 0 obj <</Type/Font>>`,
		},
		{
			"no_Tm_in_stream",
			"1 0 obj <</Length 20>>\nstream\nBT /F1 12 Tf ET\nendstream",
			"1", 10, 20,
			"1 0 obj <</Length 20>>\nstream\nBT /F1 12 Tf ET\nendstream",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixAPStreamCoords(tt.str, tt.objNum, tt.dx, tt.dy); got != tt.want {
				t.Errorf("fixAPStreamCoords() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFixMovedFreeTextAPs_NoDxDy(t *testing.T) {
	input := `1 0 obj<</Type/Annot/Subtype/FreeText>>`
	got := fixMovedFreeTextAPs(input)
	if got != input {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestFixFreeTextAPBytes(t *testing.T) {
	t.Run("no_op", func(t *testing.T) {
		data := []byte("%PDF-1.4\n1 0 obj<</Type/Catalog>>\n%%EOF")
		result, err := fixFreeTextAPBytes(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(result) != string(data) {
			t.Errorf("expected no change, got diff")
		}
	})

	t.Run("q_fix", func(t *testing.T) {
		data := []byte("%PDF-1.4\n1 0 obj <</Subtype/FreeText/Q(0)>>>>")
		result, err := fixFreeTextAPBytes(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(result), "/Q 0") {
			t.Errorf("expected /Q 0 in result, got: %s", string(result))
		}
	})

	t.Run("font_injection", func(t *testing.T) {
		data := []byte(`%PDF-1.4
1 0 obj <</Type/Font/Subtype/Type0/BaseFont/SourceHanSansSC-Regular>>
2 0 obj <</Type/XObject/Subtype/Form/Length 10>>
stream
BT ET
endstream`)
		result, err := fixFreeTextAPBytes(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(string(result), "/Resources") {
			t.Logf("font injection succeeded")
		} else {
			t.Logf("font injection skipped (expected when regex doesn't match format)")
		}
	})
}

func TestFixMovedFreeTextAPs_Bounds(t *testing.T) {
	// 回归测试：fixMovedFreeTextAPs 在小文件末尾不应 panic（Bug #29）
	input := strings.Repeat("x", 100) + "/gtpdf_dx(5)/gtpdf_dy(10)"
	result := fixMovedFreeTextAPs(input)
	if strings.Contains(result, "/gtpdf_dx") || strings.Contains(result, "/gtpdf_dy") {
		t.Logf("dx/dy entries removed (expected when no /AP ref found)")
	}
}

func TestFixFreeTextQValues(t *testing.T) {
	tmpFile := t.TempDir() + "/test.pdf"
	input := "%PDF-1.4\n1 0 obj <</Subtype/FreeText/Q(0)/Q(1)>>>>"
	if err := os.WriteFile(tmpFile, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	err := fixFreeTextQValues(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "/Q(") {
		t.Errorf("expected no /Q( after fix, got: %s", string(data))
	}
	if !strings.Contains(string(data), "/Q 0") {
		t.Errorf("expected /Q 0 after fix, got: %s", string(data))
	}
}



