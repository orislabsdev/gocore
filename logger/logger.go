// Package logger provides a lightweight, structured, leveled logger for the
// gocore library and for application code.
//
// The Logger writes to any io.Writer (stdout by default) in either JSON or
// human-readable text format. Each log entry carries a timestamp, severity
// level, message, and optional structured fields.
//
// Usage:
//
//	log := logger.New(logger.Config{
//	    Level:  logger.LevelInfo,
//	    Format: logger.FormatJSON,
//	    Output: os.Stdout,
//	})
//	log.Info("server starting", "port", 8080)
//	log.With("request_id", "abc123").Error("request failed", "status", 500)
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Level
// ─────────────────────────────────────────────────────────────────────────────

// Level represents a log severity level.
type Level int8

const (
	LevelDebug Level = iota // Verbose detail useful during development.
	LevelInfo               // Routine operational messages.
	LevelWarn               // Conditions that are unusual but not errors.
	LevelError              // Errors that require attention.
	LevelOff                // Disable all logging.
)

// levelNames maps Level values to human-readable strings.
var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError:  "ERROR",
}

// ParseLevel converts a string ("debug", "info", "warn", "error") to a Level.
// Returns LevelInfo for unknown strings.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Format
// ─────────────────────────────────────────────────────────────────────────────

// Format selects the log encoding.
type Format uint8

const (
	FormatJSON Format = iota // Structured JSON — best for log aggregators.
	FormatText               // Human-readable key=value — best for development.
)

// ParseFormat converts "json" or "text" to a Format constant.
func ParseFormat(s string) Format {
	if strings.ToLower(s) == "text" {
		return FormatText
	}
	return FormatJSON
}

// ─────────────────────────────────────────────────────────────────────────────
// Config & Constructor
// ─────────────────────────────────────────────────────────────────────────────

// Config holds logger initialization parameters.
type Config struct {
	// Level is the minimum severity to emit.
	Level Level

	// Format selects JSON or text encoding.
	Format Format

	// Output is the writer to emit log entries to. Defaults to os.Stdout.
	Output io.Writer

	// AddCaller includes the source file and line number in every entry.
	// Adds a small CPU overhead — useful for debugging.
	AddCaller bool
}

// Logger is a structured, leveled logger. It is safe for concurrent use.
type Logger struct {
	cfg    Config
	fields []any // pre-allocated key-value pairs added via With()
	mu     sync.Mutex
}

// New creates a Logger from the provided Config.
// If cfg.Output is nil, os.Stdout is used.
func New(cfg Config) *Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	return &Logger{cfg: cfg}
}

// NewFromStrings is a convenience constructor that accepts string-typed config
// values as produced by config.LogConfig.
func NewFromStrings(level, format, output string) *Logger {
	var w io.Writer
	switch output {
	case "stderr":
		w = os.Stderr
	case "stdout", "":
		w = os.Stdout
	default:
		f, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			w = os.Stdout
		} else {
			w = f
		}
	}
	return New(Config{
		Level:  ParseLevel(level),
		Format: ParseFormat(format),
		Output: w,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Core logging methods
// ─────────────────────────────────────────────────────────────────────────────

// Debug emits an entry at DEBUG level with the given message and key-value pairs.
func (l *Logger) Debug(msg string, kv ...any) { l.log(LevelDebug, msg, kv) }

// Info emits an entry at INFO level.
func (l *Logger) Info(msg string, kv ...any) { l.log(LevelInfo, msg, kv) }

// Warn emits an entry at WARN level.
func (l *Logger) Warn(msg string, kv ...any) { l.log(LevelWarn, msg, kv) }

// Error emits an entry at ERROR level.
func (l *Logger) Error(msg string, kv ...any) { l.log(LevelError, msg, kv) }

// Fatal emits an entry at ERROR level and then calls os.Exit(1).
func (l *Logger) Fatal(msg string, kv ...any) {
	l.log(LevelError, msg, kv)
	os.Exit(1)
}

// ─────────────────────────────────────────────────────────────────────────────
// Contextual logger
// ─────────────────────────────────────────────────────────────────────────────

// With returns a new Logger that always includes the given key-value pairs in
// every entry it emits. This is useful for adding request-scoped fields such
// as a trace ID without threading them through every function call.
//
//	reqLog := log.With("request_id", id, "user_id", uid)
//	reqLog.Info("processing request")
//	reqLog.Error("request failed", "err", err)
func (l *Logger) With(kv ...any) *Logger {
	child := &Logger{cfg: l.cfg}
	// Merge parent fields with new fields.
	child.fields = make([]any, 0, len(l.fields)+len(kv))
	child.fields = append(child.fields, l.fields...)
	child.fields = append(child.fields, kv...)
	return child
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal emit
// ─────────────────────────────────────────────────────────────────────────────

// log is the central emit function. It merges pre-allocated fields, optional
// caller info, and per-call key-value pairs into a single log entry.
func (l *Logger) log(level Level, msg string, kv []any) {
	if level < l.cfg.Level {
		return // below minimum threshold — fast path
	}

	now := time.Now().UTC()

	// Merge fields: pre-bound fields from With() + per-call pairs.
	merged := make([]any, 0, len(l.fields)+len(kv))
	merged = append(merged, l.fields...)
	merged = append(merged, kv...)

	var caller string
	if l.cfg.AddCaller {
		_, file, line, ok := runtime.Caller(2) // 2 = caller of Debug/Info/…
		if ok {
			// Keep only the last two path segments for brevity.
			parts := strings.Split(file, "/")
			if len(parts) > 2 {
				parts = parts[len(parts)-2:]
			}
			caller = fmt.Sprintf("%s:%d", strings.Join(parts, "/"), line)
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cfg.Format == FormatJSON {
		l.emitJSON(now, level, msg, caller, merged)
	} else {
		l.emitText(now, level, msg, caller, merged)
	}
}

// emitJSON writes a structured JSON log entry to the output writer.
func (l *Logger) emitJSON(t time.Time, level Level, msg, caller string, kv []any) {
	entry := make(map[string]any, 4+len(kv)/2)
	entry["time"] = t.Format(time.RFC3339Nano)
	entry["level"] = levelNames[level]
	entry["msg"] = msg
	if caller != "" {
		entry["caller"] = caller
	}
	// Flatten key-value pairs into the map.
	for i := 0; i+1 < len(kv); i += 2 {
		if key, ok := kv[i].(string); ok {
			entry[key] = kv[i+1]
		}
	}
	b, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(l.cfg.Output, `{"level":"ERROR","msg":"logger marshal error","err":%q}`+"\n", err.Error())
		return
	}
	b = append(b, '\n')
	_, _ = l.cfg.Output.Write(b)
}

// emitText writes a human-readable text log entry to the output writer.
func (l *Logger) emitText(t time.Time, level Level, msg, caller string, kv []any) {
	var sb strings.Builder
	sb.WriteString(t.Format("2006-01-02T15:04:05.000Z"))
	sb.WriteByte(' ')
	sb.WriteString(fmt.Sprintf("%-5s", levelNames[level]))
	sb.WriteByte(' ')
	sb.WriteString(msg)
	if caller != "" {
		sb.WriteString(" caller=")
		sb.WriteString(caller)
	}
	for i := 0; i+1 < len(kv); i += 2 {
		sb.WriteByte(' ')
		if key, ok := kv[i].(string); ok {
			sb.WriteString(key)
			sb.WriteByte('=')
			sb.WriteString(sanitizeValue(kv[i+1]))
		}
	}
	sb.WriteByte('\n')
	_, _ = l.cfg.Output.Write([]byte(sb.String()))
}

// sanitizeValue escapes control characters and newlines to prevent log injection.
func sanitizeValue(v any) string {
	s := fmt.Sprintf("%v", v)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
