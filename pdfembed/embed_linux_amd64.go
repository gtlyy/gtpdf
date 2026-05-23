//go:build linux && amd64

package pdfembed

import _ "embed"

//go:embed libpdfium_linux_amd64.so
var libpdfiumData []byte

func init() {
	data = libpdfiumData
}
