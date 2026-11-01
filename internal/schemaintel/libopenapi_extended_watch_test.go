package schemaintel

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "os"
 "os/exec"
 "strconv"
 "strings"
 "syscall"
 "testing"
 "time"
)

const watchComposedSpec = "openapi: 3.1.0\ninfo:\n  title: Composed evidence matrix\n  version: \"1\"\npaths:\n  /profiles/{id}:\n    patch:\n      requestBody:\n        content:\n          application/json:\n            schema:\n              $ref: \"#/components/schemas/Input\"\n      responses:\n        \"200\":\n          description: ok\n          content:\n            application/json:\n              schema:\n                allOf:\n                  - $ref: \"#/components/schemas/Input\"\n                  - type: object\n                    required: [role]\n                    properties:\n                      role:\n                        type: string\n                        readOnly: true\n                      profile:\n                        type: object\n                        properties:\n                          beta:\n                            type: boolean\n                            readOnly: true\n                      scaffold:\n                        type: object\n                        properties:\n                          feature:\n                            type: boolean\n                      deep:\n                        type: object\n                        properties:\n                          inner:\n                            type: object\n                            properties:\n                              hidden:\n                                type: boolean\n                      option:\n                        oneOf:\n                          - type: string\n                          - type: integer\n                      variant:\n                        anyOf:\n                          - type: string\n                          - type: boolean\n                      records:\n                        type: array\n                        items:\n                          type: object\n                          properties:\n                            secret:\n                              type: string\ncomponents:\n  schemas:\n    Input:\n      type: object\n      properties:\n        display_name:\n          type: string\n        profile:\n          type: object\n          properties:\n            name:\n              type: string\n"
const watchNullableSpec = "openapi: 3.0.3\ninfo:\n  title: Nullable and referenced scalar evidence\n  version: \"1\"\npaths:\n  /settings:\n    patch:\n      requestBody:\n        content:\n          application/json:\n            schema:\n              type: object\n              properties:\n                mode:\n                  type: string\n      responses:\n        \"200\":\n          description: ok\n          content:\n            application/json:\n              schema:\n                $ref: \"#/components/schemas/Settings\"\ncomponents:\n  schemas:\n    AccessCode:\n      type: string\n    Settings:\n      type: object\n      required: [consent]\n      properties:\n        mode:\n          type: string\n        consent:\n          type: boolean\n          readOnly: true\n          nullable: true\n        access_code:\n          $ref: \"#/components/schemas/AccessCode\"\n"
const watchUnionSpec = "openapi: 3.1.0\ninfo:\n  title: JSON Schema type union\n  version: \"1\"\npaths:\n  /union:\n    patch:\n      requestBody:\n        content:\n          application/json:\n            schema:\n              type: object\n      responses:\n        \"200\":\n          description: ok\n          content:\n            application/json:\n              schema:\n                type: object\n                properties:\n                  maybe_name:\n                    type: [string, \"null\"]\n                  details:\n                    type: object\n                    properties:\n                      flag:\n                        type: boolean\n                  ignored_array:\n                    type: array\n                    items:\n                      type: array\n                      items:\n                        type: string\ncomponents: {}\n"
const watchRecursiveSpec = "openapi: 3.1.0\ninfo:\n  title: Recursive field schema\n  version: \"1\"\npaths:\n  /node:\n    patch:\n      requestBody:\n        content:\n          application/json:\n            schema:\n              type: object\n      responses:\n        \"200\":\n          description: ok\n          content:\n            application/json:\n              schema:\n                $ref: \"#/components/schemas/Node\"\ncomponents:\n  schemas:\n    Node:\n      type: object\n      properties:\n        label:\n          type: string\n        next:\n          $ref: \"#/components/schemas/Node\"\n"

func watchAnalyzeSpec(t *testing.T, spec, path, body string) Report {
 t.Helper()
 doc, err := Parse([]byte(spec))
 if err != nil { t.Fatalf("Parse: %v", err) }
 report, err := Analyze(doc, requestTemplate(http.MethodPatch, "https://api.example.test"+path, body), stableJSONBaseline(200, "application/json"), DefaultConfig())
 if err != nil { t.Fatalf("Analyze: %v", err) }
 return report
}

func watchCandidateAt(report Report, path string) (CandidateDescriptor, bool) {
 for _, c := range report.Candidates { if c.Path == path { return c, true } }
 return CandidateDescriptor{}, false
}

func watchSnapshot(t *testing.T, name string, report Report) {
 t.Helper()
 data, err := json.Marshal(report)
 if err != nil { t.Fatal(err) }
 t.Logf("WATCH_EVIDENCE %s %s", name, data)
 file := os.Getenv("PARAMINTEL_WATCH_SNAPSHOT_PATH")
 if file == "" { return }
 f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
 if err != nil { t.Fatal(err) }
 _, writeErr := fmt.Fprintf(f, "%s\t%s\n", name, data)
 closeErr := f.Close()
 if writeErr != nil { t.Fatal(writeErr) }
 if closeErr != nil { t.Fatal(closeErr) }
}

func TestLibopenapiWatchExtendedEvidence(t *testing.T) {
 t.Run("allOf-nested-arrays-and-ambiguous-unions",func(t *testing.T) {
  report:=watchAnalyzeSpec(t,watchComposedSpec,"/profiles/123","{\"display_name\":\"test\",\"profile\":{\"name\":\"test\"}}")
  if len(report.Candidates)!=3 { t.Fatalf("expected exactly 3 response-only candidates, got %+v",report.Candidates) }
  for path,placement:=range map[string]string{
    "$.role":PlacementExistingParent, "$.profile.beta":PlacementExistingParent, "$.scaffold.feature":PlacementOneLevelScaffold,
  } {
    candidate,ok:=watchCandidateAt(report,path)
    if !ok || candidate.Placement!=placement || candidate.Source!=SourceResponseOnlyJSONProperty || candidate.Reason=="" {
      t.Fatalf("candidate evidence mismatch path=%s got=%+v present=%v",path,candidate,ok)
    }
    if path=="$.role" && (!candidate.ReadOnly||!candidate.Required||len(candidate.DeclaredTypes)!=1||candidate.DeclaredTypes[0]!="string"){
      t.Fatalf("role metadata mismatch: %+v",candidate)
    }
  }
  for _,path:=range []string{"$.option","$.variant","$.records","$.deep.inner.hidden"} {
    if _,ok:=watchCandidateAt(report,path);ok { t.Fatalf("unsafe candidate admitted: %s",path) }
  }
  for _,pair:=range [][2]string{{"$.option","oneOf/anyOf"},{"$.variant","oneOf/anyOf"},{"$.records","array"},{"$.deep.inner.hidden","deeper missing"}} {
    if !skipContains(report.Skipped,pair[0],pair[1]) { t.Fatalf("missing conservative skip %s %s: %+v",pair[0],pair[1],report.Skipped) }
  }
  watchSnapshot(t,"composed",report)
 })
 t.Run("3.0-nullability-and-component-ref",func(t *testing.T) {
  report:=watchAnalyzeSpec(t,watchNullableSpec,"/settings","{\"mode\":\"normal\"}")
  if len(report.Candidates)!=2 { t.Fatalf("expected 2 candidates, got %+v",report.Candidates) }
  consent,ok:=watchCandidateAt(report,"$.consent")
  if !ok||!consent.Required||!consent.ReadOnly||!consent.Nullable||consent.Source!=SourceResponseOnlyJSONProperty {
    t.Fatalf("nullable metadata changed: %+v present=%v",consent,ok)
  }
  access,ok:=watchCandidateAt(report,"$.access_code")
  if !ok||access.SchemaRef!="#/components/schemas/AccessCode"||access.Placement!=PlacementExistingParent {
    t.Fatalf("component reference provenance changed: %+v present=%v",access,ok)
  }
  watchSnapshot(t,"nullable",report)
 })
 t.Run("3.1-union-and-nested-arrays",func(t *testing.T) {
  report:=watchAnalyzeSpec(t,watchUnionSpec,"/union","{}")
  if _,ok:=watchCandidateAt(report,"$.ignored_array");ok { t.Fatal("array candidate admitted") }
  if _,ok:=watchCandidateAt(report,"$.ignored_array.secret");ok { t.Fatal("array item candidate admitted") }
  maybe,ok:=watchCandidateAt(report,"$.maybe_name")
  if !ok { t.Fatalf("missing type union candidate: %+v",report.Candidates) }
  if len(maybe.DeclaredTypes)!=2 || maybe.DeclaredTypes[0]!="null" || maybe.DeclaredTypes[1]!="string" {
    t.Fatalf("type union changed: %+v",maybe)
  }
  watchSnapshot(t,"union",report)
 })
}

// These are local synthetic fixtures only. They never connect to a network.
func watchResourceDocument(name string) string {
 switch name {
 case "alias-11":
  var b strings.Builder
  b.WriteString("openapi: 3.1.0\ninfo: {title: Alias ceiling, version: '1'}\npaths: {}\n")
  b.WriteString("x-a0: &a0 [one, two, three, four, five, six]\n")
  for lvl:=1;lvl<=11;lvl++ {
   fmt.Fprintf(&b,"x-a%d: &a%d [",lvl,lvl)
   for i:=0;i<6;i++ {if i>0 {b.WriteString(", ")};fmt.Fprintf(&b,"*a%d",lvl-1)}
   b.WriteString("]\n")
  }
  b.WriteString("x-stress: *a11\n")
  return b.String()
 case "deep-220":
  var b strings.Builder
  b.WriteString("openapi: 3.1.0\ninfo: {title: Deep extension, version: '1'}\npaths: {}\n")
  for n:=0;n<220;n++ { b.WriteString(strings.Repeat("  ",n)+"x-depth:\n") }
  b.WriteString(strings.Repeat("  ",220)+"terminal: true\n")
  return b.String()
 case "large-3m":
  return "openapi: 3.1.0\ninfo: {title: Large extension, version: '1'}\npaths: {}\nx-payload: "+strings.Repeat("a",3<<20)+"\n"
 case "malformed":
  return "openapi: 3.1.0\ninfo: {title: Malformed, version: '1'}\npaths: [\n"
 case "ref-chain-180":
  var b strings.Builder
  b.WriteString("openapi: 3.1.0\ninfo: {title: Ref chain, version: '1'}\npaths:\n  /chain:\n    get:\n      responses:\n        '200':\n          description: ok\n          content:\n            application/json:\n              schema:\n                $ref: '#/components/schemas/R0'\ncomponents:\n  schemas:\n")
  for i:=0;i<180;i++ {
   if i==179 {fmt.Fprintf(&b,"    R%d:\n      type: object\n      properties:\n        end: {type: string}\n",i)} else {fmt.Fprintf(&b,"    R%d:\n      $ref: '#/components/schemas/R%d'\n",i,i+1)}
  }
  return b.String()
 case "recursive-property":
  return watchRecursiveSpec
 default:
  panic("unknown resource fixture: "+name)
 }
}

func TestLibopenapiWatchResourceChild(t *testing.T) {
 name:=os.Getenv("PARAMINTEL_WATCH_RESOURCE_CHILD")
 if name=="" {t.Skip("resource helper only")}
 spec:=watchResourceDocument(name)
 doc,err:=Parse([]byte(spec))
 outcome:="accepted"
 if err!=nil {outcome="rejected"}
 if name=="recursive-property"&&err==nil {
  _,anErr:=Analyze(doc,requestTemplate(http.MethodPatch,"https://api.example.test/node","{}"),stableJSONBaseline(200,"application/json"),DefaultConfig())
  if anErr!=nil {outcome="analyze-error"}
 }
 if name=="malformed"&&err==nil {t.Fatal("invalid YAML unexpectedly parsed")}
 var usage syscall.Rusage
 if err:=syscall.Getrusage(syscall.RUSAGE_SELF,&usage);err!=nil {t.Fatalf("measure peak RSS: %v",err)}
 const peakRSSCeilingKiB=int64(256*1024)
 if usage.Maxrss>peakRSSCeilingKiB {t.Fatalf("kernel-reported peak RSS cap exceeded fixture=%s maxrss_kib=%d ceiling_kib=%d",name,usage.Maxrss,peakRSSCeilingKiB)}
 fmt.Printf("WATCH_RESOURCE case=%s bytes=%d outcome=%s peak_rss_kib=%d cap_kib=%d\n",name,len(spec),outcome,usage.Maxrss,peakRSSCeilingKiB)
}

func watchReadPeakRSS(pid int) int64 {
 b,err:=os.ReadFile(fmt.Sprintf("/proc/%d/status",pid))
 if err!=nil { return 0 }
 var peak int64
 for _,line:=range bytes.Split(b,[]byte("\n")) {
  if !bytes.HasPrefix(line,[]byte("VmHWM:")) && !bytes.HasPrefix(line,[]byte("VmRSS:")) {continue}
  fields:=bytes.Fields(line)
  if len(fields)<2 {continue}
  kib,parseErr:=strconv.ParseInt(string(fields[1]),10,64)
  if parseErr==nil && kib*1024>peak {peak=kib*1024}
 }
 return peak
}

func TestLibopenapiWatchExtendedResourceBoundaries(t *testing.T) {
 if os.Getenv("PARAMINTEL_WATCH_RESOURCE")!="1" {
  t.Skip("larger synthetic resource test is isolated to compatibility workflow")
 }
 cases:=[]string{"alias-11","deep-220","large-3m","malformed","ref-chain-180","recursive-property"}
 for _,fixture:=range cases {
  t.Run(fixture,func(t *testing.T) {
   ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
   defer cancel()
   cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestLibopenapiWatchResourceChild$","-test.v","-test.timeout=10s")
   cmd.Env=append(os.Environ(),"PARAMINTEL_WATCH_RESOURCE_CHILD="+fixture,"GOMEMLIMIT=128MiB","GOGC=50")
   var output bytes.Buffer
   cmd.Stdout=&output
   cmd.Stderr=&output
   if err:=cmd.Start();err!=nil {t.Fatalf("start: %v",err)}
   pid:=cmd.Process.Pid
   results:=make(chan error,1)
   go func(){results<-cmd.Wait()}()
   ticker:=time.NewTicker(5*time.Millisecond)
   defer ticker.Stop()
   start:=time.Now()
   const maxRSS=int64(256<<20)
   var peak int64
   for {
    select {
    case err:=<-results:
     data:=output.String()
     if ctx.Err()!=nil {t.Fatalf("timeout fixture=%s elapsed=%s output=%s",fixture,time.Since(start),data)}
     if err!=nil {t.Fatalf("crash or invalid fixture=%s err=%v output=%s",fixture,err,data)}
     if !strings.Contains(data,"WATCH_RESOURCE case="+fixture+" ") {t.Fatalf("missing fixture result: %s",data)}
     t.Logf("WATCH_RESOURCE_BOUND fixture=%s elapsed=%s observed_peak_rss_mib=%.1f cap_mib=256 output=%s",fixture,time.Since(start),float64(peak)/1048576,strings.TrimSpace(data))
     return
    case <-ticker.C:
     rss:=watchReadPeakRSS(pid)
     if rss>peak {peak=rss}
     if rss>maxRSS {
      _=cmd.Process.Kill()
      <-results
      t.Fatalf("observed RSS ceiling exceeded fixture=%s observed_peak_mib=%.1f cap_mib=256",fixture,float64(peak)/1048576)
     }
    case <-ctx.Done():
     <-results
     t.Fatalf("time limit exceeded fixture=%s elapsed=%s observed_peak_mib=%.1f",fixture,time.Since(start),float64(peak)/1048576)
    }
   }
  })
 }
}
