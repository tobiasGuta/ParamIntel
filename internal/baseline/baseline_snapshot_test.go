package baseline

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestBuildWithSnapshotReusesBaselineSampleWithoutExtraRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"request_id":%d,"filters":{"include_archived":false}}`, requests)
	}))
	defer srv.Close()

	profile, snapshot, err := BuildWithSnapshot(context.Background(), srv.Client(), model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests=%d want=3; snapshot reuse must not add target traffic", requests)
	}
	if profile.Samples != 3 || !profile.IsJSON {
		t.Fatalf("profile=%+v", profile)
	}
	if snapshot.StatusCode != http.StatusOK || len(snapshot.Body) == 0 || !snapshot.IsJSON {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if got := snapshot.JSONPaths["$.filters.include_archived"]; got != "b:false" {
		t.Fatalf("snapshot include_archived=%q", got)
	}
}
