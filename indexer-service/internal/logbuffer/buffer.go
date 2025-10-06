package logbuffer

import (
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

type Buffer struct {
	entries []LogEntry
	maxSize int
	mu      sync.RWMutex
}

var globalBuffer *Buffer

func Init(maxSize int) {
	globalBuffer = &Buffer{
		entries: make([]LogEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

func Add(level, message string) {
	if globalBuffer == nil {
		return
	}

	globalBuffer.mu.Lock()
	defer globalBuffer.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}

	globalBuffer.entries = append(globalBuffer.entries, entry)

	// Keep only last maxSize entries
	if len(globalBuffer.entries) > globalBuffer.maxSize {
		globalBuffer.entries = globalBuffer.entries[1:]
	}
}

func GetRecent(limit int) []LogEntry {
	if globalBuffer == nil {
		return []LogEntry{}
	}

	globalBuffer.mu.RLock()
	defer globalBuffer.mu.RUnlock()

	if limit <= 0 || limit > len(globalBuffer.entries) {
		limit = len(globalBuffer.entries)
	}

	// Return last 'limit' entries
	start := len(globalBuffer.entries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, limit)
	copy(result, globalBuffer.entries[start:])

	return result
}

func Clear() {
	if globalBuffer == nil {
		return
	}

	globalBuffer.mu.Lock()
	defer globalBuffer.mu.Unlock()

	globalBuffer.entries = make([]LogEntry, 0, globalBuffer.maxSize)
}
