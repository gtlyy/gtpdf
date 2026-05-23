package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const recentMax = 10

type recentEntry struct {
	Path string `json:"path"`
}

var recentFile string

func initRecent() {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	dir := filepath.Join(cfgDir, "gtpdf")
	os.MkdirAll(dir, 0755)
	recentFile = filepath.Join(dir, "recent.json")
}

func loadRecent() []string {
	if recentFile == "" {
		return nil
	}
	data, err := os.ReadFile(recentFile)
	if err != nil {
		return nil
	}
	var entries []recentEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if e.Path != "" {
			paths = append(paths, e.Path)
		}
	}
	return paths
}

func saveRecent(paths []string) {
	if recentFile == "" {
		return
	}
	if len(paths) > recentMax {
		paths = paths[:recentMax]
	}
	entries := make([]recentEntry, len(paths))
	for i, p := range paths {
		entries[i] = recentEntry{Path: p}
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	os.WriteFile(recentFile, data, 0644)
}

func addRecentFile(path string) {
	if path == "" {
		return
	}
	paths := loadRecent()
	// 去重：已存在则移到最前
	var filtered []string
	for _, p := range paths {
		if p != path {
			filtered = append(filtered, p)
		}
	}
	filtered = append([]string{path}, filtered...)
	if len(filtered) > recentMax {
		filtered = filtered[:recentMax]
	}
	saveRecent(filtered)
}

func clearRecent() {
	saveRecent(nil)
}
