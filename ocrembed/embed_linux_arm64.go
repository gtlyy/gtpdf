//go:build linux && arm64

package ocrembed

import _ "embed"

//go:embed libtesseract.so.5.5
var tesseractArmData []byte

//go:embed libleptonica.so.6
var leptonicaArmData []byte

//go:embed tessdata/chi_sim.traineddata
var chiSimArmData []byte

//go:embed tessdata/chi_tra.traineddata
var chiTraArmData []byte

//go:embed tessdata/eng.traineddata
var engArmData []byte

func init() {
	embedFiles = map[string][]byte{
		"libtesseract.so.5.5":             tesseractArmData,
		"libleptonica.so.6":               leptonicaArmData,
		"tessdata/chi_sim.traineddata":    chiSimArmData,
		"tessdata/chi_tra.traineddata":    chiTraArmData,
		"tessdata/eng.traineddata":        engArmData,
	}
}
