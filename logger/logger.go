package logger

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"ai-gateway/telemetry"
)

// Logger provides structured logging with API key redaction
type Logger struct {
	redactKeys []string
}

// NewLogger creates a new logger instance
func NewLogger() *Logger {
	// Disable timestamp and other prefixes from standard logger
	log.SetFlags(0)
	return &Logger{
		redactKeys: []string{},
	}
}

// AddRedactKey adds a key to be redacted from logs
func (l *Logger) AddRedactKey(key string) {
	l.redactKeys = append(l.redactKeys, key)
}

// Info logs an info message with structured fields
func (l *Logger) Info(message string, fields map[string]interface{}) {
	l.log("INFO", message, fields, nil)
	telemetry.RecordLog(context.Background(), "info", message, fields)
}

// Error logs an error message with structured fields
func (l *Logger) Error(message string, err error, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	l.log("ERROR", message, fields, err)
	telemetry.RecordLog(context.Background(), "error", message, fields)
}

// log writes a structured log entry
func (l *Logger) log(level, message string, fields map[string]interface{}, err error) {
	entry := map[string]interface{}{
		"level":   level,
		"message": message,
	}

	// Redact sensitive keys
	redactedFields := l.redactSensitiveData(fields)
	if len(redactedFields) > 0 {
		entry["fields"] = redactedFields
	}

	// Convert to JSON
	jsonData, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		log.Printf("[%s] %s: %v", level, message, fields)
		return
	}

	log.Println(string(jsonData))
}

// redactSensitiveData removes or redacts sensitive information from fields
func (l *Logger) redactSensitiveData(fields map[string]interface{}) map[string]interface{} {
	if fields == nil {
		return nil
	}

	redacted := make(map[string]interface{})
	for k, v := range fields {
		keyLower := strings.ToLower(k)

		// Redact API keys
		if strings.Contains(keyLower, "api_key") || strings.Contains(keyLower, "apikey") ||
			strings.Contains(keyLower, "token") || strings.Contains(keyLower, "secret") {
			redacted[k] = "[REDACTED]"
			continue
		}

		// For request/response summaries, only log counts and basic info
		if keyLower == "request" || keyLower == "response" {
			if m, ok := v.(map[string]interface{}); ok {
				summary := make(map[string]interface{})
				if model, exists := m["model"]; exists {
					summary["model"] = model
				}
				if messages, exists := m["messages"]; exists {
					if msgList, ok := messages.([]interface{}); ok {
						summary["message_count"] = len(msgList)
					}
				}
				redacted[k] = summary
				continue
			}
		}

		redacted[k] = v
	}

	return redacted
}
