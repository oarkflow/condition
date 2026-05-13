package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
	_ "github.com/oarkflow/condition/bcl"
)

func main() {
	ctx := context.Background()
	runtime, err := condition.NewLiveDecisionRuntime(condition.LiveDecisionRuntimeConfig{
		Packages: []condition.DecisionPackageFile{{
			Path: "examples/decision_intelligence/packages/fraud.bcl",
		}},
		RunPackageTests:       true,
		ValidateBeforePublish: true,
		KeepLastGood:          true,
	})
	if err != nil {
		log.Fatal(err)
	}

	for _, event := range mustReload(ctx, runtime) {
		printJSON("reload", event)
	}

	req, err := condition.LoadDecisionRequestFile(ctx, "examples/decision_intelligence/data/fraud_request.json")
	if err != nil {
		log.Fatal(err)
	}
	res, err := runtime.Evaluate(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	printJSON("decision", res)

	yamlReq, err := condition.LoadDecisionRequestFile(ctx, "examples/decision_intelligence/data/fraud_request.yaml")
	if err != nil {
		log.Fatal(err)
	}
	yamlRes, err := runtime.Evaluate(ctx, yamlReq)
	if err != nil {
		log.Fatal(err)
	}
	printJSON("yaml_request_decision", yamlRes)

	baseline, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: "examples/decision_intelligence/packages/fraud.bcl"})
	if err != nil {
		log.Fatal(err)
	}
	candidate, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: "examples/decision_intelligence/packages/fraud_candidate.bcl"})
	if err != nil {
		log.Fatal(err)
	}
	sim, err := runtime.Orchestrator().Simulate(ctx, condition.SimulationRequest{
		Baseline:  &baseline,
		Candidate: &candidate,
		Cases:     []condition.DecisionRequest{req},
	})
	if err != nil {
		log.Fatal(err)
	}
	printJSON("simulation", sim)

	tests, err := condition.RunDecisionPackageTests(ctx, baseline)
	if err != nil {
		log.Fatal(err)
	}
	printJSON("package_tests", tests)

	jsonPkg, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: "examples/decision_intelligence/packages/fraud.json"})
	if err != nil {
		log.Fatal(err)
	}
	yamlPkg, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: "examples/decision_intelligence/packages/fraud.yaml"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nfile formats loaded: bcl=%s json=%s yaml=%s\n", baseline.Name, jsonPkg.Name, yamlPkg.Name)
}

func mustReload(ctx context.Context, runtime *condition.LiveDecisionRuntime) []condition.ReloadEvent {
	events, err := runtime.Reload(ctx)
	if err != nil {
		log.Fatal(err)
	}
	return events
}

func printJSON(label string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%s:\n%s\n", label, data)
}
