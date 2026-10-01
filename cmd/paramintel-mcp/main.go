package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tobiasGuta/ParamIntel/internal/aiadvisor"
	"github.com/tobiasGuta/ParamIntel/internal/externalhints"
	"github.com/tobiasGuta/ParamIntel/internal/httpraw"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const (
	mcpServerVersion   = "0.1.0"
	maxRequestFileSize = 2 * 1024 * 1024
)

type inspectRequestInput struct {
	RequestPath string `json:"request_path" jsonschema:"Path to a raw HTTP request file under the configured ParamIntel MCP request root."`
	Scheme      string `json:"scheme,omitempty" jsonschema:"Scheme for a relative raw request: http or https. Defaults to https."`
}

type inspectRequestOutput struct {
	Method           string         `json:"method"`
	Path             string         `json:"path"`
	ActiveLocations  []string       `json:"active_locations"`
	QueryKeys        []string       `json:"query_keys,omitempty"`
	FormKeys         []string       `json:"form_keys,omitempty"`
	JSONParents      []string       `json:"json_parents,omitempty"`
	RequestJSONShape map[string]any `json:"request_json_shape,omitempty"`
}

type analyzeRequestInput struct {
	RequestPath        string                 `json:"request_path" jsonschema:"Path to a raw HTTP request file under the configured ParamIntel MCP request root."`
	Hints              externalhints.Document `json:"hints" jsonschema:"Bounded semantic parameter and value hypotheses. ParamIntel treats them only as hypotheses and independently verifies behavior."`
	Scheme             string                 `json:"scheme,omitempty" jsonschema:"Scheme for a relative raw request: http or https. Defaults to https."`
	Locations          []string               `json:"locations,omitempty" jsonschema:"Optional discovery locations: query, form, json. Empty uses ParamIntel auto-detection."`
	AllowStateChanging bool                   `json:"allow_state_changing,omitempty" jsonschema:"Explicit confirmation for repeated probing of a state-changing request. The MCP server operator must also opt in through PARAMINTEL_MCP_ALLOW_STATE_CHANGING=1."`
}

type analyzeRequestOutput struct {
	Report           model.ScanReport `json:"report"`
	InternalAIUsed   bool             `json:"internal_ai_used"`
	HintSource       string           `json:"hint_source"`
}

func inspectRequestFile(_ context.Context, _ *mcp.CallToolRequest, input inspectRequestInput) (*mcp.CallToolResult, inspectRequestOutput, error) {
	path, err := resolveRequestPath(input.RequestPath)
	if err != nil {
		return nil, inspectRequestOutput{}, err
	}
	scheme, err := normalizeScheme(input.Scheme)
	if err != nil {
		return nil, inspectRequestOutput{}, err
	}
	raw, err := readBoundedRequest(path)
	if err != nil {
		return nil, inspectRequestOutput{}, err
	}
	tmpl, err := httpraw.Parse(raw, scheme)
	if err != nil {
		return nil, inspectRequestOutput{}, fmt.Errorf("parse raw HTTP request: %w", err)
	}
	structure, err := aiadvisor.BuildInput(tmpl, nil, []string{"auto"}, 3)
	if err != nil {
		return nil, inspectRequestOutput{}, fmt.Errorf("sanitize request structure: %w", err)
	}
	return nil, inspectRequestOutput{
		Method:           structure.Method,
		Path:             structure.Path,
		ActiveLocations:  structure.ActiveLocations,
		QueryKeys:        structure.QueryKeys,
		FormKeys:         structure.FormKeys,
		JSONParents:      structure.JSONParents,
		RequestJSONShape: structure.RequestJSONShape,
	}, nil
}

func analyzeRequestFile(ctx context.Context, _ *mcp.CallToolRequest, input analyzeRequestInput) (*mcp.CallToolResult, analyzeRequestOutput, error) {
	path, err := resolveRequestPath(input.RequestPath)
	if err != nil {
		return nil, analyzeRequestOutput{}, err
	}
	scheme, err := normalizeScheme(input.Scheme)
	if err != nil {
		return nil, analyzeRequestOutput{}, err
	}
	locationSpec, err := normalizeLocations(input.Locations)
	if err != nil {
		return nil, analyzeRequestOutput{}, err
	}

	raw, err := readBoundedRequest(path)
	if err != nil {
		return nil, analyzeRequestOutput{}, err
	}
	tmpl, err := httpraw.Parse(raw, scheme)
	if err != nil {
		return nil, analyzeRequestOutput{}, fmt.Errorf("parse raw HTTP request: %w", err)
	}
	stateChanging := methodMayChangeState(tmpl.Method)
	if stateChanging {
		if !input.AllowStateChanging {
			return nil, analyzeRequestOutput{}, fmt.Errorf("request method %s may change state; explicit allow_state_changing confirmation is required", tmpl.Method)
		}
		if os.Getenv("PARAMINTEL_MCP_ALLOW_STATE_CHANGING") != "1" {
			return nil, analyzeRequestOutput{}, fmt.Errorf("state-changing MCP execution is disabled by the server operator")
		}
	}

	hintsRaw, err := json.Marshal(input.Hints)
	if err != nil {
		return nil, analyzeRequestOutput{}, fmt.Errorf("encode semantic hints: %w", err)
	}
	if _, err := externalhints.Parse(hintsRaw); err != nil {
		return nil, analyzeRequestOutput{}, err
	}

	tmpDir, err := os.MkdirTemp("", "paramintel-mcp-*")
	if err != nil {
		return nil, analyzeRequestOutput{}, fmt.Errorf("create temporary MCP workspace: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	hintsPath := filepath.Join(tmpDir, "hints.json")
	reportPath := filepath.Join(tmpDir, "report.json")
	if err := os.WriteFile(hintsPath, hintsRaw, 0o600); err != nil {
		return nil, analyzeRequestOutput{}, fmt.Errorf("write temporary hints: %w", err)
	}

	args := []string{
		"-request", path,
		"-hints", hintsPath,
		"-output", reportPath,
		"-scheme", scheme,
	}
	if locationSpec != "" {
		args = append(args, "-locations", locationSpec)
	}
	if stateChanging {
		args = append(args, "-allow-state-changing")
	}

	bin := strings.TrimSpace(os.Getenv("PARAMINTEL_MCP_BIN"))
	if bin == "" {
		bin = "paramintel"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := boundedDiagnostic(stderr.String())
		if detail == "" {
			detail = boundedDiagnostic(stdout.String())
		}
		if detail == "" {
			return nil, analyzeRequestOutput{}, fmt.Errorf("ParamIntel execution failed: %w", err)
		}
		return nil, analyzeRequestOutput{}, fmt.Errorf("ParamIntel execution failed: %w: %s", err, detail)
	}

	reportRaw, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, analyzeRequestOutput{}, fmt.Errorf("read ParamIntel report: %w", err)
	}
	var report model.ScanReport
	if err := json.Unmarshal(reportRaw, &report); err != nil {
		return nil, analyzeRequestOutput{}, fmt.Errorf("decode ParamIntel report: %w", err)
	}
	return nil, analyzeRequestOutput{
		Report:         report,
		InternalAIUsed: false,
		HintSource:     externalhints.SourceExternalSemanticHint,
	}, nil
}

func resolveRequestPath(requestPath string) (string, error) {
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		return "", fmt.Errorf("request_path is required")
	}

	root := strings.TrimSpace(os.Getenv("PARAMINTEL_MCP_REQUEST_ROOT"))
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve MCP request root: %w", err)
		}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve MCP request root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve MCP request root: %w", err)
	}
	rootInfo, err := os.Stat(rootReal)
	if err != nil {
		return "", fmt.Errorf("stat MCP request root: %w", err)
	}
	if !rootInfo.IsDir() {
		return "", fmt.Errorf("configured MCP request root is not a directory")
	}

	pathAbs := requestPath
	if !filepath.IsAbs(pathAbs) {
		pathAbs = filepath.Join(rootReal, pathAbs)
	}
	pathAbs, err = filepath.Abs(pathAbs)
	if err != nil {
		return "", fmt.Errorf("resolve request_path: %w", err)
	}
	pathReal, err := filepath.EvalSymlinks(pathAbs)
	if err != nil {
		return "", fmt.Errorf("resolve request_path: %w", err)
	}
	rel, err := filepath.Rel(rootReal, pathReal)
	if err != nil {
		return "", fmt.Errorf("compare request_path with MCP request root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("request_path is outside the configured MCP request root")
	}
	info, err := os.Stat(pathReal)
	if err != nil {
		return "", fmt.Errorf("stat request_path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("request_path must identify a regular file")
	}
	return pathReal, nil
}

func readBoundedRequest(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open request_path: %w", err)
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, maxRequestFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read request_path: %w", err)
	}
	if len(raw) > maxRequestFileSize {
		return nil, fmt.Errorf("request file exceeds %d bytes", maxRequestFileSize)
	}
	return raw, nil
}

func normalizeScheme(scheme string) (string, error) {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme == "" {
		return "https", nil
	}
	switch scheme {
	case "http", "https":
		return scheme, nil
	default:
		return "", fmt.Errorf("invalid scheme %q; use http or https", scheme)
	}
}

func normalizeLocations(locations []string) (string, error) {
	if len(locations) == 0 {
		return "", nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(locations))
	for _, raw := range locations {
		location := strings.ToLower(strings.TrimSpace(raw))
		switch location {
		case "query", "form", "json":
		default:
			return "", fmt.Errorf("invalid discovery location %q; use query, form, or json", location)
		}
		if _, ok := seen[location]; ok {
			continue
		}
		seen[location] = struct{}{}
		out = append(out, location)
	}
	return strings.Join(out, ","), nil
}

func methodMayChangeState(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func boundedDiagnostic(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const maxRunes = 1000
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes])
	}
	return value
}

func newMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "paramintel",
		Version: mcpServerVersion,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "inspect_request_file",
		Description: "Read a local raw HTTP request from the configured request root and return only sanitized structure for semantic reasoning. This tool does not send a target request and omits headers, cookies, authorization values, query/form values, JSON primitive values, hostnames, and raw response text.",
	}, inspectRequestFile)

	mcp.AddTool(server, &mcp.Tool{
		Name: "analyze_request_file",
		Description: "Run ParamIntel against an authorized raw HTTP request using externally supplied semantic parameter/value hypotheses. The hints are never treated as findings: ParamIntel still performs its normal live verification, repeated trials, paired random-name controls, confidence checks, and evidence collection. This tool sends HTTP requests to the captured target.",
	}, analyzeRequestFile)

	return server
}

func main() {
	if err := newMCPServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
