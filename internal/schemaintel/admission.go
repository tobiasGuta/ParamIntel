package schemaintel

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	// DefaultMaxDocumentBytes admits large public API descriptions while
	// rejecting unbounded allocations before YAML model construction.
	DefaultMaxDocumentBytes int64 = 16 << 20
	// MaxConfigurableDocumentBytes keeps user-configured admission bounded.
	MaxConfigurableDocumentBytes int64 = 64 << 20
)

var ErrDocumentTooLarge = errors.New("openapi document exceeds configured byte limit")

func ValidateDocumentLimit(limit int64) error {
	if limit <= 0 || limit > MaxConfigurableDocumentBytes {
		return fmt.Errorf("openapi byte limit must be between 1 and %d: got %d", MaxConfigurableDocumentBytes, limit)
	}
	return nil
}

func validateDocumentBytes(data []byte, limit int64) error {
	if err := ValidateDocumentLimit(limit); err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("%w: got %d bytes, limit %d", ErrDocumentTooLarge, len(data), limit)
	}
	return nil
}

// ReadFileLimited checks file type/size, then reads at most limit+1 bytes.
// The extra byte detects concurrent growth without silently truncating input.
func ReadFileLimited(path string, limit int64) ([]byte, error) {
	if err := ValidateDocumentLimit(limit); err != nil {
		return nil, err
	}
	file, err := openRegularOpenAPIDocument(path)
	if err != nil {
		return nil, fmt.Errorf("open regular openapi document: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat openapi document: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("openapi document must be a regular file")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%w: file is %d bytes, limit %d", ErrDocumentTooLarge, info.Size(), limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read openapi document: %w", err)
	}
	if err := validateDocumentBytes(data, limit); err != nil {
		return nil, err
	}
	return data, nil
}
