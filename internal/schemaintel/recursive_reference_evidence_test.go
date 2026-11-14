package schemaintel

import (
	"net/http"
	"testing"
)

// These fixtures protect candidate evidence fidelity across libopenapi
// upgrades. Recursive schemas must terminate without losing unrelated
// response-only fields or turning recursive objects into probe candidates.
const recursiveEvidenceSpec = `openapi: 3.1.0
info: {title: Recursive response evidence, version: "1"}
paths:
  /accounts:
    patch:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                display_name: {type: string}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Account'
components:
  schemas:
    Account:
      type: object
      properties:
        display_name: {type: string}
        privileged:
          type: boolean
          readOnly: true
        next:
          $ref: '#/components/schemas/Account'
`

const nestedReferenceEvidenceSpec = `openapi: 3.1.0
info: {title: Nested reference evidence, version: "1"}
paths:
  /accounts:
    patch:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                profile:
                  type: object
                  properties:
                    display_name: {type: string}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AccountResponse'
components:
  schemas:
    AccountResponse:
      type: object
      properties:
        profile:
          $ref: '#/components/schemas/Profile'
    Profile:
      type: object
      required: [beta_access]
      properties:
        display_name: {type: string}
        beta_access:
          type: boolean
          readOnly: true
`

func TestRecursiveReferencePreservesSiblingCandidateEvidence(t *testing.T) {
	doc, err := Parse([]byte(recursiveEvidenceSpec))
	if err != nil {
		t.Fatalf("parse recursive response schema: %v", err)
	}
	report, err := Analyze(doc,
		requestTemplate(http.MethodPatch, "https://api.example.test/accounts", `{"display_name":"Test"}`),
		stableJSONBaseline(http.StatusOK, "application/json"), DefaultConfig())
	if err != nil {
		t.Fatalf("analyze recursive response schema: %v", err)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].Path != "$.privileged" {
		t.Fatalf("recursive reference changed response-only candidate set: %+v", report.Candidates)
	}
	candidate := report.Candidates[0]
	if candidate.Placement != PlacementExistingParent || !candidate.ReadOnly ||
		len(candidate.DeclaredTypes) != 1 || candidate.DeclaredTypes[0] != "boolean" {
		t.Fatalf("sibling candidate evidence changed: %+v", candidate)
	}
	if !skipContains(report.Skipped, "$.next", "cyclic internal schema reference") {
		t.Fatalf("missing explicit recursive reference skip: %+v", report.Skipped)
	}
}

func TestNestedInternalReferencesRetainExistingParentCandidate(t *testing.T) {
	doc, err := Parse([]byte(nestedReferenceEvidenceSpec))
	if err != nil {
		t.Fatalf("parse nested reference schema: %v", err)
	}
	report, err := Analyze(doc,
		requestTemplate(http.MethodPatch, "https://api.example.test/accounts", `{"profile":{"display_name":"Test"}}`),
		stableJSONBaseline(http.StatusOK, "application/json"), DefaultConfig())
	if err != nil {
		t.Fatalf("analyze nested reference schema: %v", err)
	}
	if len(report.Candidates) != 1 {
		t.Fatalf("unexpected nested reference candidate set: %+v", report.Candidates)
	}
	candidate := report.Candidates[0]
	if candidate.Path != "$.profile.beta_access" || candidate.Placement != PlacementExistingParent ||
		!candidate.ReadOnly || !candidate.Required ||
		len(candidate.DeclaredTypes) != 1 || candidate.DeclaredTypes[0] != "boolean" {
		t.Fatalf("nested reference metadata or candidate placement changed: %+v", candidate)
	}
}
