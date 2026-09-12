package responsefeatures

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
)

// HeaderFingerprints returns deterministic hashes for response headers that
// are eligible for future evidence correlation. Raw values never leave the
// snapshot through this helper.
func HeaderFingerprints(headers http.Header) map[string]string {
	out := map[string]string{}
	for rawName, rawValues := range headers {
		name := http.CanonicalHeaderKey(strings.TrimSpace(rawName))
		if name == "" || ignoredHeader(name) {
			continue
		}

		values := make([]string, len(rawValues))
		for i, value := range rawValues {
			values[i] = strings.TrimSpace(value)
		}
		sort.Strings(values)
		sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
		out[name] = hex.EncodeToString(sum[:])
	}
	return out
}

func ignoredHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization",
		"proxy-authorization",
		"cookie",
		"set-cookie",
		"content-type",
		"content-length",
		"transfer-encoding",
		"connection",
		"keep-alive",
		"date",
		"server",
		"server-timing",
		"alt-svc",
		"via",
		"age",
		"etag",
		"last-modified",
		"cf-ray",
		"x-request-id",
		"x-correlation-id",
		"request-id",
		"traceparent",
		"tracestate",
		"x-amzn-trace-id",
		"x-cloud-trace-context":
		return true
	default:
		return false
	}
}
