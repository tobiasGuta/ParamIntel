package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type paceRecorder struct {
	mu   sync.Mutex
	last time.Time
	n    int
}

func (p *paceRecorder) record(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	if p.last.IsZero() {
		log.Printf("[pace] request %d start", p.n)
	} else {
		log.Printf("[pace] request %d start gap=%s", p.n, now.Sub(p.last).Round(time.Millisecond))
	}
	p.last = now
}

func main() {
	var pace paceRecorder

	http.HandleFunc("/mid-scan", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) > 0 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"error":"active probes rate limited"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"items":["a","b"]}`)
	})

	http.HandleFunc("/asymmetric", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		options, _ := body["options"].(map[string]any)
		for name := range options {
			if strings.HasPrefix(name, "zz_pi_") {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = fmt.Fprint(w, `{"error":"control rate limited"}`)
				return
			}
		}
		if _, ok := options["include_deleted"]; ok {
			_, _ = fmt.Fprint(w, `{"items":["active-a","deleted-b"],"deleted_included":true}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"items":["active-a"]}`)
	})

	http.HandleFunc("/pace", func(w http.ResponseWriter, r *http.Request) {
		pace.record(time.Now())
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	})

	log.Println("ParamIntel v0.5 acceptance lab listening on http://127.0.0.1:8092")
	log.Fatal(http.ListenAndServe("127.0.0.1:8092", nil))
}
