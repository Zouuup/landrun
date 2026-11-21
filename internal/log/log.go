package log

import (
	"log"
	"os"
	"strings"
)

type Level int

const (
	LevelError Level = iota
	LevelInfo
	LevelDebug
)

var (
	debug = log.New(os.Stderr, "[landrun:debug] ", log.LstdFlags)
	info  = log.New(os.Stderr, "[landrun] ", log.LstdFlags)
	error = log.New(os.Stderr, "[landrun:error] ", log.LstdFlags)
	warn  = log.New(os.Stderr, "[landrun:warn] ", log.LstdFlags)

	currentLevel = LevelInfo // default level
)

// SetLevel sets the logging level
func SetLevel(level string) {
	switch strings.ToLower(level) {
	case "error":
		currentLevel = LevelError
	case "info":
		currentLevel = LevelInfo
	case "debug":
		currentLevel = LevelDebug
	default:
		currentLevel = LevelError
	}
}

// Debug logs a debug message
func Debug(format string, v ...interface{}) {
	if currentLevel >= LevelDebug {
		debug.Printf(format, v...)
	}
}

// Info logs an info message
func Info(format string, v ...interface{}) {
	if currentLevel >= LevelInfo {
		info.Printf(format, v...)
	}
}

// Warn logs a warning. Warnings are shown at every log level, since they
// report something the user would not otherwise notice.
func Warn(format string, v ...interface{}) {
	warn.Printf(format, v...)
}

// Error logs an error message
func Error(format string, v ...interface{}) {
	if currentLevel >= LevelError {
		error.Printf(format, v...)
	}
}

// Fatal logs an error message and exits
func Fatal(format string, v ...interface{}) {
	error.Printf(format, v...)
	os.Exit(1)
}
