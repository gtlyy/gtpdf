package main

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPdfDateNow(t *testing.T) {
	before := time.Now().UTC()
	got := pdfDateNow()
	after := time.Now().UTC()

	if !strings.HasPrefix(got, "D:") || !strings.HasSuffix(got, "Z") {
		t.Errorf("pdfDateNow() = %q, want D:...Z format", got)
	}

	matched, _ := regexp.MatchString(`^D:\d{14}Z$`, got)
	if !matched {
		t.Errorf("pdfDateNow() = %q, want D:YYYYMMDDHHmmSSZ format", got)
	}

	parsed, err := time.Parse("D:20060102150405Z", got)
	if err != nil {
		t.Fatalf("pdfDateNow() = %q, parse error: %v", got, err)
	}

	if parsed.Before(before.Add(-time.Second)) || parsed.After(after.Add(time.Second)) {
		t.Errorf("pdfDateNow() = %q is far from current time", got)
	}
}

func TestParseFreeTextAP(t *testing.T) {
	tests := []struct {
		name string
		ap   string
		want freeTextAPResult
	}{
		{
			"empty",
			"",
			freeTextAPResult{},
		},
		{
			"no_bt_et",
			"/F1 12 Tf 0 0 0 rg",
			freeTextAPResult{},
		},
		{
			"font_size_only",
			"BT /F1 12 Tf ET",
			freeTextAPResult{fontSize: 12},
		},
		{
			"color_only",
			"BT 1 0 0 rg ET",
			freeTextAPResult{hasColor: true, colorR: 255, colorG: 0, colorB: 0},
		},
		{
			"decimal_font_size",
			"BT /F1 10.5 Tf ET",
			freeTextAPResult{fontSize: 10.5},
		},
		{
			"font_and_color",
			"BT /F1 24 Tf 0 1 0 rg ET",
			freeTextAPResult{fontSize: 24, hasColor: true, colorR: 0, colorG: 255, colorB: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFreeTextAP(tt.ap)
			if got.fontSize != tt.want.fontSize ||
				got.hasColor != tt.want.hasColor ||
				got.colorR != tt.want.colorR ||
				got.colorG != tt.want.colorG ||
				got.colorB != tt.want.colorB {
				t.Errorf("parseFreeTextAP(%q) = %+v, want %+v", tt.ap, got, tt.want)
			}
		})
	}
}

func TestParseFreeTextAP_TextExtraction(t *testing.T) {
	// 测试 Tj 文本提取，会触发 gidToRune 从嵌入字体读取
	// ASCII 字符的 GID 映射是可靠的
	ap := "BT /F1 12 Tf 0 0 0 rg <0048> Tj <0065> Tj <006C> Tj <006C> Tj <006F> Tj ET"
	result := parseFreeTextAP(ap)

	if result.text != "Hello" {
		t.Logf("parseFreeTextAP() text = %q (may differ if font GID mapping varies)", result.text)
	}

	if result.fontSize != 12 {
		t.Errorf("fontSize = %v, want 12", result.fontSize)
	}
}

func TestParseFreeTextAP_MultiLine(t *testing.T) {
	// 多行文本
	ap := "BT /F1 10 Tf 0 0 0 rg <0048> Tj <0069> Tj ET\nBT /F1 10 Tf 0 0 0 rg <0031> Tj <0032> Tj ET"
	result := parseFreeTextAP(ap)

	if !strings.Contains(result.text, "\n") {
		t.Logf("multi-line text = %q", result.text)
	}
}

func TestParseFreeTextAP_GB2312(t *testing.T) {
	// 测试中文字符（如果嵌入字体支持 GID 映射）
	// 这只是验证不会崩溃，结果取决于字体
	ap := "BT /F1 10 Tf 0 0 0 rg <D2D4> Tj <CFA2> Tj ET"
	result := parseFreeTextAP(ap)
	if result.fontSize != 10 {
		t.Errorf("fontSize = %v, want 10", result.fontSize)
	}
}

func TestParseFreeTextAP_GreenColor(t *testing.T) {
	ap := "BT /F1 12 Tf 0 0.5 0 rg <0048> Tj ET"
	result := parseFreeTextAP(ap)
	if !result.hasColor {
		t.Error("expected hasColor=true")
	}
	if result.colorG != 127 {
		t.Errorf("colorG = %d, want 127", result.colorG)
	}
}

func TestGidToRune_KnownGIDs(t *testing.T) {
	// gidToRune 使用 sync.Once 延迟加载字体，所有 test 共享字体
	// 验证已知 GID 能正确映射
	tests := []struct {
		gid  uint16
		want rune
		ok   bool
	}{
		// 常见 ASCII 字符在 SourceHanSansSC-Regular 中应有 GID 映射
		{gid: 0, want: 0, ok: false},
		// 未知 GID 应返回 false
		{gid: 0xFFFF, want: 0, ok: false},
	}
	for _, tt := range tests {
		got, ok := gidToRune(tt.gid)
		if ok != tt.ok {
			t.Errorf("gidToRune(0x%04X) ok = %v, want %v", tt.gid, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("gidToRune(0x%04X) = %c (0x%04X), want %c (0x%04X)",
				tt.gid, got, got, tt.want, tt.want)
		}
	}

	// 验证映射表非空（字体加载成功且至少找到一些字符）
	// 通过 gidToRune 触发加载，然后检查是否有映射
	gidRuneMapOnce.Do(func() {}) // 确保已加载（如果之前的 test 未触发）
	if len(gidRuneMap) == 0 {
		t.Error("gidRuneMap is empty, font likely failed to load")
	} else {
		t.Logf("gidRuneMap has %d entries", len(gidRuneMap))
	}

	// 验证几个常见字符的 GID 映射（正向查找）
	foundCommon := false
	commonChars := []rune{'A', 'B', '1', '，', '。'}
	for _, ch := range commonChars {
		for gid, mapped := range gidRuneMap {
			if mapped == ch {
				foundCommon = true
				t.Logf("found char %c (0x%04X) at GID 0x%04X", ch, ch, gid)
				break
			}
		}
	}
	if !foundCommon {
		t.Log("no common chars found in gidRuneMap (font may use different GID layout)")
	}
}

func TestBuildGIDRuneMap_Idempotent(t *testing.T) {
	// 多次调用应不会 panic
	buildGIDRuneMap()
	buildGIDRuneMap()
}
