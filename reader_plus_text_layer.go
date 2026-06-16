package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"gtpdf/pdfium_plus"
)

const maxHighlights = 500

type CharHitBox struct {
	Text    string
	ScreenX float32
	ScreenY float32
	ScreenW float32
	ScreenH float32
	PdfX    float64
	PdfY    float64
	PdfW    float64
	PdfH    float64
}

type TextSelectionLayer struct {
	widget.BaseWidget
	pdfViewer        *PDFViewerPlus
	PageIdx          int  // per-page instance: 0-based page index; global instance: -1
	selRect          *canvas.Rectangle
	hiPool           []*canvas.Rectangle
	hiCount          int
	dragStart        fyne.Position
	dragCurrent      fyne.Position
	isDragging       bool
	selectedText     string
	copyCallback     func(string)
	selectionEnabled bool
	hitBoxes         []CharHitBox
	hitBoxesPage     int
	hitBoxesZoom     float32
	pageWidth        float64
	pageHeight       float64
	imgOffX          float32
	imgOffY          float32
	imgW             float32
	imgH             float32
	ocrMode          bool
	ocrImg           image.Image
	ocrDPI           float64
}

func NewTextSelectionLayer(viewer *PDFViewerPlus) *TextSelectionLayer {
	layer := &TextSelectionLayer{
		pdfViewer:        viewer,
		PageIdx:          -1,
		selectionEnabled: false,
		hiCount:          0,
	}
	layer.ExtendBaseWidget(layer)
	return layer
}

func (t *TextSelectionLayer) getPageIdx() int {
	if t.PageIdx >= 0 {
		return t.PageIdx
	}
	if t.pdfViewer != nil {
		return t.pdfViewer.currentPage - 1
	}
	return 0
}

func (t *TextSelectionLayer) effectiveSelectionEnabled() bool {
	if t.selectionEnabled {
		return true
	}
	if t.pdfViewer != nil && t.pdfViewer.textLayer != nil && t.pdfViewer.textLayer.selectionEnabled {
		return true
	}
	return false
}

func (t *TextSelectionLayer) getCopyCallback() func(string) {
	if t.copyCallback != nil {
		return t.copyCallback
	}
	if t.pdfViewer != nil && t.pdfViewer.textLayer != nil {
		return t.pdfViewer.textLayer.copyCallback
	}
	return nil
}

func (t *TextSelectionLayer) BuildHitBoxes() {
	if t.pdfViewer == nil || t.pdfViewer.pdfDoc == nil {
		t.hitBoxes = nil
		return
	}

	pageIdx := t.getPageIdx()
	pageNum1 := pageIdx + 1
	if pageNum1 < 1 || pageNum1 > t.pdfViewer.totalPages {
		t.hitBoxes = nil
		return
	}

	if t.hitBoxesPage == pageNum1 && t.hitBoxesZoom == t.pdfViewer.zoom && t.hitBoxes != nil {
		return
	}

	page := t.pdfViewer.pdfDoc.GetPagePlus(pageIdx)
	if page == nil {
		t.hitBoxes = nil
		return
	}

	t.pageWidth = page.Width
	t.pageHeight = page.Height

	pageText, err := t.pdfViewer.pdfDoc.GetPageText(pageIdx)
	if err != nil || pageText.PageWidth <= 0 {
		t.hitBoxes = nil
		return
	}

	zoom := float64(t.pdfViewer.zoom)
	var boxes []CharHitBox

	for _, block := range pageText.Blocks {
		for _, line := range block.Lines {
			for _, ch := range line.Chars {
				boxes = append(boxes, CharHitBox{
					Text:    ch.Text,
					ScreenX: float32(ch.X * zoom),
					ScreenY: float32(ch.Y * zoom),
					ScreenW: float32(ch.Width * zoom),
					ScreenH: float32(ch.Height * zoom),
					PdfX:    ch.X,
					PdfY:    ch.Y,
					PdfW:    ch.Width,
					PdfH:    ch.Height,
				})
			}
		}
	}

	t.hitBoxes = boxes
	t.hitBoxesPage = pageNum1
	t.hitBoxesZoom = t.pdfViewer.zoom
}

func (t *TextSelectionLayer) GetHitBoxesForRange(minX, minY, maxX, maxY float32) []CharHitBox {
	t.BuildHitBoxes()
	if t.hitBoxes == nil {
		return nil
	}
	var result []CharHitBox
	for _, hb := range t.hitBoxes {
		cx := hb.ScreenX + hb.ScreenW/2
		cy := hb.ScreenY + hb.ScreenH/2
		if cx >= minX && cx <= maxX && cy >= minY && cy <= maxY {
			result = append(result, hb)
		}
	}
	return result
}

func (t *TextSelectionLayer) calcImageRect() {
	t.imgOffX, t.imgOffY, t.imgW, t.imgH = 0, 0, 0, 0
	if t.pdfViewer == nil || t.pdfViewer.pdfDoc == nil {
		return
	}
	t.imgOffX, t.imgOffY, t.imgW, t.imgH = calcImageRectCommon(t.Size(), t.pageWidth, t.pageHeight, t.pdfViewer.zoom)
}

func rectsOverlap(ax1, ay1, ax2, ay2, bx1, by1, bx2, by2 float32) bool {
	return ax1 < bx2 && ax2 > bx1 && ay1 < by2 && ay2 > by1
}

func (t *TextSelectionLayer) CreateRenderer() fyne.WidgetRenderer {
	t.selRect = canvas.NewRectangle(color.NRGBA{R: 33, G: 150, B: 243, A: 45})
	t.selRect.Hide()

	t.hiPool = make([]*canvas.Rectangle, maxHighlights)
	charColor := color.NRGBA{R: 0, G: 120, B: 215, A: 75}
	for i := 0; i < maxHighlights; i++ {
		t.hiPool[i] = canvas.NewRectangle(charColor)
		t.hiPool[i].Hide()
	}
	t.hiCount = 0

	return &textSelectionRenderer{layer: t}
}

func (t *TextSelectionLayer) MouseIn(ev *desktop.MouseEvent)  {}
func (t *TextSelectionLayer) MouseOut()                       {}

func (t *TextSelectionLayer) MouseDown(ev *desktop.MouseEvent) {
	if t.pdfViewer == nil || t.pdfViewer.pdfDoc == nil || !t.effectiveSelectionEnabled() {
		return
	}
	if t.ocrMode && t.ocrImg == nil {
		return
	}
	t.isDragging = true
	t.dragStart = ev.Position
	t.dragCurrent = ev.Position
	t.selectedText = ""
	t.updateSelection(false)
	canvas.Refresh(t)
}

func (t *TextSelectionLayer) MouseUp(ev *desktop.MouseEvent) {
	if !t.isDragging {
		return
	}
	t.isDragging = false

	dx := math.Abs(float64(t.dragCurrent.X - t.dragStart.X))
	dy := math.Abs(float64(t.dragCurrent.Y - t.dragStart.Y))
	if dx < 3 && dy < 3 {
		t.hideAll()
		t.selectedText = ""
		canvas.Refresh(t)
		return
	}

	t.updateSelection(true)
	canvas.Refresh(t)
}

func (t *TextSelectionLayer) MouseMoved(ev *desktop.MouseEvent) {
	if !t.isDragging {
		return
	}
	if t.pdfViewer == nil || t.pdfViewer.pdfDoc == nil || !t.effectiveSelectionEnabled() {
		return
	}
	if t.ocrMode && t.ocrImg == nil {
		return
	}
	t.dragCurrent = ev.Position
	t.updateSelection(false)
	canvas.Refresh(t)
}

type textSelectionRenderer struct {
	layer   *TextSelectionLayer
	objects []fyne.CanvasObject
}

func (r *textSelectionRenderer) Layout(size fyne.Size) {}

func (r *textSelectionRenderer) MinSize() fyne.Size {
	if r.layer.pdfViewer != nil && r.layer.pdfViewer.pdfDoc != nil {
		page := r.layer.pdfViewer.pdfDoc.GetPagePlus(r.layer.getPageIdx())
		if page != nil {
			zoom := float64(r.layer.pdfViewer.zoom)
			return fyne.NewSize(float32(page.Width*zoom), float32(page.Height*zoom))
		}
	}
	return fyne.NewSize(100, 100)
}

func (r *textSelectionRenderer) Refresh() {
	r.objects = nil
	if r.layer.selRect != nil && !r.layer.selRect.Hidden {
		r.objects = append(r.objects, r.layer.selRect)
	}
	for i := 0; i < r.layer.hiCount; i++ {
		r.objects = append(r.objects, r.layer.hiPool[i])
	}
}

func (r *textSelectionRenderer) Objects() []fyne.CanvasObject {
	r.objects = nil
	if r.layer.selRect != nil && !r.layer.selRect.Hidden {
		r.objects = append(r.objects, r.layer.selRect)
	}
	for i := 0; i < r.layer.hiCount; i++ {
		r.objects = append(r.objects, r.layer.hiPool[i])
	}
	return r.objects
}

func (r *textSelectionRenderer) Destroy() {}

func (t *TextSelectionLayer) hideAll() {
	if t.selRect != nil {
		t.selRect.Hide()
	}
	for i := 0; i < t.hiCount; i++ {
		t.hiPool[i].Hide()
	}
	t.hiCount = 0
	t.Refresh()
}

func (t *TextSelectionLayer) getBounds() (minX, minY, maxX, maxY float32) {
	minX = float32(math.Min(float64(t.dragStart.X), float64(t.dragCurrent.X)))
	minY = float32(math.Min(float64(t.dragStart.Y), float64(t.dragCurrent.Y)))
	maxX = float32(math.Max(float64(t.dragStart.X), float64(t.dragCurrent.X)))
	maxY = float32(math.Max(float64(t.dragStart.Y), float64(t.dragCurrent.Y)))
	return
}

func (t *TextSelectionLayer) matchHitBoxes(selMinX, selMinY, selMaxX, selMaxY float32) []int {
	t.BuildHitBoxes()
	var matched []int
	for i, hb := range t.hitBoxes {
		offX := t.imgOffX
		offY := t.imgOffY
		if rectsOverlap(selMinX, selMinY, selMaxX, selMaxY,
			hb.ScreenX+offX, hb.ScreenY+offY, hb.ScreenX+offX+hb.ScreenW, hb.ScreenY+offY+hb.ScreenH) {
			matched = append(matched, i)
		}
	}
	return matched
}

func (t *TextSelectionLayer) screenToPdf(screenX, screenY float64) (pdfX, pdfY float64) {
	if t.pageWidth <= 0 || t.pageHeight <= 0 || t.imgW <= 0 || t.imgH <= 0 {
		pdfX = screenX / float64(t.pdfViewer.zoom)
		pdfY = screenY / float64(t.pdfViewer.zoom)
		return
	}

	ix := screenX - float64(t.imgOffX)
	iy := screenY - float64(t.imgOffY)

	pdfX = ix / float64(t.imgW) * t.pageWidth
	pdfY = (1.0 - iy/float64(t.imgH)) * t.pageHeight

	return
}

func (t *TextSelectionLayer) updateSelection(doExtract bool) {
	if t.pdfViewer == nil || t.pdfViewer.pdfDoc == nil {
		return
	}

	t.calcImageRect()

	selMinX, selMinY, selMaxX, selMaxY := t.getBounds()
	dx := selMaxX - selMinX
	dy := selMaxY - selMinY

	for i := 0; i < t.hiCount; i++ {
		t.hiPool[i].Hide()
	}
	t.hiCount = 0

	if dx < 2 && dy < 2 {
		if t.selRect != nil {
			t.selRect.Hide()
		}
		t.Refresh()
		return
	}

	margin := float32(4)
	if t.selRect != nil {
		t.selRect.Move(fyne.NewPos(selMinX-margin, selMinY-margin))
		t.selRect.Resize(fyne.NewSize(dx+margin*2, dy+margin*2))
		t.selRect.Show()
	}

	if t.ocrMode {
		t.Refresh()
		if doExtract {
			t.extractTextOCR(selMinX, selMinY, selMaxX, selMaxY)
		}
		return
	}

	matched := t.matchHitBoxes(selMinX, selMinY, selMaxX, selMaxY)

	offX := t.imgOffX
	offY := t.imgOffY
	for _, idx := range matched {
		if t.hiCount < maxHighlights {
			hb := t.hitBoxes[idx]
			rect := t.hiPool[t.hiCount]
			rect.Move(fyne.NewPos(hb.ScreenX+offX, hb.ScreenY+offY))
			rect.Resize(fyne.NewSize(hb.ScreenW, hb.ScreenH))
			rect.Show()
			t.hiCount++
		}
	}

	t.Refresh()

	if doExtract {
		t.extractText(selMinX, selMinY, selMaxX, selMaxY)
	}
}

func (t *TextSelectionLayer) extractText(selMinX, selMinY, selMaxX, selMaxY float32) {
	if t.pageWidth <= 0 || t.pageHeight <= 0 || t.imgW <= 0 || t.imgH <= 0 {
		t.selectedText = ""
		return
	}

	pdfX1, pdfY1 := t.screenToPdf(float64(selMinX), float64(selMinY))
	pdfX2, pdfY2 := t.screenToPdf(float64(selMaxX), float64(selMaxY))

	left := math.Min(pdfX1, pdfX2)
	right := math.Max(pdfX1, pdfX2)
	top := math.Max(pdfY1, pdfY2)
	bottom := math.Min(pdfY1, pdfY2)

	text, err := t.pdfViewer.pdfDoc.doc.GetBoundedText(t.getPageIdx(), left, top, right, bottom)
	if err != nil {
		t.selectedText = ""
		return
	}

	t.selectedText = text

	if t.selectedText != "" {
		if cb := t.getCopyCallback(); cb != nil {
			cb(t.selectedText)
		}
	}
}

func (t *TextSelectionLayer) ClearSelection() {
	t.selectedText = ""
	t.isDragging = false
	t.hideAll()
	t.hitBoxes = nil
	t.hitBoxesPage = 0
	t.hitBoxesZoom = 0
	t.pageWidth = 0
	t.pageHeight = 0
	t.imgOffX = 0
	t.imgOffY = 0
	t.imgW = 0
	t.imgH = 0
	t.ocrMode = false
	t.ocrImg = nil
	t.ocrDPI = 0
}

func (t *TextSelectionLayer) GetSelectedText() string {
	return t.selectedText
}

func (t *TextSelectionLayer) CopyToClipboard() {
	if t.selectedText != "" && t.pdfViewer != nil && t.pdfViewer.parentWin != nil {
		t.pdfViewer.parentWin.Clipboard().SetContent(t.selectedText)
	}
}

func (t *TextSelectionLayer) SetOCRMode(ocrImg image.Image, dpi float64) {
	t.ocrMode = true
	t.ocrImg = ocrImg
	t.ocrDPI = dpi
	t.hitBoxes = nil
	t.hitBoxesPage = 0
	t.hitBoxesZoom = 0
}

func (t *TextSelectionLayer) extractTextOCR(selMinX, selMinY, selMaxX, selMaxY float32) {
	if t.ocrImg == nil {
		t.selectedText = ""
		return
	}

	page := t.pdfViewer.pdfDoc.GetPagePlus(t.getPageIdx())
	if page == nil {
		t.selectedText = ""
		return
	}

	zoom := float64(t.pdfViewer.zoom)
	imgW := float32(page.Width * zoom)
	imgH := float32(page.Height * zoom)

	if imgW <= 0 || imgH <= 0 {
		t.selectedText = ""
		return
	}

	widgetSize := t.Size()
	imgOffX := (widgetSize.Width - imgW) / 2
	imgOffY := (widgetSize.Height - imgH) / 2

	ix1 := float64(selMinX) - float64(imgOffX)
	iy1 := float64(selMinY) - float64(imgOffY)
	ix2 := float64(selMaxX) - float64(imgOffX)
	iy2 := float64(selMaxY) - float64(imgOffY)

	ocrBounds := t.ocrImg.Bounds()
	oxW := float64(ocrBounds.Dx())
	oxH := float64(ocrBounds.Dy())

	ox1 := int(ix1 / float64(imgW) * oxW)
	oy1 := int(iy1 / float64(imgH) * oxH)
	ox2 := int(ix2 / float64(imgW) * oxW)
	oy2 := int(iy2 / float64(imgH) * oxH)

	x := ox1
	y := oy1
	w := ox2 - ox1
	h := oy2 - oy1
	if w < 0 {
		x = ox2
		w = -w
	}
	if h < 0 {
		y = oy2
		h = -h
	}

	text, err := ocrImageRegion(t.ocrImg, x, y, w, h)
	if err != nil || text == "" {
		t.selectedText = ""
		return
	}

	t.selectedText = text

	if t.selectedText != "" {
		if cb := t.getCopyCallback(); cb != nil {
			cb(t.selectedText)
		}
	}
}

func BuildHitBoxesFromPageText(pageText *pdfium_plus.PageText, zoom float64) []CharHitBox {
	if pageText == nil || pageText.PageWidth <= 0 {
		return nil
	}
	var boxes []CharHitBox
	for _, block := range pageText.Blocks {
		for _, line := range block.Lines {
			for _, ch := range line.Chars {
				boxes = append(boxes, CharHitBox{
					Text:    ch.Text,
					ScreenX: float32(ch.X * zoom),
					ScreenY: float32(ch.Y * zoom),
					ScreenW: float32(ch.Width * zoom),
					ScreenH: float32(ch.Height * zoom),
					PdfX:    ch.X,
					PdfY:    ch.Y,
					PdfW:    ch.Width,
					PdfH:    ch.Height,
				})
			}
		}
	}
	return boxes
}

func MatchHitBoxes(boxes []CharHitBox, selMinX, selMinY, selMaxX, selMaxY float32) []int {
	var matched []int
	for i, hb := range boxes {
		if rectsOverlap(selMinX, selMinY, selMaxX, selMaxY,
			hb.ScreenX, hb.ScreenY, hb.ScreenX+hb.ScreenW, hb.ScreenY+hb.ScreenH) {
			matched = append(matched, i)
		}
	}
	return matched
}

func ExtractTextFromHitBoxes(boxes []CharHitBox, matched []int) string {
	if len(matched) == 0 {
		return ""
	}

	type hitEntry struct {
		text  string
		pdfX  float64
		lineY float64
	}

	entries := make([]hitEntry, len(matched))
	for i, idx := range matched {
		hb := boxes[idx]
		entries[i] = hitEntry{
			text:  hb.Text,
			pdfX:  hb.PdfX,
			lineY: hb.PdfY,
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].lineY < entries[j].lineY-2 {
			return true
		}
		if entries[j].lineY < entries[i].lineY-2 {
			return false
		}
		return entries[i].pdfX < entries[j].pdfX
	})

	type textLine struct {
		chars []string
		y     float64
	}
	var lines []textLine
	var curLine *textLine

	for _, e := range entries {
		if curLine == nil || math.Abs(e.lineY-curLine.y) > 2 {
			if curLine != nil {
				lines = append(lines, *curLine)
			}
			curLine = &textLine{chars: []string{e.text}, y: e.lineY}
		} else {
			curLine.chars = append(curLine.chars, e.text)
		}
	}
	if curLine != nil {
		lines = append(lines, *curLine)
	}

	result := ""
	for i, line := range lines {
		for _, ch := range line.chars {
			result += ch
		}
		if i < len(lines)-1 {
			result += "\n"
		}
	}
	return result
}

type SearchResultHighlightPlus struct {
	widget.BaseWidget
	matches []MatchPositionPlus
}

type MatchPositionPlus struct {
	X, Y, Width, Height float32
}

func NewSearchResultHighlightPlus() *SearchResultHighlightPlus {
	h := &SearchResultHighlightPlus{}
	h.ExtendBaseWidget(h)
	return h
}

func (h *SearchResultHighlightPlus) CreateRenderer() fyne.WidgetRenderer {
	return &searchHighlightRendererPlus{h: h, objects: []fyne.CanvasObject{}}
}

type searchHighlightRendererPlus struct {
	h       *SearchResultHighlightPlus
	objects []fyne.CanvasObject
}

func (r *searchHighlightRendererPlus) Layout(size fyne.Size) {}
func (r *searchHighlightRendererPlus) MinSize() fyne.Size {
	return fyne.NewSize(1, 1)
}
func (r *searchHighlightRendererPlus) Destroy() {}

func (r *searchHighlightRendererPlus) Refresh() {
	r.objects = nil
	for _, pos := range r.h.matches {
		rect := canvas.NewRectangle(color.RGBA{R: 255, G: 255, B: 0, A: 120})
		rect.Move(fyne.NewPos(pos.X, pos.Y))
		rect.Resize(fyne.NewSize(pos.Width, pos.Height))
		r.objects = append(r.objects, rect)
	}
}

func (r *searchHighlightRendererPlus) Objects() []fyne.CanvasObject { return r.objects }

func (h *SearchResultHighlightPlus) SetMatches(matches []MatchPositionPlus) {
	h.matches = matches
	h.Refresh()
}

var _ fmt.Stringer = (*CharHitBox)(nil)

func (c *CharHitBox) String() string {
	return fmt.Sprintf("Char{%s x=%.1f y=%.1f w=%.1f h=%.1f}", c.Text, c.PdfX, c.PdfY, c.PdfW, c.PdfH)
}