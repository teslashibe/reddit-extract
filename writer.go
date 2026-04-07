package redditextract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type writerOptions struct {
	appendMode bool
}

// WriterOption configures JSONL writing behavior.
type WriterOption func(*writerOptions)

// WithAppendMode appends records to an existing JSONL file if present.
func WithAppendMode() WriterOption {
	return func(o *writerOptions) {
		o.appendMode = true
	}
}

// WriteJSONL writes one JSON object per line to the target path.
//
// By default writes are atomic (temp file + rename). In append mode, records
// are appended directly to the destination file.
func WriteJSONL[T any](path string, results []Result[T], opts ...WriterOption) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	cfg := writerOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if cfg.appendMode {
		return writeJSONLAppend(path, results)
	}
	return writeJSONLAtomic(path, results)
}

func writeJSONLAppend[T any](path string, results []Result[T]) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open output file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, result := range results {
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("encode result: %w", err)
		}
	}
	return nil
}

func writeJSONLAtomic[T any](path string, results []Result[T]) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".reddit-extract-*.jsonl")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	encoder := json.NewEncoder(tmpFile)
	for _, result := range results {
		if err := encoder.Encode(result); err != nil {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("encode result: %w", err)
		}
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace output file: %w", err)
	}
	return nil
}
