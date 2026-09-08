package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/aiadvisor"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestAIAdvisorSummaryTracksAdmissionAndVerification(t *testing.T) {
	result := aiadvisor.Result{
		Provider:       "gemini",
		Model:          "gemini-test",
		SuggestedCount: 2,
		AcceptedCount:  1,
		Audit: []aiadvisor.SuggestionAudit{
			{Name: "include_archived", Location: "query", Priority: 90, Reason: "archive state", Admission: aiadvisor.AdmissionAdmitted},
			{Name: "bad name!", Location: "query", Priority: 80, Admission: aiadvisor.AdmissionRejected, RejectionReason: "invalid_name"},
		},
	}
	summary := buildAIAdvisorSummary(result)
	if summary.TestedCandidates != 1 || summary.RejectedCandidates != 1 || summary.VerifiedCandidates != 0 {
		t.Fatalf("initial summary=%+v", summary)
	}

	params := []model.ParameterResult{{
		Name:                 "include_archived",
		Location:             model.LocationQuery,
		CandidateSources:     []model.CandidateSource{{Source: aiadvisor.SourceAISemanticHypothesis}},
		Confidence:           model.ConfidenceScore(1),
		CandidateChanged:     3,
		CandidateTrials:      3,
		RandomControlChanged: 0,
		RandomControlTrials:  3,
	}}
	finalizeAIAdvisorSummary(summary, params)
	if summary.VerifiedCandidates != 1 {
		t.Fatalf("verified=%d", summary.VerifiedCandidates)
	}
	audit := summary.CandidateAudit[0]
	if !audit.Verified || audit.DiscoveryOutcome != "verified" {
		t.Fatalf("audit=%+v", audit)
	}
	if audit.Confidence == nil || float64(*audit.Confidence) != 1 {
		t.Fatalf("confidence=%v", audit.Confidence)
	}
	if audit.CandidateChanged == nil || *audit.CandidateChanged != 3 || audit.RandomControlChanged == nil || *audit.RandomControlChanged != 0 {
		t.Fatalf("verification counts=%+v", audit)
	}
	if summary.CandidateAudit[1].Tested || summary.CandidateAudit[1].DiscoveryOutcome != "locally_rejected" {
		t.Fatalf("rejected audit=%+v", summary.CandidateAudit[1])
	}

	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"random_control_changed":0`) {
		t.Fatalf("zero control count must remain explicit: %s", encoded)
	}
}

func TestAIAdvisorSummaryDoesNotClaimGenericFindingAsAI(t *testing.T) {
	result := aiadvisor.Result{
		Provider:       "gemini",
		Model:          "gemini-test",
		SuggestedCount: 1,
		AcceptedCount:  1,
		Audit: []aiadvisor.SuggestionAudit{{
			Name: "debug", Location: "query", Priority: 70, Admission: aiadvisor.AdmissionAdmitted,
		}},
	}
	summary := buildAIAdvisorSummary(result)
	finalizeAIAdvisorSummary(summary, []model.ParameterResult{{
		Name:       "debug",
		Location:   model.LocationQuery,
		Confidence: model.ConfidenceScore(1),
	}})
	if summary.VerifiedCandidates != 0 || summary.CandidateAudit[0].Verified {
		t.Fatalf("generic result was incorrectly attributed to AI: %+v", summary)
	}
}

func TestAIAdvisorSummaryMatchesJSONParent(t *testing.T) {
	result := aiadvisor.Result{
		Provider:       "gemini",
		Model:          "gemini-test",
		SuggestedCount: 1,
		AcceptedCount:  1,
		Audit: []aiadvisor.SuggestionAudit{{
			Name: "include_deleted", Location: "json", JSONParent: "$.options", Priority: 90, Admission: aiadvisor.AdmissionAdmitted,
		}},
	}
	summary := buildAIAdvisorSummary(result)
	finalizeAIAdvisorSummary(summary, []model.ParameterResult{{
		Name:             "include_deleted",
		Location:         model.LocationJSON,
		JSONPath:         "$.other.include_deleted",
		CandidateSources: []model.CandidateSource{{Source: aiadvisor.SourceAISemanticHypothesis}},
		Confidence:       model.ConfidenceScore(1),
	}})
	if summary.VerifiedCandidates != 0 {
		t.Fatalf("wrong JSON placement matched: %+v", summary)
	}
}
