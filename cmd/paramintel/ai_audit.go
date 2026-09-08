package main

import (
	"fmt"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/aiadvisor"
	"github.com/tobiasGuta/ParamIntel/internal/confidence"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func buildAIAdvisorSummary(result aiadvisor.Result) *model.AIAdvisorSummary {
	summary := &model.AIAdvisorSummary{
		Provider:            result.Provider,
		Model:               result.Model,
		InputPolicy:         "sanitized_structure_only",
		SuggestedCandidates: result.SuggestedCount,
		AcceptedCandidates:  result.AcceptedCount,
	}
	for _, item := range result.Audit {
		audit := model.AIAdvisorCandidateAudit{
			Name:            item.Name,
			Location:        item.Location,
			JSONParent:      item.JSONParent,
			Priority:        item.Priority,
			Reason:          item.Reason,
			Admission:       item.Admission,
			RejectionReason: item.RejectionReason,
		}
		if item.Admission == aiadvisor.AdmissionAdmitted {
			audit.Tested = true
			audit.DiscoveryOutcome = "not_verified"
			summary.TestedCandidates++
		} else {
			audit.DiscoveryOutcome = "locally_rejected"
			summary.RejectedCandidates++
		}
		summary.CandidateAudit = append(summary.CandidateAudit, audit)
	}
	return summary
}

func finalizeAIAdvisorSummary(summary *model.AIAdvisorSummary, params []model.ParameterResult) {
	if summary == nil {
		return
	}
	for i := range summary.CandidateAudit {
		audit := &summary.CandidateAudit[i]
		if audit.Admission != aiadvisor.AdmissionAdmitted {
			continue
		}
		for _, result := range params {
			if !parameterMatchesAIAudit(result, *audit) || !hasAISource(result.CandidateSources) {
				continue
			}
			audit.Verified = true
			audit.DiscoveryOutcome = "verified"
			confidenceScore := result.Confidence
			audit.Confidence = &confidenceScore
			audit.CandidateChanged = result.CandidateChanged
			audit.CandidateTrials = result.CandidateTrials
			audit.RandomControlChanged = result.RandomControlChanged
			audit.RandomControlTrials = result.RandomControlTrials
			summary.VerifiedCandidates++
			break
		}
	}
}

func printAIAdvisorAudit(summary *model.AIAdvisorSummary) {
	if summary == nil {
		return
	}
	fmt.Printf("[*] AI Candidate Advisor audit\n")
	fmt.Printf("    suggested: %d | admitted: %d | rejected: %d | tested: %d | verified: %d\n",
		summary.SuggestedCandidates,
		summary.AcceptedCandidates,
		summary.RejectedCandidates,
		summary.TestedCandidates,
		summary.VerifiedCandidates,
	)
	for i, audit := range summary.CandidateAudit {
		label := audit.Name
		if audit.Location == model.LocationJSON {
			parent := audit.JSONParent
			if parent == "" || parent == "$" {
				label = "$." + audit.Name
			} else {
				label = parent + "." + audit.Name
			}
		}
		fmt.Printf("    [%d] %s (%s) priority=%d\n", i+1, label, audit.Location, audit.Priority)
		if audit.Admission == aiadvisor.AdmissionRejected {
			fmt.Printf("        admission: rejected (%s)\n", audit.RejectionReason)
			fmt.Printf("        discovery: not tested\n")
		} else if audit.Verified {
			fmt.Printf("        admission: admitted\n")
			fmt.Printf("        discovery: verified (%d/%d candidate, %d/%d control, %.0f%% %s)\n",
				audit.CandidateChanged,
				audit.CandidateTrials,
				audit.RandomControlChanged,
				audit.RandomControlTrials,
				float64(*audit.Confidence)*100,
				strings.ToUpper(confidence.Label(float64(*audit.Confidence))),
			)
		} else {
			fmt.Printf("        admission: admitted\n")
			fmt.Printf("        discovery: tested, not verified\n")
		}
		if audit.Reason != "" {
			fmt.Printf("        reason: %s\n", audit.Reason)
		}
	}
}

func parameterMatchesAIAudit(result model.ParameterResult, audit model.AIAdvisorCandidateAudit) bool {
	if result.Name != audit.Name || result.Location != audit.Location {
		return false
	}
	if result.Location != model.LocationJSON {
		return true
	}
	parent := audit.JSONParent
	if parent == "" {
		parent = "$"
	}
	return result.JSONPath == joinAuditJSONPath(parent, audit.Name)
}

func hasAISource(sources []model.CandidateSource) bool {
	for _, source := range sources {
		if source.Source == aiadvisor.SourceAISemanticHypothesis {
			return true
		}
	}
	return false
}

func joinAuditJSONPath(parent, name string) string {
	if parent == "" || parent == "$" {
		return "$." + name
	}
	return parent + "." + name
}
