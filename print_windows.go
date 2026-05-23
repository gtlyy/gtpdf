//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"
)

func printPDF(pdfPath string, _ PrintOptions) error {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("ShellExecuteW")

	op := syscall.StringToUTF16Ptr("print")
	file := syscall.StringToUTF16Ptr(pdfPath)

	ret, _, _ := proc.Call(
		0,
		uintptr(unsafe.Pointer(op)),
		uintptr(unsafe.Pointer(file)),
		0, 0, 1,
	)
	if ret <= 32 {
		return errors.New("调用系统打印失败")
	}
	return nil
}

func listPrinters() ([]string, error) {
	return []string{"系统默认打印机"}, nil
}
