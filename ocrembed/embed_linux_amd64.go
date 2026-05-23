//go:build linux && amd64

package ocrembed

import _ "embed"

//go:embed libtesseract.so.5.5
var tesseractData []byte

//go:embed libleptonica.so.6
var leptonicaData []byte

//go:embed tessdata/chi_sim.traineddata
var chiSimData []byte

//go:embed tessdata/chi_tra.traineddata
var chiTraData []byte

//go:embed tessdata/eng.traineddata
var engData []byte

func init() {
	embedFiles = map[string][]byte{
		"libtesseract.so.5.5":             tesseractData,
		"libleptonica.so.6":               leptonicaData,
		"tessdata/chi_sim.traineddata":    chiSimData,
		"tessdata/chi_tra.traineddata":    chiTraData,
		"tessdata/eng.traineddata":        engData,
	}
}
