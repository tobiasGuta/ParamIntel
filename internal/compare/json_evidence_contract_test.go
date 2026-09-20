package compare

import (
	"net/http"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestJSONEvidenceContract(t *testing.T) {
	t.Run("normal nested JSON", func(t *testing.T) {
		s := Snapshot(200, nil, []byte(`{"user":{"role":"member"},"items":[1,2],"ok":true}`))
		if !s.IsJSON {
			t.Fatal("valid nested JSON was not recognized")
		}
		if got := s.JSONPaths["$.user.role"]; got != "s:member" {
			t.Fatalf("role=%q want=s:member", got)
		}
		if got := s.JSONPaths["$.items.__len"]; got != "2" {
			t.Fatalf("array len=%q want=2", got)
		}
	})

	t.Run("duplicate keys retain legacy last-value semantics", func(t *testing.T) {
		s := Snapshot(200, nil, []byte(`{"role":"user","role":"admin","nested":{"level":1,"level":2}}`))
		if !s.IsJSON {
			t.Fatal("legacy encoding/json duplicate-key behavior must remain parseable")
		}
		if got := s.JSONPaths["$.role"]; got != "s:admin" {
			t.Fatalf("duplicate top-level key=%q want=s:admin", got)
		}
		if got := s.JSONPaths["$.nested.level"]; got != "n:2" {
			t.Fatalf("duplicate nested key=%q want=n:2", got)
		}
	})

	t.Run("invalid UTF-8 retains legacy replacement semantics", func(t *testing.T) {
		body := []byte{'{', '"', 'v', '"', ':', '"', 0xff, '"', '}'}
		s := Snapshot(200, nil, body)
		if !s.IsJSON {
			t.Fatal("legacy encoding/json invalid UTF-8 behavior must remain parseable")
		}
		if got := s.JSONPaths["$.v"]; got != "s:\ufffd" {
			t.Fatalf("invalid UTF-8 normalized as %q want replacement rune", got)
		}
	})

	t.Run("large integers remain distinct", func(t *testing.T) {
		a := Snapshot(200, nil, []byte(`{"user_id":9007199254740992}`))
		b := Snapshot(200, nil, []byte(`{"user_id":9007199254740992}`))
		p := BuildBaseline([]model.Snapshot{a, b})
		probe := Snapshot(200, nil, []byte(`{"user_id":9007199254740993}`))

		if got := a.JSONPaths["$.user_id"]; got != "n:9007199254740992" {
			t.Fatalf("baseline large integer=%q", got)
		}
		if got := probe.JSONPaths["$.user_id"]; got != "n:9007199254740993" {
			t.Fatalf("probe large integer=%q", got)
		}

		comparison := AgainstBaseline(p, probe)
		if !hasDifference(comparison.Differences, "json_value_changed", "$.user_id") {
			t.Fatalf("large integer change was lost: %+v", comparison.Differences)
		}
	})

	t.Run("high-precision decimals remain distinct", func(t *testing.T) {
		a := Snapshot(200, nil, []byte(`{"ratio":0.123456789012345678901234567890}`))
		b := Snapshot(200, nil, []byte(`{"ratio":0.123456789012345678901234567890}`))
		p := BuildBaseline([]model.Snapshot{a, b})
		probe := Snapshot(200, nil, []byte(`{"ratio":0.123456789012345678901234567891}`))

		comparison := AgainstBaseline(p, probe)
		if !hasDifference(comparison.Differences, "json_value_changed", "$.ratio") {
			t.Fatalf("high-precision decimal change was lost: %+v", comparison.Differences)
		}
	})

	t.Run("equivalent numeric spellings stay semantically equal", func(t *testing.T) {
		a := Snapshot(200, nil, []byte(`{"value":1}`))
		b := Snapshot(200, nil, []byte(`{"value":1.0}`))
		p := BuildBaseline([]model.Snapshot{a, b})
		probe := Snapshot(200, nil, []byte(`{"value":1e0}`))

		if got := a.JSONPaths["$.value"]; got != "n:1" {
			t.Fatalf("integer normalization=%q want=n:1", got)
		}
		if got := b.JSONPaths["$.value"]; got != "n:1" {
			t.Fatalf("decimal normalization=%q want=n:1", got)
		}
		if comparison := AgainstBaseline(p, probe); comparison.Meaningful {
			t.Fatalf("equivalent numeric spelling became evidence: %+v", comparison.Differences)
		}
	})

	t.Run("null empty object and empty array remain structural evidence", func(t *testing.T) {
		s := Snapshot(200, nil, []byte(`{"nil":null,"object":{},"array":[]}`))
		if !s.IsJSON {
			t.Fatal("valid JSON was not recognized")
		}
		if got := s.JSONPaths["$.nil"]; got != "null" {
			t.Fatalf("null=%q want=null", got)
		}
		if got := s.JSONPaths["$.object.__keys"]; got != "" {
			t.Fatalf("empty object keys=%q want empty", got)
		}
		if got := s.JSONPaths["$.array.__len"]; got != "0" {
			t.Fatalf("empty array len=%q want=0", got)
		}
	})

	t.Run("escaped Unicode and literal Unicode compare semantically", func(t *testing.T) {
		a := Snapshot(200, nil, []byte(`{"value":"a"}`))
		b := Snapshot(200, nil, []byte(`{"value":"\u0061"}`))
		p := BuildBaseline([]model.Snapshot{a, b})
		if comparison := AgainstBaseline(p, Snapshot(200, nil, []byte(`{"value":"a"}`))); comparison.Meaningful {
			t.Fatalf("equivalent Unicode spelling became evidence: %+v", comparison.Differences)
		}
	})

	t.Run("truncated JSON falls back to body evidence", func(t *testing.T) {
		a := Snapshot(200, nil, []byte(`{"role":"user"`))
		b := Snapshot(200, nil, []byte(`{"role":"user"`))
		if a.IsJSON || b.IsJSON {
			t.Fatal("truncated JSON must not be marked JSON")
		}
		p := BuildBaseline([]model.Snapshot{a, b})
		comparison := AgainstBaseline(p, Snapshot(200, nil, []byte(`{"role":"admin"`)))
		if !hasDifference(comparison.Differences, "body_changed", "") {
			t.Fatalf("malformed JSON did not use body fallback: %+v", comparison.Differences)
		}
	})

	t.Run("array order changes remain meaningful", func(t *testing.T) {
		p := BuildBaseline([]model.Snapshot{
			Snapshot(200, nil, []byte(`{"items":[1,2]}`)),
			Snapshot(200, nil, []byte(`{"items":[1,2]}`)),
		})
		comparison := AgainstBaseline(p, Snapshot(200, nil, []byte(`{"items":[2,1]}`)))
		if !comparison.Meaningful {
			t.Fatal("array order change should remain meaningful")
		}
		if !hasDifference(comparison.Differences, "json_value_changed", "$.items[0]") {
			t.Fatalf("missing ordered-array evidence: %+v", comparison.Differences)
		}
	})

	t.Run("misleading content type does not suppress JSON semantics", func(t *testing.T) {
		headers := http.Header{"Content-Type": []string{"text/plain"}}
		s := Snapshot(200, headers, []byte(`{"ok":true}`))
		if !s.IsJSON {
			t.Fatal("valid JSON body should be recognized independently of Content-Type")
		}
		if got := s.JSONPaths["$.ok"]; got != "b:true" {
			t.Fatalf("JSON path=%q want=b:true", got)
		}
	})

	t.Run("multiple top-level JSON values are rejected", func(t *testing.T) {
		s := Snapshot(200, nil, []byte(`{"a":1}{"b":2}`))
		if s.IsJSON {
			t.Fatal("multiple top-level JSON values must not be accepted")
		}
	})
}
