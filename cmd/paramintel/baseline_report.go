package main

import "github.com/tobiasGuta/ParamIntel/internal/model"

const (
	statusExpectationNotConfigured = "not_configured"
	statusExpectationMatched       = "matched"
	statusExpectationMismatched    = "mismatched"
	statusExpectationUnstable      = "unstable"
)

// buildBaselineSummary keeps the operator's declared HTTP expectation distinct
// from the sampled response status and stability. It does not infer login state
// and never includes raw headers, cookies, or response bodies in this field.
// A mismatch or an unstable expected status is rejected earlier by
// checkBaselineStatusExpectation and cannot become a normal findings report.
func buildBaselineSummary(p model.BaselineProfile, expectedStatus int) model.BaselineSummary {
	evidence := model.BaselineStatusEvidence{
		FirstObservedStatusCode: p.StatusCode,
		StatusStable:            p.StatusStable,
		Expectation:             statusExpectationNotConfigured,
	}

	if expectedStatus != 0 {
		evidence.ExpectedStatusCode = &expectedStatus
		switch {
		case !p.StatusStable:
			evidence.Expectation = statusExpectationUnstable
		case p.StatusCode != expectedStatus:
			evidence.Expectation = statusExpectationMismatched
		default:
			evidence.Expectation = statusExpectationMatched
		}
	}

	return model.BaselineSummary{
		Samples:         p.Samples,
		StableJSONPaths: len(p.StableJSONPaths),
		BodyLenMin:      p.BodyLenMin,
		BodyLenMax:      p.BodyLenMax,
		StatusEvidence:  evidence,
	}
}
