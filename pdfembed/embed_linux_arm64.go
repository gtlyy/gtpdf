//go:build linux && arm64

package pdfembed

import _ "embed"

//go:embed libpdfium_linux_arm64.so
var libpdfiumData []byte

func init() {
	data = libpdfiumData
}
