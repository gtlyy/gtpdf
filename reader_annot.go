package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gtpdf/pdfium_plus"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/klippa-app/go-pdfium/enums"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"github.com/klippa-app/go-pdfium/structs"
)

type AnnotTool string

const (
	AnnotToolHighlight AnnotTool = "highlight"
	AnnotToolUnderline AnnotTool = "underline"
	AnnotToolSquiggly  AnnotTool = "squiggly"
	AnnotToolStrikeOut AnnotTool = "strikeout"
	AnnotToolRectangle AnnotTool = "rectangle"
	AnnotToolLine      AnnotTool = "line"
	AnnotToolFreeText  AnnotTool = "freetext"
	AnnotToolFill      AnnotTool = "fill"
	AnnotToolMove      AnnotTool = "move"
	AnnotToolEraser    AnnotTool = "eraser"
)

type AnnotSettings struct {
	ColorR uint `json:"color_r"`
	ColorG uint `json:"color_g"`
	ColorB uint `json:"color_b"`
	ColorA uint `json:"color_a"`
}

var (
	annotColorPresets = []struct {
		name string
		r,g,b uint
	}{
		{"黑", 0, 0, 0},
		{"红", 255, 0, 0},
		{"橙", 255, 140, 0},
		{"黄", 255, 215, 0},
		{"绿", 0, 204, 0},
		{"青", 102, 188, 212},
		{"蓝", 0, 102, 255},
		{"紫", 153, 0, 204},
		{"白", 255, 255, 255},
	}

	defaultAnnotSettings = map[AnnotTool]AnnotSettings{
		AnnotToolHighlight: {255, 215, 0, 102},
		AnnotToolUnderline: {0, 204, 0, 255},
		AnnotToolSquiggly:  {255, 0, 0, 255},
		AnnotToolStrikeOut: {255, 0, 0, 255},
		AnnotToolRectangle: {255, 0, 0, 255},
		AnnotToolFill:      {255, 255, 255, 255},
	}
)

var annotSettingsFile string

func initAnnotSettings() {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	dir := filepath.Join(cfgDir, "gtpdf")
	os.MkdirAll(dir, 0755)
	annotSettingsFile = filepath.Join(dir, "annot_settings.json")
}

func loadAnnotSettings() map[AnnotTool]AnnotSettings {
	result := make(map[AnnotTool]AnnotSettings)
	for k, v := range defaultAnnotSettings {
		result[k] = v
	}
	if annotSettingsFile == "" {
		return result
	}
	data, err := os.ReadFile(annotSettingsFile)
	if err != nil {
		return result
	}
	var saved map[string]AnnotSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		return result
	}
	for k, v := range saved {
		result[AnnotTool(k)] = v
	}
	return result
}

func saveAnnotSettings(settings map[AnnotTool]AnnotSettings) {
	if annotSettingsFile == "" {
		return
	}
	saved := make(map[string]AnnotSettings)
	for k, v := range settings {
		saved[string(k)] = v
	}
	data, _ := json.MarshalIndent(saved, "", "  ")
	os.WriteFile(annotSettingsFile, data, 0644)
}

var annotToolSubtypes = map[AnnotTool]enums.FPDF_ANNOTATION_SUBTYPE{
	AnnotToolHighlight: enums.FPDF_ANNOT_SUBTYPE_HIGHLIGHT,
	AnnotToolUnderline: enums.FPDF_ANNOT_SUBTYPE_UNDERLINE,
	AnnotToolSquiggly:  enums.FPDF_ANNOT_SUBTYPE_SQUIGGLY,
	AnnotToolStrikeOut: enums.FPDF_ANNOT_SUBTYPE_STRIKEOUT,
	AnnotToolRectangle: enums.FPDF_ANNOT_SUBTYPE_SQUARE,
	AnnotToolLine:      enums.FPDF_ANNOT_SUBTYPE_LINE,
	AnnotToolFreeText:  enums.FPDF_ANNOT_SUBTYPE_FREETEXT,
}

var annotToolLabels = map[AnnotTool]string{
	AnnotToolHighlight: "高亮",
	AnnotToolUnderline: "下划线",
	AnnotToolSquiggly:  "波浪线",
	AnnotToolStrikeOut: "删除线",
	AnnotToolRectangle: "矩形",
	AnnotToolLine:      "线段",
	AnnotToolFreeText:  "夹批",
	AnnotToolFill:      "填充",
	AnnotToolMove:      "移动",
	AnnotToolEraser:    "删除",
}

var annotToolIcons = map[AnnotTool]fyne.Resource{
	AnnotToolHighlight: theme.ColorChromaticIcon(),
	AnnotToolUnderline: theme.ContentPasteIcon(),
	AnnotToolSquiggly:  theme.InfoIcon(),
	AnnotToolStrikeOut: theme.CancelIcon(),
	AnnotToolRectangle: theme.CheckButtonIcon(),
	AnnotToolLine:      theme.MoveDownIcon(),
	AnnotToolFreeText:  theme.FileTextIcon(),
	AnnotToolFill:      theme.ContentPasteIcon(),
	AnnotToolMove:      theme.NavigateNextIcon(),
	AnnotToolEraser:    theme.DeleteIcon(),
}

func isTextMarkup(tool AnnotTool) bool {
	return tool == AnnotToolHighlight || tool == AnnotToolUnderline ||
		tool == AnnotToolSquiggly || tool == AnnotToolStrikeOut
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// --- GID → Unicode reverse mapping (from embedded font) ---

var (
	gidRuneMapOnce sync.Once
	gidRuneMap     map[uint16]rune
)

func buildGIDRuneMap() {
	fontData, err := fontFS.ReadFile("SourceHanSansSC-Regular.otf")
	if err != nil {
		return
	}
	fnt, err := sfnt.Parse(fontData)
	if err != nil {
		return
	}
	gidRuneMap = make(map[uint16]rune)
	var buf sfnt.Buffer

	ranges := []struct{ start, end rune }{
		{0x0020, 0x007E}, // ASCII
		{0x00A0, 0x00FF}, // Latin-1 Supplement
		{0x2000, 0x206F}, // General Punctuation
		{0x3000, 0x303F}, // CJK Symbols and Punctuation
		{0x3400, 0x4DBF}, // CJK Extension A
		{0x4E00, 0x9FFF}, // CJK Unified Ideographs
		{0xF900, 0xFAFF}, // CJK Compatibility Ideographs
		{0xFF00, 0xFFEF}, // Halfwidth and Fullwidth Forms
	}
	for _, r := range ranges {
		for ch := r.start; ch <= r.end; ch++ {
			gid, err := fnt.GlyphIndex(&buf, ch)
			if err == nil && gid > 0 {
				gidRuneMap[uint16(gid)] = ch
			}
		}
	}
}

func gidToRune(gid uint16) (rune, bool) {
	gidRuneMapOnce.Do(buildGIDRuneMap)
	ch, ok := gidRuneMap[gid]
	return ch, ok
}

// --- Parse FreeText appearance stream (/AP) ---

type freeTextAPResult struct {
	text     string
	fontSize float32
	colorR   uint
	colorG   uint
	colorB   uint
	hasColor bool
}

func parseFreeTextAP(ap string) freeTextAPResult {
	var res freeTextAPResult
	btIdx := strings.Index(ap, "BT")
	etIdx := strings.Index(ap, "ET")
	if btIdx < 0 || etIdx <= btIdx {
		return res
	}
	body := ap[btIdx+2 : etIdx]
	tokens := strings.Fields(body)

	var lines []string
	var lineRunes []rune
	collectLine := func() {
		if len(lineRunes) > 0 {
			lines = append(lines, string(lineRunes))
			lineRunes = nil
		}
	}

	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "Tf":
			if i >= 1 {
				if v, err := strconv.ParseFloat(tokens[i-1], 32); err == nil {
					res.fontSize = float32(v)
				}
			}
		case "rg":
			if i >= 3 {
				res.hasColor = true
				if vr, err := strconv.ParseFloat(tokens[i-3], 32); err == nil {
					res.colorR = uint(vr * 255)
				}
				if vg, err := strconv.ParseFloat(tokens[i-2], 32); err == nil {
					res.colorG = uint(vg * 255)
				}
				if vb, err := strconv.ParseFloat(tokens[i-1], 32); err == nil {
					res.colorB = uint(vb * 255)
				}
			}
		case "Tj":
			if i >= 1 {
				hexStr := tokens[i-1]
				if len(hexStr) >= 2 && hexStr[0] == '<' && hexStr[len(hexStr)-1] == '>' {
					hexStr = hexStr[1 : len(hexStr)-1]
					data, err := hex.DecodeString(hexStr)
					if err == nil {
						for j := 0; j+1 < len(data); j += 2 {
							gid := uint16(data[j])<<8 | uint16(data[j+1])
							if ch, ok := gidToRune(gid); ok {
								lineRunes = append(lineRunes, ch)
							} else {
								lineRunes = append(lineRunes, '?')
							}
						}
					}
				}
			}
			collectLine()
		}
	}
	collectLine()

	if len(lines) > 0 {
		res.text = strings.Join(lines, "\n")
	}
	return res
}

type AnnotLayer struct {
	widget.BaseWidget
	viewer      *PDFViewerPlus
	toolLayer   *AnnotToolLayer
	PageIdx     int
	pageWidth   float64
	pageHeight  float64
	imgOffX     float32
	imgOffY     float32
	imgW        float32
	imgH        float32
	annotations []pdfium_plus.AnnotationInfo

	dragAnnotIdx int
	dragOffsetX  float64
	dragOffsetY  float64
}

func NewAnnotLayer(viewer *PDFViewerPlus) *AnnotLayer {
	al := &AnnotLayer{viewer: viewer, dragAnnotIdx: -1}
	al.ExtendBaseWidget(al)
	return al
}

func calcImageRectCommon(size fyne.Size, pageWidth, pageHeight float64, zoom float32) (offX, offY, w, h float32) {
	if pageWidth <= 0 || pageHeight <= 0 {
		return
	}
	w = float32(pageWidth * float64(zoom))
	h = float32(pageHeight * float64(zoom))
	if size.Width > 0 && size.Height > 0 {
		offX = (size.Width - w) / 2
		offY = (size.Height - h) / 2
	}
	return
}

func (al *AnnotLayer) calcImageRect() {
	al.imgOffX, al.imgOffY, al.imgW, al.imgH = 0, 0, 0, 0
	if al.viewer == nil || al.viewer.pdfDoc == nil {
		return
	}
	al.imgOffX, al.imgOffY, al.imgW, al.imgH = calcImageRectCommon(al.Size(), al.pageWidth, al.pageHeight, al.viewer.zoom)
}

func (al *AnnotLayer) pdfToScreen(pdfX, pdfY float64) (float32, float32) {
	zoom := float64(al.viewer.zoom)
	sx := al.imgOffX + float32(pdfX*zoom)
	sy := al.imgOffY + float32(pdfY*zoom)
	return sx, sy
}

func (al *AnnotLayer) screenToPdf(sx, sy float32) (float64, float64) {
	if al.pageWidth <= 0 || al.pageHeight <= 0 || al.imgW <= 0 || al.imgH <= 0 {
		return float64(sx) / float64(al.viewer.zoom), float64(sy) / float64(al.viewer.zoom)
	}
	ix := float64(sx - al.imgOffX)
	iy := float64(sy - al.imgOffY)
	pdfX := ix / float64(al.imgW) * al.pageWidth
	pdfY := iy / float64(al.imgH) * al.pageHeight
	return pdfX, pdfY
}

func (al *AnnotLayer) CreateRenderer() fyne.WidgetRenderer {
	return &annotLayerRenderer{layer: al}
}

type AnnotToolLayer struct {
	widget.BaseWidget
	viewer        *PDFViewerPlus
	annotLayer    *AnnotLayer
	PageIdx       int
	toolBtns      map[AnnotTool]*widget.Button
	currentTool   AnnotTool
	annotToolbar  *fyne.Container
	hoverAnnot    int
	dragging      bool
	dragStart     fyne.Position
	dragEnd       fyne.Position
	movingAnnotIdx int
}

func NewAnnotToolLayer(viewer *PDFViewerPlus, annotLayer *AnnotLayer) *AnnotToolLayer {
	atl := &AnnotToolLayer{
		viewer:         viewer,
		annotLayer:     annotLayer,
		toolBtns:       make(map[AnnotTool]*widget.Button),
		hoverAnnot:     -1,
		movingAnnotIdx: -1,
	}
	atl.ExtendBaseWidget(atl)
	atl.createToolbar()
	atl.Hide()
	return atl
}

func (atl *AnnotToolLayer) createToolbar() {
	var btns []fyne.CanvasObject
	tools := []AnnotTool{
		AnnotToolHighlight, AnnotToolUnderline, AnnotToolSquiggly, AnnotToolStrikeOut,
		AnnotToolRectangle, AnnotToolFill, AnnotToolFreeText, AnnotToolMove, AnnotToolEraser,
	}
	for _, tool := range tools {
		t := tool
		btn := widget.NewButtonWithIcon(annotToolLabels[t], annotToolIcons[t], func() {
			atl.selectTool(t)
		})
		btn.Importance = widget.MediumImportance
		atl.toolBtns[t] = btn
		btns = append(btns, btn)
	}

	btns = append(btns, widget.NewSeparator())

	visBtn := widget.NewButtonWithIcon("隐藏", theme.VisibilityIcon(), nil)
	visBtn.Importance = widget.MediumImportance
	visBtn.OnTapped = func() {
		if atl.viewer.pdfDoc == nil {
			return
		}
		if atl.annotLayer.Visible() {
			atl.annotLayer.Hide()
			visBtn.SetText("显示")
			visBtn.SetIcon(theme.VisibilityOffIcon())
		} else {
			atl.annotLayer.Show()
			visBtn.SetText("隐藏")
			visBtn.SetIcon(theme.VisibilityIcon())
		}
		visBtn.Refresh()
		atl.viewer.parentWin.Canvas().Refresh(atl.annotLayer)
		atl.viewer.syncContinuousAnnotMode()
	}
	btns = append(btns, visBtn)

	deleteBtn := widget.NewButtonWithIcon("清空", theme.DeleteIcon(), func() {
		if atl.viewer.pdfDoc == nil {
			return
		}
		dialog.ShowConfirm("确认", "确定删除全部标注？此操作不可撤销。", func(ok bool) {
			if !ok {
				return
			}
			go func() {
				err := atl.viewer.pdfDoc.AnnotRemoveAll()
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(fmt.Errorf("删除标注失败: %w", err), atl.viewer.parentWin)
						return
					}
					atl.viewer.markDirty()
					atl.viewer.annotLayer.Refresh()
					atl.viewer.parentWin.Canvas().Refresh(atl.viewer.annotLayer)
				})
			}()
		}, atl.viewer.parentWin)
	})
	btns = append(btns, deleteBtn)

	settingBtn := widget.NewButtonWithIcon("设置", theme.SettingsIcon(), func() {
		atl.showSettingsDialog()
	})
	btns = append(btns, settingBtn)

	saveBtn := widget.NewButtonWithIcon("保存", theme.DocumentSaveIcon(), func() {
		atl.viewer.SaveNow()
	})
	btns = append(btns, saveBtn)

	atl.annotToolbar = container.NewHBox(btns...)
}

func (atl *AnnotToolLayer) showSettingsDialog() {
	settingTools := []AnnotTool{
		AnnotToolHighlight, AnnotToolUnderline,
		AnnotToolSquiggly, AnnotToolStrikeOut, AnnotToolRectangle, AnnotToolFill,
	}

	colorOpts := []string{"黑", "红", "橙", "黄", "绿", "青", "蓝", "紫", "白"}

	var items []*widget.FormItem
	for _, tool := range settingTools {
		t := tool
		settings := atl.viewer.annotSettings[t]

		row := container.NewHBox()
		row.Add(widget.NewLabel(annotToolLabels[t] + ":"))

		colorSel := widget.NewSelect(colorOpts, func(val string) {
			cur := atl.viewer.annotSettings[t]
			for _, cp := range annotColorPresets {
				if cp.name == val {
					cur.ColorR = cp.r
					cur.ColorG = cp.g
					cur.ColorB = cp.b
					break
				}
			}
			atl.viewer.annotSettings[t] = cur
			saveAnnotSettings(atl.viewer.annotSettings)
		})
		bestColor := 0
		for i, cp := range annotColorPresets {
			if settings.ColorR == cp.r && settings.ColorG == cp.g && settings.ColorB == cp.b {
				bestColor = i
				break
			}
		}
		colorSel.SetSelected(colorOpts[bestColor])
		row.Add(colorSel)

		items = append(items, widget.NewFormItem("", row))
	}

	dialog.ShowForm("标注设置", "关闭", "", items, func(bool) {}, atl.viewer.parentWin)
}

func (atl *AnnotToolLayer) Toolbar() *fyne.Container {
	return atl.annotToolbar
}

func (atl *AnnotToolLayer) SetDefaultTool(tool AnnotTool) {
	atl.selectTool(tool)
}

func (atl *AnnotToolLayer) selectTool(tool AnnotTool) {
	if atl.currentTool == tool {
		atl.currentTool = ""
		for _, b := range atl.toolBtns {
			b.Importance = widget.MediumImportance
			b.Refresh()
		}
		return
	}
	atl.currentTool = tool
	for t, b := range atl.toolBtns {
		if t == tool {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.MediumImportance
		}
		b.Refresh()
	}
}

func (atl *AnnotToolLayer) SetAnnotMode(enabled bool) {
	if enabled {
		atl.Show()
	} else {
		atl.Hide()
		atl.currentTool = ""
		atl.dragging = false
		atl.hoverAnnot = -1
		for _, b := range atl.toolBtns {
			b.Importance = widget.MediumImportance
			b.Refresh()
		}
	}
}

func (atl *AnnotToolLayer) CreateRenderer() fyne.WidgetRenderer {
	return &annotToolLayerRenderer{layer: atl}
}

func (atl *AnnotToolLayer) MouseIn(ev *desktop.MouseEvent) {}

func (atl *AnnotToolLayer) MouseOut() {
	atl.hoverAnnot = -1
	atl.annotLayer.Refresh()
}

func (atl *AnnotToolLayer) MouseDown(ev *desktop.MouseEvent) {
	if atl.viewer.pdfDoc == nil {
		return
	}

	// Per-page layers: always sync tool from global layer
	if atl.PageIdx >= 0 && atl.viewer.annotToolLayer != atl {
		atl.currentTool = atl.viewer.annotToolLayer.currentTool
	}

	if atl.currentTool == "" {
		atl.editClicked(ev)
		return
	}

	if atl.currentTool == AnnotToolEraser {
		atl.eraseClicked(ev)
		return
	}

	if atl.currentTool == AnnotToolFreeText {
		atl.freeTextClicked(ev)
		return
	}

	if atl.currentTool == AnnotToolMove {
		atl.moveClicked(ev)
		return
	}

	atl.annotLayer.calcImageRect()
	if atl.annotLayer.imgW <= 0 || atl.annotLayer.imgH <= 0 {
		return
	}
	atl.dragging = true
	atl.dragStart = ev.Position
	atl.dragEnd = ev.Position
	canvas.Refresh(atl)
}

func (atl *AnnotToolLayer) MouseUp(ev *desktop.MouseEvent) {
	if !atl.dragging {
		return
	}
	atl.dragging = false
	if atl.currentTool == AnnotToolEraser {
		return
	}
	if atl.currentTool == AnnotToolMove && atl.movingAnnotIdx >= 0 {
		idx := atl.movingAnnotIdx
		atl.movingAnnotIdx = -1
		atl.annotLayer.dragAnnotIdx = -1
		atl.annotLayer.dragOffsetX = 0
		atl.annotLayer.dragOffsetY = 0
		startIX, startIY := atl.annotLayer.screenToPdf(atl.dragStart.X, atl.dragStart.Y)
		endIX, endIY := atl.annotLayer.screenToPdf(ev.Position.X, ev.Position.Y)
		dx := float32(endIX - startIX)
		dy := float32(-(endIY - startIY))
		if abs32(float32(endIX-startIX)) >= 1 || abs32(float32(endIY-startIY)) >= 1 {
			pageIdx := atl.PageIdx
			atl.viewer.pdfDoc.AnnotMoveFreeText(pageIdx, idx, dx, dy)
			atl.viewer.markDirty()
		}
		atl.annotLayer.Refresh()
		atl.viewer.parentWin.Canvas().Refresh(atl.annotLayer)
		canvas.Refresh(atl)
		return
	}
	dist := fyne.NewPos(atl.dragEnd.X-atl.dragStart.X, atl.dragEnd.Y-atl.dragStart.Y)
	if abs32(dist.X) < 3 && abs32(dist.Y) < 3 {
		canvas.Refresh(atl)
		return
	}
	if isTextMarkup(atl.currentTool) {
		atl.createTextMarkup()
	} else {
		atl.createShapeAnnot()
	}
	atl.annotLayer.Refresh()
	atl.viewer.parentWin.Canvas().Refresh(atl.annotLayer)
	atl.Refresh()
	canvas.Refresh(atl)
}

func (atl *AnnotToolLayer) MouseMoved(ev *desktop.MouseEvent) {
	if atl.dragging {
		atl.dragEnd = ev.Position
		if atl.currentTool == AnnotToolMove && atl.movingAnnotIdx >= 0 {
			startIX, startIY := atl.annotLayer.screenToPdf(atl.dragStart.X, atl.dragStart.Y)
			curIX, curIY := atl.annotLayer.screenToPdf(ev.Position.X, ev.Position.Y)
			atl.annotLayer.dragOffsetX = curIX - startIX
			atl.annotLayer.dragOffsetY = -(curIY - startIY)
			atl.annotLayer.Refresh()
			return
		}
		atl.Refresh()
		canvas.Refresh(atl)
		return
	}
	atl.findHoverAnnot(ev)
}

func (atl *AnnotToolLayer) findHoverAnnot(ev *desktop.MouseEvent) {
	atl.annotLayer.calcImageRect()
	if atl.annotLayer.imgW <= 0 || atl.annotLayer.imgH <= 0 {
		return
	}
	prev := atl.hoverAnnot
	atl.hoverAnnot = -1
	for i, a := range atl.annotLayer.annotations {
		l, t1 := atl.annotLayer.pdfToScreen(a.Rect.Left, atl.annotLayer.pageHeight-a.Rect.Top)
		r, b1 := atl.annotLayer.pdfToScreen(a.Rect.Right, atl.annotLayer.pageHeight-a.Rect.Bottom)
		if l > r {
			l, r = r, l
		}
		if t1 > b1 {
			t1, b1 = b1, t1
		}
		if ev.Position.X >= l && ev.Position.X <= r && ev.Position.Y >= t1 && ev.Position.Y <= b1 {
			atl.hoverAnnot = i
			break
		}
	}
	if prev != atl.hoverAnnot {
		atl.annotLayer.Refresh()
	}
}

func (atl *AnnotToolLayer) freeTextClicked(ev *desktop.MouseEvent) {
	atl.annotLayer.calcImageRect()
	if atl.annotLayer.imgW <= 0 || atl.annotLayer.imgH <= 0 {
		return
	}

	pdfX, pdfY := atl.annotLayer.screenToPdf(ev.Position.X, ev.Position.Y)

	layoutSel := widget.NewSelect([]string{"横排", "竖排"}, nil)
	layoutSel.SetSelected("横排")

	sizeSel := widget.NewSelect([]string{"6", "8", "10", "12", "14", "16", "18", "20", "24", "28", "32", "36", "40", "44", "48"}, nil)
	sizeSel.SetSelected("10")

	fgColorSel := widget.NewSelect([]string{"黑", "红", "橙", "黄", "绿", "蓝", "靛", "紫", "白", "灰"}, nil)
	fgColorSel.SetSelected("红")

	rowOpts := container.NewGridWithColumns(6,
		widget.NewLabel("排版"), layoutSel,
		widget.NewLabel("字号"), sizeSel,
		widget.NewLabel("字色"), fgColorSel,
	)

	entry := widget.NewMultiLineEntry()
	entry.SetPlaceHolder("输入夹批内容...")
	entry.Wrapping = fyne.TextWrapBreak
	entryScroll := container.NewVScroll(entry)
	entryScroll.SetMinSize(fyne.NewSize(380, 100))

	content := container.NewBorder(
		nil, nil, nil, nil,
		container.NewVBox(
			widget.NewSeparator(),
			rowOpts,
			widget.NewSeparator(),
			widget.NewLabel("内容:"),
			entryScroll,
		),
	)

	dlg := dialog.NewForm("添加夹批", "添加", "取消", []*widget.FormItem{
		widget.NewFormItem("", content),
	}, func(ok bool) {
		if !ok || entry.Text == "" {
			return
		}
		text := entry.Text
		reorder := layoutSel.Selected == "竖排"
		fontSize := sizeSel.Selected
		atl.createFreeTextAt(pdfX, pdfY, text, reorder, "", fontSize, fgColorSel.Selected)
	}, atl.viewer.parentWin)
	dlg.Resize(fyne.NewSize(480, 320))
	dlg.Show()
}

func (atl *AnnotToolLayer) createFreeTextAt(pdfX, pdfY float64, text string, verticalLayout bool, _, fontSize, fgName string) {
	pageIdx := atl.PageIdx

	sizeNum, _ := strconvToFloat(fontSize)
	if sizeNum <= 0 {
		sizeNum = 10
	}

	fontData, err := fontFS.ReadFile("SourceHanSansSC-Regular.otf")
	if err != nil {
		dialog.ShowError(fmt.Errorf("读取字体文件失败: %w", err), atl.viewer.parentWin)
		return
	}

	fontResp, err := atl.viewer.pdfDoc.FontLoad(fontData, enums.FPDF_FONT_TRUETYPE, true)
	if err != nil {
		dialog.ShowError(fmt.Errorf("字体加载失败: %w", err), atl.viewer.parentWin)
		return
	}

	fnt, err := sfnt.Parse(fontData)
	if err != nil {
		dialog.ShowError(fmt.Errorf("字体解析失败: %w", err), atl.viewer.parentWin)
		return
	}

	fgN := "0 0 0 rg"
	switch fgName {
	case "红":
		fgN = "1 0 0 rg"
	case "橙":
		fgN = "1 0.5 0 rg"
	case "黄":
		fgN = "1 1 0 rg"
	case "绿":
		fgN = "0 1 0 rg"
	case "蓝":
		fgN = "0 0 1 rg"
	case "靛":
		fgN = "0.29 0 0.51 rg"
	case "紫":
		fgN = "0.5 0 1 rg"
	case "白":
		fgN = "1 1 1 rg"
	case "灰":
		fgN = "0.5 0.5 0.5 rg"
	}

	var buf sfnt.Buffer
	ppem := fixed.Int26_6(int(sizeNum * 64))

	var l, t, r, b float32
	var apContent string
	chars := []rune(text)

	if verticalLayout {
		// 竖排: each character on its own line
		var verticalText strings.Builder
		var apBuf strings.Builder
		apBuf.WriteString(fmt.Sprintf("BT /%s %.2f Tf %s", fontResp.FontName, sizeNum, fgN))
		padX := sizeNum * 0.3
		if padX < 3 {
			padX = 3
		}
		padY := sizeNum * 0.5
		if padY < 4 {
			padY = 4
		}
		tx := pdfX + float64(padX)
		charW := float64(sizeNum) * 1.2
		ascender := sizeNum * 0.85
		firstCharY := atl.annotLayer.pageHeight - pdfY - float64(padY) - ascender
		charAdv := float64(sizeNum) * 1.4

		for i, ch := range chars {
			gid, err := fnt.GlyphIndex(&buf, ch)
			if err != nil {
				continue
			}
			if i > 0 {
				verticalText.WriteString("\n")
			}
			verticalText.WriteRune(ch)
			cy := firstCharY - float64(i)*charAdv
			apBuf.WriteString(fmt.Sprintf(" 1 0 0 1 %.2f %.2f Tm <%04X> Tj", tx, cy, uint16(gid)))
		}
		apBuf.WriteString(" ET")
		apContent = apBuf.String()

		textW := charW + float64(padX)*2
		textH := float64(len(chars))*charAdv + float64(padY)*2
		l = float32(pdfX)
		t = float32(atl.annotLayer.pageHeight - pdfY)
		r = float32(pdfX + textW)
		b = float32(atl.annotLayer.pageHeight - pdfY - textH)
		text = verticalText.String()
	} else {
		// 横排: all chars in one line (original behavior)
		var hexBuf strings.Builder
		totalAdvance := fixed.Int26_6(0)
		for _, ch := range chars {
			gid, err := fnt.GlyphIndex(&buf, ch)
			if err != nil {
				continue
			}
			binary.Write(&hexBuf, binary.BigEndian, uint16(gid))
			adv, err := fnt.GlyphAdvance(&buf, gid, ppem, font.HintingNone)
			if err == nil {
				totalAdvance += adv
			} else {
				totalAdvance += ppem
			}
		}
		hexGIDs := fmt.Sprintf("<%X>", hexBuf.String())
		textW := float64(totalAdvance)/64.0 + 4.0
		textH := float64(sizeNum)*1.2 + 4.0
		l = float32(pdfX)
		t = float32(atl.annotLayer.pageHeight - pdfY)
		r = float32(pdfX + textW)
		b = float32(atl.annotLayer.pageHeight - pdfY - textH)

		logD("[FREETEXT] page=%d pdfX=%.1f pdfY=%.1f l=%.1f t=%.1f r=%.1f b=%.1f size=%.1f color=%q hex=%s adv=%.1f",
			pageIdx+1, pdfX, pdfY, l, t, r, b, sizeNum, fgN, hexGIDs, float64(totalAdvance)/64.0)

		err = atl.viewer.pdfDoc.CreateFreeTextAnnot(pageIdx, fontResp.Font, fontResp.FontName,
			l, t, r, b, text, float32(sizeNum), fgN, hexGIDs, "")
		if err != nil {
			dialog.ShowError(fmt.Errorf("创建夹批标注失败: %w", err), atl.viewer.parentWin)
			return
		}
		atl.viewer.markDirty()
		atl.annotLayer.Refresh()
		atl.viewer.parentWin.Canvas().Refresh(atl.annotLayer)
		return
	}

	err = atl.viewer.pdfDoc.CreateFreeTextAnnot(pageIdx, fontResp.Font, fontResp.FontName,
		l, t, r, b, text, float32(sizeNum), fgN, "", apContent)
	if err != nil {
		dialog.ShowError(fmt.Errorf("创建夹批标注失败: %w", err), atl.viewer.parentWin)
		return
	}

	atl.viewer.markDirty()
	atl.viewer.annotLayer.Refresh()
	atl.viewer.parentWin.Canvas().Refresh(atl.viewer.annotLayer)
}

func pdfDateNow() string {
	return time.Now().UTC().Format("D:20060102150405Z")
}

func strconvToFloat(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func (atl *AnnotToolLayer) editClicked(ev *desktop.MouseEvent) {
	if !atl.viewer.annotMode {
		return
	}
	atl.annotLayer.calcImageRect()
	if atl.annotLayer.imgW <= 0 || atl.annotLayer.imgH <= 0 {
		return
	}
	pageIdx := atl.PageIdx
	for i, a := range atl.annotLayer.annotations {
		l, t1 := atl.annotLayer.pdfToScreen(a.Rect.Left, atl.annotLayer.pageHeight-a.Rect.Top)
		r, b1 := atl.annotLayer.pdfToScreen(a.Rect.Right, atl.annotLayer.pageHeight-a.Rect.Bottom)
		if l > r {
			l, r = r, l
		}
		if t1 > b1 {
			t1, b1 = b1, t1
		}
		if ev.Position.X >= l && ev.Position.X <= r && ev.Position.Y >= t1 && ev.Position.Y <= b1 {
			idx := i
			if a.Type == "FreeText" {
				atl.editFreeText(pageIdx, idx, a)
				return
			}
			entry := widget.NewMultiLineEntry()
			entry.SetText(a.Contents)
			entry.Wrapping = fyne.TextWrapWord
			entryScroll := container.NewVScroll(entry)
			entryScroll.SetMinSize(fyne.NewSize(400, 80))

			content := container.NewVBox(
				widget.NewLabelWithStyle("编辑标注备注", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				entryScroll,
			)

			dlg := dialog.NewCustom("标注备注", "关闭", content, atl.viewer.parentWin)
			dlg.SetOnClosed(func() {
				text := entry.Text
				if text == "" {
					text = " "
				}
				if text != a.Contents {
					atl.viewer.pdfDoc.AnnotSetContentByIndex(pageIdx, idx, text)
					atl.viewer.markDirty()
					atl.viewer.annotLayer.Refresh()
					atl.viewer.parentWin.Canvas().Refresh(atl.viewer.annotLayer)
				}
			})
			dlg.Resize(fyne.NewSize(450, 200))
			dlg.Show()
			return
		}
	}
}

func (atl *AnnotToolLayer) editFreeText(pageIdx, annotIdx int, a pdfium_plus.AnnotationInfo) {
	entry := widget.NewMultiLineEntry()
	entry.SetText(a.Contents)
	entry.Wrapping = fyne.TextWrapWord
	entryScroll := container.NewVScroll(entry)
	entryScroll.SetMinSize(fyne.NewSize(400, 80))

	content := container.NewVBox(
		widget.NewLabelWithStyle("编辑夹批文本", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		entryScroll,
	)

	dlg := dialog.NewCustom("编辑夹批", "关闭", content, atl.viewer.parentWin)
	dlg.SetOnClosed(func() {
		newText := entry.Text
		if newText == "" || newText == a.Contents {
			return
		}

		// 删除旧夹批，然后重新创建
		if err := atl.viewer.pdfDoc.AnnotRemove(pageIdx, annotIdx); err != nil {
			dialog.ShowError(fmt.Errorf("删除旧夹批失败: %w", err), atl.viewer.parentWin)
			return
		}

		pdfX := a.Rect.Left
		pdfY := atl.annotLayer.pageHeight - a.Rect.Top

		fontData, err := fontFS.ReadFile("SourceHanSansSC-Regular.otf")
		if err != nil {
			dialog.ShowError(fmt.Errorf("读取字体文件失败: %w", err), atl.viewer.parentWin)
			return
		}

		fnt, err := sfnt.Parse(fontData)
		if err != nil {
			dialog.ShowError(fmt.Errorf("字体解析失败: %w", err), atl.viewer.parentWin)
			return
		}

		fontName := "SourceHanSansSC-Regular"
		// 显式加载字体到文档，确保 PDFium 保存时包含该字体资源
		fontResp, err := atl.viewer.pdfDoc.FontLoad(fontData, enums.FPDF_FONT_TRUETYPE, true)
		if err != nil {
			dialog.ShowError(fmt.Errorf("字体加载失败: %w", err), atl.viewer.parentWin)
			return
		}

		sizeNum := float64(a.DASize)
		if sizeNum < 6 {
			sizeNum = 10
		}

		var buf sfnt.Buffer
		var hexBuf strings.Builder
		ppem := fixed.Int26_6(int(sizeNum * 64))
		totalAdv := fixed.Int26_6(0)
		for _, ch := range newText {
			gid, err := fnt.GlyphIndex(&buf, ch)
			if err != nil {
				continue
			}
			binary.Write(&hexBuf, binary.BigEndian, uint16(gid))
			adv, err := fnt.GlyphAdvance(&buf, gid, ppem, font.HintingNone)
			if err == nil {
				totalAdv += adv
			} else {
				totalAdv += ppem
			}
		}
		hexGIDs := fmt.Sprintf("<%X>", hexBuf.String())
		textW := float64(totalAdv)/64.0 + 4.0
		textH := sizeNum*1.2 + 4.0

		l := float32(pdfX)
		t := float32(pdfY)
		r := float32(pdfX + textW)
		b := float32(pdfY + textH)

		tPdf := float32(atl.annotLayer.pageHeight) - t
		bPdf := float32(atl.annotLayer.pageHeight) - b

		fgN := fmt.Sprintf("%.3f %.3f %.3f rg",
			float32(a.DAColorR)/255.0,
			float32(a.DAColorG)/255.0,
			float32(a.DAColorB)/255.0)

		if err := atl.viewer.pdfDoc.CreateFreeTextAnnot(pageIdx, fontResp.Font, fontName,
			l, tPdf, r, bPdf, newText, float32(sizeNum), fgN, hexGIDs, ""); err != nil {
			dialog.ShowError(fmt.Errorf("创建夹批失败: %w", err), atl.viewer.parentWin)
			return
		}

		atl.viewer.markDirty()
		atl.viewer.forceRenderPlus()
		atl.viewer.renderCurrentPagePlus()
	})
	dlg.Resize(fyne.NewSize(450, 200))
	dlg.Show()
}

func (atl *AnnotToolLayer) eraseClicked(ev *desktop.MouseEvent) {
	atl.annotLayer.calcImageRect()
	if atl.annotLayer.imgW <= 0 || atl.annotLayer.imgH <= 0 {
		return
	}
	pageIdx := atl.PageIdx
	count, err := atl.viewer.pdfDoc.AnnotGetCount(pageIdx)
	if err != nil {
		return
	}
	for i := 0; i < count && i < len(atl.annotLayer.annotations); i++ {
		a := atl.annotLayer.annotations[i]
		l, t1 := atl.annotLayer.pdfToScreen(a.Rect.Left, atl.annotLayer.pageHeight-a.Rect.Top)
		r, b1 := atl.annotLayer.pdfToScreen(a.Rect.Right, atl.annotLayer.pageHeight-a.Rect.Bottom)
		if l > r {
			l, r = r, l
		}
		if t1 > b1 {
			t1, b1 = b1, t1
		}
		if ev.Position.X >= l && ev.Position.X <= r && ev.Position.Y >= t1 && ev.Position.Y <= b1 {
			idx := i
			dialog.ShowConfirm("确认", "删除此标注？", func(ok bool) {
				if ok {
					atl.viewer.pdfDoc.AnnotRemove(pageIdx, idx)
					atl.viewer.markDirty()
					atl.viewer.annotLayer.Refresh()
					atl.viewer.parentWin.Canvas().Refresh(atl.viewer.annotLayer)
				}
			}, atl.viewer.parentWin)
			return
		}
	}
}

func (atl *AnnotToolLayer) moveClicked(ev *desktop.MouseEvent) {
	atl.annotLayer.calcImageRect()
	if atl.annotLayer.imgW <= 0 || atl.annotLayer.imgH <= 0 {
		return
	}
	for i, a := range atl.annotLayer.annotations {
		if a.Type != "FreeText" {
			continue
		}
		l, t1 := atl.annotLayer.pdfToScreen(a.Rect.Left, atl.annotLayer.pageHeight-a.Rect.Top)
		r, b1 := atl.annotLayer.pdfToScreen(a.Rect.Right, atl.annotLayer.pageHeight-a.Rect.Bottom)
		if l > r {
			l, r = r, l
		}
		if t1 > b1 {
			t1, b1 = b1, t1
		}
		if ev.Position.X >= l && ev.Position.X <= r && ev.Position.Y >= t1 && ev.Position.Y <= b1 {
			atl.movingAnnotIdx = i
			atl.dragging = true
			atl.dragStart = ev.Position
			atl.dragEnd = ev.Position
			atl.annotLayer.dragAnnotIdx = i
			atl.annotLayer.dragOffsetX = 0
			atl.annotLayer.dragOffsetY = 0
			return
		}
	}
}

func (atl *AnnotToolLayer) createTextMarkup() {
	subtype := annotToolSubtypes[atl.currentTool]
	pdfX1, pdfY1 := atl.annotLayer.screenToPdf(atl.dragStart.X, atl.dragStart.Y)
	pdfX2, pdfY2 := atl.annotLayer.screenToPdf(atl.dragEnd.X, atl.dragEnd.Y)
	minX, maxX := pdfX1, pdfX2
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := pdfY1, pdfY2
	if minY > maxY {
		minY, maxY = maxY, minY
	}

	pageIdx := atl.PageIdx
	zoom := float32(atl.viewer.zoom)
	hitBoxes := atl.viewer.textLayer.GetHitBoxesForRange(
		float32(minX)*zoom, float32(minY)*zoom, float32(maxX)*zoom, float32(maxY)*zoom,
	)
	if len(hitBoxes) == 0 {
		return
	}

	minTop := float32(100000.0)
	maxBottom := float32(0.0)
	for _, hb := range hitBoxes {
		if hb.ScreenY < minTop {
			minTop = hb.ScreenY
		}
		if hb.ScreenY+hb.ScreenH > maxBottom {
			maxBottom = hb.ScreenY + hb.ScreenH
		}
	}

	running := false
	var lineBoxes []CharHitBox
	for _, hb := range hitBoxes {
		if !running {
			lineBoxes = []CharHitBox{hb}
			running = true
		} else {
			prev := lineBoxes[len(lineBoxes)-1]
			if abs32(hb.ScreenY-prev.ScreenY) < hb.ScreenH {
				lineBoxes = append(lineBoxes, hb)
			} else if abs32(hb.ScreenY-prev.ScreenY) < prev.ScreenH {
				lineBoxes = append(lineBoxes, hb)
			} else {
				atl.createQuadAnnot(subtype, lineBoxes, pageIdx, minTop, maxBottom)
				lineBoxes = []CharHitBox{hb}
			}
		}
	}
	if len(lineBoxes) > 0 {
		atl.createQuadAnnot(subtype, lineBoxes, pageIdx, minTop, maxBottom)
	}
}

func (atl *AnnotToolLayer) createQuadAnnot(subtype enums.FPDF_ANNOTATION_SUBTYPE, boxes []CharHitBox, pageIdx int, minTop, maxBottom float32) {
	if len(boxes) == 0 {
		return
	}

	left := boxes[0].ScreenX
	right := boxes[len(boxes)-1].ScreenX + boxes[len(boxes)-1].ScreenW
	top := minTop
	bottom := maxBottom

	z := float64(atl.viewer.zoom)
	pl := float64(left) / z
	pt := float64(top) / z
	pr := float64(right) / z
	pb := float64(bottom) / z

	l := float32(pl)
	t := float32(atl.annotLayer.pageHeight - pt)
	r := float32(pr)
	b := float32(atl.annotLayer.pageHeight - pb)

	quadPoints := structs.FPDF_FS_QUADPOINTSF{
		X1: l, Y1: t, X2: r, Y2: t,
		X3: l, Y3: b, X4: r, Y4: b,
	}

	settings := atl.viewer.annotSettings[atl.currentTool]
	borderWidth := float32(1)
	if atl.currentTool == AnnotToolHighlight {
		borderWidth = 0
	}

	if err := atl.viewer.pdfDoc.CreateTextMarkupAnnot(pageIdx, subtype,
		l, t, r, b, quadPoints,
		settings.ColorR, settings.ColorG, settings.ColorB, settings.ColorA, borderWidth); err != nil {
		dialog.ShowError(fmt.Errorf("创建标注失败: %w", err), atl.viewer.parentWin)
		return
	}

	atl.viewer.pdfDoc.PageGenerateContent(pageIdx)
	atl.viewer.markDirty()
}

func (atl *AnnotToolLayer) createShapeAnnot() {
	subtype := annotToolSubtypes[atl.currentTool]
	pdfX1, pdfY1 := atl.annotLayer.screenToPdf(atl.dragStart.X, atl.dragStart.Y)
	pdfX2, pdfY2 := atl.annotLayer.screenToPdf(atl.dragEnd.X, atl.dragEnd.Y)
	l := float32(pdfX1)
	t := float32(atl.annotLayer.pageHeight - pdfY1)
	r := float32(pdfX2)
	b := float32(atl.annotLayer.pageHeight - pdfY2)
	if l > r {
		l, r = r, l
	}
	if t > b {
		t, b = b, t
	}
	if r-l < 1 && b-t < 1 {
		return
	}

	pageIdx := atl.PageIdx
	settings := atl.viewer.annotSettings[atl.currentTool]
	if atl.currentTool == AnnotToolFill {
		if err := atl.viewer.pdfDoc.CreateFillAnnot(pageIdx,
			l, b, r, t,
			settings.ColorR, settings.ColorG, settings.ColorB, settings.ColorA); err != nil {
			dialog.ShowError(fmt.Errorf("创建填充标注失败: %w", err), atl.viewer.parentWin)
			return
		}
	} else {
		borderWidth := float32(1)
		if err := atl.viewer.pdfDoc.CreateShapeAnnot(pageIdx, subtype,
			l, b, r, t,
			settings.ColorR, settings.ColorG, settings.ColorB, settings.ColorA, borderWidth); err != nil {
			dialog.ShowError(fmt.Errorf("创建标注失败: %w", err), atl.viewer.parentWin)
			return
		}
	}
	atl.viewer.pdfDoc.PageGenerateContent(pageIdx)
	atl.viewer.markDirty()
}

type annotLayerRenderer struct {
	layer   *AnnotLayer
	objects []fyne.CanvasObject
}

func (r *annotLayerRenderer) Layout(size fyne.Size) {
	if r.layer.viewer == nil || r.layer.viewer.pdfDoc == nil {
		return
	}
	r.layer.Refresh()
}

func (r *annotLayerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(1, 1)
}

func (r *annotLayerRenderer) Refresh() {
	r.objects = nil
	if r.layer.viewer == nil || r.layer.viewer.pdfDoc == nil {
		return
	}
	r.layer.calcImageRect()
	if r.layer.imgW <= 0 || r.layer.imgH <= 0 {
		return
	}
	pageIdx := r.layer.PageIdx
	annots, err := r.layer.viewer.pdfDoc.GetAnnotations(pageIdx)
	if err == nil {
		for i := range annots {
			if annots[i].Type == "FreeText" && annots[i].APContent != "" {
				apr := parseFreeTextAP(annots[i].APContent)
				if apr.text != "" {
					annots[i].Contents = apr.text
				}
				if apr.fontSize > 0 {
					annots[i].DASize = apr.fontSize
				}
				if apr.hasColor {
					annots[i].DAColorR = apr.colorR
					annots[i].DAColorG = apr.colorG
					annots[i].DAColorB = apr.colorB
				}
			}
		}
		r.layer.annotations = annots
	}
	if r.layer.dragAnnotIdx >= 0 && r.layer.dragAnnotIdx < len(r.layer.annotations) {
		a := &r.layer.annotations[r.layer.dragAnnotIdx]
		a.Rect.Left += r.layer.dragOffsetX
		a.Rect.Right += r.layer.dragOffsetX
		a.Rect.Top += r.layer.dragOffsetY
		a.Rect.Bottom += r.layer.dragOffsetY
	}
	hoverIdx := -1
	if r.layer.toolLayer != nil {
		hoverIdx = r.layer.toolLayer.hoverAnnot
	}

	for i, a := range r.layer.annotations {
		if a.Type == "Link" || a.Type == "Widget" || a.Type == "Popup" {
			continue
		}
		l, t1 := r.layer.pdfToScreen(a.Rect.Left, r.layer.pageHeight-a.Rect.Top)
		rr, b1 := r.layer.pdfToScreen(a.Rect.Right, r.layer.pageHeight-a.Rect.Bottom)
		if l > rr {
			l, rr = rr, l
		}
		if t1 > b1 {
			t1, b1 = b1, t1
		}
		w := rr - l
		h := b1 - t1
		if w < 2 {
			w = 2
		}
		if h < 2 {
			h = 2
		}
		isHover := (i == hoverIdx)

		switch a.Type {
		case "Highlight":
			ac := color.NRGBA{R: uint8(a.ColorR), G: uint8(a.ColorG), B: uint8(a.ColorB), A: uint8(a.ColorA)}
			rect := canvas.NewRectangle(ac)
			if isHover {
				rect.StrokeColor = color.NRGBA{R: 255, G: 0, B: 0, A: 200}
				rect.StrokeWidth = 1.5
			}
			rect.Move(fyne.NewPos(l, t1))
			rect.Resize(fyne.NewSize(w, h))
			r.objects = append(r.objects, rect)
		case "Underline":
			ac := color.NRGBA{R: uint8(a.ColorR), G: uint8(a.ColorG), B: uint8(a.ColorB), A: uint8(a.ColorA)}
			line := canvas.NewRectangle(ac)
			ly := t1 + h - 2
			if ly < t1 {
				ly = t1
			}
			line.Move(fyne.NewPos(l, ly))
			line.Resize(fyne.NewSize(w, 2))
			if isHover {
				line.StrokeColor = color.NRGBA{R: 255, G: 0, B: 0, A: 200}
				line.StrokeWidth = 1
			}
			r.objects = append(r.objects, line)
		case "Squiggly":
			ac := color.NRGBA{R: uint8(a.ColorR), G: uint8(a.ColorG), B: uint8(a.ColorB), A: uint8(a.ColorA)}
			lineColor := ac
			lineWidth := float32(1.5)
			if isHover {
				lineColor = color.NRGBA{R: 255, G: 0, B: 0, A: 200}
				lineWidth = 1.5
			}
			bottomY := t1 + h - 2
			if bottomY < t1 {
				bottomY = t1
			}
			segments := int(w / 8)
			if segments < 8 {
				segments = 8
			}
			amp := float32(2.0)
			for i := 0; i < segments; i++ {
				x1 := l + float32(i)*w/float32(segments)
				x2 := l + float32(i+1)*w/float32(segments)
				angle1 := float64(i) * 2.0 * math.Pi / float64(segments) * 3.0
				angle2 := float64(i+1) * 2.0 * math.Pi / float64(segments) * 3.0
				y1 := bottomY + amp*float32(math.Sin(angle1))
				y2 := bottomY + amp*float32(math.Sin(angle2))
				seg := canvas.NewLine(lineColor)
				seg.StrokeWidth = lineWidth
				seg.Position1 = fyne.NewPos(x1, y1)
				seg.Position2 = fyne.NewPos(x2, y2)
				r.objects = append(r.objects, seg)
			}
		case "StrikeOut":
			ac := color.NRGBA{R: uint8(a.ColorR), G: uint8(a.ColorG), B: uint8(a.ColorB), A: uint8(a.ColorA)}
			line := canvas.NewRectangle(ac)
			cy := t1 + h/2
			line.Move(fyne.NewPos(l, cy))
			line.Resize(fyne.NewSize(w, 2))
			if isHover {
				line.StrokeColor = color.NRGBA{R: 0, G: 0, B: 0, A: 200}
				line.StrokeWidth = 1
			}
			r.objects = append(r.objects, line)
		case "Square":
			ac := color.NRGBA{R: uint8(a.ColorR), G: uint8(a.ColorG), B: uint8(a.ColorB), A: uint8(a.ColorA)}
			// 检测是否为填充标注：InteriorColor 不为零（由 CreateFillAnnot 设置）
			if a.InteriorR != 0 || a.InteriorG != 0 || a.InteriorB != 0 {
				fill := color.NRGBA{R: uint8(a.InteriorR), G: uint8(a.InteriorG), B: uint8(a.InteriorB), A: 255}
				rect := canvas.NewRectangle(fill)
				if isHover {
					rect.StrokeColor = color.NRGBA{R: 255, G: 100, B: 0, A: 255}
					rect.StrokeWidth = 2
				}
				rect.Move(fyne.NewPos(l, t1))
				rect.Resize(fyne.NewSize(w, h))
				r.objects = append(r.objects, rect)
			} else {
				rect := canvas.NewRectangle(color.Transparent)
				rect.StrokeColor = ac
				rect.StrokeWidth = 2
				if isHover {
					rect.StrokeColor = color.NRGBA{R: 255, G: 100, B: 0, A: 255}
					rect.StrokeWidth = 3
				}
				rect.Move(fyne.NewPos(l, t1))
				rect.Resize(fyne.NewSize(w, h))
				r.objects = append(r.objects, rect)
			}
		case "FreeText":
			fgR, fgG, fgB := uint8(a.DAColorR), uint8(a.DAColorG), uint8(a.DAColorB)
			if fgR == 0 && fgG == 0 && fgB == 0 {
				fgR, fgG, fgB = uint8(a.ColorR), uint8(a.ColorG), uint8(a.ColorB)
			}
			fgA := uint8(a.ColorA)
			if fgA == 0 {
				fgA = 255
			}

			size := a.DASize
			if size < 8 {
				size = 11
			}
			fontSize := size * r.layer.viewer.zoom
			if fontSize < 8 {
				fontSize = 8
			}
			lines := strings.Split(a.Contents, "\n")
			if len(lines) > 1 {
				// 竖排: each character on its own line
				lineH := fontSize * 1.2
				for i, ch := range lines {
					if ch == "" {
						continue
					}
					chLabel := canvas.NewText(ch, color.NRGBA{R: fgR, G: fgG, B: fgB, A: fgA})
					chLabel.TextSize = fontSize
					chLabel.Alignment = fyne.TextAlignLeading
					chLabel.Move(fyne.NewPos(l+2, t1+2+float32(i)*lineH))
					chLabel.Resize(fyne.NewSize(w-4, lineH))
					r.objects = append(r.objects, chLabel)
				}
			} else {
				label := canvas.NewText(a.Contents, color.NRGBA{R: fgR, G: fgG, B: fgB, A: fgA})
				label.TextSize = fontSize
				label.Alignment = fyne.TextAlignLeading
				label.Move(fyne.NewPos(l+2, t1+2))
				label.Resize(fyne.NewSize(w-4, h-4))
				r.objects = append(r.objects, label)
			}

			if isHover {
				hl := canvas.NewRectangle(color.Transparent)
				hl.StrokeColor = color.NRGBA{R: 255, G: 0, B: 0, A: 200}
				hl.StrokeWidth = 1.5
				hl.Move(fyne.NewPos(l, t1))
				hl.Resize(fyne.NewSize(w, h))
				r.objects = append(r.objects, hl)
			}
		default:
			rect := canvas.NewRectangle(color.Transparent)
			rect.StrokeColor = color.NRGBA{R: 128, G: 128, B: 128, A: 120}
			rect.StrokeWidth = 1
			if isHover {
				rect.StrokeColor = color.NRGBA{R: 255, G: 0, B: 0, A: 200}
				rect.StrokeWidth = 2
			}
			rect.Move(fyne.NewPos(l, t1))
			rect.Resize(fyne.NewSize(w, h))
			r.objects = append(r.objects, rect)
		}
	}
}

func (r *annotLayerRenderer) Destroy() {}

func (r *annotLayerRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

type annotToolLayerRenderer struct {
	layer   *AnnotToolLayer
	objects []fyne.CanvasObject
}

func (r *annotToolLayerRenderer) Layout(size fyne.Size) {}

func (r *annotToolLayerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(1, 1)
}

func (r *annotToolLayerRenderer) Refresh() {
	r.objects = nil
	if r.layer.dragging {
		l := r.layer.dragStart.X
		t := r.layer.dragStart.Y
		rr := r.layer.dragEnd.X
		b := r.layer.dragEnd.Y
		if l > rr {
			l, rr = rr, l
		}
		if t > b {
			t, b = b, t
		}
		w := rr - l
		h := b - t
		if w < 2 {
			w = 2
		}
		if h < 2 {
			h = 2
		}
		rect := canvas.NewRectangle(color.NRGBA{R: 0, G: 120, B: 255, A: 50})
		rect.StrokeColor = color.NRGBA{R: 0, G: 120, B: 255, A: 200}
		rect.StrokeWidth = 1
		rect.Move(fyne.NewPos(l, t))
		rect.Resize(fyne.NewSize(w, h))
		r.objects = append(r.objects, rect)
	}
}

func (r *annotToolLayerRenderer) Destroy() {}

func (r *annotToolLayerRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}
