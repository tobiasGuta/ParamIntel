package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/candidates"
	"github.com/tobiasGuta/ParamIntel/internal/discovery"
	"github.com/tobiasGuta/ParamIntel/internal/httpraw"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func main() {
	var reqPath, wordPath, outPath, scheme string
	var baselineN, chunk, trials int
	var timeout time.Duration
	var minConf float64
	var verbose bool
	flag.StringVar(&reqPath, "request", "", "raw HTTP request file (required)")
	flag.StringVar(&wordPath, "wordlist", "", "optional parameter wordlist")
	flag.StringVar(&outPath, "output", "", "JSON output path; stdout if empty")
	flag.StringVar(&scheme, "scheme", "https", "scheme for relative raw requests: http or https")
	flag.IntVar(&baselineN, "baseline", 3, "number of baseline requests")
	flag.IntVar(&chunk, "chunk", 64, "initial batch size")
	flag.IntVar(&trials, "trials", 3, "verification and negative-control trials")
	flag.Float64Var(&minConf, "min-confidence", 0.60, "minimum confidence to report")
	flag.DurationVar(&timeout, "timeout", 15*time.Second, "per-request timeout")
	flag.BoolVar(&verbose, "verbose", false, "show candidate verification and rejection diagnostics")
	flag.Parse()
	if reqPath == "" {
		fmt.Fprintln(os.Stderr, "error: -request is required")
		os.Exit(2)
	}
	raw, err := os.ReadFile(reqPath)
	fatal(err)
	tmpl, err := httpraw.Parse(raw, scheme)
	fatal(err)
	words, err := candidates.Load(wordPath)
	fatal(err)
	client := &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	ctx := context.Background()
	profile, err := baseline.Build(ctx, client, tmpl, baselineN)
	fatal(err)
	if verbose {
		fmt.Printf("[+] Baseline ready\n")
		fmt.Printf("    samples: %d\n", profile.Samples)
		fmt.Printf("    status: %d (stable=%t)\n", profile.StatusCode, profile.StatusStable)
		fmt.Printf("    body length: %d-%d bytes\n", profile.BodyLenMin, profile.BodyLenMax)
		fmt.Printf("    stable JSON paths: %d\n", len(profile.StableJSONPaths))
	}
	engine := discovery.Engine{Client: client, Config: discovery.Config{
		ChunkSize:     chunk,
		Trials:        trials,
		MinConfidence: minConf,
		Verbose:       verbose,
		Logf:          func(format string, args ...any) { fmt.Printf(format, args...) },
	}}
	params, err := engine.Scan(ctx, tmpl, profile, words)
	fatal(err)
	report := model.ScanReport{Target: tmpl.URL, Method: tmpl.Method, Baseline: model.BaselineSummary{Samples: profile.Samples, StableJSONPaths: len(profile.StableJSONPaths), BodyLenMin: profile.BodyLenMin, BodyLenMax: profile.BodyLenMax}, Parameters: params}
	b, err := json.MarshalIndent(report, "", "  ")
	fatal(err)
	b = append(b, '\n')
	if outPath != "" {
		fatal(os.WriteFile(outPath, b, 0600))
		fmt.Printf("wrote %s (%d parameters)\n", outPath, len(params))
		return
	}
	_, _ = os.Stdout.Write(b)
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
