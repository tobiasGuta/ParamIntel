package discovery

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const (
	// Additional unchanged baseline requests per anomalous probe.
	driftConfirmationSamples = 2
	// Bound the *total* additional requests per scan. A target that emits
	// repeated 401/403 responses cannot drive unlimited baseline rechecks.
	driftMaxRecheckRequests = 12
)

type driftRecheckBudget struct {
	remaining int
}

// BaselineStatusDriftError is returned instead of trusting HTTP 401/403 probe
// evidence when the operator expected a stable 2xx response. It never embeds
// the captured URL, auth headers, request body, or response body.
type BaselineStatusDriftError struct {
	ExpectedStatus int
	ProbeStatus    int
	RecheckStatus  []int
	Reason         string
}

func (e *BaselineStatusDriftError) Error() string {
	return fmt.Sprintf(
		"baseline status drift (%s): expected HTTP %d, probe HTTP %d, clean baseline rechecks %v; scan stopped without a findings report",
		e.Reason, e.ExpectedStatus, e.ProbeStatus, e.RecheckStatus,
	)
}

// sendProbe is used at every scanner probe stage: group screening, repeated
// candidate/control verification, semantic rescue, and characterization.
//
// Recheck the *unmodified* captured request only after an auth-like status
// is observed. If clean requests still receive the expected status, the
// original candidate-specific 401/403 remains admissible to normal repeated
// candidate/control verification. This check does not establish identity.
func (e Engine) sendProbe(
	ctx context.Context,
	tmpl model.RequestTemplate,
	p model.BaselineProfile,
	mutations []model.Mutation,
) (model.Snapshot, error) {
	s, err := baseline.SendMutations(ctx, e.Client, tmpl, mutations)
	if err != nil {
		return model.Snapshot{}, err
	}
	expected := e.Config.ExpectedBaselineStatus
	if expected < 200 || expected > 299 ||
		!p.StatusStable || p.StatusCode != expected ||
		(s.StatusCode != http.StatusUnauthorized && s.StatusCode != http.StatusForbidden) {
		return s, nil
	}

	evidence := &BaselineStatusDriftError{
		ExpectedStatus: expected,
		ProbeStatus:    s.StatusCode,
	}

	// Automatically replaying unmodified POST/PUT/PATCH/DELETE can generate
	// additional side effects. Fail closed instead of sending hidden requests.
	switch strings.ToUpper(strings.TrimSpace(tmpl.Method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		evidence.Reason = "unsafe_to_recheck"
		return model.Snapshot{}, evidence
	}

	if e.driftBudget != nil {
		if e.driftBudget.remaining < driftConfirmationSamples {
			evidence.Reason = "recheck_budget_exhausted"
			return model.Snapshot{}, evidence
		}
		e.driftBudget.remaining -= driftConfirmationSamples
	}

	for i := 0; i < driftConfirmationSamples; i++ {
		clean, recheckErr := baseline.SendMutations(ctx, e.Client, tmpl, nil)
		if recheckErr != nil {
			// Preserve the existing typed 429/503 backoff errors. Never treat
			// rate-limit responses as valid baseline or candidate evidence.
			return model.Snapshot{}, fmt.Errorf("clean baseline recheck %d: %w", i+1, recheckErr)
		}
		evidence.RecheckStatus = append(evidence.RecheckStatus, clean.StatusCode)
	}

	if evidence.RecheckStatus[0] == expected && evidence.RecheckStatus[1] == expected {
		return s, nil
	}
	if (evidence.RecheckStatus[0] == http.StatusUnauthorized || evidence.RecheckStatus[0] == http.StatusForbidden) &&
		(evidence.RecheckStatus[1] == http.StatusUnauthorized || evidence.RecheckStatus[1] == http.StatusForbidden) {
		evidence.Reason = "confirmed"
	} else {
		evidence.Reason = "ambiguous"
	}
	return model.Snapshot{}, evidence
}
