package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const listenAddr = "127.0.0.1:8095"

type requestBody struct {
	Profile map[string]any `json:"profile"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/real", realHandler)
	mux.HandleFunc("/noise", noiseHandler)
	mux.HandleFunc("/scaffold", scaffoldHandler)

	log.Printf("ParamIntel v0.9 OpenAPI acceptance lab listening on http://%s", listenAddr)
	log.Printf("endpoints: POST /real, POST /noise, POST /scaffold")
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}

func realHandler(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePOST(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, exists := body.Profile["beta_access"]; exists {
		writeJSON(w, map[string]any{
			"profile": map[string]any{
				"name":        "tobias",
				"beta_access": true,
			},
		})
		return
	}
	writeJSON(w, map[string]any{
		"profile": map[string]any{"name": "tobias"},
	})
}

func noiseHandler(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePOST(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	for key := range body.Profile {
		if key != "name" {
			writeJSON(w, map[string]any{
				"profile": map[string]any{
					"name":    "tobias",
					"changed": true,
				},
			})
			return
		}
	}
	writeJSON(w, map[string]any{
		"profile": map[string]any{"name": "tobias"},
	})
}

func scaffoldHandler(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePOST(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	beta := false
	if settings, ok := body.Profile["settings"].(map[string]any); ok {
		_, beta = settings["beta_access"]
	}
	writeJSON(w, map[string]any{
		"profile": map[string]any{
			"name": "tobias",
			"settings": map[string]any{
				"beta_access": beta,
			},
		},
	})
}

func decodePOST(w http.ResponseWriter, r *http.Request) (requestBody, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return requestBody{}, false
	}
	defer r.Body.Close()
	var body requestBody
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return requestBody{}, false
	}
	if body.Profile == nil {
		body.Profile = map[string]any{}
	}
	return body, true
}

func writeJSON(w http.ResponseWriter, value any) {
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
