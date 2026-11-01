package schemaintel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAPIDocumentReadBoundaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.yaml")
	content := []byte("openapi: 3.1.0\ninfo: {title: Boundaries, version: '1'}\npaths: {}\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	atBoundary, err := ReadFileLimited(path, int64(len(content)))
	if err != nil {
		t.Fatalf("exact size boundary: %v", err)
	}
	if string(atBoundary) != string(content) {
		t.Fatalf("file was changed or truncated")
	}
	_, err = ReadFileLimited(path, int64(len(content)-1))
	if !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("expected size rejection, got %v", err)
	}
	if _, err := ReadFileLimited(dir, DefaultMaxDocumentBytes); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory should be refused: %v", err)
	}
	for _, limit := range []int64{0, -1, MaxConfigurableDocumentBytes + 1} {
		if _, err := ReadFileLimited(path, limit); err == nil {
			t.Fatalf("invalid limit %d accepted", limit)
		}
	}
}

func TestParseDoesNotAdmitOversizedInMemoryDocument(t *testing.T) {
	data := []byte(strings.Repeat("x", int(DefaultMaxDocumentBytes)+1))
	_, err := Parse(data)
	if !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("expected in-memory admission rejection, got %v", err)
	}
}

func TestParseWithLimitPreservesSmallDocument(t *testing.T) {
	data := []byte("openapi: 3.1.0\ninfo: {title: Small, version: '1'}\npaths: {}\n")
	if _, err := ParseWithLimit(data, int64(len(data)-1)); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("expected size rejection: %v", err)
	}
	doc, err := ParseWithLimit(data, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Version() != "3.1.0" {
		t.Fatalf("version=%q", doc.Version())
	}
	if _, err := ParseWithLimit(nil, DefaultMaxDocumentBytes); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty input should fail: %v", err)
	}
}
