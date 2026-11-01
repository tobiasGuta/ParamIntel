package schemaintel

import (
 "context"
 "fmt"
 "net/http"
 "os"
 "os/exec"
 "strings"
 "testing"
 "time"
)

const watchValidSpec = "openapi: 3.1.0\ninfo:\n  title: ParamIntel compatibility watch\n  version: \"1\"\npaths:\n  /accounts/{id}:\n    patch:\n      parameters:\n        - name: id\n          in: path\n          required: true\n          schema:\n            type: string\n      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n              type: object\n              properties:\n                display_name:\n                  type: string\n      responses:\n        \"200\":\n          description: Account response\n          content:\n            application/json:\n              schema:\n                $ref: \"#/components/schemas/Account\"\ncomponents:\n  schemas:\n    Account:\n      type: object\n      properties:\n        display_name:\n          type: string\n        role:\n          type: string\n          readOnly: true\n"
const watchExampleSpec = "openapi: 3.1.0\ninfo:\n  title: ParamIntel compatibility watch\n  version: \"1\"\npaths:\n  /accounts/{id}:\n    patch:\n      parameters:\n        - name: id\n          in: path\n          required: true\n          schema:\n            type: string\n      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n              type: object\n              properties:\n                display_name:\n                  type: string\n      responses:\n        \"200\":\n          description: Account response\n          content:\n            application/json:\n              schema:\n                $ref: \"#/components/schemas/Account\"\ncomponents:\n  schemas:\n    Account:\n      example:\n        $ref: literal-client-data-not-a-schema-reference\n        note: do-not-resolve\n      type: object\n      properties:\n        display_name:\n          type: string\n        role:\n          type: string\n          readOnly: true\n"
const watchCycleSpec = "openapi: 3.1.0\ninfo:\n  title: Reference-only cycle\n  version: \"1\"\npaths:\n  /cycle:\n    get:\n      responses:\n        \"200\":\n          description: Cycle\n          content:\n            application/json:\n              schema:\n                $ref: \"#/components/schemas/A\"\ncomponents:\n  schemas:\n    A:\n      $ref: \"#/components/schemas/B\"\n    B:\n      $ref: \"#/components/schemas/A\"\n"

func TestLibopenapiWatchValidCandidateInvariant(t *testing.T) {
 doc, err := Parse([]byte(watchValidSpec))
 if err != nil { t.Fatalf("valid spec rejected: %v", err) }
 if doc.Version() != "3.1.0" { t.Fatalf("version=%q", doc.Version()) }
 report, err := Analyze(doc, requestTemplate(http.MethodPatch, "https://api.example.test/accounts/42", "{\"display_name\":\"test\"}"), stableJSONBaseline(200, "application/json"), DefaultConfig())
 if err != nil { t.Fatalf("Analyze valid spec: %v", err) }
 if len(report.Candidates) != 1 || report.Candidates[0].Path != "$.role" {
   t.Fatalf("candidate invariant changed: %+v", report.Candidates)
 }
 t.Logf("WATCH_RESULT valid candidates=%d path=%s", len(report.Candidates), report.Candidates[0].Path)
}

func TestLibopenapiWatchExampleLiteralRef(t *testing.T) {
 // Keep the pinned-version limitation visible without making unrelated CI red.
 // The isolated comparison workflow sets "accept" when testing the fixed parser.
 expected := os.Getenv("PARAMINTEL_WATCH_EXAMPLE_REF_EXPECTED")
 if expected == "" { expected = "reject" }
 if expected != "reject" && expected != "accept" {
   t.Fatalf("invalid expected literal example-ref behavior: %q", expected)
 }
 doc, err := Parse([]byte(watchExampleSpec))
 if expected == "reject" {
   if err == nil { t.Fatal("known pinned-version rejection unexpectedly disappeared; update the compatibility expectation") }
   msg := err.Error()
   if !strings.Contains(msg, "literal-client-data-not-a-schema-reference") ||
      (!strings.Contains(msg, "cannot resolve reference") && !strings.Contains(msg, "does not exist in the specification")) {
     t.Fatalf("unexpected parser error (not the known example-ref issue): %v", err)
   }
   t.Logf("WATCH_RESULT example-ref expected-rejection, known pinned-version limitation: %v", err)
   return
 }
 if err != nil { t.Fatalf("literal $ref within example broke parsing: %v", err) }
 report, err := Analyze(doc, requestTemplate(http.MethodPatch, "https://api.example.test/accounts/42", "{\"display_name\":\"test\"}"), stableJSONBaseline(200, "application/json"), DefaultConfig())
 if err != nil { t.Fatalf("Analyze example: %v", err) }
 if len(report.Candidates) != 1 || report.Candidates[0].Path != "$.role" {
   t.Fatalf("example contaminated schema candidates: %+v", report.Candidates)
 }
 t.Logf("WATCH_RESULT example-ref accepted, candidates=%d path=%s", len(report.Candidates), report.Candidates[0].Path)
}

func TestLibopenapiWatchChild(t *testing.T) {
 name := os.Getenv("PARAMINTEL_WATCH_CHILD")
 if name == "" { t.Skip("helper invoked only in subprocess") }
 var spec string
 switch name {
 case "cycle": spec = watchCycleSpec
 case "aliases": spec = watchAliasSpec()
 default: t.Fatalf("unknown fixture %q", name)
 }
 _, err := Parse([]byte(spec))
 if err != nil {
   fmt.Printf("WATCH_RESULT %s rejected err=%s\n", name, strings.ReplaceAll(err.Error(), "\n", " "))
 } else {
   fmt.Printf("WATCH_RESULT %s accepted\n", name)
 }
}

func watchAliasSpec() string {
 // Compact nested aliases amplify parsing work while keeping input below 3 KB.
 var b strings.Builder
 b.WriteString("openapi: 3.1.0\ninfo:\n  title: Alias stress\n  version: '1'\npaths: {}\n")
 b.WriteString("x-alias0: &a0 [one, two, three, four, five, six]\n")
 for level := 1; level <= 7; level++ {
   fmt.Fprintf(&b, "x-alias%d: &a%d [", level, level)
   for i := 0; i < 6; i++ {
     if i > 0 { b.WriteString(", ") }
     fmt.Fprintf(&b, "*a%d", level-1)
   }
   b.WriteString("]\n")
 }
 b.WriteString("x-stress: *a7\n")
 return b.String()
}

func TestLibopenapiWatchBoundedSubprocesses(t *testing.T) {
 for _, fixture := range []string{"cycle", "aliases"} {
  t.Run(fixture, func(t *testing.T) {
   ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
   defer cancel()
   cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLibopenapiWatchChild$", "-test.v", "-test.timeout=15s")
   cmd.Env = append(os.Environ(), "PARAMINTEL_WATCH_CHILD="+fixture, "GOMEMLIMIT=256MiB", "GOGC=50")
   output, err := cmd.CombinedOutput()
   t.Logf("WATCH_SUBPROCESS fixture=%s output=%s", fixture, strings.TrimSpace(string(output)))
   if ctx.Err() != nil { t.Fatalf("%s exceeded 12-second execution limit: %v", fixture, ctx.Err()) }
   if err != nil { t.Fatalf("%s process crashed or failed: %v", fixture, err) }
   if !strings.Contains(string(output), "WATCH_RESULT "+fixture+" ") {
     t.Fatalf("%s process completed without outcome marker", fixture)
   }
  })
 }
}
