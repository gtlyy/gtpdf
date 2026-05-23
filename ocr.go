package main

import (
	"fmt"
	"image"
	"sync"

	"gtpdf/ocrembed"
)

const ocrDPI = 250.0

var (
	ocrOnce       sync.Once
	ocrInitOK     bool
	ocrDataPath   string
	ocrMu         sync.Mutex
	ocrHandle     *ocrembed.OcrHandle
	ocrHandleLang string
)

func isTesseractAvailable() bool {
	ocrOnce.Do(func() {
		var ok bool
		ocrDataPath, ok = ocrembed.Init()
		ocrInitOK = ok
	})
	return ocrInitOK
}

func getOcrHandle(lang string) (*ocrembed.OcrHandle, error) {
	ocrMu.Lock()
	defer ocrMu.Unlock()

	if ocrHandle != nil && ocrHandleLang == lang && ocrHandle.IsValid() {
		return ocrHandle, nil
	}

	if ocrHandle != nil {
		ocrHandle.Close()
		ocrHandle = nil
		ocrHandleLang = ""
	}

	h, err := ocrembed.NewOcrHandle(ocrDataPath, lang)
	if err != nil {
		return nil, err
	}

	ocrHandle = h
	ocrHandleLang = lang
	return ocrHandle, nil
}

func ocrImage(img image.Image, psm int) (string, error) {
	if !isTesseractAvailable() {
		return "", fmt.Errorf("OCR: engine not available")
	}

	rgba, ok := img.(*image.RGBA)
	if !ok {
		bounds := img.Bounds()
		rgba = image.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				rgba.Set(x, y, img.At(x, y))
			}
		}
	}

	h, err := getOcrHandle("chi_sim+eng")
	if err != nil {
		return "", err
	}

	h.SetPageSegMode(psm)
	h.SetImage(rgba.Pix, rgba.Bounds().Dx(), rgba.Bounds().Dy(), 4, rgba.Stride)

	if err := h.Recognize(); err != nil {
		return "", err
	}

	text := h.GetText()
	if text == "" {
		h, err = getOcrHandle("eng")
		if err != nil {
			return "", err
		}
		h.SetPageSegMode(psm)
		h.SetImage(rgba.Pix, rgba.Bounds().Dx(), rgba.Bounds().Dy(), 4, rgba.Stride)
		if err := h.Recognize(); err != nil {
			return "", err
		}
		text = h.GetText()
	}

	return text, nil
}

func ocrImageRegion(img image.Image, x, y, w, h int) (string, error) {
	if !isTesseractAvailable() {
		return "", fmt.Errorf("OCR: engine not available")
	}

	if w <= 0 || h <= 0 || x < 0 || y < 0 {
		return "", fmt.Errorf("OCR: invalid crop region")
	}

	bounds := img.Bounds()
	if x+w > bounds.Dx() {
		w = bounds.Dx() - x
	}
	if y+h > bounds.Dy() {
		h = bounds.Dy() - y
	}
	if w <= 0 || h <= 0 {
		return "", fmt.Errorf("OCR: crop region out of bounds")
	}

	ocrH, err := getOcrHandle("chi_sim+eng")
	if err != nil {
		return "", err
	}

	rgba, ok := img.(*image.RGBA)
	if !ok {
		bounds := img.Bounds()
		rgba = image.NewRGBA(bounds)
		for cy := bounds.Min.Y; cy < bounds.Max.Y; cy++ {
			for cx := bounds.Min.X; cx < bounds.Max.X; cx++ {
				rgba.Set(cx, cy, img.At(cx, cy))
			}
		}
	}

	ocrH.SetPageSegMode(6) // uniform block for cropped regions
	ocrH.SetImage(rgba.Pix, rgba.Bounds().Dx(), rgba.Bounds().Dy(), 4, rgba.Stride)
	ocrH.SetRectangle(x, y, w, h)

	if err := ocrH.Recognize(); err != nil {
		return "", err
	}

	text := ocrH.GetText()
	if text == "" {
		ocrH2, err := getOcrHandle("eng")
		if err != nil {
			return "", err
		}
		ocrH2.SetPageSegMode(6)
		ocrH2.SetImage(rgba.Pix, rgba.Bounds().Dx(), rgba.Bounds().Dy(), 4, rgba.Stride)
		ocrH2.SetRectangle(x, y, w, h)
		if err := ocrH2.Recognize(); err != nil {
			return "", err
		}
		text = ocrH2.GetText()
	}

	return text, nil
}
