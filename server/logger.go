package main

import (
	"bufio"
	"log"
	"os"
	"strings"
	"sync"
)

// MaxLogLines defines the maximum number of lines to keep in the log file
const MaxLogLines = 5000

// LimitedLogger wraps the standard log.Logger with line count limiting
type LimitedLogger struct {
	file      *os.File
	logger    *log.Logger
	lineCount int
	mutex     sync.Mutex
}

// NewLimitedLogger creates a new LimitedLogger
func NewLimitedLogger(file *os.File) *LimitedLogger {
	logger := log.New(file, "", log.LstdFlags)
	ll := &LimitedLogger{
		file:      file,
		logger:    logger,
		lineCount: 0,
	}

	// Count existing lines in the file
	ll.countExistingLines()
	return ll
}

// countExistingLines counts the number of lines in the current log file
func (ll *LimitedLogger) countExistingLines() {
	ll.mutex.Lock()
	defer ll.mutex.Unlock()

	// Seek to beginning of file
	ll.file.Seek(0, 0)
	scanner := bufio.NewScanner(ll.file)

	count := 0
	for scanner.Scan() {
		count++
	}

	ll.lineCount = count

	// Seek back to end of file for appending
	ll.file.Seek(0, 2)
}

// Write implements io.Writer interface
func (ll *LimitedLogger) Write(p []byte) (n int, err error) {
	ll.mutex.Lock()
	defer ll.mutex.Unlock()

	// Write to file
	n, err = ll.file.Write(p)
	if err != nil {
		return n, err
	}

	// Count newlines in the written data
	newlines := strings.Count(string(p), "\n")
	ll.lineCount += newlines

	// Check if we need to rotate the log file
	if ll.lineCount > MaxLogLines {
		ll.rotateLogFile()
	}

	return n, err
}

// rotateLogFile trims the log file to keep only the last MaxLogLines lines
func (ll *LimitedLogger) rotateLogFile() {
	// Read all lines from the file
	ll.file.Seek(0, 0)
	scanner := bufio.NewScanner(ll.file)
	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Keep only the last MaxLogLines
	if len(lines) > MaxLogLines {
		lines = lines[len(lines)-MaxLogLines:]
	}

	// Truncate and rewrite the file
	ll.file.Truncate(0)
	ll.file.Seek(0, 0)

	for _, line := range lines {
		ll.file.WriteString(line + "\n")
	}

	ll.lineCount = len(lines)
}

// Close closes the underlying file
func (ll *LimitedLogger) Close() error {
	return ll.file.Close()
}
