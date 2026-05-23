package main

import (
	"log"
	"os"
	"strings"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel = LevelDebug

func init() {
	switch strings.ToLower(os.Getenv("GTPDF_LOG")) {
	case "debug":
		currentLevel = LevelDebug
	case "info":
		currentLevel = LevelInfo
	case "warn":
		currentLevel = LevelWarn
	case "error":
		currentLevel = LevelError
	default:
		currentLevel = LevelInfo
	}
}

func logD(format string, args ...interface{}) {
	if currentLevel <= LevelDebug {
		log.Printf(format, args...)
	}
}

func logI(format string, args ...interface{}) {
	if currentLevel <= LevelInfo {
		log.Printf(format, args...)
	}
}

func logW(format string, args ...interface{}) {
	if currentLevel <= LevelWarn {
		log.Printf(format, args...)
	}
}

func logE(format string, args ...interface{}) {
	if currentLevel <= LevelError {
		log.Printf(format, args...)
	}
}
