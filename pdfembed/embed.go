package pdfembed

import (
	"os"
	"path/filepath"
)

var (
	data      []byte
	loaded    bool
	cleanPath string
)

func Init() bool {
	if loaded {
		return true
	}
	if data == nil {
		return false
	}

	exe, err := os.Executable()
	if err != nil {
		return false
	}
	dir := filepath.Dir(exe)
	soPath := filepath.Join(dir, "libpdfium.so")

	if err := os.WriteFile(soPath, data, 0755); err != nil {
		dir = filepath.Join(os.TempDir(), "gtpdf")
		os.MkdirAll(dir, 0755)
		soPath = filepath.Join(dir, "libpdfium.so")
		if err := os.WriteFile(soPath, data, 0755); err != nil {
			return false
		}
	}

	if !loadLibrary(soPath) {
		return false
	}

	loaded = true
	cleanPath = soPath
	return true
}

func Cleanup() {
	if cleanPath != "" {
		os.Remove(cleanPath)
		os.Remove(filepath.Dir(cleanPath))
		cleanPath = ""
	}
}
