package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gtpdf/pdfium_plus"
)

var (
	fixObjRe = regexp.MustCompile(`(\d+)\s+0\s+obj\s+<<([^>]+?)>>`)
	fixQRe   = regexp.MustCompile(`/Q\((\d+)\)`)
)

// fixFreeTextAPBytes applies byte-level post-processing following the pdffont approach:
// 1. Finds the Type0 font object number in the PDF raw bytes
// 2. Injects /Resources<</Font<</SourceHanSansSC-Regular ObjRef>>>> into every
//    Form XObject that lacks /Resources (annotation AP streams created by PDFium
//    don't include font resources)
// 3. Fixes /Q(N) -> /Q N (FPDFAnnot_SetStringValue writes string, spec requires integer)
func fixFreeTextAPBytes(data []byte) ([]byte, error) {
	fontName := "SourceHanSansSC-Regular"
	str := string(data)

	// Quick early-out: no SourceHan font = no font injection to do
	hasSourceHan := strings.Contains(str, "SourceHan")
	hasQFix := strings.Contains(str, "/Q(")
	if !hasSourceHan && !hasQFix {
		return data, nil
	}

	// ---- Diagnostic: search for font name in raw bytes ----
	srcCount := 0
	bpCount := 0
	t0Count := 0
	if hasSourceHan {
		srcCount = strings.Count(str, "SourceHan")
		bpCount = strings.Count(str, "/BaseFont")
		t0Count = strings.Count(str, "Type0")
	}
	logD("[AP-FIX] DIAG: SourceHan=%d BaseFont=%d Type0=%d bytes=%d",
		srcCount, bpCount, t0Count, len(data))
	if srcCount > 0 {
		idx := strings.Index(str, "SourceHan")
		end := idx + 100
		if end > len(str) {
			end = len(str)
		}
		logD("[AP-FIX] DIAG: first 'SourceHan' at offset=%d ctx=%.100s", idx, str[idx:end])
	}

	// ---- Step 1: Find the Type0 font object number ----
	fontObjNum := findType0FontObjNum(str, fontName)
	logD("[AP-FIX] Type0 font %q: obj=%q", fontName, fontObjNum)

	// Debug: list all objects found by regex
	allObjs := fixObjRe.FindAllStringSubmatch(str, -1)
	logD("[AP-FIX] regex matched %d objects total", len(allObjs))
	for i, m := range allObjs {
		if i >= 5 {
			break
		}
		content := m[2]
		if len(content) > 60 {
			content = content[:60] + "..."
		}
		logD("[AP-FIX]   obj=%s dict=%.80s", m[1], content)
	}

	// Also check if ObjStm exists (would hide font objects)
	objStmCount := strings.Count(str, "Type/ObjStm")
	logD("[AP-FIX] DIAG: Type/ObjStm entries=%d", objStmCount)

	// Search for font name in any form (with or without subset prefix)
	for _, suffix := range []string{"SourceHanSansSC-Regular", "SourceHanSansSC"} {
		idx := strings.Index(str, suffix)
		if idx >= 0 {
			off := 0
			count := 0
			for {
				pos := strings.Index(str[off:], suffix)
				if pos < 0 {
					break
				}
				absPos := off + pos
				end := absPos + 120
				if end > len(str) {
					end = len(str)
				}
				ctx := str[absPos:end]
				if len(ctx) > 80 {
					ctx = ctx[:80]
				}
				logD("[AP-FIX] DIAG: %q at offset %d: %.80s", suffix, absPos, ctx)
				off = absPos + len(suffix)
				count++
				if count >= 5 {
					break
				}
			}
		}
	}

	// Search for Type0 context
	t0Idx := strings.Index(str, "Type0")
	if t0Idx >= 0 {
		ctx := str[max(0, t0Idx-40):min(len(str), t0Idx+80)]
		logD("[AP-FIX] DIAG: 'Type0' at offset %d: %.120s", t0Idx, ctx)
	}

	// ---- Step 2: Inject /Resources into every Form XObject missing it ----
	// Uses fixObjRe (N 0 obj <<...>>) to find all objects, then checks each
	// dict for /Form + /XObject. This is order-independent unlike fixFormRe,
	// which fails when PDFium outputs /Type /XObject before /Subtype /Form.
	injected := 0
	if fontObjNum != "" {
		formObjs := fixObjRe.FindAllStringSubmatch(str, -1)
		logD("[AP-FIX] found %d objects total", len(formObjs))
		for _, m := range formObjs {
			dict := m[2]
			if !strings.Contains(dict, "/Form") || !strings.Contains(dict, "/XObject") {
				continue
			}
			if strings.Contains(dict, "/Resources") {
				logD("[AP-FIX]   SKIP (has /Resources): obj=%s dict=%.80s...", m[1], dict)
				continue
			}
			subIdx := strings.Index(dict, "/Subtype")
			if subIdx < 0 {
				continue
			}
			insert := fmt.Sprintf("/Resources<</Font<</%s %s 0 R>>>> ", fontName, fontObjNum)
			newDict := dict[:subIdx] + insert + dict[subIdx:]
			// Use m[0] to preserve original whitespace (Sprintf would lose it)
			openIdx := strings.Index(m[0], "<<")
			closeIdx := strings.LastIndex(m[0], ">>")
			if openIdx < 0 || closeIdx <= openIdx {
				continue
			}
			newFull := m[0][:openIdx] + "<<" + newDict + ">>"
			str = strings.Replace(str, m[0], newFull, 1)
			injected++
			logD("[AP-FIX]   INJECTED obj=%s font=%s Obj=%s → %.80s...", m[1], fontName, fontObjNum, newFull)
		}
	} else {
		logD("[AP-FIX] WARNING: Type0 font %q not found in raw bytes!", fontName)
	}

	// ---- Step 2.5: Adjust AP coords for moved FreeText annotations ----
	str = fixMovedFreeTextAPs(str)

	// ---- Step 3: Fix /Q(N) -> /Q N ----
	qFixed := 0
	qMatches := fixQRe.FindAllString(str, -1)
	if len(qMatches) > 0 {
		str = fixQRe.ReplaceAllString(str, "/Q $1")
		qFixed = len(qMatches)
		logD("[AP-FIX] /Q fixed: %v", qMatches)
	}

	logD("[AP-FIX] SUMMARY: injected=%d /Q-fixed=%d", injected, qFixed)

	if injected == 0 && qFixed == 0 && fontObjNum == "" {
		return data, nil
	}

	return []byte(str), nil
}

// findType0FontObjNum searches raw PDF bytes for the Type0 font dict
// with BaseFont=fontName and Subtype=Type0, and returns its object number.
func findType0FontObjNum(str, fontName string) string {
	for _, m := range fixObjRe.FindAllStringSubmatch(str, -1) {
		if strings.Contains(m[2], fontName) && strings.Contains(m[2], "Type0") {
			return m[1]
		}
	}
	return ""
}

// fixFreeTextQValues fixes malformed /Q values in FreeText annotations.
// FPDFAnnot_SetStringValue with key "Q" writes /Q(0) but PDF spec requires /Q 0.
// This byte-level fix replaces /Q(N) -> /Q N throughout the file.
func fixFreeTextQValues(pdfPath string) error {
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return err
	}

	original := string(data)
	matches := fixQRe.FindAllString(original, -1)
	if len(matches) > 0 {
		logD("[Q-FIX] found %d /Q(N) occurrences: %v", len(matches), matches)
	}
	replaced := fixQRe.ReplaceAllString(original, "/Q $1")
	if replaced == original {
		return nil
	}

	return os.WriteFile(pdfPath, []byte(replaced), 0644)
}

// fixMovedFreeTextAPs finds FreeText annotations with /gtpdf_dx, /gtpdf_dy
// and adjusts their AP stream coords by the stored delta, then removes the keys.
func fixMovedFreeTextAPs(str string) string {
	movedCount := 0
	dxRe := regexp.MustCompile(`/gtpdf_dx\s*\(([^)]*)\)`)

	for {
		dxMatch := dxRe.FindStringSubmatch(str)
		if dxMatch == nil {
			break
		}
		dxFull := dxMatch[0]
		dxVal, err := strconv.ParseFloat(dxMatch[1], 64)
		if err != nil {
			str = strings.Replace(str, dxFull, "", 1)
			continue
		}

		// Find corresponding /gtpdf_dy in nearby context
		dxIdx := strings.Index(str, dxFull)
		afterDx := str[dxIdx:]
		dyRe := regexp.MustCompile(`/gtpdf_dy\s*\(([^)]*)\)`)
		dyMatch := dyRe.FindStringSubmatch(afterDx)
		if dyMatch == nil {
			str = strings.Replace(str, dxFull, "", 1)
			continue
		}
		dyFull := dyMatch[0]
		dyVal, err := strconv.ParseFloat(dyMatch[1], 64)
		if err != nil {
			str = strings.Replace(str, dxFull, "", 1)
			continue
		}

		// Find /AP << /N objNum 0 R >> in the annotation dict context
		ctxStart := max(0, dxIdx-3000)
		ctxEnd := dxIdx + len(dxFull) + 1000
		if ctxEnd > len(str) {
			ctxEnd = len(str)
		}
		ctx := str[ctxStart:ctxEnd]
		apNRe := regexp.MustCompile(`/AP\s*<<\s*/N\s+(\d+)\s+0\s+R\s*>>`)
		apMatch := apNRe.FindStringSubmatch(ctx)
		if apMatch == nil {
			apMatch = apNRe.FindStringSubmatch(afterDx)
		}
		if apMatch == nil {
			str = strings.Replace(str, dxFull, "", 1)
			str = strings.Replace(str, dyFull, "", 1)
			continue
		}
		apObjNum := apMatch[1]

		// Adjust Tm coordinates in the AP stream
		str = fixAPStreamCoords(str, apObjNum, dxVal, dyVal)

		// Remove /gtpdf_dx and /gtpdf_dy entries
		str = regexp.MustCompile(`/gtpdf_dx\s*\([^)]*\)\s*`).ReplaceAllString(str, "")
		str = regexp.MustCompile(`/gtpdf_dy\s*\([^)]*\)\s*`).ReplaceAllString(str, "")

		movedCount++
	}

	if movedCount > 0 {
		logD("[AP-FIX] moved %d FreeText AP(s)", movedCount)
	}
	return str
}

// fixAPStreamCoords finds stream object objNum in the raw PDF text,
// extracts its stream content, adjusts all Tm coordinates by (dx, dy),
// and replaces the content in place.
func fixAPStreamCoords(str string, objNum string, dx, dy float64) string {
	// Find object header: N 0 obj
	re := regexp.MustCompile(regexp.QuoteMeta(objNum) + `\s+0\s+obj`)
	loc := re.FindStringIndex(str)
	if loc == nil {
		return str
	}

	// Find "stream" keyword after object header
	afterObj := str[loc[0]:]
	streamRe := regexp.MustCompile(`\nstream\r?\n`)
	streamLoc := streamRe.FindStringIndex(afterObj)
	if streamLoc == nil {
		return str
	}
	streamStart := loc[0] + streamLoc[1]

	// Find "endstream"
	endRe := regexp.MustCompile(`\nendstream`)
	endLoc := endRe.FindStringIndex(str[streamStart:])
	if endLoc == nil {
		endRe = regexp.MustCompile(`endstream`) // fallback: no newline prefix
		endLoc = endRe.FindStringIndex(str[streamStart:])
	}
	if endLoc == nil {
		return str
	}
	streamEnd := streamStart + endLoc[0]

	content := str[streamStart:streamEnd]
	newContent := pdfium_plus.AdjustFreeTextAP(content, float32(dx), float32(dy))
	if newContent == content {
		return str
	}

	str = str[:streamStart] + newContent + str[streamEnd:]
	return str
}
