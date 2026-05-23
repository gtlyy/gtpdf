//go:build pdfium_cgo
// +build pdfium_cgo

package pdfium_plus

import (
	// "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/single_threaded"
	"gtpdf/pdfembed"
)

func init() {
	pdfembed.Init()
	pool = single_threaded.Init(single_threaded.Config{})

	if pool != nil {
		var err error
		instance, err = pool.GetInstance(30)
		if err != nil {
			panic(err)
		}
	}
}
