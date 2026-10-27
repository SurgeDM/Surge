package clioutput

import (
	"encoding/json"
	"fmt"
	"io"
)

const SchemaVersion = 1

// Result has the same envelope for list, detail, empty, and failure results.
// Items is always an array, including when the operation fails.
type Result[T any] struct {
	SchemaVersion int      `json:"schema_version"`
	Operation     string   `json:"operation"`
	OK            bool     `json:"ok"`
	Items         []T      `json:"items"`
	Error         *Failure `json:"error,omitempty"`
}

func Success[T any](operation string, items []T) Result[T] {
	if items == nil {
		items = []T{}
	}
	return Result[T]{SchemaVersion: SchemaVersion, Operation: operation, OK: true, Items: items}
}

func Failed(operation string, err error) Result[any] {
	failure := Describe(err)
	return Result[any]{SchemaVersion: SchemaVersion, Operation: operation, Items: []any{}, Error: &failure}
}

// WriteError means output may already have been partially written. Callers must
// not append another JSON document in an attempt to report this error.
type WriteError struct{ Cause error }

func (e *WriteError) Error() string { return fmt.Sprintf("write command output: %v", e.Cause) }
func (e *WriteError) Unwrap() error { return e.Cause }

// WriteJSON encodes before writing, so encoding failures cannot leave half a
// document on stdout. It retains the indentation used by legacy ls --json.
func WriteJSON(w io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode command output: %w", err)
	}
	return write(w, append(data, '\n'))
}

func WriteText(w io.Writer, text string) error { return write(w, []byte(text)) }

func write(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return &WriteError{Cause: err}
	}
	return nil
}
