package pdfium_plus

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
)

type PDFiumDocument struct {
	instance pdfium.Pdfium
	doc      *responses.OpenDocument
	filePath string
	numPages int
	mu       sync.Mutex

	textPages map[int]references.FPDF_TEXTPAGE
	pages     map[int]references.FPDF_PAGE
	fontCache map[string]*FontLoadResult
}

type PageInfo struct {
	Index  int
	Width  float64
	Height float64
}

type TextChar struct {
	Text     string
	X        float64
	Y        float64
	Width    float64
	Height   float64
	FontSize float64
	FontName string
}

type TextLine struct {
	Chars  []TextChar
	Text   string
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type TextBlock struct {
	Lines  []TextLine
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type PageText struct {
	Page      int
	Blocks    []TextBlock
	PlainText string
	PageWidth float64
	PageHeight float64
}

type SearchResult struct {
	Page       int
	Text       string
	Context    string
	Position   int
	MatchIndex int
}

type BookmarkItem struct {
	Title    string
	Page     int
	Children []BookmarkItem
}

type LinkInfo struct {
	Page int
	URI  string
	Rect LinkRect
}

type LinkRect struct {
	Left, Top, Right, Bottom float64
}

type AnnotationInfo struct {
	Page      int
	Type      string
	Contents  string
	Title     string
	Rect      LinkRect
	DASize    float32
	InteriorR uint
	InteriorG uint
	InteriorB uint
	InteriorA uint
	ColorR    uint
	ColorG    uint
	ColorB    uint
	ColorA    uint
	DAColorR  uint
	DAColorG  uint
	DAColorB  uint
	FillOpacity float32

	APContent string
}

func OpenDocument(path string) (*PDFiumDocument, error) {
	inst := GetInstance()
	if inst == nil {
		return nil, fmt.Errorf("pdfium instance not available")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return openDocumentFromData(inst, data, path)
}

// OpenDocumentFromBuffer opens a PDF from an in-memory byte buffer.
// The path parameter is optional (can be empty) and used only for reference.
func OpenDocumentFromBuffer(data []byte) (*PDFiumDocument, error) {
	inst := GetInstance()
	if inst == nil {
		return nil, fmt.Errorf("pdfium instance not available")
	}
	return openDocumentFromData(inst, data, "")
}

func openDocumentFromData(inst pdfium.Pdfium, data []byte, filePath string) (*PDFiumDocument, error) {
	doc, err := inst.OpenDocument(&requests.OpenDocument{
		File: &data,
	})
	if err != nil {
		return nil, err
	}

	pageCount, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
			Document: doc.Document,
		})
		return nil, err
	}

	return &PDFiumDocument{
		instance:       inst,
		doc:            doc,
		filePath:       filePath,
		numPages:       pageCount.PageCount,
		textPages: make(map[int]references.FPDF_TEXTPAGE),
		pages:     make(map[int]references.FPDF_PAGE),
		fontCache:      make(map[string]*FontLoadResult),
	}, nil
}

func (d *PDFiumDocument) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, tp := range d.textPages {
		d.instance.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: tp})
	}
	d.textPages = make(map[int]references.FPDF_TEXTPAGE)

	for _, pg := range d.pages {
		d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: pg})
	}
	d.pages = make(map[int]references.FPDF_PAGE)
	d.fontCache = make(map[string]*FontLoadResult)

	if d.instance != nil && d.doc != nil {
		d.instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
			Document: d.doc.Document,
		})
	}
}

func (d *PDFiumDocument) PageCount() int {
	return d.numPages
}

func (d *PDFiumDocument) FilePath() string {
	return d.filePath
}

func (d *PDFiumDocument) GetPageInfo(pageIndex int) (*PageInfo, error) {
	if pageIndex < 0 || pageIndex >= d.numPages {
		return nil, fmt.Errorf("invalid page index: %d", pageIndex)
	}

	size, err := d.instance.GetPageSizeInPixels(&requests.GetPageSizeInPixels{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageIndex,
			},
		},
		DPI: 72,
	})
	if err != nil {
		return nil, err
	}

	return &PageInfo{
		Index:  pageIndex,
		Width:  float64(size.Width),
		Height: float64(size.Height),
	}, nil
}

func (d *PDFiumDocument) GetPageRotation(pageIndex int) (int, error) {
	if pageIndex < 0 || pageIndex >= d.numPages {
		return 0, fmt.Errorf("invalid page index: %d", pageIndex)
	}

	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return 0, err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	resp, err := d.instance.FPDFPage_GetRotation(&requests.FPDFPage_GetRotation{
		Page: requests.Page{
			ByReference: &page.Page,
		},
	})
	if err != nil {
		return 0, err
	}

	return int(resp.PageRotation), nil
}

func (d *PDFiumDocument) RenderPage(pageIndex int, dpi float64, annots bool) (image.Image, func(), error) {
	if pageIndex < 0 || pageIndex >= d.numPages {
		return nil, nil, fmt.Errorf("invalid page index: %d", pageIndex)
	}

	renderFlags := enums.FPDF_RENDER_FLAG(0)
	result, err := d.instance.RenderPageInDPI(&requests.RenderPageInDPI{
		DPI:         int(dpi),
		RenderFlags: renderFlags,
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageIndex,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	var img image.Image = result.Result.Image

	if annots {
		img = d.renderSealOnPage(img, pageIndex, dpi)
	}

	cleanup := func() {
		if result.CleanupFunc != nil {
			result.CleanupFunc()
		}
	}

	return img, cleanup, nil
}

// renderSealOnPage renders Widget (seal) annotations from a temp document
// and composites them onto the page image. This avoids modifying the original
// document's annotation state with FPDF_FFLDraw.
func (d *PDFiumDocument) renderSealOnPage(img image.Image, pageIndex int, dpi float64) image.Image {
	// Check if page has Widget annotations
	annots, err := d.GetAnnotations(pageIndex)
	if err != nil {
		return img
	}

	var sealRect *LinkRect
	for _, a := range annots {
		if a.Type == "Widget" {
			sealRect = &a.Rect
			break
		}
	}
	if sealRect == nil {
		return img
	}

	buf, err := d.SaveAsCopyToBuffer()
	if err != nil { return img }
	tmpDoc, err := OpenDocumentFromBuffer(buf)
	if err != nil { return img }
	defer tmpDoc.Close()

	formImg, err := tmpDoc.renderFormFields(pageIndex, dpi)
	if err != nil { return img }

	bounds := img.Bounds()
	dst, ok := img.(draw.Image)
	if !ok { return img }

	scale := dpi / 72.0
	for y := int(float64(bounds.Dy())-sealRect.Top*scale); y < int(float64(bounds.Dy())-sealRect.Bottom*scale) && y < bounds.Dy(); y++ {
		if y < 0 { continue }
		for x := int(sealRect.Left*scale); x < int(sealRect.Right*scale) && x < bounds.Dx(); x++ {
			if x < 0 { continue }
			r1, g1, b1, _ := formImg.At(x, y).RGBA()
			r2, g2, b2, _ := img.At(x, y).RGBA()
			diff := diff8(r1>>8, r2>>8) + diff8(g1>>8, g2>>8) + diff8(b1>>8, b2>>8)
			if diff > 15 {
				dst.Set(x, y, formImg.At(x, y))
			}
		}
	}

	return img
}

func diff8(a, b uint32) uint32 {
	if a > b { return a - b }
	return b - a
}

func (d *PDFiumDocument) renderFormFields(pageIndex int, dpi float64) (image.Image, error) {
	resp, err := d.instance.RenderPageInDPI(&requests.RenderPageInDPI{
		DPI:         int(dpi),
		RenderFlags: 0,
		RenderForm:  true,
		Document:    &d.doc.Document,
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageIndex,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if resp.CleanupFunc != nil {
			resp.CleanupFunc()
		}
	}()
	return resp.Result.Image, nil
}

// RenderPageWithAllAnnots renders the page with both FPDF_RENDER_FLAG_ANNOT
// and RenderForm, used for export to include all annotations.
func (d *PDFiumDocument) RenderPageWithAllAnnots(pageIndex int, dpi float64) (*responses.RenderPageInDPI, error) {
	resp, err := d.instance.RenderPageInDPI(&requests.RenderPageInDPI{
		DPI:         int(dpi),
		RenderFlags: enums.FPDF_RENDER_FLAG_ANNOT,
		RenderForm:  true,
		Document:    &d.doc.Document,
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageIndex,
			},
		},
	})
	return resp, err
}

func (d *PDFiumDocument) GetPageText(pageIndex int) (*PageText, error) {
	if pageIndex < 0 || pageIndex >= d.numPages {
		return nil, fmt.Errorf("invalid page index: %d", pageIndex)
	}

	structured, err := d.instance.GetPageTextStructured(&requests.GetPageTextStructured{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageIndex,
			},
		},
		Mode:                   requests.GetPageTextStructuredModeBoth,
		CollectFontInformation: true,
	})
	if err != nil {
		return nil, err
	}

	plainText, _ := d.instance.GetPageText(&requests.GetPageText{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageIndex,
			},
		},
	})

	pageText := &PageText{
		Page:      pageIndex,
		PlainText: plainText.Text,
	}

	size, _ := d.instance.GetPageSizeInPixels(&requests.GetPageSizeInPixels{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageIndex,
			},
		},
		DPI: 72,
	})
	if size != nil {
		pageText.PageWidth = float64(size.Width)
		pageText.PageHeight = float64(size.Height)
	}

	var currentBlock TextBlock
	var currentLine TextLine
	var lastY float64
	first := true

	for _, ch := range structured.Chars {
		pos := ch.PointPosition
		// PDF coordinates: origin at bottom-left, Y increases upward
		// Convert to screen coordinates: origin at top-left, Y increases downward
		yPos := pageText.PageHeight - pos.Top
		charText := TextChar{
			Text:   ch.Text,
			X:      pos.Left,
			Y:      yPos,
			Width:  math.Abs(pos.Right - pos.Left),
			Height: math.Abs(pos.Bottom - pos.Top),
		}

		if ch.FontInformation != nil {
			charText.FontSize = ch.FontInformation.Size
			charText.FontName = ch.FontInformation.Name
		}

		isNewLine := false
		if !first {
			if yPos > lastY+charText.Height*0.5 {
				isNewLine = true
			}
		}

		if isNewLine {
			if len(currentLine.Chars) > 0 {
				currentLine.Text = buildLineText(currentLine.Chars)
				currentBlock.Lines = append(currentBlock.Lines, currentLine)
			}
			currentLine = TextLine{Chars: []TextChar{charText}, X: charText.X, Y: charText.Y}
		} else {
			currentLine.Chars = append(currentLine.Chars, charText)
			if charText.X < currentLine.X {
				currentLine.X = charText.X
			}
		}

		currentLine.Width = currentLine.Chars[len(currentLine.Chars)-1].X + currentLine.Chars[len(currentLine.Chars)-1].Width - currentLine.X
		if charText.Height > currentLine.Height {
			currentLine.Height = charText.Height
		}

		lastY = yPos
		first = false
	}

	if len(currentLine.Chars) > 0 {
		currentLine.Text = buildLineText(currentLine.Chars)
		currentBlock.Lines = append(currentBlock.Lines, currentLine)
	}

	if len(currentBlock.Lines) > 0 {
		pageText.Blocks = append(pageText.Blocks, currentBlock)
	}

	_ = structured.Rects

	return pageText, nil
}

func buildLineText(chars []TextChar) string {
	var sb strings.Builder
	for _, c := range chars {
		sb.WriteString(c.Text)
	}
	return sb.String()
}

func (d *PDFiumDocument) SearchPages(query string) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	var results []SearchResult

	punctuations := []rune{'，', '。', '；', '：', '！', '？', '）', '》', ',', '.', ';', ':', '!', '?', ')', '>', '"', '\'', '\n', '\r'}

	for i := 0; i < d.numPages; i++ {
		pageText, err := d.GetPageText(i)
		if err != nil {
			continue
		}

		if pageText.PlainText == "" {
			continue
		}

		text := pageText.PlainText
		queryLower := strings.ToLower(query)
		textLower := strings.ToLower(text)
		queryLen := len(query)

		if queryLen == 0 || len(textLower) < queryLen {
			continue
		}

		matchIdx := 0
		maxMatches := 100

		searchFrom := 0
		for {
			pos := strings.Index(textLower[searchFrom:], queryLower)
			if pos < 0 {
				break
			}

			actualPos := searchFrom + pos
			keywordStart := actualPos
			keywordEnd := actualPos + queryLen
			searchFrom = keywordEnd

			if matchIdx >= maxMatches {
				break
			}

			if keywordStart < 0 || keywordEnd > len(text) {
				continue
			}

			beforeStart := keywordStart - 50
			if beforeStart < 0 {
				beforeStart = 0
			}
			beforeText := text[beforeStart:keywordStart]
			beforeRunes := []rune(beforeText)
			before := ""
			for idx := len(beforeRunes) - 1; idx >= 0; idx-- {
				for _, p := range punctuations {
					if beforeRunes[idx] == p {
						before = string(beforeRunes[idx+1:])
						break
					}
				}
				if len(before) > 0 {
					break
				}
			}
			if len(before) == 0 {
				before = beforeText
			}

			afterStart := keywordEnd
			afterEnd := keywordEnd + 150
			if afterEnd > len(text) {
				afterEnd = len(text)
			}
			after := text[afterStart:afterEnd]
			after = cleanTextForSearchPlus(after)

			afterRunes := []rune(after)
			endIdx := len(afterRunes)
			if endIdx > 50 {
				endIdx = 50
			}
			foundPunc := false
			for idx, r := range afterRunes[:endIdx] {
				for _, p := range punctuations {
					if r == p {
						endIdx = idx + 1
						foundPunc = true
						break
					}
				}
				if foundPunc {
					break
				}
			}
			if !foundPunc && len(afterRunes) > 50 {
				for idx := 50; idx < len(afterRunes); idx++ {
					for _, p := range punctuations {
						if afterRunes[idx] == p {
							endIdx = idx + 1
							foundPunc = true
							break
						}
					}
					if foundPunc {
						break
					}
				}
			}
			after = string(afterRunes[:endIdx])

			before = cleanTextForSearchPlus(before)
			keyword := text[keywordStart:keywordEnd]
			context := before + keyword + after

			results = append(results, SearchResult{
				Page:     i,
				Text:     query,
				Context:  context,
				Position: actualPos,
				MatchIndex: matchIdx,
			})
			matchIdx++

			if len(results) > 500 {
				break
			}
		}
	}

	return results, nil
}

func cleanTextForSearchPlus(s string) string {
	result := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			result = append(result, ' ')
		} else if r >= 32 {
			result = append(result, r)
		}
	}
	return string(result)
}

func (d *PDFiumDocument) GetBookmarks() ([]BookmarkItem, error) {
	bookmarks, err := d.instance.GetBookmarks(&requests.GetBookmarks{
		Document: d.doc.Document,
	})
	if err != nil {
		return nil, err
	}

	return convertBookmarks(bookmarks.Bookmarks, d.instance), nil
}

func (d *PDFiumDocument) GetLinks(pageIndex int) ([]LinkInfo, error) {
	if pageIndex < 0 || pageIndex >= d.numPages {
		return nil, fmt.Errorf("invalid page index: %d", pageIndex)
	}

	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return nil, err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{
		Page: page.Page,
	})

	pageHeightResp, err := d.instance.FPDF_GetPageHeight(&requests.FPDF_GetPageHeight{
		Page: requests.Page{
			ByReference: &page.Page,
		},
	})
	if err != nil {
		return nil, err
	}
	height := pageHeightResp.Height

	var links []LinkInfo

	startPos := 0
	for {
		linkEnum, err := d.instance.FPDFLink_Enumerate(&requests.FPDFLink_Enumerate{
			Page: requests.Page{
				ByReference: &page.Page,
			},
			StartPos: startPos,
		})
		if err != nil || linkEnum.Link == nil {
			break
		}

		startPos = *linkEnum.NextStartPos

		linkInfo := LinkInfo{
			Page: pageIndex,
		}

		rectResp, _ := d.instance.FPDFLink_GetAnnotRect(&requests.FPDFLink_GetAnnotRect{
			Link: *linkEnum.Link,
		})
		if rectResp.Rect != nil {
			linkInfo.Rect = LinkRect{
				Left:   float64(rectResp.Rect.Left),
				Top:    height - float64(rectResp.Rect.Top),
				Right:  float64(rectResp.Rect.Right),
				Bottom: height - float64(rectResp.Rect.Bottom),
			}
		}

		destResp, _ := d.instance.FPDFLink_GetDest(&requests.FPDFLink_GetDest{
			Document: d.doc.Document,
			Link:     *linkEnum.Link,
		})
		if destResp.Dest != nil {
			destPageResp, _ := d.instance.FPDFDest_GetDestPageIndex(&requests.FPDFDest_GetDestPageIndex{
				Document: d.doc.Document,
				Dest:     *destResp.Dest,
			})
			linkInfo.Page = destPageResp.Index
		} else {
			actionResp, _ := d.instance.FPDFLink_GetAction(&requests.FPDFLink_GetAction{
				Link: *linkEnum.Link,
			})
			if actionResp.Action != nil {
				uriResp, _ := d.instance.FPDFAction_GetURIPath(&requests.FPDFAction_GetURIPath{
					Document: d.doc.Document,
					Action:   *actionResp.Action,
				})
				if uriResp.URIPath != nil {
					linkInfo.URI = *uriResp.URIPath
				}
			}
		}

		links = append(links, linkInfo)
	}

	return links, nil
}

func (d *PDFiumDocument) GetAnnotations(pageIndex int) ([]AnnotationInfo, error) {
	if pageIndex < 0 || pageIndex >= d.numPages {
		return nil, fmt.Errorf("invalid page index: %d", pageIndex)
	}

	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return nil, err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{
		Page: page.Page,
	})

	annotCount, err := d.instance.FPDFPage_GetAnnotCount(&requests.FPDFPage_GetAnnotCount{
		Page: requests.Page{
			ByReference: &page.Page,
		},
	})
	if err != nil {
		return nil, err
	}

	var annotations []AnnotationInfo

	for i := 0; i < annotCount.Count; i++ {
		annot, err := d.instance.FPDFPage_GetAnnot(&requests.FPDFPage_GetAnnot{
			Page: requests.Page{
				ByReference: &page.Page,
			},
			Index: i,
		})
		if err != nil {
			continue
		}

		annotInfo := AnnotationInfo{
			Page: pageIndex,
		}

		annotType, _ := d.instance.FPDFAnnot_GetSubtype(&requests.FPDFAnnot_GetSubtype{
			Annotation: annot.Annotation,
		})
		annotInfo.Type = annotSubtypeToString(annotType.Subtype)

		contents, _ := d.instance.FPDFAnnot_GetStringValue(&requests.FPDFAnnot_GetStringValue{
			Annotation: annot.Annotation,
			Key:        "Contents",
		})
		annotInfo.Contents = contents.Value

		title, _ := d.instance.FPDFAnnot_GetStringValue(&requests.FPDFAnnot_GetStringValue{
			Annotation: annot.Annotation,
			Key:        "Title",
		})
		annotInfo.Title = title.Value

		da, _ := d.instance.FPDFAnnot_GetStringValue(&requests.FPDFAnnot_GetStringValue{
			Annotation: annot.Annotation,
			Key:        "DA",
		})
		annotInfo.DASize = parseDASize(da.Value)
		annotInfo.DAColorR, annotInfo.DAColorG, annotInfo.DAColorB = parseDAColor(da.Value)

		if annotInfo.Type == "FreeText" {
			apResp, apErr := d.instance.FPDFAnnot_GetAP(&requests.FPDFAnnot_GetAP{
				Annotation:     annot.Annotation,
				AppearanceMode: enums.FPDF_ANNOT_APPEARANCEMODE_NORMAL,
			})
			if apErr == nil {
				annotInfo.APContent = apResp.Value
			}
		}

		rect, _ := d.instance.FPDFAnnot_GetRect(&requests.FPDFAnnot_GetRect{
			Annotation: annot.Annotation,
		})
		annotInfo.Rect = LinkRect{
			Left:   float64(rect.Rect.Left),
			Top:    float64(rect.Rect.Top),
			Right:  float64(rect.Rect.Right),
			Bottom: float64(rect.Rect.Bottom),
		}

		if interior, err := d.instance.FPDFAnnot_GetColor(&requests.FPDFAnnot_GetColor{
			Annotation: annot.Annotation,
			ColorType:  enums.FPDFANNOT_COLORTYPE_InteriorColor,
		}); err == nil {
			annotInfo.InteriorR = interior.R
			annotInfo.InteriorG = interior.G
			annotInfo.InteriorB = interior.B
			annotInfo.InteriorA = interior.A
		}

		if stroke, err := d.instance.FPDFAnnot_GetColor(&requests.FPDFAnnot_GetColor{
			Annotation: annot.Annotation,
			ColorType:  enums.FPDFANNOT_COLORTYPE_Color,
		}); err == nil {
			annotInfo.ColorR = stroke.R
			annotInfo.ColorG = stroke.G
			annotInfo.ColorB = stroke.B
			annotInfo.ColorA = stroke.A
		}

		if opacity, err := d.instance.FPDFAnnot_GetStringValue(&requests.FPDFAnnot_GetStringValue{
			Annotation: annot.Annotation,
			Key:        "gtpdf_opacity",
		}); err == nil && opacity.Value != "" {
			if val, parseErr := strconv.ParseFloat(opacity.Value, 32); parseErr == nil {
				annotInfo.FillOpacity = float32(val)
			} else {
				annotInfo.FillOpacity = 1.0
			}
		} else {
			annotInfo.FillOpacity = 1.0
		}

		annotations = append(annotations, annotInfo)
	}

	return annotations, nil
}

func (d *PDFiumDocument) GetPageDimensionsInPixels(pageIndex int, dpi float64) (int, int, error) {
	if pageIndex < 0 || pageIndex >= d.numPages {
		return 0, 0, fmt.Errorf("invalid page index: %d", pageIndex)
	}

	size, err := d.instance.GetPageSizeInPixels(&requests.GetPageSizeInPixels{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.doc.Document,
				Index:    pageIndex,
			},
		},
		DPI: int(dpi),
	})
	if err != nil {
		return 0, 0, err
	}

	return size.Width, size.Height, nil
}

func parseDASize(da string) float32 {
	if da == "" {
		return 0
	}
	parts := strings.Fields(da)
	for i, p := range parts {
		if p == "Tf" && i > 0 {
			if val, err := strconv.ParseFloat(parts[i-1], 32); err == nil {
				return float32(val)
			}
		}
	}
	return 0
}

func parseDAColor(da string) (r, g, b uint) {
	if da == "" {
		return 0, 0, 0
	}
	parts := strings.Fields(da)
	for i, p := range parts {
		if p == "rg" && i >= 3 {
			if rv, err := strconv.ParseFloat(parts[i-3], 32); err == nil {
				if gv, err := strconv.ParseFloat(parts[i-2], 32); err == nil {
					if bv, err := strconv.ParseFloat(parts[i-1], 32); err == nil {
						return uint(rv * 255), uint(gv * 255), uint(bv * 255)
					}
				}
			}
		}
		if p == "g" && i >= 1 {
			if gv, err := strconv.ParseFloat(parts[i-1], 32); err == nil {
				val := uint(gv * 255)
				return val, val, val
			}
		}
	}
	return 0, 0, 0
}

func annotSubtypeToString(subtype enums.FPDF_ANNOTATION_SUBTYPE) string {
	switch subtype {
	case enums.FPDF_ANNOT_SUBTYPE_TEXT:
		return "Text"
	case enums.FPDF_ANNOT_SUBTYPE_LINK:
		return "Link"
	case enums.FPDF_ANNOT_SUBTYPE_FREETEXT:
		return "FreeText"
	case enums.FPDF_ANNOT_SUBTYPE_LINE:
		return "Line"
	case enums.FPDF_ANNOT_SUBTYPE_SQUARE:
		return "Square"
	case enums.FPDF_ANNOT_SUBTYPE_CIRCLE:
		return "Circle"
	case enums.FPDF_ANNOT_SUBTYPE_POLYGON:
		return "Polygon"
	case enums.FPDF_ANNOT_SUBTYPE_POLYLINE:
		return "PolyLine"
	case enums.FPDF_ANNOT_SUBTYPE_HIGHLIGHT:
		return "Highlight"
	case enums.FPDF_ANNOT_SUBTYPE_UNDERLINE:
		return "Underline"
	case enums.FPDF_ANNOT_SUBTYPE_SQUIGGLY:
		return "Squiggly"
	case enums.FPDF_ANNOT_SUBTYPE_STRIKEOUT:
		return "StrikeOut"
	case enums.FPDF_ANNOT_SUBTYPE_STAMP:
		return "Stamp"
	case enums.FPDF_ANNOT_SUBTYPE_CARET:
		return "Caret"
	case enums.FPDF_ANNOT_SUBTYPE_INK:
		return "Ink"
	case enums.FPDF_ANNOT_SUBTYPE_POPUP:
		return "Popup"
	case enums.FPDF_ANNOT_SUBTYPE_FILEATTACHMENT:
		return "FileAttachment"
	case enums.FPDF_ANNOT_SUBTYPE_WIDGET:
		return "Widget"
	default:
		return "Unknown"
	}
}

func convertBookmarks(bookmarks []responses.GetBookmarksBookmark, inst pdfium.Pdfium) []BookmarkItem {
	var result []BookmarkItem
	for _, bm := range bookmarks {
		item := BookmarkItem{
			Title: bm.Title,
		}

		if bm.DestInfo != nil {
			item.Page = bm.DestInfo.PageIndex
		} else if bm.ActionInfo != nil && bm.ActionInfo.DestInfo != nil {
			item.Page = bm.ActionInfo.DestInfo.PageIndex
		}

		if len(bm.Children) > 0 {
			item.Children = convertBookmarks(bm.Children, inst)
		}
		result = append(result, item)
	}
	return result
}

func (d *PDFiumDocument) LoadTextPage(pageIndex int) (references.FPDF_TEXTPAGE, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if tp, ok := d.textPages[pageIndex]; ok {
		return tp, nil
	}

	page, err := d.loadPage(pageIndex)
	if err != nil {
		return "", err
	}

	tpResp, err := d.instance.FPDFText_LoadPage(&requests.FPDFText_LoadPage{
		Page: requests.Page{
			ByReference: &page,
		},
	})
	if err != nil {
		return "", err
	}

	d.textPages[pageIndex] = tpResp.TextPage
	return tpResp.TextPage, nil
}

func (d *PDFiumDocument) GetBoundedText(pageIndex int, left, top, right, bottom float64) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tp, ok := d.textPages[pageIndex]
	if !ok {
		var err error
		tp, err = d.loadTextPageUnlocked(pageIndex)
		if err != nil {
			return "", err
		}
	}

	textResp, err := d.instance.FPDFText_GetBoundedText(&requests.FPDFText_GetBoundedText{
		TextPage: tp,
		Left:     left,
		Top:      top,
		Right:    right,
		Bottom:   bottom,
	})
	if err != nil {
		return "", err
	}
	return textResp.Text, nil
}

func (d *PDFiumDocument) loadTextPageUnlocked(pageIndex int) (references.FPDF_TEXTPAGE, error) {
	page, err := d.loadPageUnlocked(pageIndex)
	if err != nil {
		return "", err
	}

	tpResp, err := d.instance.FPDFText_LoadPage(&requests.FPDFText_LoadPage{
		Page: requests.Page{
			ByReference: &page,
		},
	})
	if err != nil {
		return "", err
	}

	d.textPages[pageIndex] = tpResp.TextPage
	return tpResp.TextPage, nil
}

func (d *PDFiumDocument) loadPage(pageIndex int) (references.FPDF_PAGE, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.loadPageUnlocked(pageIndex)
}

func (d *PDFiumDocument) loadPageUnlocked(pageIndex int) (references.FPDF_PAGE, error) {
	if pg, ok := d.pages[pageIndex]; ok {
		return pg, nil
	}

	pageResp, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return "", err
	}

	d.pages[pageIndex] = pageResp.Page
	return pageResp.Page, nil
}

func (d *PDFiumDocument) SaveAsCopy(filePath string) error {
	_, err := d.instance.FPDF_SaveAsCopy(&requests.FPDF_SaveAsCopy{
		Document: d.doc.Document,
		FilePath: &filePath,
		Flags:    requests.SaveFlagNoIncremental,
	})
	return err
}

// SaveAsCopyToBuffer saves the document to a byte buffer using FileWriter.
// Unlike SaveAsCopy (which uses FilePath and may use object streams),
// FileWriter produces objects in clear text, enabling byte-level post-processing.
func (d *PDFiumDocument) SaveAsCopyToBuffer() ([]byte, error) {
	var buf bytes.Buffer
	_, err := d.instance.FPDF_SaveAsCopy(&requests.FPDF_SaveAsCopy{
		Document:   d.doc.Document,
		FileWriter: &buf,
		Flags:      requests.SaveFlagNoIncremental,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
