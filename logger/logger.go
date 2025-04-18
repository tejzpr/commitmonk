package logger

import (
	"io"
	"io/ioutil"
	"log"
	"os"
)

var (
	// Default logger
	defaultLogger *log.Logger
	// Whether verbose logging is enabled
	verboseEnabled bool
)

// Init initializes the logger with verbose mode
func Init(verbose bool) {
	verboseEnabled = verbose

	// Set output based on verbose flag
	var output io.Writer
	if verbose {
		output = os.Stdout
	} else {
		output = ioutil.Discard
	}

	// Initialize default logger
	defaultLogger = log.New(output, "", log.LstdFlags)
}

// IsVerbose returns whether verbose logging is enabled
func IsVerbose() bool {
	return verboseEnabled
}

// Printf logs a formatted message if verbose mode is enabled
func Printf(format string, v ...interface{}) {
	defaultLogger.Printf(format, v...)
}

// Println logs a message if verbose mode is enabled
func Println(v ...interface{}) {
	defaultLogger.Println(v...)
}

// Error always logs an error message regardless of verbose mode
func Error(v ...interface{}) {
	log.Println(v...)
}

// Errorf always logs a formatted error message regardless of verbose mode
func Errorf(format string, v ...interface{}) {
	log.Printf(format, v...)
}
