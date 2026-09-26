package trace

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Phase             string
	Method            string
	OperationID       string
	JobID             string
	EventID           string
	AggregateID       string
	ChangeSequence    int64
	ProcessGeneration int64
	Status            string
	ErrorCode         string
}

type Writer struct {
	mu   sync.Mutex
	file *os.File
	path string
	log  *slog.Logger
}

type contextKey struct{}

func Open(directory string) (*Writer, error) {
	path := filepath.Join(directory, "trace.jsonl")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}

	writer := &Writer{file: file, path: path}
	writer.log = slog.New(slog.NewJSONHandler(writer, nil))

	return writer, nil
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.file.Close()
}

func (w *Writer) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	info, err := w.file.Stat()
	if err != nil {
		return 0, err
	}

	if info.Size()+int64(len(data)) > 10<<20 {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	return w.file.Write(data)
}

func (w *Writer) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}

	previous := filepath.Join(filepath.Dir(w.path), "trace.previous.jsonl")
	if err := os.Remove(previous); err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(w.path, previous); err != nil {
		return err
	}

	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	w.file = file

	return nil
}

func WithWriter(ctx context.Context, writer *Writer) context.Context {
	return context.WithValue(ctx, contextKey{}, writer)
}

func Record(ctx context.Context, entry Entry) {
	writer, ok := ctx.Value(contextKey{}).(*Writer)
	if !ok || writer == nil {
		return
	}

	writer.log.LogAttrs(ctx, slog.LevelInfo, "trace",
		slog.Time("timestamp", time.Now().UTC()),
		slog.String("phase", entry.Phase),
		slog.String("method", entry.Method),
		slog.String("operationId", safeID(entry.OperationID)),
		slog.String("jobId", safeID(entry.JobID)),
		slog.String("eventId", safeID(entry.EventID)),
		slog.String("aggregateId", safeID(entry.AggregateID)),
		slog.Int64("changeSequence", entry.ChangeSequence),
		slog.Int64("processGeneration", entry.ProcessGeneration),
		slog.String("status", entry.Status),
		slog.String("errorCode", safeID(entry.ErrorCode)),
	)
}

func safeID(value string) string {
	if len(value) > 128 {
		return "[redacted]"
	}

	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' || char == '-' || char == '_' || char == ':' {
			continue
		}

		return "[redacted]"
	}

	return value
}
