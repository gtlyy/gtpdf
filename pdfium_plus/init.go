package pdfium_plus

import (
	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/single_threaded"
	"gtpdf/pdfembed"
)

var pool pdfium.Pool
var instance pdfium.Pdfium

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

func GetInstance() pdfium.Pdfium {
	return instance
}

func GetPool() pdfium.Pool {
	return pool
}

func Cleanup() {
	pdfembed.Cleanup()
}
