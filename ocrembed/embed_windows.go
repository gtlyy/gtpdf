//go:build windows

package ocrembed

import (
	"os"
	"path/filepath"
)

var (
	loaded      bool
	tessdataDir string
)

func Init() (string, bool) {
	if loaded {
		return tessdataDir, true
	}

	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	exeDir := filepath.Dir(exe)

	libPath := filepath.Join(exeDir, "libtesseract-5.dll")
	if !doLoad(libPath) {
		return "", false
	}

	tessdata := filepath.Join(exeDir, "tessdata")
	loaded = true
	tessdataDir = tessdata
	return tessdataDir, true
}

func Cleanup() {
	loaded = false
	tessdataDir = ""
}
