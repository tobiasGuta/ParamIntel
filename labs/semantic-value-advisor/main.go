package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const addr = "127.0.0.1:41781"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", handleProjects)
	server := &http.Server{Addr: addr, Handler: mux}
	fmt.Printf("ParamIntel Semantic Value Advisor lab listening on http://%s/projects\n", addr)
	log.Fatal(server.ListenAndServe())
}

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

	// The parameter name is discoverable, but generic random values and the
	// built-in semantic profiles do not trigger behavior. Only the
	// application-specific semantic value "internal" exposes the extra branch.
	if r.URL.Query().Get("visibility") == "internal" {
		resp["internal_projects"] = []map[string]any{
			{"id": "pi", "name": "Internal roadmap", "visibility": "internal"},
		}
		resp["mode"] = "internal"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
