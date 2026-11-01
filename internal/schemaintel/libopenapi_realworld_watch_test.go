package schemaintel

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "testing"
)

type realWorldResult struct {
 Name string `json:"name"`
 Bytes int `json:"bytes"`
 InputSHA256 string `json:"input_sha256"`
 ParseStatus string `json:"parse_status"`
 ParseClass string `json:"parse_class,omitempty"`
 OpenAPIVersion string `json:"openapi_version,omitempty"`
 OperationCount int `json:"operation_count,omitempty"`
 Checks []realWorldCheck `json:"checks,omitempty"`
}

type realWorldCheck struct {
 Method string `json:"method"`
 Path string `json:"path"`
 Status int `json:"status"`
 ErrorClass string `json:"error_class,omitempty"`
 Report *Report `json:"report,omitempty"`
}

func realWorldParseClass(err error) string {
 if err==nil{return ""}
 message:=err.Error()
 switch {
 case strings.Contains(message,"infinite circular reference detected"):return "circular-reference"
 case strings.Contains(message,"../policies.yaml") && strings.Contains(message,"does not exist in the specification"):return "external-reference-blocked"
 default:return "other-parser-error"
 }
}

func realWorldClassify(err error) string {
 if err==nil {return ""}
 switch {
 case errors.Is(err,ErrUnstableBaseline):return "unstable-baseline"
 case errors.Is(err,ErrOperationNotFound):return "operation-not-found"
 case errors.Is(err,ErrAmbiguousOperation):return "ambiguous-operation"
 case errors.Is(err,ErrMediaTypeNotFound):return "missing-media-type"
 case errors.Is(err,ErrNonJSONMediaType):return "non-json"
 case errors.Is(err,ErrResponseNotFound):return "missing-response"
 case errors.Is(err,ErrSchemaBudgetExceeded):return "schema-budget"
 case errors.Is(err,ErrRequestJSON):return "request-json"
 default:return "other"
 }
}

func TestLibopenapiWatchRealWorld(t *testing.T) {
 root:=os.Getenv("PARAMINTEL_REALWORLD_SPEC_DIR")
 if root=="" {t.Skip("requires pinned public specs from isolated workflow")}
 names:=[]string{"1password","ably","spotify","trello","stripe"}
 results:=make([]realWorldResult,0,len(names))
 parsed:=0
 successes:=0
 for _,name:=range names {
  data,err:=os.ReadFile(filepath.Join(root,name+".yaml"))
  if err!=nil {t.Fatalf("read fixture %s: %v",name,err)}
  if len(data)>6<<20 || len(data)==0 {t.Fatalf("unexpected fixture byte count %s=%d",name,len(data))}
  hash:=sha256.Sum256(data)
  record:=realWorldResult{Name:name,Bytes:len(data),InputSHA256:hex.EncodeToString(hash[:])}
  doc,err:=Parse(data)
  if err!=nil {
   record.ParseStatus="rejected"
   record.ParseClass=realWorldParseClass(err)
   want:=map[string]string{"spotify":"external-reference-blocked","stripe":"circular-reference"}[name]
   if record.ParseClass!=want {t.Fatalf("unexpected parser failure in %s: class=%s expected=%s error=%v",name,record.ParseClass,want,err)}
   t.Logf("REALWORLD_PARSE %s rejected class=%s: %v",name,record.ParseClass,err)
   results=append(results,record)
   continue
  }
  record.ParseStatus="accepted"
  if name=="spotify"||name=="stripe" {t.Fatalf("parsing behavior changed for %s; review before updating expectations",name)}
  record.OpenAPIVersion=doc.Version()
  parsed++
  if doc.model.Model.Paths!=nil && doc.model.Model.Paths.PathItems!=nil {
   paths:=make([]string,0)
   for path := range doc.model.Model.Paths.PathItems.FromOldest(){paths=append(paths,path)}
   sort.Strings(paths)
   record.OperationCount=len(paths)
   // Select a deterministic, bounded group of operations from the real API.
   // Explicitly include body-bearing operations and bodyless GETs.
   for _,methods:=range [][]string{{http.MethodPatch,http.MethodPost,http.MethodPut},{http.MethodGet}} {
    sampled:=0
    for _,path:=range paths {
     item:=doc.model.Model.Paths.PathItems.GetOrZero(path)
     if item==nil{continue}
     for _,method:=range methods {
      op:=operationForMethod(item,method)
      if op==nil||op.Responses==nil{continue}
      if method!=http.MethodGet && op.RequestBody==nil {continue}
      if method==http.MethodGet && op.RequestBody!=nil {continue}
      body:="{}"
      if method==http.MethodGet{body=""}
      var check realWorldCheck
      for _,status:=range []int{200,201,202} {
       _,_,respErr:=selectResponse(op.Responses,status)
       if respErr!=nil {continue}
       template:=requestTemplate(method,"https://api.example.test"+path,body)
       report,anErr:=Analyze(doc,template,stableJSONBaseline(status,"application/json"),DefaultConfig())
       check=realWorldCheck{Method:method,Path:path,Status:status}
       if anErr!=nil {check.ErrorClass=realWorldClassify(anErr)} else {check.Report=&report;successes++}
       break
      }
      if check.Method!=""{
       record.Checks=append(record.Checks,check)
       sampled++
      }
      if sampled>=6{break}
     }
     if sampled>=6{break}
    }
   }
  }
  t.Logf("REALWORLD_PARSE %s accepted version=%s path_count=%d sampled_checks=%d",name,record.OpenAPIVersion,record.OperationCount,len(record.Checks))
  results=append(results,record)
 }
 if parsed!=3 {t.Fatalf("expected 3 accepted public docs, got %d",parsed)}
 if successes<16 {t.Fatalf("expected at least 16 successful real-world schema analyses; got %d",successes)}
 output,err:=json.Marshal(results)
 if err!=nil {t.Fatal(err)}
 outPath:=os.Getenv("PARAMINTEL_REALWORLD_SNAPSHOT_PATH")
 if outPath==""{t.Fatal("snapshot path required")}
 if err:=os.WriteFile(outPath,append(output,'\n'),0600);err!=nil{t.Fatal(err)}
 hash:=sha256.Sum256(output)
 var checks int
 for _,r:=range results{checks+=len(r.Checks)}
 fmt.Printf("REALWORLD_SUMMARY parsed=%d/%d analyzed=%d checks=%d output_sha256=%s\n",parsed,len(names),successes,checks,hex.EncodeToString(hash[:]))
 for _,r:=range results {
  var admitted int
  for _,c:=range r.Checks {if c.Report!=nil {admitted+=len(c.Report.Candidates)}}
  fmt.Printf("REALWORLD_SPEC name=%s parsed=%s bytes=%d openapi=%s paths=%d checks=%d candidates=%d\n",r.Name,r.ParseStatus,r.Bytes,r.OpenAPIVersion,r.OperationCount,len(r.Checks),admitted)
 }
 if strings.Contains(string(output),"https://api.example.test") {t.Fatal("unexpected generated baseline URL within report snapshot")}
}
