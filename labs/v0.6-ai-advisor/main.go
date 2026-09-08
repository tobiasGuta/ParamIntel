package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const addr = "127.0.0.1:41780"

type project struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

type response struct {
	Projects      []project       `json:"projects"`
	Filters       map[string]bool `json:"filters"`
	ArchivedCount int             `json:"archived_count"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", handleProjects)
	server := &http.Server{Addr: addr, Handler: mux}
	fmt.Printf("ParamIntel v0.6 AI advisor lab listening on http://%s/projects\n", addr)
	log.Fatal(server.ListenAndServe())
}

func handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, includeArchived := r.URL.Query()["include_archived"]
	projects := []project{{ID: "p_active", Name: "Current project", Archived: false}}
	if includeArchived {
		projects = append(projects, project{ID: "p_archived", Name: "Archived project", Archived: true})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{
		Projects:      projects,
		Filters:       map[string]bool{"include_archived": includeArchived},
		ArchivedCount: 1,
	})
}
