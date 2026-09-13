package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func requestSettings(r *http.Request) map[string]any {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil
	}
	profile, _ := body["profile"].(map[string]any)
	settings, _ := profile["settings"].(map[string]any)
	return settings
}

func writeState(w http.ResponseWriter, state string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"state":%q}`, state)
}

func main() {
	http.HandleFunc("/real", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		settings := requestSettings(r)
		if _, ok := settings["beta_access"]; ok {
			writeState(w, "beta")
			return
		}
		writeState(w, "normal")
	})

	http.HandleFunc("/noise", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		settings := requestSettings(r)
		if len(settings) > 0 {
			writeState(w, "settings-seen")
			return
		}
		writeState(w, "normal")
	})

	log.Println("ParamIntel v0.8 controlled JSON scaffolding lab listening on http://127.0.0.1:8094")
	log.Println("  POST /real  - beta_access is candidate-specific and must be confirmed")
	log.Println("  POST /noise - any child under the scaffold changes behavior; control must reject")
	log.Fatal(http.ListenAndServe("127.0.0.1:8094", nil))
}
