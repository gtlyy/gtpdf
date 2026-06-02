//go:build !windows

package ocrembed

import (
	"fmt"
	"os"
	"path/filepath"
)

var (
	embedFiles  map[string][]byte
	loaded      bool
	cleanDir    string
	tessdataDir string
)

func Init() (string, bool) {
	if loaded {
		return tessdataDir, true
	}
	if embedFiles == nil || len(embedFiles) == 0 {
		return "", false
	}

	dir := filepath.Join(os.TempDir(), fmt.Sprintf("gtpdf_ocr_%d", os.Getpid()))
	os.MkdirAll(dir, 0755)

	for name, data := range embedFiles {
		outPath := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return "", false
		}
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return "", false
		}
	}

	os.Setenv("LD_LIBRARY_PATH", dir)

	libPath := filepath.Join(dir, "libtesseract.so.5.5")
	if !doLoad(libPath) {
		return "", false
	}

	tessdata := filepath.Join(dir, "tessdata")
	loaded = true
	cleanDir = dir
	tessdataDir = tessdata

	return tessdataDir, true
}

func Cleanup() {
	if cleanDir != "" {
		os.RemoveAll(cleanDir)
		cleanDir = ""
	}
	loaded = false
	tessdataDir = ""
	embedFiles = nil
}
