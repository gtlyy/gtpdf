package main

// Platform-specific print implementations
// Linux: uses lp command via os/exec
// Windows: uses ShellExecuteW("print")

type PrintOptions struct {
	Printer   string
	Copies    int
	Pages     string // "all" = all pages, otherwise page spec like "1-3,5"
	Reverse   bool
	Collate   bool
	Duplex    string // "", "long-edge", "short-edge" (empty = printer default)
	Grayscale bool   // true = force grayscale
	NumberUp  int    // 1, 2, 4, 6, 9, 16 (pages per side)
	MediaSize string // "", "A4", "Letter", "A3", etc. (empty = printer default)
	Scaling   string // "", "fit", or percentage like "100" (empty = printer default)
}
