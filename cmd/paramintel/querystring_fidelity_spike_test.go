//go:build queryfidelityspike

package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "sync"
    "testing"
)

type querystringOperationObservation struct {
    BaselineURI        string   `json:"baseline_uri"`
    Requests           []string `json:"requests"`
    OrdinaryQueryProbe bool     `json:"ordinary_query_probe"`
}

// TestOpenAPIQuerystringOperationDoesNotFallThroughToGenericQueryDiscovery
// checks the integration boundary between passive OAS 3.2 querystring
// intelligence and the generic discovery engine.
//
// A schema-declared whole-query value is mutually exclusive with ordinary
// in: query parameters. The safest current behavior, before ParamIntel has a
// querystring serializer, is to withhold generic ordinary-query probes for that
// matched operation rather than reinterpret the captured whole query as a map.
func TestOpenAPIQuerystringOperationDoesNotFallThroughToGenericQueryDiscovery(t *testing.T) {
    const rawQuery = "%7B%22foo%22%3A%22a%20%2B%20b%22%7D"

    var mu sync.Mutex
    var seen []string
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        mu.Lock()
        seen = append(seen, r.RequestURI)
        mu.Unlock()
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"ok":true}`))
    }))
    defer srv.Close()

    parsed, err := url.Parse(srv.URL)
    if err != nil {
        t.Fatal(err)
    }

    tmp := t.TempDir()
    requestPath := filepath.Join(tmp, "request.txt")
    openAPIPath := filepath.Join(tmp, "openapi.yaml")
    wordlistPath := filepath.Join(tmp, "wordlist.txt")
    outputPath := filepath.Join(tmp, "findings.json")

    rawRequest := fmt.Sprintf(
        "GET %s/search?%s HTTP/1.1\r\nHost: %s\r\nAccept: application/json\r\nConnection: close\r\n\r\n",
        srv.URL,
        rawQuery,
        parsed.Host,
    )
    if err := os.WriteFile(requestPath, []byte(rawRequest), 0600); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(openAPIPath, []byte(querystringOperationSpec), 0600); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(wordlistPath, []byte("debug\n"), 0600); err != nil {
        t.Fatal(err)
    }

    cmd := exec.Command(
        "go", "run", ".",
        "-request", requestPath,
        "-scheme", "http",
        "-openapi", openAPIPath,
        "-wordlist", wordlistPath,
        "-baseline", "2",
        "-trials", "2",
        "-chunk", "8",
        "-characterize=false",
        "-value-aware=false",
        "-output", outputPath,
    )
    combined, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("ParamIntel CLI failed: %v\n%s", err, combined)
    }

    mu.Lock()
    requests := append([]string(nil), seen...)
    mu.Unlock()

    baselineURI := "/search?" + rawQuery
    ordinaryProbe := false
    for _, requestURI := range requests {
        if requestURI != baselineURI {
            ordinaryProbe = true
            break
        }
    }

    obs := querystringOperationObservation{
        BaselineURI:        baselineURI,
        Requests:           requests,
        OrdinaryQueryProbe: ordinaryProbe,
    }
    encoded, err := json.Marshal(obs)
    if err != nil {
        t.Fatal(err)
    }
    t.Logf("QUERYSTRING_OPERATION_JSON %s", encoded)

    if ordinaryProbe {
        t.Errorf("OAS 3.2 querystring operation fell through to generic ordinary-query discovery; requests=%s", strings.Join(requests, " | "))
    }
}

const querystringOperationSpec = "openapi: 3.2.1\n" +
    "info:\n" +
    "  title: whole query JSON fidelity\n" +
    "  version: \"1\"\n" +
    "paths:\n" +
    "  /search:\n" +
    "    get:\n" +
    "      parameters:\n" +
    "        - name: filters\n" +
    "          in: querystring\n" +
    "          content:\n" +
    "            application/json:\n" +
    "              schema:\n" +
    "                type: object\n" +
    "                properties:\n" +
    "                  foo:\n" +
    "                    type: string\n" +
    "      responses:\n" +
    "        '200':\n" +
    "          description: ok\n" +
    "          content:\n" +
    "            application/json:\n" +
    "              schema:\n" +
    "                type: object\n" +
    "                properties:\n" +
    "                  ok:\n" +
    "                    type: boolean\n"
