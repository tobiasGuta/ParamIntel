package responsefeatures

import (
	"net/http"
	"testing"
)

func TestHeaderFingerprintsIgnoreSensitiveAndNoisyHeaders(t *testing.T) {
	headers := http.Header{
		"X-Feature-Mode": []string{"preview"},
		"Location":       []string{"/dashboard"},
		"Set-Cookie":     []string{"session=secret"},
		"Date":           []string{"Sat, 12 Sep 2026 22:00:00 GMT"},
		"X-Request-Id":   []string{"rotating-id"},
		"Content-Type":   []string{"text/html"},
	}
	got := HeaderFingerprints(headers)

	for _, name := range []string{"X-Feature-Mode", "Location"} {
		if got[name] == "" {
			t.Fatalf("eligible header %s missing fingerprint", name)
		}
	}
	for _, name := range []string{"Set-Cookie", "Date", "X-Request-Id", "Content-Type"} {
		if _, ok := got[name]; ok {
			t.Fatalf("ignored header %s unexpectedly fingerprinted", name)
		}
	}
}

func TestHeaderFingerprintsNormalizeNameAndValueOrder(t *testing.T) {
	a := http.Header{
		"x-capabilities": []string{" export ", "admin"},
	}
	b := http.Header{
		"X-Capabilities": []string{"admin", "export"},
	}

	afp := HeaderFingerprints(a)
	bfp := HeaderFingerprints(b)
	if afp["X-Capabilities"] == "" || bfp["X-Capabilities"] == "" {
		t.Fatalf("missing canonical header fingerprints: a=%v b=%v", afp, bfp)
	}
	if afp["X-Capabilities"] != bfp["X-Capabilities"] {
		t.Fatal("equivalent multi-value headers should have the same fingerprint")
	}
}

func TestHeaderFingerprintChangesWhenValueChanges(t *testing.T) {
	a := HeaderFingerprints(http.Header{"X-Feature-Mode": []string{"normal"}})
	b := HeaderFingerprints(http.Header{"X-Feature-Mode": []string{"preview"}})
	if a["X-Feature-Mode"] == b["X-Feature-Mode"] {
		t.Fatal("different header values must not share a fingerprint")
	}
}
