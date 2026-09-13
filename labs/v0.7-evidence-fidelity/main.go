package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

var requestNumber uint64

func nextRequestID() string {
	n := atomic.AddUint64(&requestNumber, 1)
	return fmt.Sprintf("request-%08d", n)
}

func main() {
	http.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) {
		requestID := nextRequestID()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Rotates on every response and is intentionally on ParamIntel's ignored
		// response-header list, so it must never become evidence.
		w.Header().Set("X-Request-ID", requestID)
		if r.URL.Query().Has("verbose") {
			w.Header().Set("X-Debug-Mode", "enabled")
		}

		tag := "p"
		if r.URL.Query().Has("preview") {
			tag = "b"
		}
		// p and b are the same length. The fixed-width request ID keeps response
		// size stable while the body changes on every request, so preview must be
		// proven by HTML structure rather than body hash or length drift.
		_, _ = fmt.Fprintf(w, "<html><body><main><%s>%s</%s></main></body></html>", tag, requestID, tag)
	})

	http.HandleFunc("/noise", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if len(r.URL.Query()) > 0 {
			_, _ = fmt.Fprint(w, "unknown parameter changed the response")
			return
		}
		_, _ = fmt.Fprint(w, "baseline response")
	})

	http.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		role := "member"
		if _, ok := requestBody["role"]; ok {
			role = "admin"
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"request_id": nextRequestID(),
			"user": map[string]any{
				"role": role,
			},
		})
	})

	log.Println("ParamIntel v0.7 evidence-fidelity lab listening on http://127.0.0.1:8093")
	log.Println("  GET  /html  - same-size HTML structure + header-only signals")
	log.Println("  GET  /noise - shared unknown-parameter noise; controls must reject")
	log.Println("  POST /json  - existing JSON semantic evidence regression")
	log.Fatal(http.ListenAndServe("127.0.0.1:8093", nil))
}
