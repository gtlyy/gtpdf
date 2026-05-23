//go:build windows

package ocrembed

/*
#include <windows.h>
#include <stdlib.h>

static HMODULE tesseractHandle = NULL;

typedef void* (*tess_create_t)(void);
typedef int   (*tess_init_t)(void*, const char*, const char*);
typedef void  (*tess_set_image_t)(void*, const unsigned char*, int, int, int, int);
typedef void  (*tess_set_rect_t)(void*, int, int, int, int);
typedef char* (*tess_get_text_t)(void*);
typedef void  (*tess_delete_t)(void*);
typedef void  (*tess_delete_text_t)(char*);
typedef int   (*tess_recognize_t)(void*, void*);
typedef void  (*tess_set_psm_t)(void*, int);

static tess_create_t      pCreate         = NULL;
static tess_init_t        pInit           = NULL;
static tess_set_image_t   pSetImage       = NULL;
static tess_set_rect_t    pSetRect        = NULL;
static tess_get_text_t    pGetText        = NULL;
static tess_delete_t      pDelete         = NULL;
static tess_delete_text_t pDeleteText     = NULL;
static tess_recognize_t   pRecognize      = NULL;
static tess_set_psm_t     pSetPageSegMode = NULL;

int loadTesseractLib(const char* path) {
	tesseractHandle = LoadLibraryA(path);
	if (!tesseractHandle) return 0;

	pCreate     = (tess_create_t)GetProcAddress(tesseractHandle, "TessBaseAPICreate");
	pInit       = (tess_init_t)GetProcAddress(tesseractHandle, "TessBaseAPIInit3");
	pSetImage   = (tess_set_image_t)GetProcAddress(tesseractHandle, "TessBaseAPISetImage");
	pSetRect    = (tess_set_rect_t)GetProcAddress(tesseractHandle, "TessBaseAPISetRectangle");
	pGetText    = (tess_get_text_t)GetProcAddress(tesseractHandle, "TessBaseAPIGetUTF8Text");
	pDelete     = (tess_delete_t)GetProcAddress(tesseractHandle, "TessBaseAPIDelete");
	pDeleteText = (tess_delete_text_t)GetProcAddress(tesseractHandle, "TessDeleteText");
	pRecognize  = (tess_recognize_t)GetProcAddress(tesseractHandle, "TessBaseAPIRecognize");
	pSetPageSegMode = (tess_set_psm_t)GetProcAddress(tesseractHandle, "TessBaseAPISetPageSegMode");

	return (pCreate && pInit && pSetImage && pGetText && pDelete && pRecognize) ? 1 : 0;
}

void* ocrCreate(void)                           { return pCreate(); }
int   ocrInit(void* h, char* dp, char* lang)    { return pInit(h, dp, lang); }
void  ocrSetImage(void* h, unsigned char* d, int w, int hgt, int bpp, int bpl) { pSetImage(h, d, w, hgt, bpp, bpl); }
void  ocrSetRect(void* h, int l, int t, int w, int hgt) { pSetRect(h, l, t, w, hgt); }
char* ocrGetText(void* h)                       { return pGetText(h); }
void  ocrDelete(void* h)                        { pDelete(h); }
void  ocrDeleteText(char* t)                    { pDeleteText(t); }
int   ocrRecognize(void* h)                     { return pRecognize(h, NULL); }
void  ocrSetPageSegMode(void* h, int mode)      { if (pSetPageSegMode) pSetPageSegMode(h, mode); }
*/
import "C"
import (
	"errors"
	"unsafe"
)

func loadLibrary(path string) bool {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	return C.loadTesseractLib(cpath) != 0
}

func doLoad(path string) bool {
	return loadLibrary(path)
}

type OcrHandle struct {
	p unsafe.Pointer
}

func NewOcrHandle(dataPath, language string) (*OcrHandle, error) {
	if !loaded {
		return nil, errors.New("OCR engine not initialized")
	}
	if dataPath == "" || language == "" {
		return nil, errors.New("OCR: dataPath and language are required")
	}

	h := C.ocrCreate()
	if h == nil {
		return nil, errors.New("OCR: failed to create handle")
	}

	cdp := C.CString(dataPath)
	clang := C.CString(language)
	defer C.free(unsafe.Pointer(cdp))
	defer C.free(unsafe.Pointer(clang))

	if C.ocrInit(h, cdp, clang) != 0 {
		C.ocrDelete(h)
		return nil, errors.New("OCR: init failed, lang=" + language + " datapath=" + dataPath)
	}

	return &OcrHandle{p: h}, nil
}

func (h *OcrHandle) SetImage(data []byte, w, hgt, bpp, bpl int) {
	C.ocrSetImage(h.p, (*C.uchar)(unsafe.Pointer(&data[0])), C.int(w), C.int(hgt), C.int(bpp), C.int(bpl))
}

func (h *OcrHandle) SetRectangle(x, y, w, hgt int) {
	C.ocrSetRect(h.p, C.int(x), C.int(y), C.int(w), C.int(hgt))
}

func (h *OcrHandle) Recognize() error {
	if C.ocrRecognize(h.p) != 0 {
		return errors.New("OCR: recognition failed")
	}
	return nil
}

func (h *OcrHandle) GetText() string {
	cstr := C.ocrGetText(h.p)
	if cstr == nil {
		return ""
	}
	result := C.GoString(cstr)
	C.ocrDeleteText(cstr)
	return result
}

func (h *OcrHandle) Close() {
	if h.p != nil {
		C.ocrDelete(h.p)
		h.p = nil
	}
}

func (h *OcrHandle) IsValid() bool {
	return h.p != nil
}

func (h *OcrHandle) SetPageSegMode(mode int) {
	C.ocrSetPageSegMode(h.p, C.int(mode))
}
