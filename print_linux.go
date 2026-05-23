//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func printPDF(pdfPath string, opts PrintOptions) error {
	args := []string{}
	if opts.Printer != "" {
		args = append(args, "-d", opts.Printer)
	}
	if opts.Copies > 1 {
		args = append(args, "-n", fmt.Sprintf("%d", opts.Copies))
	}
	if opts.Pages != "" && opts.Pages != "all" {
		args = append(args, "-P", opts.Pages)
	}
	if opts.Reverse {
		args = append(args, "-o", "outputorder=reverse")
	}
	if opts.Collate {
		args = append(args, "-o", "collate=true")
	}
	if opts.Duplex == "long-edge" {
		args = append(args, "-o", "sides=two-sided-long-edge")
	} else if opts.Duplex == "short-edge" {
		args = append(args, "-o", "sides=two-sided-short-edge")
	}
	if opts.Grayscale {
		args = append(args, "-o", "ColorModel=Gray")
	}
	if opts.NumberUp > 1 {
		args = append(args, "-o", fmt.Sprintf("number-up=%d", opts.NumberUp))
	}
	if opts.MediaSize != "" {
		args = append(args, "-o", fmt.Sprintf("media=%s", opts.MediaSize))
	}
	switch opts.Scaling {
	case "fit":
		args = append(args, "-o", "fit-to-page")
	default:
		if pct, err := fmt.Sscanf(opts.Scaling, "%d"); err == nil && pct > 0 && pct != 100 {
			args = append(args, "-o", fmt.Sprintf("scaling=%d", pct))
		}
	}
	args = append(args, pdfPath)

	cmd := exec.Command("lp", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lp 失败: %w\n%s", err, string(out))
	}
	return nil
}

func listPrinters() ([]string, error) {
	cmd := exec.Command("lpstat", "-a")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var printers []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			printers = append(printers, parts[0])
		}
	}
	if len(printers) == 0 {
		return nil, nil
	}
	return printers, nil
}
