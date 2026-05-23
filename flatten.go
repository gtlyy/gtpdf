package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"math"
	"os"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func detectScanDPI(path string, pageWidthPt, pageHeightPt float64) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 300, err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	images, err := api.ExtractImagesRaw(f, nil, conf)
	if err != nil {
		return 300, nil
	}

	maxDPI := 200.0
	for _, pageImages := range images {
		for _, img := range pageImages {
			cfg, err := jpeg.DecodeConfig(img)
			if err != nil {
				continue
			}
			w := float64(cfg.Width)
			h := float64(cfg.Height)
			dpi := math.Max(w, h) * 72.0 / math.Max(pageWidthPt, pageHeightPt)
			if dpi > maxDPI {
				maxDPI = dpi
			}
		}
	}
	if maxDPI > 600 {
		maxDPI = 600
	}
	return maxDPI, nil
}

func (v *PDFViewerPlus) flattenRotatedPDF(dpi float64) ([]byte, error) {
	totalPages := v.totalPages

	conf := model.NewDefaultConfiguration()
	var pageSeekers []io.ReadSeeker

	for pageIdx := 0; pageIdx < totalPages; pageIdx++ {
		img, cleanup, err := v.pdfDoc.RenderPageRaw(pageIdx, dpi, false)
		if err != nil {
			return nil, fmt.Errorf("渲染第 %d 页失败: %w", pageIdx+1, err)
		}

		var jpgBuf bytes.Buffer
		if err := jpeg.Encode(&jpgBuf, img, &jpeg.Options{Quality: 85}); err != nil {
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

	var result bytes.Buffer
	if err := api.MergeRaw(pageSeekers, &result, false, conf); err != nil {
		return nil, fmt.Errorf("合并PDF失败: %w", err)
	}

	return result.Bytes(), nil
}
