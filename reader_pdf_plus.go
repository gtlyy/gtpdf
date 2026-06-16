package main

import (
	"container/list"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"sort"
	"sync"

	"gtpdf/pdfium_plus"

	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/structs"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

type PDFiumDoc struct {
	doc         *pdfium_plus.PDFiumDocument
	filePath    string
	numPages    int
	pages       []*PDFiumPage
	mu          sync.RWMutex
	zoomCache   map[zoomCacheKeyPlus]*image.RGBA
	zoomLRU     list.List
	zoomLRUElem map[zoomCacheKeyPlus]*list.Element
	zoomMu      sync.Mutex
	textCache   map[int]*pdfium_plus.PageText
	textCacheMu sync.Mutex
}

type zoomCacheKeyPlus struct {
	page      int
	dpi       int
	nightMode bool
}

type PDFiumPage struct {
	Index    int
	Width    float64
	Height   float64
	Rotation int
	Loaded   bool
}

func LoadPDFiumDocument(path string) (*PDFiumDoc, error) {
	doc, err := pdfium_plus.OpenDocument(path)
	if err != nil {
		return nil, err
	}

	pdfDoc := &PDFiumDoc{
		doc:       doc,
		filePath:  path,
		numPages:  doc.PageCount(),
		zoomCache: make(map[zoomCacheKeyPlus]*image.RGBA),
		zoomLRUElem: make(map[zoomCacheKeyPlus]*list.Element),
		textCache: make(map[int]*pdfium_plus.PageText),
	}

	pdfDoc.pages = make([]*PDFiumPage, pdfDoc.numPages)

	// Pre-load page dimensions via pdfcpu (pure Go, no PDFium, <10ms for all pages)
	// This avoids FPDF_LoadPage for simple GetPagePlus calls (which can take 21s for complex pages)
	f, err := os.Open(path)
	if err == nil {
		boundaries, err := api.Boxes(f, nil, model.NewDefaultConfiguration())
		f.Close()
		if err == nil {
			for i, pb := range boundaries {
				if i >= pdfDoc.numPages {
					break
				}
				mb := pb.CropBox()
				if mb == nil {
					mb = pb.MediaBox()
				}
				if mb != nil {
					pdfDoc.pages[i] = &PDFiumPage{
						Index:    i,
						Width:    mb.Width(),
						Height:   mb.Height(),
						Rotation: pb.Rot,
						Loaded:   true,
					}
				}
			}
		}
	}

	// For any pages not loaded via pdfcpu, fall back to default sizes
	for i := range pdfDoc.pages {
		if pdfDoc.pages[i] == nil {
			pdfDoc.pages[i] = &PDFiumPage{
				Index:  i,
				Width:  612,
				Height: 792,
				Loaded: false,
			}
		}
	}

	return pdfDoc, nil
}

func (d *PDFiumDoc) PageCountPlus() int {
	return d.numPages
}

func (d *PDFiumDoc) InvalidateDimensions(pageIndex int) {
	d.mu.Lock()
	if pageIndex >= 0 && pageIndex < len(d.pages) {
		d.pages[pageIndex] = nil
	}
	d.mu.Unlock()
}

func (d *PDFiumDoc) FilePath() string {
	return d.filePath
}

func (d *PDFiumDoc) GetPagePlus(index int) *PDFiumPage {
	if index < 0 || index >= d.numPages {
		return nil
	}
	d.mu.RLock()
	p := d.pages[index]
	d.mu.RUnlock()
	if p != nil {
		return p
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pages[index] != nil {
		return d.pages[index]
	}

	info, err := d.doc.GetPageInfo(index)
	if err == nil {
		rotation, _ := d.doc.GetPageRotation(index)
		d.pages[index] = &PDFiumPage{
			Index:    index,
			Width:    info.Width,
			Height:   info.Height,
			Rotation: rotation,
			Loaded:   true,
		}
	} else {
		d.pages[index] = &PDFiumPage{
			Index:  index,
			Width:  612,
			Height: 792,
			Loaded: false,
		}
	}
	return d.pages[index]
}

func (d *PDFiumDoc) RenderPagePlus(pageIndex int, zoom float32, canvasScale float32, nightMode bool) (image.Image, error) {
	if pageIndex < 0 || pageIndex >= d.numPages {
		return createWhiteImagePDFiumPlus(612, 792), fmt.Errorf("invalid page index: %d", pageIndex)
	}

	if canvasScale < 1.0 {
		canvasScale = 1.0
	}

	dpi := 72.0 * float64(zoom) * float64(canvasScale)
	roundedDPI := int(math.Round(dpi))

	key := zoomCacheKeyPlus{
		page:      pageIndex,
		dpi:       roundedDPI,
		nightMode: nightMode,
	}

	const cacheLimit = 50

	d.zoomMu.Lock()
	if img, ok := d.zoomCache[key]; ok {
		// Cache HIT: move to front (most recently used)
		if elem, ok := d.zoomLRUElem[key]; ok {
			d.zoomLRU.MoveToFront(elem)
		}
		d.zoomMu.Unlock()
		logD("[cache HIT]  page=%d zoom=%.2f dpi=%.1f rounded=%d nm=%v  (cache size=%d)",
			pageIndex+1, zoom, dpi, roundedDPI, nightMode, len(d.zoomCache))
		return img, nil
	}
	logD("[cache MISS] page=%d zoom=%.2f dpi=%.1f rounded=%d nm=%v  (cache size=%d)",
		pageIndex+1, zoom, dpi, roundedDPI, nightMode, len(d.zoomCache))
	d.zoomMu.Unlock()

	logD("[render start] page=%d zoom=%.2f dpi=%.1f", pageIndex+1, zoom, dpi)
	img, cleanup, err := d.doc.RenderPage(pageIndex, dpi, true)
	if err != nil {
		logE("[render ERR]  page=%d err=%v", pageIndex+1, err)
		return createWhiteImagePDFiumPlus(612, 792), err
	}
	logD("[render done]  page=%d zoom=%.2f dpi=%.1f", pageIndex+1, zoom, dpi)
	defer cleanup()

	var result *image.RGBA
	if nightMode {
		result = invertImageToRGBAPDFiumPlus(img)
	} else {
		result = imageToRGBAPDFiumPlus(img)
	}

	d.zoomMu.Lock()
	// Evict LRU entry if at limit
	if len(d.zoomCache) >= cacheLimit {
		if back := d.zoomLRU.Back(); back != nil {
			evict := back.Value.(zoomCacheKeyPlus)
			delete(d.zoomCache, evict)
			delete(d.zoomLRUElem, evict)
			d.zoomLRU.Remove(back)
			logD("[cache evict] page=%d dpi=%d (limit %d)", evict.page+1, evict.dpi, cacheLimit)
		}
	}
	d.zoomCache[key] = result
	d.zoomLRUElem[key] = d.zoomLRU.PushFront(key)
	logD("[cache store] page=%d dpi=%d -> cache size=%d", pageIndex+1, roundedDPI, len(d.zoomCache))
	d.zoomMu.Unlock()

	return result, nil
}

func (d *PDFiumDoc) RenderPageRaw(pageIndex int, dpi float64, annots bool) (image.Image, func(), error) {
	return d.doc.RenderPage(pageIndex, dpi, annots)
}

func (d *PDFiumDoc) GetPageText(pageIndex int) (*pdfium_plus.PageText, error) {
	d.textCacheMu.Lock()
	if cached, ok := d.textCache[pageIndex]; ok {
		d.textCacheMu.Unlock()
		return cached, nil
	}
	d.textCacheMu.Unlock()

	text, err := d.doc.GetPageText(pageIndex)
	if err != nil {
		return nil, err
	}

	d.textCacheMu.Lock()
	d.textCache[pageIndex] = text
	d.textCacheMu.Unlock()

	return text, nil
}

func (d *PDFiumDoc) SearchPlus(query string) ([]pdfium_plus.SearchResult, error) {
	return d.doc.SearchPages(query)
}

func (d *PDFiumDoc) GetBookmarksPlus() ([]pdfium_plus.BookmarkItem, error) {
	return d.doc.GetBookmarks()
}

func (d *PDFiumDoc) GetLinksPlus(pageIndex int) ([]pdfium_plus.LinkInfo, error) {
	return d.doc.GetLinks(pageIndex)
}

func (d *PDFiumDoc) AnnotCreate(pageIndex int, subtype enums.FPDF_ANNOTATION_SUBTYPE) (*pdfium_plus.AnnotCreateResult, error) {
	return d.doc.AnnotCreate(pageIndex, subtype)
}

func (d *PDFiumDoc) AnnotSetRect(annot references.FPDF_ANNOTATION, left, top, right, bottom float32) error {
	return d.doc.AnnotSetRect(annot, left, top, right, bottom)
}

func (d *PDFiumDoc) AnnotSetColor(annot references.FPDF_ANNOTATION, colorType enums.FPDFANNOT_COLORTYPE, r, g, b, a uint) error {
	return d.doc.AnnotSetColor(annot, colorType, r, g, b, a)
}

func (d *PDFiumDoc) AnnotSetBorder(annot references.FPDF_ANNOTATION, width float32) error {
	return d.doc.AnnotSetBorder(annot, width)
}

func (d *PDFiumDoc) AnnotSetContent(annot references.FPDF_ANNOTATION, content string) error {
	return d.doc.AnnotSetContent(annot, content)
}

func (d *PDFiumDoc) AnnotSetQuadPoints(annot references.FPDF_ANNOTATION, quadPoints structs.FPDF_FS_QUADPOINTSF) error {
	return d.doc.AnnotSetQuadPoints(annot, quadPoints)
}

func (d *PDFiumDoc) CreateTextMarkupAnnot(pageIndex int, subtype enums.FPDF_ANNOTATION_SUBTYPE,
	l, t, r, b float32, quadPoints structs.FPDF_FS_QUADPOINTSF,
	colorR, colorG, colorB, colorA uint, borderWidth float32) error {
	return d.doc.CreateTextMarkupAnnot(pageIndex, subtype, l, t, r, b, quadPoints, colorR, colorG, colorB, colorA, borderWidth)
}

func (d *PDFiumDoc) CreateShapeAnnot(pageIndex int, subtype enums.FPDF_ANNOTATION_SUBTYPE,
	l, t, r, b float32,
	colorR, colorG, colorB, colorA uint, borderWidth float32) error {
	return d.doc.CreateShapeAnnot(pageIndex, subtype, l, t, r, b, colorR, colorG, colorB, colorA, borderWidth)
}

func (d *PDFiumDoc) CreateFillAnnot(pageIndex int,
	l, t, r, b float32,
	colorR, colorG, colorB, colorA uint) error {
	return d.doc.CreateFillAnnot(pageIndex, l, t, r, b, colorR, colorG, colorB, colorA)
}

func (d *PDFiumDoc) AnnotRemoveAll() error {
	return d.doc.AnnotRemoveAll()
}

func (d *PDFiumDoc) AnnotRemove(pageIndex int, annotIndex int) error {
	return d.doc.AnnotRemove(pageIndex, annotIndex)
}

func (d *PDFiumDoc) AnnotGetCount(pageIndex int) (int, error) {
	return d.doc.AnnotGetCount(pageIndex)
}

func (d *PDFiumDoc) GetAnnotations(pageIndex int) ([]pdfium_plus.AnnotationInfo, error) {
	return d.doc.GetAnnotations(pageIndex)
}

func (d *PDFiumDoc) AnnotSetContentByIndex(pageIndex int, annotIndex int, content string) error {
	return d.doc.SetContentByIndex(pageIndex, annotIndex, content)
}

func (d *PDFiumDoc) AnnotSetDictString(annot references.FPDF_ANNOTATION, key, value string) error {
	return d.doc.AnnotSetDictString(annot, key, value)
}

func (d *PDFiumDoc) FontLoad(data []byte, fontType enums.FPDF_FONT, cid bool) (*pdfium_plus.FontLoadResult, error) {
	return d.doc.FontLoad(data, fontType, cid)
}

func (d *PDFiumDoc) FontGetBaseFontName(font references.FPDF_FONT) (string, error) {
	return d.doc.FontGetBaseFontName(font)
}

func (d *PDFiumDoc) PageObjCreateTextObj(font references.FPDF_FONT, fontSize float32) (*pdfium_plus.PageObjCreateTextObjResult, error) {
	return d.doc.PageObjCreateTextObj(font, fontSize)
}

func (d *PDFiumDoc) TextSetText(textObj references.FPDF_PAGEOBJECT, text string) error {
	return d.doc.TextSetText(textObj, text)
}

func (d *PDFiumDoc) PageObjSetMatrix(textObj references.FPDF_PAGEOBJECT, a, b, c, d_, e, f float32) error {
	return d.doc.PageObjSetMatrix(textObj, a, b, c, d_, e, f)
}

func (d *PDFiumDoc) PageInsertObject(pageIndex int, obj references.FPDF_PAGEOBJECT) error {
	return d.doc.PageInsertObject(pageIndex, obj)
}

func (d *PDFiumDoc) PageGenerateContent(pageIndex int) error {
	return d.doc.PageGenerateContent(pageIndex)
}

func (d *PDFiumDoc) AnnotSetAP(annot references.FPDF_ANNOTATION, apContent string) error {
	return d.doc.AnnotSetAP(annot, apContent)
}

func (d *PDFiumDoc) CreateFreeTextAnnot(pageIndex int, fontHandle references.FPDF_FONT, fontName string,
	l, t, r, b float32, text string, fontSize float32, fgColor string, hexGIDs string, apContent string) error {
	return d.doc.CreateFreeTextAnnot(pageIndex, fontHandle, fontName, l, t, r, b, text, fontSize, fgColor, hexGIDs, apContent)
}

func (d *PDFiumDoc) AnnotMoveFreeText(pageIndex, annotIndex int, dx, dy float32) error {
	return d.doc.AnnotMoveFreeText(pageIndex, annotIndex, dx, dy)
}

func (d *PDFiumDoc) ClearZoomCache() {
	d.zoomMu.Lock()
	d.zoomCache = make(map[zoomCacheKeyPlus]*image.RGBA)
	d.zoomLRU.Init()
	d.zoomLRUElem = make(map[zoomCacheKeyPlus]*list.Element)
	d.zoomMu.Unlock()
}

func (d *PDFiumDoc) Close() {
	d.zoomMu.Lock()
	d.zoomCache = make(map[zoomCacheKeyPlus]*image.RGBA)
	d.zoomLRU.Init()
	d.zoomLRUElem = make(map[zoomCacheKeyPlus]*list.Element)
	d.zoomMu.Unlock()
	d.textCacheMu.Lock()
	d.textCache = make(map[int]*pdfium_plus.PageText)
	d.textCacheMu.Unlock()
	d.mu.Lock()
	d.pages = nil
	d.mu.Unlock()
	if d.doc != nil {
		d.doc.Close()
	}
}

func (d *PDFiumDoc) HasRotatedPages() bool {
	for i := 0; i < d.numPages; i++ {
		p := d.GetPagePlus(i)
		if p != nil && p.Rotation != 0 {
			return true
		}
	}
	return false
}

func (d *PDFiumDoc) IsScannedDocument() bool {
	for i := 0; i < d.numPages; i++ {
		text, err := d.CopyPageTextPlus(i)
		if err != nil {
			return false
		}
		if len(text) > 0 {
			return false
		}
	}
	return d.numPages > 0
}

func (d *PDFiumDoc) CopyPageTextPlus(pageIndex int) (string, error) {
	text, err := d.GetPageText(pageIndex)
	if err != nil {
		return "", err
	}
	return text.PlainText, nil
}

type selCharPlus struct {
	text   string
	x, y   float64
	width  float64
	height float64
}

func (d *PDFiumDoc) CopySelectedText(pageIndex int, selStartX, selStartY, selEndX, selEndY float64, zoom float64) string {
	pageText, err := d.GetPageText(pageIndex)
	if err != nil || pageText.PageWidth <= 0 || pageText.PageHeight <= 0 {
		return ""
	}
	return SelectTextFromPage(pageText, selStartX, selStartY, selEndX, selEndY, zoom)
}

func SelectTextFromPage(pageText *pdfium_plus.PageText, selStartX, selStartY, selEndX, selEndY, zoom float64) string {
	if selStartX > selEndX {
		selStartX, selEndX = selEndX, selStartX
	}
	if selStartY > selEndY {
		selStartY, selEndY = selEndY, selStartY
	}

	scale := zoom

	pdfStartX := selStartX / scale
	pdfEndX := selEndX / scale
	pdfStartY := selStartY / scale
	pdfEndY := selEndY / scale

	var selected []selCharPlus

	for _, block := range pageText.Blocks {
		for _, line := range block.Lines {
			for _, ch := range line.Chars {
				charRight := ch.X + ch.Width
				charBottom := ch.Y + ch.Height
				if charRight > pdfStartX && ch.X < pdfEndX &&
					charBottom > pdfStartY && ch.Y < pdfEndY {
					selected = append(selected, selCharPlus{
						text:   ch.Text,
						x:      ch.X,
						y:      ch.Y,
						width:  ch.Width,
						height: ch.Height,
					})
				}
			}
		}
	}

	if len(selected) == 0 {
		var lineSelected []selCharPlus
		for _, block := range pageText.Blocks {
			for _, line := range block.Lines {
				lineBottom := line.Y + line.Height
				if lineBottom > pdfStartY && line.Y < pdfEndY {
					lineSelected = append(lineSelected, selCharPlus{
						text:   line.Text,
						x:      line.X,
						y:      line.Y,
						width:  line.Width,
						height: line.Height,
					})
				}
			}
		}
		if len(lineSelected) > 0 {
			var result string
			for i, s := range lineSelected {
				if i > 0 {
					prevBottom := lineSelected[i-1].y + lineSelected[i-1].height
					if s.y > prevBottom+s.height*0.5 {
						result += "\n"
					} else {
						result += " "
					}
				}
				result += s.text
			}
			return result
		}
		return ""
	}

	SortSelectedChars(selected)

	return FormatSelectedChars(selected)
}

func SortSelectedChars(selected []selCharPlus) {
	lineThreshold := 2.0
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].y+selected[i].height <= selected[j].y-lineThreshold {
			return true
		}
		if selected[j].y+selected[j].height <= selected[i].y-lineThreshold {
			return false
		}
		if selected[i].y < selected[j].y-lineThreshold {
			return true
		}
		if selected[j].y < selected[i].y-lineThreshold {
			return false
		}
		return selected[i].x < selected[j].x
	})
}

func FormatSelectedChars(selected []selCharPlus) string {
	var lines [][]selCharPlus
	var currentLine []selCharPlus
	lineThreshold := 2.0

	for _, s := range selected {
		if len(currentLine) == 0 {
			currentLine = append(currentLine, s)
		} else {
			lastY := currentLine[0].y
			if s.y > lastY+lineThreshold || s.y < lastY-lineThreshold {
				lines = append(lines, currentLine)
				currentLine = []selCharPlus{s}
			} else {
				currentLine = append(currentLine, s)
			}
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	var result string
	for i, line := range lines {
		for j := 1; j < len(line); j++ {
			for k := j; k > 0; k-- {
				if line[k].x < line[k-1].x {
					line[k], line[k-1] = line[k-1], line[k]
				} else {
					break
				}
			}
		}

		for _, ch := range line {
			result += ch.text
		}
		if i < len(lines)-1 {
			result += "\n"
		}
	}

	return result
}

func ExtractTextFromPage(pageText *pdfium_plus.PageText, selMinX, selMinY, selMaxX, selMaxY, zoom float64) string {
	scale := zoom
	pdfMinX := selMinX / scale
	pdfMaxX := selMaxX / scale
	pdfMinY := selMinY / scale
	pdfMaxY := selMaxY / scale

	type lineInfo struct {
		chars []string
		y     float64
	}
	var lines []lineInfo
	var curLine *lineInfo

	for _, block := range pageText.Blocks {
		for _, line := range block.Lines {
			lineChars := []string{}
			for _, ch := range line.Chars {
				charRight := ch.X + ch.Width
				charBottom := ch.Y + ch.Height
				if charRight > pdfMinX && ch.X < pdfMaxX &&
					charBottom > pdfMinY && ch.Y < pdfMaxY {
					lineChars = append(lineChars, ch.Text)
				}
			}
			if len(lineChars) > 0 {
				if curLine == nil || mathAbs(line.Y-curLine.y) > 5 {
					if curLine != nil {
						lines = append(lines, *curLine)
					}
					curLine = &lineInfo{chars: lineChars, y: line.Y}
				} else {
					curLine.chars = append(curLine.chars, lineChars...)
				}
			}
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

func FindCharsInRect(pageText *pdfium_plus.PageText, pdfMinX, pdfMinY, pdfMaxX, pdfMaxY float64) []pdfium_plus.TextChar {
	var matched []pdfium_plus.TextChar
	for _, block := range pageText.Blocks {
		for _, line := range block.Lines {
			for _, ch := range line.Chars {
				charRight := ch.X + ch.Width
				charBottom := ch.Y + ch.Height
				if charRight > pdfMinX && ch.X < pdfMaxX &&
					charBottom > pdfMinY && ch.Y < pdfMaxY {
					matched = append(matched, ch)
				}
			}
		}
	}
	return matched
}

func mathAbs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func createWhiteImagePDFiumPlus(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	c := color.RGBA{255, 255, 255, 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func imageToRGBAPDFiumPlus(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	return dst
}

func invertImageToRGBAPDFiumPlus(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, bv, a := src.At(x, y).RGBA()
			dst.Set(x, y, color.RGBA{
				R: uint8(255 - r>>8),
				G: uint8(255 - g>>8),
				B: uint8(255 - bv>>8),
				A: uint8(a >> 8),
			})
		}
	}

	return dst
}