//go:build queryfidelityspike

package mutate_test

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "net/url"
    "strings"
    "testing"

    "github.com/tobiasGuta/ParamIntel/internal/baseline"
    "github.com/tobiasGuta/ParamIntel/internal/model"
    "github.com/tobiasGuta/ParamIntel/internal/mutate"
)

type queryFidelityObservation struct {
    Name             string `json:"name"`
    OriginalRawQuery string `json:"original_raw_query"`
    ExpectedRawQuery string `json:"expected_raw_query"`
    MutatedRawQuery  string `json:"mutated_raw_query,omitempty"`
    ServerRequestURI string `json:"server_request_uri,omitempty"`
    ApplyError       string `json:"apply_error,omitempty"`
    SendError        string `json:"send_error,omitempty"`
    ExactMutation    bool   `json:"exact_mutation"`
    ExactWire        bool   `json:"exact_wire"`
}

// TestQuerySerializationFidelitySpike checks one narrow transport invariant:
//
// Adding a new ordinary query parameter must not rewrite, reorder, normalize,
// or drop the already-captured raw query bytes. Only the appended probe pair is
// allowed to be new.
//
// This is intentionally stricter than semantic url.Values equivalence. Replay
// fidelity matters for signed/mobile requests, cache keys, framework-specific
// parsing, duplicated parameters, and query formats that are not ordinary
// name/value maps.
func TestQuerySerializationFidelitySpike(t *testing.T) {
    cases := []struct {
        name string
        raw  string
    }{
        {
            name: "form_space_plus_already_canonical",
            raw:  "mode=basic&q=a+b",
        },
        {
            name: "literal_plus_percent_encoded",
            raw:  "mode=basic&q=a%2Bb",
        },
        {
            name: "rfc3986_space_percent20",
            raw:  "mode=basic&q=a%20b",
        },
        {
            name: "original_order_not_lexicographic",
            raw:  "q=search&mode=basic",
        },
        {
            name: "bare_flag_without_equals",
            raw:  "flag&mode=basic",
        },
        {
            name: "deep_object_brackets",
            raw:  "filter[role]=admin&mode=basic",
        },
        {
            name: "repeated_array_values",
            raw:  "tag=a&tag=b",
        },
        {
            name: "whole_query_percent_encoded_json",
            raw:  "%7B%22foo%22%3A%22a%20%2B%20b%22%7D",
        },
        {
            name: "literal_semicolon_in_value",
            raw:  "mode=basic&sig=abc;def",
        },
        {
            name: "lowercase_percent_triplet",
            raw:  "mode=basic&path=%2fprivate",
        },
        {
            name: "explicit_empty_value",
            raw:  "empty=&mode=basic",
        },
    }

    const probeName = "zz_pi_probe"
    const probeValue = "a + b"
    encodedProbe := url.QueryEscape(probeName) + "=" + url.QueryEscape(probeValue)

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            observedURI := make(chan string, 1)
            srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                observedURI <- r.RequestURI
                w.Header().Set("Content-Type", "application/json")
                _, _ = w.Write([]byte(`{"ok":true}`))
            }))
            defer srv.Close()

            expectedRaw := tc.raw
            if expectedRaw != "" {
                expectedRaw += "&"
            }
            expectedRaw += encodedProbe

            tmpl := model.RequestTemplate{
                Method:  http.MethodGet,
                URL:     srv.URL + "/probe?" + tc.raw,
                Headers: http.Header{"Accept": []string{"application/json"}},
            }
            mutation := model.Mutation{
                Candidate: model.Candidate{Name: probeName, Location: model.LocationQuery},
                Value:     model.StringValue(probeValue),
            }

            obs := queryFidelityObservation{
                Name:             tc.name,
                OriginalRawQuery: tc.raw,
                ExpectedRawQuery: expectedRaw,
            }

            mutated, err := mutate.Apply(tmpl, []model.Mutation{mutation})
            if err != nil {
                obs.ApplyError = err.Error()
            } else {
                parsed, parseErr := url.Parse(mutated.URL)
                if parseErr != nil {
                    obs.ApplyError = parseErr.Error()
                } else {
                    obs.MutatedRawQuery = parsed.RawQuery
                    obs.ExactMutation = parsed.RawQuery == expectedRaw
                }
            }

            if _, err := baseline.SendMutations(context.Background(), srv.Client(), tmpl, []model.Mutation{mutation}); err != nil {
                obs.SendError = err.Error()
            } else {
                select {
                case obs.ServerRequestURI = <-observedURI:
                default:
                    obs.SendError = "server did not record RequestURI"
                }
                obs.ExactWire = obs.ServerRequestURI == "/probe?"+expectedRaw
            }

            raw, err := json.Marshal(obs)
            if err != nil {
                t.Fatal(err)
            }
            t.Logf("QUERY_FIDELITY_JSON %s", raw)

            if obs.ApplyError != "" || obs.SendError != "" {
                t.Errorf("transport error: apply=%q send=%q", obs.ApplyError, obs.SendError)
                return
            }
            if !obs.ExactMutation || !obs.ExactWire {
                t.Errorf(
                    "raw query fidelity changed: original=%q expected=%q mutated=%q wire=%q",
                    tc.raw,
                    expectedRaw,
                    obs.MutatedRawQuery,
                    strings.TrimPrefix(obs.ServerRequestURI, "/probe?"),
                )
            }
        })
    }
}

func TestQueryProbeEncodingMatchesOASFormExamplePrimitive(t *testing.T) {
    got := url.QueryEscape("a + b")
    want := "a+%2B+b"
    if got != want {
        t.Fatalf("Go query probe encoding=%q want=%q", got, want)
    }
    t.Logf("QUERY_FIDELITY_ENCODING logical=%q serialized=%q", "a + b", got)
}

func Example_queryFidelityInvariant() {
    original := "mode=basic&q=a%20b"
    appended := original + "&zz_pi_probe=" + url.QueryEscape("a + b")
    fmt.Println(appended)
    // Output: mode=basic&q=a%20b&zz_pi_probe=a+%2B+b
}
