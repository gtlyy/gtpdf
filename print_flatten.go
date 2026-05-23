package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// flattenForPrint builds a PDF for printing.
// Strategy (B):
//   - If none of the requested pages have annotations:
//     return the original vector PDF (no precision loss).
//   - If annotations exist:
//     For pages WITHOUT annotations → extract vector page from original.
//     For pages WITH annotations → render at 600 DPI and embed as image.
//     All single-page PDFs are then merged in order.
func (v *PDFViewerPlus) flattenForPrint(pages []int) ([]byte, error) {
	origBuf, err := v.pdfDoc.doc.SaveAsCopyToBuffer()
	if err != nil {
		return nil, err
	}

	hasAnnots := make(map[int]bool)
	needsFlatten := false
	for _, pageIdx := range pages {
		count, err := v.pdfDoc.doc.AnnotGetCount(pageIdx)
		if err == nil && count > 0 {
			hasAnnots[pageIdx] = true
			needsFlatten = true
		}
	}
	if !needsFlatten {
		// No annotations: return vector PDF directly (after AP fix for FreeText).
		origBuf, _ = fixFreeTextAPBytes(origBuf)
		return origBuf, nil
	}

	conf := model.NewDefaultConfiguration()

	ctx, err := api.ReadAndValidate(bytes.NewReader(origBuf), conf)
	if err != nil {
		return nil, fmt.Errorf("读取PDF失败: %w", err)
	}

	var pageSeekers []io.ReadSeeker

	for _, pageIdx := range pages {
		if hasAnnots[pageIdx] {
			img, cleanup, err := v.pdfDoc.RenderPageRaw(pageIdx, 600.0, true)
			if err != nil {
				return nil, fmt.Errorf("渲染第 %d 页失败: %w", pageIdx+1, err)
			}

			var jpgBuf bytes.Buffer
			if err := jpeg.Encode(&jpgBuf, img, &jpeg.Options{Quality: 95}); err != nil {
				if cleanup != nil {
					cleanup()
				}
				return nil, fmt.Errorf("编码第 %d 页失败: %w", pageIdx+1, err)
			}
			if cleanup != nil {
				cleanup()
			}

			imp := pdfcpu.DefaultImportConfig()
			imp.PageSize = ""
			var singlePDF bytes.Buffer
			if err := api.ImportImages(nil, &singlePDF, []io.Reader{bytes.NewReader(jpgBuf.Bytes())}, imp, conf); err != nil {
				return nil, fmt.Errorf("生成第 %d 页失败: %w", pageIdx+1, err)
			}
			pageSeekers = append(pageSeekers, bytes.NewReader(singlePDF.Bytes()))
		} else {
			pageReader, err := api.ExtractPage(ctx, pageIdx+1)
			if err == nil {
				pageData, err := io.ReadAll(pageReader)
				if err == nil {
					pageSeekers = append(pageSeekers, bytes.NewReader(pageData))
					continue
				}
				logD("[打印] 第 %d 页矢量提取数据读取失败: %v，回退 600 DPI 渲染", pageIdx+1, err)
			} else {
				logD("[打印] 第 %d 页矢量提取失败: %v，回退 600 DPI 渲染", pageIdx+1, err)
			}
			// Fallback: render clean page at 600 DPI
			img, cleanup, err := v.pdfDoc.RenderPageRaw(pageIdx, 600.0, false)
			if err != nil {
				return nil, fmt.Errorf("渲染第 %d 页失败: %w", pageIdx+1, err)
			}
			var jpgBuf bytes.Buffer
			if err := jpeg.Encode(&jpgBuf, img, &jpeg.Options{Quality: 95}); err != nil {
				if cleanup != nil {
					cleanup()
				}
				return nil, fmt.Errorf("编码第 %d 页失败: %w", pageIdx+1, err)
			}
			if cleanup != nil {
				cleanup()
			}
			imp := pdfcpu.DefaultImportConfig()
			imp.PageSize = ""
			var singlePDF bytes.Buffer
			if err := api.ImportImages(nil, &singlePDF, []io.Reader{bytes.NewReader(jpgBuf.Bytes())}, imp, conf); err != nil {
				return nil, fmt.Errorf("生成第 %d 页失败: %w", pageIdx+1, err)
			}
			pageSeekers = append(pageSeekers, bytes.NewReader(singlePDF.Bytes()))
		}
	}

	var result bytes.Buffer
	if err := api.MergeRaw(pageSeekers, &result, false, conf); err != nil {
		return nil, fmt.Errorf("合并PDF失败: %w", err)
	}

	return result.Bytes(), nil
}

// parsePrintPages converts a page spec string into 0-indexed page indices.
// Supports "all" for all pages, a single number, or comma-separated ranges like "1-3,5,7".
func parsePrintPages(spec string, totalPages int) []int {
	if spec == "" || spec == "all" {
		pages := make([]int, totalPages)
		for i := 0; i < totalPages; i++ {
			pages[i] = i
		}
		return pages
	}

	seen := map[int]bool{}
	var pages []int

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, _ := strconv.Atoi(strings.TrimSpace(bounds[0]))
			end, _ := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if start < 1 {
				start = 1
			}
			if end > totalPages {
				end = totalPages
			}
			for i := start; i <= end; i++ {
				if !seen[i] {
					pages = append(pages, i-1)
					seen[i] = true
				}
			}
		} else {
			p, _ := strconv.Atoi(part)
			if p >= 1 && p <= totalPages && !seen[p] {
				pages = append(pages, p-1)
				seen[p] = true
			}
		}
	}

	sort.Ints(pages)
	return pages
}
