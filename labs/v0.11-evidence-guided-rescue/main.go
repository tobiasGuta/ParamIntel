package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const addr = "127.0.0.1:41782"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", handleItems)
	mux.HandleFunc("/search", handleSearch)
	mux.HandleFunc("/no-signal", handleNoSignal)
	mux.HandleFunc("/projects", handleProjects)

	server := &http.Server{Addr: addr, Handler: mux}
	fmt.Printf("ParamIntel v0.11 evidence-guided rescue lab listening on http://%s\n", addr)
	fmt.Printf("  context ranking:  http://%s/items\n", addr)
	fmt.Printf("  cost tie-breaker: http://%s/search\n", addr)
	fmt.Printf("  zero-signal:      http://%s/no-signal\n", addr)
	fmt.Printf("  Gemini value:     http://%s/projects\n", addr)
	log.Fatal(server.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// /items validates application-context prioritization.
//
// The wordlist deliberately places weaker semantic candidates before format.
// The baseline response advertises "supported_formats", so the v0.11 local
// relevance scorer should prioritize format. Only format=json changes behavior.
func handleItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]any{
		"items":             []map[string]any{{"id": "i1", "name": "Alpha"}},
		"supported_formats": []string{"json", "csv"},
	}
	if r.URL.Query().Get("format") == "json" {
		resp["selected_format"] = "json"
		resp["render_mode"] = "structured"
	}
	writeJSON(w, resp)
}

// /search validates the deterministic cost tie-breaker.
//
// debug and sort are both local semantic-profile candidates, but debug has four
// screen values while sort has two. With equal evidence and an eight-request
// budget, v0.11 should try sort first. Only sort=asc changes behavior.
func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]any{
		"results": []map[string]any{
			{"id": 2, "name": "Beta"},
			{"id": 1, "name": "Alpha"},
		},
	}
	if r.URL.Query().Get("sort") == "asc" {
		resp["results"] = []map[string]any{
			{"id": 1, "name": "Alpha"},
			{"id": 2, "name": "Beta"},
		}
		resp["sorted"] = true
	}
	writeJSON(w, resp)
}

// /no-signal is the negative control.
//
// None of the semantic candidates change the response. This scenario measures
// request cost honestly and exercises the verification-feasibility floor.
func handleNoSignal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"ok":     true,
		"status": "stable",
	})
}

// /projects validates the Gemini Semantic Value Advisor interaction.
//
// "visibility" has no built-in semantic profile. The baseline response provides
// application vocabulary, and only the application-specific value "internal"
// changes behavior. Gemini may propose the value; ParamIntel must still prove it
// using its ordinary paired control and repeated verification.
func handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]any{
		"projects": []map[string]any{
			{"id": "p1", "name": "Public project", "visibility": "public"},
			{"id": "p2", "name": "Private project", "visibility": "private"},
		},
		"available_visibilities": []string{"public", "private", "internal"},
	}
	if r.URL.Query().Get("visibility") == "internal" {
		resp["internal_projects"] = []map[string]any{
			{"id": "pi", "name": "Internal roadmap", "visibility": "internal"},
		}
		resp["mode"] = "internal"
	}
	writeJSON(w, resp)
}
