package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const listenAddr = "127.0.0.1:8096"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/boolean-real", booleanReal)
	mux.HandleFunc("/boolean-noise", booleanNoise)
	mux.HandleFunc("/integer-real", integerReal)
	mux.HandleFunc("/union", unionCase)

	fmt.Printf("ParamIntel v0.9 schema-typed acceptance lab listening on http://%s\n", listenAddr)
	fmt.Println("endpoints: POST /boolean-real, POST /boolean-noise, POST /integer-real, POST /union")
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}

func booleanReal(w http.ResponseWriter, r *http.Request) {
	profile, ok := readProfile(w, r)
	if !ok {
		return
	}
	if raw, exists := profile["beta_access"]; exists {
		if enabled, ok := raw.(bool); ok && enabled {
			writeState(w, "beta")
			return
		}
	}
	writeState(w, "normal")
}

func booleanNoise(w http.ResponseWriter, r *http.Request) {
	profile, ok := readProfile(w, r)
	if !ok {
		return
	}
	for name, raw := range profile {
		if name == "name" {
			continue
		}
		if enabled, ok := raw.(bool); ok && enabled {
			writeState(w, "boolean-seen")
			return
		}
	}
	writeState(w, "normal")
}

func integerReal(w http.ResponseWriter, r *http.Request) {
	profile, ok := readProfile(w, r)
	if !ok {
		return
	}
	if raw, exists := profile["access_level"]; exists {
		if n, ok := raw.(json.Number); ok && n.String() == "1" {
			writeState(w, "level-one")
			return
		}
	}
	writeState(w, "normal")
}

func unionCase(w http.ResponseWriter, r *http.Request) {
	profile, ok := readProfile(w, r)
	if !ok {
		return
	}
	if raw, exists := profile["beta_access"]; exists {
		if enabled, ok := raw.(bool); ok && enabled {
			writeState(w, "beta")
			return
		}
	}
	writeState(w, "normal")
}

func readProfile(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return nil, false
	}
	var body map[string]any
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return nil, false
	}
	profile, _ := body["profile"].(map[string]any)
	return profile, true
}

func writeState(w http.ResponseWriter, state string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"state": state})
}
