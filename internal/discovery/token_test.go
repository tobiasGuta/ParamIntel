package discovery

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestTokenFromReaderReturnsSixteenHexCharacters(t *testing.T) {
	token, err := tokenFromReader(strings.NewReader("12345678"))
	if err != nil {
		t.Fatal(err)
	}
	if token != "3132333435363738" {
		t.Fatalf("token=%q", token)
	}
}

func TestTokenFromReaderFailsClosed(t *testing.T) {
	token, err := tokenFromReader(failingReader{})
	if err == nil {
		t.Fatal("expected entropy error")
	}
	if token != "" {
		t.Fatalf("token=%q want empty", token)
	}
	if !strings.Contains(err.Error(), "generate probe token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

var _ io.Reader = failingReader{}
