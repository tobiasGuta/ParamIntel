package main

import (
	"fmt"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

// validateExpectedBaselineStatus rejects invalid operator expectations before
// any target requests. A value of zero preserves the existing opt-in default.
func validateExpectedBaselineStatus(expected int) error {
	if expected == 0 || (expected >= 100 && expected <= 599) {
		return nil
	}
	return fmt.Errorf("-expect-baseline-status must be 0 (disabled) or an HTTP status from 100 to 599")
}

// checkBaselineStatusExpectation separates a reproducible response from an
// operator-intended application state. It does not infer authentication from
// a status code: the operator must explicitly specify the expected status.
func checkBaselineStatusExpectation(profile model.BaselineProfile, expected int) error {
	if expected == 0 {
		return nil
	}
	if !profile.StatusStable {
		return fmt.Errorf("baseline status is not stable; cannot verify expected HTTP %d (-expect-baseline-status)", expected)
	}
	if profile.StatusCode != expected {
		return fmt.Errorf("baseline HTTP %d does not match expected HTTP %d (-expect-baseline-status); stopping before discovery", profile.StatusCode, expected)
	}
	return nil
}
