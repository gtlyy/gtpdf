//go:build linux

package pdfembed

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>

int loadPDFium(const char* path) {
	return dlopen(path, RTLD_NOW | RTLD_GLOBAL) != NULL;
}
*/
import "C"

import "unsafe"

func loadLibrary(path string) bool {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	return C.loadPDFium(cpath) != 0
}
