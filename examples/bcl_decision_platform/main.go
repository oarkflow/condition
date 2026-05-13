package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/oarkflow/condition"
	"github.com/oarkflow/condition/bcl"
)

func main() {
	ctx := context.Background()
	path := "examples/bcl_decision_platform/fraud.bcl"
	runtime, err := condition.NewLiveDecisionRuntime(condition.LiveDecisionRuntimeConfig{
		Packages: []condition.DecisionPackageFile{{Path: path}},
	})
	if err != nil {
		log.Fatal(err)
	}
	pkg, err := condition.LoadDecisionPackage(ctx, condition.FileSource{Path: path}, bcl.Decoder())
	if err != nil {
		log.Fatal(err)
	}

	server := condition.NewDecisionServer(condition.WithDecisionServerRuntime(runtime))
	if _, err := server.PublishPackage(ctx, pkg); err != nil {
		log.Fatal(err)
	}

	body, err := os.ReadFile("examples/bcl_decision_platform/data/evaluate.json")
	if err != nil {
		log.Fatal(err)
	}
	decision := call(server, http.MethodPost, "/v1/decision/"+pkg.Name+"/fraud-review", "application/json", body)
	printJSON("decision", decision)

	next, err := condition.LoadDecisionPackage(ctx, condition.FileSource{Path: path}, bcl.Decoder())
	if err != nil {
		log.Fatal(err)
	}
	next.Version = "2"
	next.RuleSets[0].Rules[0].Condition = `transaction.amount > 200000`
	cases, err := condition.LoadDecisionRequestsFile(ctx, "examples/bcl_decision_platform/data/simulation.json")
	if err != nil {
		log.Fatal(err)
	}
	simReq := condition.SimulationRequest{
		Baseline:  &pkg,
		Candidate: &next,
		Cases:     cases,
	}
	body, _ = json.Marshal(simReq)
	simulation := call(server, http.MethodPost, "/v1/packages/"+pkg.Name+"/simulate", "application/json", body)
	printJSON("simulation", simulation)

	tests := call(server, http.MethodPost, "/v1/packages/"+pkg.Name+"/tests", "application/json", nil)
	printJSON("tests", tests)

	yamlReq, err := condition.LoadDecisionRequestFile(ctx, "examples/bcl_decision_platform/data/evaluate.yaml")
	if err != nil {
		log.Fatal(err)
	}
	yamlDecisionRes, err := runtime.Evaluate(ctx, yamlReq)
	if err != nil {
		log.Fatal(err)
	}
	yamlBody, _ := json.Marshal(yamlDecisionRes)
	yamlDecision := json.RawMessage(yamlBody)
	printJSON("yaml_request_decision", yamlDecision)
}

func call(server http.Handler, method, path, contentType string, body []byte) json.RawMessage {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code < 200 || w.Code >= 300 {
		log.Fatalf("%s %s returned %d: %s", method, path, w.Code, w.Body.String())
	}
	return append(json.RawMessage(nil), w.Body.Bytes()...)
}

func printJSON(label string, raw json.RawMessage) {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		fmt.Printf("\n%s:\n%s\n", label, raw)
		return
	}
	fmt.Printf("\n%s:\n%s\n", label, pretty.String())
}
