package pdfium_plus

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/structs"
)

type AnnotCreateResult struct {
	Annotation references.FPDF_ANNOTATION
	PageIndex  int
}

type FontLoadResult struct {
	Font     references.FPDF_FONT
	FontName string
}

type PageObjCreateTextObjResult struct {
	PageObject references.FPDF_PAGEOBJECT
}

func (d *PDFiumDocument) AnnotCreate(pageIndex int, subtype enums.FPDF_ANNOTATION_SUBTYPE) (*AnnotCreateResult, error) {
	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return nil, err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	resp, err := d.instance.FPDFPage_CreateAnnot(&requests.FPDFPage_CreateAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Subtype: subtype,
	})
	if err != nil {
		return nil, err
	}

	return &AnnotCreateResult{
		Annotation: resp.Annotation,
		PageIndex:  pageIndex,
	}, nil
}

func (d *PDFiumDocument) AnnotSetRect(annot references.FPDF_ANNOTATION, left, top, right, bottom float32) error {
	_, err := d.instance.FPDFAnnot_SetRect(&requests.FPDFAnnot_SetRect{
		Annotation: annot,
		Rect: structs.FPDF_FS_RECTF{
			Left:   left,
			Top:    top,
			Right:  right,
			Bottom: bottom,
		},
	})
	return err
}

func (d *PDFiumDocument) AnnotSetColor(annot references.FPDF_ANNOTATION, colorType enums.FPDFANNOT_COLORTYPE, r, g, b, a uint) error {
	_, err := d.instance.FPDFAnnot_SetColor(&requests.FPDFAnnot_SetColor{
		Annotation: annot,
		ColorType:  colorType,
		R:          r,
		G:          g,
		B:          b,
		A:          a,
	})
	return err
}

func (d *PDFiumDocument) AnnotSetBorder(annot references.FPDF_ANNOTATION, width float32) error {
	_, err := d.instance.FPDFAnnot_SetBorder(&requests.FPDFAnnot_SetBorder{
		Annotation:       annot,
		HorizontalRadius: 0,
		VerticalRadius:   0,
		BorderWidth:      width,
	})
	return err
}

func (d *PDFiumDocument) AnnotSetContent(annot references.FPDF_ANNOTATION, content string) error {
	return d.AnnotSetDictString(annot, "Contents", content)
}

func (d *PDFiumDocument) AnnotSetDictString(annot references.FPDF_ANNOTATION, key, value string) error {
	_, err := d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: annot,
		Key:        key,
		Value:      value,
	})
	return err
}

func (d *PDFiumDocument) AnnotSetQuadPoints(annot references.FPDF_ANNOTATION, quadPoints structs.FPDF_FS_QUADPOINTSF) error {
	_, err := d.instance.FPDFAnnot_AppendAttachmentPoints(&requests.FPDFAnnot_AppendAttachmentPoints{
		Annotation:       annot,
		AttachmentPoints: quadPoints,
	})
	return err
}

// CreateTextMarkupAnnot creates a text markup annotation (Highlight/Underline/etc.)
// and sets its rect, quadpoints, color, and border in a single page session.
func (d *PDFiumDocument) CreateTextMarkupAnnot(pageIndex int, subtype enums.FPDF_ANNOTATION_SUBTYPE,
	l, t, r, b float32, quadPoints structs.FPDF_FS_QUADPOINTSF,
	colorR, colorG, colorB, colorA uint, borderWidth float32) error {

	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	resp, err := d.instance.FPDFPage_CreateAnnot(&requests.FPDFPage_CreateAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Subtype: subtype,
	})
	if err != nil {
		return err
	}

	if _, err := d.instance.FPDFAnnot_SetRect(&requests.FPDFAnnot_SetRect{
		Annotation: resp.Annotation,
		Rect:       structs.FPDF_FS_RECTF{Left: l, Top: t, Right: r, Bottom: b},
	}); err != nil {
		return err
	}

	if _, err := d.instance.FPDFAnnot_AppendAttachmentPoints(&requests.FPDFAnnot_AppendAttachmentPoints{
		Annotation:       resp.Annotation,
		AttachmentPoints: quadPoints,
	}); err != nil {
		return err
	}

	if _, err := d.instance.FPDFAnnot_SetColor(&requests.FPDFAnnot_SetColor{
		Annotation: resp.Annotation,
		ColorType:  enums.FPDFANNOT_COLORTYPE_Color,
		R:          colorR, G: colorG, B: colorB, A: colorA,
	}); err != nil {
		return err
	}

	if borderWidth > 0 {
		d.instance.FPDFAnnot_SetBorder(&requests.FPDFAnnot_SetBorder{
			Annotation:       resp.Annotation,
			HorizontalRadius: 0,
			VerticalRadius:   0,
			BorderWidth:      borderWidth,
		})
	}

	mVal := time.Now().UTC().Format("D:20060102150405Z")
	d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: resp.Annotation,
		Key:        "M",
		Value:      mVal,
	})

	return nil
}

// CreateShapeAnnot creates a shape annotation (Square/Line) and sets its rect and color
// in a single page session.
func (d *PDFiumDocument) CreateShapeAnnot(pageIndex int, subtype enums.FPDF_ANNOTATION_SUBTYPE,
	l, t, r, b float32,
	colorR, colorG, colorB, colorA uint, borderWidth float32) error {

	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	resp, err := d.instance.FPDFPage_CreateAnnot(&requests.FPDFPage_CreateAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Subtype: subtype,
	})
	if err != nil {
		return err
	}

	if _, err := d.instance.FPDFAnnot_SetRect(&requests.FPDFAnnot_SetRect{
		Annotation: resp.Annotation,
		Rect:       structs.FPDF_FS_RECTF{Left: l, Top: t, Right: r, Bottom: b},
	}); err != nil {
		return err
	}

	if _, err := d.instance.FPDFAnnot_SetColor(&requests.FPDFAnnot_SetColor{
		Annotation: resp.Annotation,
		ColorType:  enums.FPDFANNOT_COLORTYPE_Color,
		R:          colorR, G: colorG, B: colorB, A: colorA,
	}); err != nil {
		return err
	}

	if borderWidth > 0 {
		d.instance.FPDFAnnot_SetBorder(&requests.FPDFAnnot_SetBorder{
			Annotation:       resp.Annotation,
			HorizontalRadius: 0,
			VerticalRadius:   0,
			BorderWidth:      borderWidth,
		})
	}

	mVal := time.Now().UTC().Format("D:20060102150405Z")
	d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: resp.Annotation,
		Key:        "M",
		Value:      mVal,
	})

	return nil
}

// CreateFillAnnot creates a filled Square annotation (no border, only fill color)
// in a single page session.
func (d *PDFiumDocument) CreateFillAnnot(pageIndex int,
	l, t, r, b float32,
	colorR, colorG, colorB, colorA uint) error {

	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	resp, err := d.instance.FPDFPage_CreateAnnot(&requests.FPDFPage_CreateAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Subtype: enums.FPDF_ANNOT_SUBTYPE_SQUARE,
	})
	if err != nil {
		return err
	}

	if _, err := d.instance.FPDFAnnot_SetRect(&requests.FPDFAnnot_SetRect{
		Annotation: resp.Annotation,
		Rect:       structs.FPDF_FS_RECTF{Left: l, Top: t, Right: r, Bottom: b},
	}); err != nil {
		return err
	}

	// Set interior (fill) color
	if _, err := d.instance.FPDFAnnot_SetColor(&requests.FPDFAnnot_SetColor{
		Annotation: resp.Annotation,
		ColorType:  enums.FPDFANNOT_COLORTYPE_InteriorColor,
		R:          colorR, G: colorG, B: colorB, A: colorA,
	}); err != nil {
		return err
	}

	// Set stroke color to same as fill (for consistency)
	d.instance.FPDFAnnot_SetColor(&requests.FPDFAnnot_SetColor{
		Annotation: resp.Annotation,
		ColorType:  enums.FPDFANNOT_COLORTYPE_Color,
		R:          colorR, G: colorG, B: colorB, A: colorA,
	})

	mVal := time.Now().UTC().Format("D:20060102150405Z")
	d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: resp.Annotation,
		Key:        "M",
		Value:      mVal,
	})

	return nil
}

func (d *PDFiumDocument) AnnotRemoveAll() error {
	for pageIdx := 0; pageIdx < d.numPages; pageIdx++ {
		count, err := d.AnnotGetCount(pageIdx)
		if err != nil {
			continue
		}
		for i := count - 1; i >= 0; i-- {
			if err := d.AnnotRemove(pageIdx, i); err != nil {
				return err
			}
		}
	}
	return nil
}

// AdjustFreeTextAP adjusts all Tm coordinates in a FreeText AP content stream by (dx, dy).
func AdjustFreeTextAP(ap string, dx, dy float32) string {
	tokens := strings.Fields(ap)
	for i := 0; i < len(tokens); i++ {
		if tokens[i] == "Tm" && i >= 6 {
			if tx, err := strconv.ParseFloat(tokens[i-2], 32); err == nil {
				tokens[i-2] = fmt.Sprintf("%.2f", tx+float64(dx))
			}
			if ty, err := strconv.ParseFloat(tokens[i-1], 32); err == nil {
				tokens[i-1] = fmt.Sprintf("%.2f", ty+float64(dy))
			}
		}
	}
	return strings.Join(tokens, " ")
}

// AnnotMoveFreeText moves a FreeText annotation by (dx, dy) PDF units.
// Updates the Rect and stores cumulative delta for save-time AP adjustment.
func (d *PDFiumDocument) AnnotMoveFreeText(pageIndex, annotIndex int, dx, dy float32) error {
	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	annot, err := d.instance.FPDFPage_GetAnnot(&requests.FPDFPage_GetAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Index: annotIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDFPage_CloseAnnot(&requests.FPDFPage_CloseAnnot{Annotation: annot.Annotation})

	rectResp, err := d.instance.FPDFAnnot_GetRect(&requests.FPDFAnnot_GetRect{
		Annotation: annot.Annotation,
	})
	if err != nil {
		return err
	}

	if _, err = d.instance.FPDFAnnot_SetRect(&requests.FPDFAnnot_SetRect{
		Annotation: annot.Annotation,
		Rect: structs.FPDF_FS_RECTF{
			Left:   rectResp.Rect.Left + dx,
			Top:    rectResp.Rect.Top + dy,
			Right:  rectResp.Rect.Right + dx,
			Bottom: rectResp.Rect.Bottom + dy,
		},
	}); err != nil {
		return err
	}

	// Accumulate total delta for save-time AP coordinate adjustment.
	// FPDFAnnot_SetAP is NOT called here – it creates a new Form XObject
	// without /Resources, breaking other PDF readers.
	oldDx, oldDy := float32(0), float32(0)
	if resp, err := d.instance.FPDFAnnot_GetStringValue(&requests.FPDFAnnot_GetStringValue{
		Annotation: annot.Annotation, Key: "gtpdf_dx",
	}); err == nil && resp.Value != "" {
		if v, e := strconv.ParseFloat(resp.Value, 32); e == nil {
			oldDx = float32(v)
		}
	}
	if resp, err := d.instance.FPDFAnnot_GetStringValue(&requests.FPDFAnnot_GetStringValue{
		Annotation: annot.Annotation, Key: "gtpdf_dy",
	}); err == nil && resp.Value != "" {
		if v, e := strconv.ParseFloat(resp.Value, 32); e == nil {
			oldDy = float32(v)
		}
	}
	d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: annot.Annotation, Key: "gtpdf_dx", Value: fmt.Sprintf("%.2f", oldDx+dx),
	})
	d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: annot.Annotation, Key: "gtpdf_dy", Value: fmt.Sprintf("%.2f", oldDy+dy),
	})

	return nil
}

func (d *PDFiumDocument) AnnotRemove(pageIndex int, annotIndex int) error {
	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	_, err = d.instance.FPDFPage_RemoveAnnot(&requests.FPDFPage_RemoveAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Index: annotIndex,
	})
	return err
}

func (d *PDFiumDocument) AnnotGetCount(pageIndex int) (int, error) {
	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return 0, err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	resp, err := d.instance.FPDFPage_GetAnnotCount(&requests.FPDFPage_GetAnnotCount{
		Page: requests.Page{
			ByReference: &page.Page,
		},
	})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

func (d *PDFiumDocument) SetContentByIndex(pageIndex int, annotIndex int, content string) error {
	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	annot, err := d.instance.FPDFPage_GetAnnot(&requests.FPDFPage_GetAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Index: annotIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDFPage_CloseAnnot(&requests.FPDFPage_CloseAnnot{Annotation: annot.Annotation})

	return d.AnnotSetContent(annot.Annotation, content)
}

func (d *PDFiumDocument) fontExistsInDoc(fontName string) bool {
	for pageIdx := 0; pageIdx < d.numPages; pageIdx++ {
		count, err := d.AnnotGetCount(pageIdx)
		if err != nil || count == 0 {
			continue
		}
		for i := 0; i < count; i++ {
			da, err := d.getAnnotDA(pageIdx, i)
			if err != nil {
				continue
			}
			if strings.Contains(da, fontName) {
				return true
			}
		}
	}
	return false
}

func (d *PDFiumDocument) getAnnotDA(pageIndex, annotIndex int) (string, error) {
	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return "", err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	annot, err := d.instance.FPDFPage_GetAnnot(&requests.FPDFPage_GetAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Index: annotIndex,
	})
	if err != nil {
		return "", err
	}
	defer d.instance.FPDFPage_CloseAnnot(&requests.FPDFPage_CloseAnnot{Annotation: annot.Annotation})

	da, err := d.instance.FPDFAnnot_GetStringValue(&requests.FPDFAnnot_GetStringValue{
		Annotation: annot.Annotation,
		Key:        "DA",
	})
	if err != nil {
		return "", err
	}
	return da.Value, nil
}

func (d *PDFiumDocument) FontLoad(data []byte, fontType enums.FPDF_FONT, cid bool) (*FontLoadResult, error) {
	key := fmt.Sprintf("%d-%v", fontType, cid)
	d.mu.Lock()
	if cached, ok := d.fontCache[key]; ok {
		d.mu.Unlock()
		return cached, nil
	}
	d.mu.Unlock()

	resp, err := d.instance.FPDFText_LoadFont(&requests.FPDFText_LoadFont{
		Document: d.doc.Document,
		Data:     data,
		FontType: fontType,
		CID:      cid,
	})
	if err != nil {
		return nil, err
	}
	nameResp, err := d.instance.FPDFFont_GetBaseFontName(&requests.FPDFFont_GetBaseFontName{
		Font: resp.Font,
	})
	if err != nil {
		return nil, err
	}
	result := &FontLoadResult{
		Font:     resp.Font,
		FontName: nameResp.BaseFontName,
	}
	d.mu.Lock()
	d.fontCache[key] = result
	d.mu.Unlock()
	return result, nil
}

func (d *PDFiumDocument) FontGetBaseFontName(font references.FPDF_FONT) (string, error) {
	resp, err := d.instance.FPDFFont_GetBaseFontName(&requests.FPDFFont_GetBaseFontName{
		Font: font,
	})
	if err != nil {
		return "", err
	}
	return resp.BaseFontName, nil
}

func (d *PDFiumDocument) PageObjCreateTextObj(font references.FPDF_FONT, fontSize float32) (*PageObjCreateTextObjResult, error) {
	resp, err := d.instance.FPDFPageObj_CreateTextObj(&requests.FPDFPageObj_CreateTextObj{
		Document: d.doc.Document,
		Font:     font,
		FontSize: fontSize,
	})
	if err != nil {
		return nil, err
	}
	return &PageObjCreateTextObjResult{
		PageObject: resp.PageObject,
	}, nil
}

func (d *PDFiumDocument) TextSetText(textObj references.FPDF_PAGEOBJECT, text string) error {
	_, err := d.instance.FPDFText_SetText(&requests.FPDFText_SetText{
		PageObject: textObj,
		Text:       text,
	})
	return err
}

func (d *PDFiumDocument) PageObjSetMatrix(textObj references.FPDF_PAGEOBJECT, a, b, c, d_, e, f float32) error {
	_, err := d.instance.FPDFPageObj_SetMatrix(&requests.FPDFPageObj_SetMatrix{
		PageObject: textObj,
		Transform: structs.FPDF_FS_MATRIX{
			A: a, B: b, C: c, D: d_, E: e, F: f,
		},
	})
	return err
}

func (d *PDFiumDocument) PageInsertObject(pageIndex int, obj references.FPDF_PAGEOBJECT) error {
	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	_, err = d.instance.FPDFPage_InsertObject(&requests.FPDFPage_InsertObject{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		PageObject: obj,
	})
	return err
}

func (d *PDFiumDocument) PageGenerateContent(pageIndex int) error {
	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	_, err = d.instance.FPDFPage_GenerateContent(&requests.FPDFPage_GenerateContent{
		Page: requests.Page{
			ByReference: &page.Page,
		},
	})
	return err
}

func (d *PDFiumDocument) AnnotSetAP(annot references.FPDF_ANNOTATION, apContent string) error {
	_, err := d.instance.FPDFAnnot_SetAP(&requests.FPDFAnnot_SetAP{
		Annotation:     annot,
		AppearanceMode: enums.FPDF_ANNOT_APPEARANCEMODE_NORMAL,
		Value:          &apContent,
	})
	return err
}

// CreateFreeTextAnnot creates a FreeText annotation with CID font embedding.
// fontHandle must be loaded via FPDFText_LoadFont with CID=true.
// hexGIDs must be a PDF hex string <GID1GID2...> with uppercase hex.
// If apContent is non-empty, it is used as the appearance stream instead of generating one.
func (d *PDFiumDocument) CreateFreeTextAnnot(pageIndex int, fontHandle references.FPDF_FONT, fontName string,
	l, t, r, b float32, text string, fontSize float32, fgColor string, hexGIDs string, apContent string) error {

	page, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{
		Document: d.doc.Document,
		Index:    pageIndex,
	})
	if err != nil {
		return err
	}
	defer d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})

	annotResp, err := d.instance.FPDFPage_CreateAnnot(&requests.FPDFPage_CreateAnnot{
		Page: requests.Page{
			ByReference: &page.Page,
		},
		Subtype: enums.FPDF_ANNOT_SUBTYPE_FREETEXT,
	})
	if err != nil {
		return err
	}

	if _, err = d.instance.FPDFAnnot_SetRect(&requests.FPDFAnnot_SetRect{
		Annotation: annotResp.Annotation,
		Rect:       structs.FPDF_FS_RECTF{Left: l, Top: t, Right: r, Bottom: b},
	}); err != nil {
		return err
	}

	if _, err = d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: annotResp.Annotation,
		Key:        "Contents",
		Value:      text,
	}); err != nil {
		return err
	}

	if _, err = d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: annotResp.Annotation,
		Key:        "M",
		Value:      time.Now().UTC().Format("D:20060102150405Z"),
	}); err != nil {
		return err
	}

	da := fmt.Sprintf("/%s %.2f Tf %s", fontName, fontSize, fgColor)
	if _, err = d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: annotResp.Annotation,
		Key:        "DA",
		Value:      da,
	}); err != nil {
		return err
	}

	if _, err = d.instance.FPDFAnnot_SetStringValue(&requests.FPDFAnnot_SetStringValue{
		Annotation: annotResp.Annotation,
		Key:        "Q",
		Value:      "0",
	}); err != nil {
		return err
	}

	if apContent == "" {
		padX := fontSize * 0.3
		if padX < 3 {
			padX = 3
		}
		padY := fontSize * 0.3
		if padY < 3 {
			padY = 3
		}
		tx := l + padX
		ty := b + padY
		apContent = fmt.Sprintf("BT /%s %.2f Tf 1 0 0 1 %.2f %.2f Tm %s %s Tj ET",
			fontName, fontSize, tx, ty, fgColor, hexGIDs)
	}

	if _, err = d.instance.FPDFAnnot_SetAP(&requests.FPDFAnnot_SetAP{
		Annotation:     annotResp.Annotation,
		AppearanceMode: enums.FPDF_ANNOT_APPEARANCEMODE_NORMAL,
		Value:          &apContent,
	}); err != nil {
		return err
	}

	return nil
}