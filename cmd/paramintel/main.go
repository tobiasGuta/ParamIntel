package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/aiadvisor"
	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/candidates"
	"github.com/tobiasGuta/ParamIntel/internal/contextintel"
	"github.com/tobiasGuta/ParamIntel/internal/decision"
	"github.com/tobiasGuta/ParamIntel/internal/discovery"
	"github.com/tobiasGuta/ParamIntel/internal/httppolicy"
	"github.com/tobiasGuta/ParamIntel/internal/httpraw"
	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/schemaintel"
)

const (
	version                  = "0.10.0"
	defaultAIProviderTimeout = 2 * time.Minute
)

func main() {
	var reqPath, wordPath, outPath, scheme, locationSpec, contextResponsePath, openAPIPath string
	var aiProviderName, aiModel, aiAPIKeyEnv, aiContextResponsePath string
	var decisionShadowCapturePath string
	var baselineN, chunk, trials, jsonDepth, valueAwareBudget, aiCandidateBudget, aiValueBudget, aiValueCandidateBudget, decisionShadowBudget int
	var timeout, delay, aiTimeout time.Duration
	var minConf float64
	var verbose, characterize, valueAware, allowStateChanging, showVersion, aiAdvisorEnabled, aiValueAdvisorEnabled, jsonScaffold bool

	flag.StringVar(&reqPath, "request", "", "raw HTTP request file (required)")
	flag.StringVar(&wordPath, "wordlist", "", "optional parameter wordlist")
	flag.StringVar(&contextResponsePath, "context-response", "", "optional related raw HTTP response or JSON body used to derive high-signal JSON candidates")
	flag.BoolVar(&jsonScaffold, "json-scaffold", false, "allow one-level response-derived JSON parent scaffolding from -context-response candidates")
	flag.StringVar(&openAPIPath, "openapi", "", "optional local OpenAPI 3.x document used to derive deterministic JSON candidates")
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

	flag.BoolVar(&aiAdvisorEnabled, "ai-advisor", false, "enable optional AI candidate hypothesis generation before discovery")
	flag.StringVar(&aiProviderName, "ai-provider", aiadvisor.ProviderGemini, "AI provider adapter (currently: gemini)")
	flag.StringVar(&aiModel, "ai-model", "", "AI model override; provider default if empty")
	flag.StringVar(&aiAPIKeyEnv, "ai-api-key-env", "", "environment variable containing the AI provider API key; provider default if empty")
	flag.StringVar(&aiContextResponsePath, "ai-context-response", "", "optional raw HTTP response or JSON body override for sanitized AI context; a collected baseline response is used by default")
	flag.IntVar(&aiCandidateBudget, "ai-candidate-budget", 12, "maximum AI-suggested candidates admitted to discovery")
	flag.BoolVar(&aiValueAdvisorEnabled, "ai-value-advisor", false, "enable bounded AI semantic value suggestions after clean generic/deterministic misses")
	flag.IntVar(&aiValueBudget, "ai-value-budget", 4, "maximum AI-suggested values admitted per candidate")
	flag.IntVar(&aiValueCandidateBudget, "ai-value-candidate-budget", 8, "maximum candidates submitted to the AI Semantic Value Advisor")
	flag.DurationVar(&aiTimeout, "ai-timeout", defaultAIProviderTimeout, "AI provider request timeout")

	flag.StringVar(&decisionShadowCapturePath, "decision-shadow-capture", "", "optional local JSONL path for sanitized residual states before semantic rescue; does not call a decision provider or alter scan behavior")
	flag.IntVar(&decisionShadowBudget, "decision-shadow-budget", 0, "maximum remaining planner request budget stored in shadow decision states; required with -decision-shadow-capture")

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
	if aiValueAdvisorEnabled && (!valueAware || valueAwareBudget == 0) {
		fatal(fmt.Errorf("-ai-value-advisor requires -value-aware=true and a positive -value-aware-budget"))
	}
	fatal(validateDelay(delay))
	fatal(validateAIOptions(aiAdvisorEnabled, aiValueAdvisorEnabled, aiCandidateBudget, aiValueBudget, aiValueCandidateBudget, aiTimeout))
	fatal(validateJSONScaffoldOptions(jsonScaffold, contextResponsePath))
	fatal(validateDecisionShadowOptions(decisionShadowCapturePath, decisionShadowBudget, outPath))

	locations, err := parseLocations(locationSpec)
	fatal(err)
	raw, err := os.ReadFile(reqPath)
	fatal(err)
	tmpl, err := httpraw.Parse(raw, scheme)
	fatal(err)
	if methodMayChangeState(tmpl.Method) && !allowStateChanging {
		fatal(fmt.Errorf("request method %s may be state-changing; confirm authorization and side-effect risk, then rerun with -allow-state-changing", tmpl.Method))
	}

	var openAPIDoc *schemaintel.Document
	if strings.TrimSpace(openAPIPath) != "" {
		openAPIRaw, err := os.ReadFile(openAPIPath)
		fatal(err)
		openAPIDoc, err = schemaintel.Parse(openAPIRaw)
		fatal(err)
	}

	words, err := candidates.Load(wordPath)
	fatal(err)

	var seeded []model.Candidate
	var contextRaw []byte
	if contextResponsePath != "" {
		contextRaw, err = os.ReadFile(contextResponsePath)
		fatal(err)
		contextReport, err := contextintel.HarvestJSONResponse(tmpl.Body, contextRaw, jsonDepth)
		fatal(err)
		seeded = append(seeded, contextReport.Actionable...)
		if jsonScaffold {
			seeded = append(seeded, contextReport.Scaffoldable...)
		}
		if verbose {
			fmt.Printf("[*] Context response intelligence\n")
			fmt.Printf("    observed JSON properties: %d\n", contextReport.ObservedProperties)
			fmt.Printf("    actionable response-only candidates: %d\n", len(contextReport.Actionable))
			fmt.Printf("    one-level scaffoldable candidates: %d\n", len(contextReport.Scaffoldable))
			fmt.Printf("    JSON scaffolding enabled: %t\n", jsonScaffold)
			fmt.Printf("    skipped already-present properties: %d\n", contextReport.SkippedExisting)
			fmt.Printf("    missing-parent properties not normally actionable: %d\n", contextReport.SkippedNoParent)
		}
	}

	ctx := context.Background()
	var aiProvider aiadvisor.Provider
	var aiOverrideRaw []byte
	if aiAdvisorEnabled || aiValueAdvisorEnabled {
		providerEnv := strings.TrimSpace(aiAPIKeyEnv)
		if providerEnv == "" {
			providerEnv, err = aiadvisor.DefaultAPIKeyEnv(aiProviderName)
			fatal(err)
		}
		apiKey := strings.TrimSpace(os.Getenv(providerEnv))
		if apiKey == "" {
			fatal(fmt.Errorf("AI provider %q requires an API key in environment variable %s", aiProviderName, providerEnv))
		}
		if aiContextResponsePath != "" {
			aiOverrideRaw, err = os.ReadFile(aiContextResponsePath)
			fatal(err)
		}
		aiClient := &http.Client{Timeout: aiTimeout}
		aiProvider, err = aiadvisor.NewProvider(aiadvisor.ProviderConfig{
			Provider: aiProviderName,
			APIKey:   apiKey,
			Model:    aiModel,
			Client:   aiClient,
		})
		fatal(err)
	}

	client := &http.Client{
		Timeout:       timeout,
		Transport:     httppolicy.NewPacedTransport(http.DefaultTransport, delay),
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	profile, baselineSnapshot, err := baseline.BuildWithSnapshot(ctx, client, tmpl, baselineN)
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

	if openAPIDoc != nil {
		openAPIReport, err := schemaintel.Analyze(openAPIDoc, tmpl, profile, schemaintel.DefaultConfig())
		fatal(err)
		openAPICandidates := schemaintel.ExistingParentCandidates(openAPIReport)
		seeded = append(seeded, openAPICandidates...)
		if verbose {
			scaffoldable := 0
			for _, descriptor := range openAPIReport.Candidates {
				if descriptor.Placement == schemaintel.PlacementOneLevelScaffold {
					scaffoldable++
				}
			}
			fmt.Printf("[*] OpenAPI candidate intelligence\n")
			fmt.Printf("    version: %s\n", openAPIReport.OpenAPIVersion)
			fmt.Printf("    operation: %s %s\n", openAPIReport.Operation.Method, openAPIReport.Operation.SpecPath)
			fmt.Printf("    request media type: %s\n", openAPIReport.RequestMediaType)
			fmt.Printf("    response: %s %s\n", openAPIReport.ResponseStatusKey, openAPIReport.ResponseMediaType)
			fmt.Printf("    response-only descriptors: %d\n", len(openAPIReport.Candidates))
			fmt.Printf("    existing-parent candidates admitted: %d\n", len(openAPICandidates))
			fmt.Printf("    scaffold descriptors withheld in Slice 2: %d\n", scaffoldable)
			fmt.Printf("    skipped schema properties: %d\n", len(openAPIReport.Skipped))
		}
	}

	var aiSummary *model.AIAdvisorSummary
	var aiValueSummary *model.AIValueAdvisorSummary
	var advisorInput aiadvisor.Input
	var aiContextRaw []byte
	var aiContextSource string
	if aiAdvisorEnabled || aiValueAdvisorEnabled {
		aiRaw, source := selectAIContext(baselineSnapshot, aiOverrideRaw, aiContextResponsePath != "")
		aiContextRaw = aiRaw
		aiContextSource = source
		advisorInput, err = aiadvisor.BuildInput(tmpl, aiRaw, locations, jsonDepth)
		fatal(err)
	}
	if aiAdvisorEnabled {
		// Only the static built-ins are shared with the provider as exclusions.
		// A user-supplied wordlist remains local, but all loaded deterministic
		// names participate in the local admission gate so AI cannot claim
		// coverage ParamIntel already had. AI candidates never receive scaffold
		// metadata; -json-scaffold only admits candidates classified from the
		// deterministic context-response path above.
		advisorInput.ExcludedCandidateNames = append([]string(nil), candidates.Builtin...)
		advisorInput.LocalCoveredNames = append([]string(nil), words...)
		advisorResult, err := aiadvisor.Generate(ctx, aiProvider, advisorInput, aiCandidateBudget)
		fatal(err)
		seeded = append(seeded, advisorResult.Candidates...)
		aiSummary = buildAIAdvisorSummary(advisorResult)
		aiSummary.ContextSource = aiContextSource
		if verbose {
			fmt.Printf("[*] AI Candidate Advisor\n")
			fmt.Printf("    provider: %s\n", advisorResult.Provider)
			fmt.Printf("    model: %s\n", advisorResult.Model)
			fmt.Printf("    input policy: sanitized structure only\n")
			fmt.Printf("    context source: %s\n", aiContextSource)
			fmt.Printf("    suggested candidates: %d\n", advisorResult.SuggestedCount)
			fmt.Printf("    accepted candidate hypotheses: %d\n", advisorResult.AcceptedCount)
		}
	}

	var semanticValueAdvisor discovery.SemanticValueAdvisor
	var semanticValuePriority discovery.SemanticValuePriority
	if aiValueAdvisorEnabled {
		aiValueSummary = &model.AIValueAdvisorSummary{
			Provider:      aiProvider.Name(),
			Model:         aiProvider.Model(),
			InputPolicy:   "sanitized structure, candidate metadata, and bounded enum-like semantic hints",
			ContextSource: aiContextSource,
		}
		remainingCandidates := aiValueCandidateBudget
		semanticValueAdvisor = func(ctx context.Context, candidate model.Candidate, deterministic []model.ProbeValue) ([]model.ProbeValue, error) {
			if remainingCandidates <= 0 {
				return nil, nil
			}
			remainingCandidates--
			excluded := make([]aiadvisor.ValueIdentity, 0, len(deterministic))
			for _, value := range deterministic {
				excluded = append(excluded, aiadvisor.ValueIdentity{Kind: value.Kind, Raw: value.Raw})
			}
			valueCandidate := aiadvisor.ValueCandidate{
				Name:       candidate.Name,
				Location:   candidate.Location,
				JSONParent: candidate.JSONParent,
			}
			valueInput := aiadvisor.ValueInput{
				Application:    advisorInput,
				Candidate:      valueCandidate,
				SemanticHints:  aiadvisor.BuildSemanticValueHints(aiContextRaw, valueCandidate, 12),
				ExcludedValues: excluded,
			}
			result, err := aiadvisor.GenerateValues(ctx, aiProvider, valueInput, aiValueBudget)
			if err != nil {
				return nil, err
			}
			aiValueSummary.CandidateQueries++
			aiValueSummary.SuggestedValues += result.SuggestedCount
			aiValueSummary.AcceptedValues += result.AcceptedCount
			if verbose {
				fmt.Printf("[*] AI Semantic Value Advisor: %s (%s)\n", candidate.Name, candidate.Location)
				fmt.Printf("    local relevance: %d\n", aiadvisor.ValueCandidateRelevance(advisorInput, valueCandidate))
				fmt.Printf("    semantic hints: %d\n", len(valueInput.SemanticHints))
				fmt.Printf("    suggested values: %d\n", result.SuggestedCount)
				fmt.Printf("    accepted value hypotheses: %d\n", result.AcceptedCount)
			}
			return result.Values, nil
		}
		semanticValuePriority = func(candidate model.Candidate) int {
			return aiadvisor.ValueCandidateRelevance(advisorInput, aiadvisor.ValueCandidate{
				Name:       candidate.Name,
				Location:   candidate.Location,
				JSONParent: candidate.JSONParent,
			})
		}
	}

	var decisionShadowObserver discovery.ResidualDecisionObserver
	var decisionShadowCaptureErr error
	decisionShadowCaptured := 0
	if strings.TrimSpace(decisionShadowCapturePath) != "" {
		decisionShadowObserver = func(result model.ParameterResult, remainingBudget int) {
			if decisionShadowCaptureErr != nil {
				return
			}
			effectiveBudget := remainingBudget
			if effectiveBudget > decisionShadowBudget {
				effectiveBudget = decisionShadowBudget
			}
			state := decision.StateFromParameterResult(result, effectiveBudget)
			if err := decision.AppendShadowCaptureJSONL(decisionShadowCapturePath, state); err != nil {
				decisionShadowCaptureErr = err
				fmt.Fprintf(os.Stderr, "warning: decision shadow capture disabled after write failure: %v\n", err)
				return
			}
			decisionShadowCaptured++
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
		ValueAwareBudget:     valueAwareBudget,
		SemanticValueAdvisor:      semanticValueAdvisor,
		SemanticValuePriority:     semanticValuePriority,
		ResidualDecisionObserver: decisionShadowObserver,
		JSONScaffold:              jsonScaffold,
	}}
	params, err := engine.ScanWithCandidates(ctx, tmpl, profile, words, seeded)
	fatal(err)
	if strings.TrimSpace(decisionShadowCapturePath) != "" && verbose {
		fmt.Printf("[*] Decision shadow capture\n")
		fmt.Printf("    policy: sanitized residual pre-semantic-rescue state only\n")
		fmt.Printf("    provider calls: 0\n")
		fmt.Printf("    captured records: %d\n", decisionShadowCaptured)
		fmt.Printf("    output: %s\n", decisionShadowCapturePath)
		if decisionShadowCaptureErr != nil {
			fmt.Printf("    warning: capture stopped after write failure: %v\n", decisionShadowCaptureErr)
		}
	}
	finalizeAIAdvisorSummary(aiSummary, params)
	if aiValueSummary != nil {
		for _, parameter := range params {
			if parameter.DiscoveryMode == "ai_value_aware" {
				aiValueSummary.VerifiedParameters++
			}
		}
	}
	if verbose && aiSummary != nil {
		printAIAdvisorAudit(aiSummary)
	}
	report := model.ScanReport{
		Version:    version,
		Target:     tmpl.URL,
		Method:     tmpl.Method,
		Baseline:   model.BaselineSummary{Samples: profile.Samples, StableJSONPaths: len(profile.StableJSONPaths), BodyLenMin: profile.BodyLenMin, BodyLenMax: profile.BodyLenMax},
		AIAdvisor:      aiSummary,
		AIValueAdvisor: aiValueSummary,
		Parameters:     params,
	}
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

func validateDelay(delay time.Duration) error {
	if delay < 0 {
		return fmt.Errorf("-delay must be 0 or greater")
	}
	return nil
}

func validateAIOptions(candidateEnabled, valueEnabled bool, candidateBudget, valueBudget, valueCandidateBudget int, timeout time.Duration) error {
	if !candidateEnabled && !valueEnabled {
		return nil
	}
	if candidateEnabled && (candidateBudget <= 0 || candidateBudget > 50) {
		return fmt.Errorf("-ai-candidate-budget must be between 1 and 50 when -ai-advisor is enabled")
	}
	if valueEnabled {
		if valueBudget <= 0 || valueBudget > 12 {
			return fmt.Errorf("-ai-value-budget must be between 1 and 12 when -ai-value-advisor is enabled")
		}
		if valueCandidateBudget <= 0 || valueCandidateBudget > 50 {
			return fmt.Errorf("-ai-value-candidate-budget must be between 1 and 50 when -ai-value-advisor is enabled")
		}
	}
	if timeout <= 0 {
		return fmt.Errorf("-ai-timeout must be greater than zero when an AI advisor is enabled")
	}
	return nil
}

func validateDecisionShadowOptions(capturePath string, budget int, outputPath string) error {
	capturePath = strings.TrimSpace(capturePath)
	if capturePath == "" {
		if budget != 0 {
			return fmt.Errorf("-decision-shadow-budget requires -decision-shadow-capture")
		}
		return nil
	}
	if budget < 1 || budget > 100 {
		return fmt.Errorf("-decision-shadow-budget must be between 1 and 100 when -decision-shadow-capture is enabled")
	}
	if strings.TrimSpace(outputPath) != "" && filepath.Clean(capturePath) == filepath.Clean(strings.TrimSpace(outputPath)) {
		return fmt.Errorf("-decision-shadow-capture must not use the same path as -output")
	}
	return nil
}

func validateJSONScaffoldOptions(enabled bool, contextResponsePath string) error {
	if !enabled {
		return nil
	}
	if strings.TrimSpace(contextResponsePath) == "" {
		return fmt.Errorf("-json-scaffold requires -context-response; missing JSON parents may only come from deterministic response-derived candidates")
	}
	return nil
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
