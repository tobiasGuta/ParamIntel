package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/candidates"
	"github.com/tobiasGuta/ParamIntel/internal/contextintel"
	"github.com/tobiasGuta/ParamIntel/internal/discovery"
	"github.com/tobiasGuta/ParamIntel/internal/httppolicy"
	"github.com/tobiasGuta/ParamIntel/internal/httpraw"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const version = "0.4.0"

func main() {
	var reqPath, wordPath, outPath, scheme, locationSpec, contextResponsePath string
	var baselineN, chunk, trials, jsonDepth, valueAwareBudget int
	var timeout, delay time.Duration
	var minConf float64
	var verbose, characterize, valueAware, allowStateChanging, showVersion bool
	flag.StringVar(&reqPath, "request", "", "raw HTTP request file (required)")
	flag.StringVar(&wordPath, "wordlist", "", "optional parameter wordlist")
	flag.StringVar(&contextResponsePath, "context-response", "", "optional related raw HTTP response or JSON body used to derive high-signal JSON candidates")
	flag.StringVar(&outPath, "output", "", "JSON output path; stdout if empty")
	flag.StringVar(&scheme, "scheme", "https", "scheme for relative raw requests: http or https")
	flag.StringVar(&locationSpec, "locations", "auto", "discovery locations: auto or comma-separated query,form,json")
	flag.IntVar(&baselineN, "baseline", 3, "number of baseline requests")
	flag.IntVar(&chunk, "chunk", 64, "initial batch size")
	flag.IntVar(&trials, "trials", 3, "verification and negative-control trials")
	flag.IntVar(&jsonDepth, "json-depth", 3, "maximum nested JSON object depth to probe")
	flag.IntVar(&valueAwareBudget, "value-aware-budget", 64, "maximum additional requests used by value-aware rescue")
	flag.Float64Var(&minConf, "min-confidence", 0.60, "minimum confidence to report")
	flag.DurationVar(&timeout, "timeout", 15*time.Second, "per-request timeout")
	flag.DurationVar(&delay, "delay", 0, "minimum delay between outbound request starts")
	flag.BoolVar(&verbose, "verbose", false, "show candidate verification and rejection diagnostics")
	flag.BoolVar(&characterize, "characterize", true, "profile likely values and infer parameter types after discovery")
	flag.BoolVar(&valueAware, "value-aware", true, "rescue value-sensitive parameters with bounded semantic probes")
	flag.BoolVar(&allowStateChanging, "allow-state-changing", false, "allow repeated probing of POST/PUT/PATCH/DELETE requests after confirming authorization and side-effect risk")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	if showVersion {
		fmt.Printf("ParamIntel v%s\n", version)
		return
	}
	if reqPath == "" {
		fmt.Fprintln(os.Stderr, "error: -request is required")
		os.Exit(2)
	}
	if valueAwareBudget < 0 {
		fatal(fmt.Errorf("-value-aware-budget must be 0 or greater"))
	}
	if delay < 0 {
		fatal(fmt.Errorf("-delay must be 0 or greater"))
	}
	locations, err := parseLocations(locationSpec)
	fatal(err)
	raw, err := os.ReadFile(reqPath)
	fatal(err)
	tmpl, err := httpraw.Parse(raw, scheme)
	fatal(err)
	if methodMayChangeState(tmpl.Method) && !allowStateChanging {
		fatal(fmt.Errorf("request method %s may be state-changing; confirm authorization and side-effect risk, then rerun with -allow-state-changing", tmpl.Method))
	}
	words, err := candidates.Load(wordPath)
	fatal(err)

	var seeded []model.Candidate
	if contextResponsePath != "" {
		contextRaw, err := os.ReadFile(contextResponsePath)
		fatal(err)
		contextReport, err := contextintel.HarvestJSONResponse(tmpl.Body, contextRaw, jsonDepth)
		fatal(err)
		seeded = contextReport.Actionable
		if verbose {
			fmt.Printf("[*] Context response intelligence\n")
			fmt.Printf("    observed JSON properties: %d\n", contextReport.ObservedProperties)
			fmt.Printf("    actionable response-only candidates: %d\n", len(contextReport.Actionable))
			fmt.Printf("    skipped already-present properties: %d\n", contextReport.SkippedExisting)
			fmt.Printf("    skipped candidates with missing request parent: %d\n", contextReport.SkippedNoParent)
		}
	}

	client := &http.Client{
		Timeout:       timeout,
		Transport:     httppolicy.NewPacedTransport(http.DefaultTransport, delay),
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	ctx := context.Background()
	profile, err := baseline.Build(ctx, client, tmpl, baselineN)
	fatal(err)
	if verbose {
		fmt.Printf("[+] Baseline ready\n")
		fmt.Printf("    samples: %d\n", profile.Samples)
		fmt.Printf("    status: %d (stable=%t)\n", profile.StatusCode, profile.StatusStable)
		fmt.Printf("    body length: %d-%d bytes\n", profile.BodyLenMin, profile.BodyLenMax)
		fmt.Printf("    stable JSON paths: %d\n", len(profile.StableJSONPaths))
		if delay > 0 {
			fmt.Printf("[*] Request pacing: minimum %s between request starts\n", delay)
		}
	}
	engine := discovery.Engine{Client: client, Config: discovery.Config{
		ChunkSize:        chunk,
		Trials:           trials,
		MinConfidence:    minConf,
		Verbose:          verbose,
		Logf:             func(format string, args ...any) { fmt.Printf(format, args...) },
		Locations:        locations,
		MaxJSONDepth:     jsonDepth,
		Characterize:     characterize,
		ValueAware:       valueAware,
		ValueAwareBudget: valueAwareBudget,
	}}
	params, err := engine.ScanWithCandidates(ctx, tmpl, profile, words, seeded)
	fatal(err)
	report := model.ScanReport{Version: version, Target: tmpl.URL, Method: tmpl.Method, Baseline: model.BaselineSummary{Samples: profile.Samples, StableJSONPaths: len(profile.StableJSONPaths), BodyLenMin: profile.BodyLenMin, BodyLenMax: profile.BodyLenMax}, Parameters: params}
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

func parseLocations(spec string) ([]string, error) {
	if strings.TrimSpace(spec) == "" || strings.EqualFold(strings.TrimSpace(spec), "auto") {
		return []string{"auto"}, nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range strings.Split(spec, ",") {
		location := strings.ToLower(strings.TrimSpace(raw))
		switch location {
		case model.LocationQuery, model.LocationForm, model.LocationJSON:
		default:
			return nil, fmt.Errorf("invalid -locations value %q; use auto, query, form, or json", location)
		}
		if _, ok := seen[location]; ok {
			continue
		}
		seen[location] = struct{}{}
		out = append(out, location)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no discovery locations selected")
	}
	return out, nil
}

func methodMayChangeState(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
