package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/schemaintel"
)

const (
	openAPIWorkerArg          = "--internal-paramintel-openapi-worker"
	defaultOpenAPITimeout     = 15 * time.Second
	defaultOpenAPIMemoryMiB   = 768
	minOpenAPIMemoryMiB       = 256
	maxOpenAPIMemoryMiB       = 2048
	maxWorkerResponseBytes    = 8 << 20
	maxWorkerDiagnosticBytes  = 4096
	openAPIWorkerMemoryEnv    = "PARAMINTEL_INTERNAL_OPENAPI_MEMORY_MIB"
	openAPIWorkerCPUEnv       = "PARAMINTEL_INTERNAL_OPENAPI_CPU_SECONDS"
)

var ErrIsolatedOpenAPIUnavailable = errors.New("untrusted OpenAPI isolation is supported only on Linux")
var ErrIsolatedOpenAPIFailed = errors.New("isolated OpenAPI worker failed")

type openAPIWorkerRequest struct {
	Phase    string                 `json:"phase"`
	Spec     []byte                 `json:"spec"`
	Limit    int64                  `json:"limit"`
	Template model.RequestTemplate  `json:"template"`
	Baseline model.BaselineProfile  `json:"baseline"`
	Config   schemaintel.Config     `json:"config"`
}

type openAPIWorkerResponse struct {
	Version string              `json:"version,omitempty"`
	Report  *schemaintel.Report `json:"report,omitempty"`
	Error   string              `json:"error,omitempty"`
}

type openAPIIsolationPolicy struct {
	MaxBytes  int64
	MemoryMiB int
	Timeout   time.Duration
}

func (p openAPIIsolationPolicy) validate() error {
	if err := schemaintel.ValidateDocumentLimit(p.MaxBytes); err != nil {
		return err
	}
	if p.MemoryMiB < minOpenAPIMemoryMiB || p.MemoryMiB > maxOpenAPIMemoryMiB {
		return fmt.Errorf("openapi worker memory limit must be between %d and %d MiB", minOpenAPIMemoryMiB, maxOpenAPIMemoryMiB)
	}
	if p.Timeout < time.Second || p.Timeout > time.Minute {
		return fmt.Errorf("openapi worker deadline must be between 1s and 1m")
	}
	return nil
}

// cappedOutput prevents a child from returning unbounded data to the parent.
type cappedOutput struct {
	bytes.Buffer
	max int
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, fmt.Errorf("worker output exceeds %d bytes", b.max)
	}
	return b.Buffer.Write(p)
}

// runIsolatedOpenAPI never calls libopenapi in the parent process. Every
// phase runs in a separate subprocess, with a kernel-enforced data/mmap
// limit on Linux and a parent-enforced wall clock deadline.
func runIsolatedOpenAPI(ctx context.Context, req openAPIWorkerRequest, policy openAPIIsolationPolicy) (openAPIWorkerResponse, error) {
	if runtime.GOOS != "linux" {
		return openAPIWorkerResponse{}, ErrIsolatedOpenAPIUnavailable
	}
	if err := policy.validate(); err != nil {
		return openAPIWorkerResponse{}, err
	}
	if req.Phase != "validate" && req.Phase != "analyze" {
		return openAPIWorkerResponse{}, fmt.Errorf("invalid isolated openapi worker phase %q", req.Phase)
	}
	if req.Limit != policy.MaxBytes {
		return openAPIWorkerResponse{}, fmt.Errorf("isolated worker document limit mismatch")
	}
	if int64(len(req.Spec)) > policy.MaxBytes {
		return openAPIWorkerResponse{}, fmt.Errorf("%w: got %d bytes, limit %d", schemaintel.ErrDocumentTooLarge, len(req.Spec), policy.MaxBytes)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return openAPIWorkerResponse{}, fmt.Errorf("encode isolated parser input: %w", err)
	}
	// JSON base64 representation of the document can exceed its raw size.
	if len(raw) > int(2*schemaintel.MaxConfigurableDocumentBytes+1<<20) {
		return openAPIWorkerResponse{}, fmt.Errorf("isolated parser request is too large")
	}
	executable, err := os.Executable()
	if err != nil {
		return openAPIWorkerResponse{}, fmt.Errorf("locate parser worker executable: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, openAPIWorkerArg)
	cmd.Stdin = bytes.NewReader(raw)
	cpuSeconds := int64(policy.Timeout/time.Second) + 2
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%d", openAPIWorkerMemoryEnv, policy.MemoryMiB),
		fmt.Sprintf("%s=%d", openAPIWorkerCPUEnv, cpuSeconds),
		fmt.Sprintf("GOMEMLIMIT=%dMiB", policy.MemoryMiB/2),
		"GOGC=50",
	)
	stdout := &cappedOutput{max: maxWorkerResponseBytes}
	stderr := &cappedOutput{max: maxWorkerDiagnosticBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		return openAPIWorkerResponse{}, fmt.Errorf("%w: process timed out or was cancelled: %v", ErrIsolatedOpenAPIFailed, ctx.Err())
	}
	if err != nil {
		return openAPIWorkerResponse{}, fmt.Errorf("%w: %v (%s)", ErrIsolatedOpenAPIFailed, err, stderr.String())
	}
	var response openAPIWorkerResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return openAPIWorkerResponse{}, fmt.Errorf("%w: invalid worker response: %v", ErrIsolatedOpenAPIFailed, err)
	}
	if response.Error != "" {
		return openAPIWorkerResponse{}, fmt.Errorf("isolated openapi parser: %s", response.Error)
	}
	if response.Version == "" || (req.Phase == "analyze" && response.Report == nil) {
		return openAPIWorkerResponse{}, fmt.Errorf("%w: missing complete worker result", ErrIsolatedOpenAPIFailed)
	}
	return response, nil
}

// runOpenAPIWorker is entered only via the dedicated internal CLI argument.
// The input is a single bounded JSON message over stdin; it never fetches
// references or touches HTTP targets, and stdout carries only one response.
func runOpenAPIWorker() int {
	memory, err := strconv.Atoi(os.Getenv(openAPIWorkerMemoryEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "missing parser worker memory configuration")
		return 2
	}
	cpu, err := strconv.ParseInt(os.Getenv(openAPIWorkerCPUEnv), 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "missing parser worker CPU configuration")
		return 2
	}
	if err := applyOpenAPIWorkerLimits(memory, cpu); err != nil {
		fmt.Fprintln(os.Stderr, "cannot enforce parser worker limits:", err)
		return 2
	}
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 2*schemaintel.MaxConfigurableDocumentBytes+1<<20))
	decoder.DisallowUnknownFields()
	var req openAPIWorkerRequest
	if err := decoder.Decode(&req); err != nil {
		fmt.Fprintln(os.Stderr, "invalid isolated parser request:", err)
		return 2
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		fmt.Fprintln(os.Stderr, "isolated parser request must be exactly one object")
		return 2
	}
	var result openAPIWorkerResponse
	if req.Phase != "validate" && req.Phase != "analyze" {
		result.Error = "invalid parser worker phase"
	} else {
		doc, parseErr := schemaintel.ParseWithLimit(req.Spec, req.Limit)
		if parseErr != nil {
			result.Error = parseErr.Error()
		} else {
			result.Version = doc.Version()
			if req.Phase == "analyze" {
				report, analyzeErr := schemaintel.Analyze(doc, req.Template, req.Baseline, req.Config)
				if analyzeErr != nil {
					result.Error = analyzeErr.Error()
				} else {
					result.Report = &report
				}
			}
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, "cannot encode parser worker result:", err)
		return 2
	}
	return 0
}
