package baseline

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestSendPreservesExistingQueryAndAddsProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test" {
			t.Fatalf("authorization=%q", got)
		}
		fmt.Fprintf(w, `{"existing":%q,"probe":%q}`, r.URL.Query().Get("existing"), r.URL.Query().Get("probe"))
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL + "?existing=1", Headers: http.Header{"Authorization": []string{"Bearer test"}}}
	s, err := Send(context.Background(), srv.Client(), tmpl, map[string]string{"probe": "yes"})
	if err != nil {
		t.Fatal(err)
	}
	if string(s.Body) != `{"existing":"1","probe":"yes"}` {
		t.Fatalf("body=%s", s.Body)
	}
}

func TestBuildLearnsJSONBaseline(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		fmt.Fprintf(w, `{"request_id":%d,"role":"member"}`, n)
	}))
	defer srv.Close()

	p, err := Build(context.Background(), srv.Client(), model.RequestTemplate{Method: "GET", URL: srv.URL}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if p.Samples != 3 || !p.IsJSON {
		t.Fatalf("profile=%+v", p)
	}
	if _, ok := p.StableJSONPaths["$.request_id"]; ok {
		t.Fatal("dynamic request_id should not be stable")
	}
	if got := p.StableJSONPaths["$.role"]; got != "s:member" {
		t.Fatalf("stable role=%q", got)
	}
}
