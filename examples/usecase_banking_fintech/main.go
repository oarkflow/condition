package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

const (
	packagePath = "examples/usecase_banking_fintech/package.yaml"
	requestPath = "examples/usecase_banking_fintech/request.json"
)

func main() {
	ctx := context.Background()

	pkg, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: packagePath})
	if err != nil {
		log.Fatal(err)
	}
	if diags := condition.ValidateDecisionPackage(pkg); condition.DiagnosticsHaveErrors(diags) {
		printJSON("validation_errors", diags)
		log.Fatal("package validation failed")
	}

	req, err := condition.LoadDecisionRequestFile(ctx, requestPath)
	if err != nil {
		log.Fatal(err)
	}

	orch := condition.NewDecisionOrchestrator()
	if err := orch.AddPackage(pkg); err != nil {
		log.Fatal(err)
	}
	res, err := orch.Evaluate(ctx, req)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("loaded package: %s@%s from %s\n", pkg.Name, pkg.Version, packagePath)
	fmt.Printf("loaded request: decision=%s from %s\n", req.Decision, requestPath)
	if res.Rank != nil {
		fmt.Printf("result: effect=%s decision=%s winner=%s score=%.4f\n", res.Effect, res.Decision, res.Rank.ID, res.Rank.Score)
	} else {
		fmt.Printf("result: effect=%s decision=%s score=%.4f\n", res.Effect, res.Decision, res.Score)
	}
	printJSON("response", res)
}

func printJSON(label string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%s:\n%s\n", label, data)
}
