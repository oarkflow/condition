package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oarkflow/condition"
	_ "github.com/oarkflow/condition/bcl"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmp := mustTempPackage()
	runtime, err := condition.NewLiveDecisionRuntime(condition.LiveDecisionRuntimeConfig{
		Packages: []condition.DecisionPackageFile{{Path: tmp}},
		// Short intervals make the example quick; production deployments should tune these.
		PollInterval:          100 * time.Millisecond,
		StableInterval:        100 * time.Millisecond,
		ValidateBeforePublish: true,
		KeepLastGood:          true,
	})
	if err != nil {
		log.Fatal(err)
	}

	req, err := condition.LoadDecisionRequestFile(ctx, "examples/decision_intelligence/data/fraud_request.json")
	if err != nil {
		log.Fatal(err)
	}
	printDecision(ctx, runtime, req, "initial")

	events, errs := runtime.Watch(ctx)
	validPackage := readFile(tmp)
	writeFile(tmp, `module "broken" { rule_set "x" { rule "bad" { when {`)
	printWatchEvent(events, errs, "invalid_reload_keeps_last_good")
	printDecision(ctx, runtime, req, "after_invalid_reload")

	fixed := strings.Replace(validPackage, `transaction.amount > 100000`, `transaction.amount > 200000`, 1)
	writeFile(tmp, fixed)
	printWatchEvent(events, errs, "valid_reload")
	printDecision(ctx, runtime, req, "after_valid_reload")

	jsonPkg, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: "examples/decision_intelligence/packages/fraud.json"})
	if err != nil {
		log.Fatal(err)
	}
	yamlPkg, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: "examples/decision_intelligence/packages/fraud.yaml"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\njson/yaml loaders: json=%s yaml=%s\n", jsonPkg.Name, yamlPkg.Name)
}

func mustTempPackage() string {
	src := readFile("examples/decision_intelligence/packages/fraud.bcl")
	dir, err := os.MkdirTemp("", "condition-live-runtime-*")
	if err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(dir, "fraud.bcl")
	writeFile(path, src)
	return path
}

func printWatchEvent(events <-chan condition.ReloadEvent, errs <-chan error, label string) {
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			printJSON(label, event)
			return
		case err := <-errs:
			if err != nil {
				fmt.Printf("\nwatch error: %v\n", err)
			}
		case <-timeout:
			log.Fatalf("timed out waiting for %s", label)
		}
	}
}

func printDecision(ctx context.Context, runtime *condition.LiveDecisionRuntime, req condition.DecisionRequest, label string) {
	res, err := runtime.Evaluate(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	printJSON(label, map[string]any{
		"decision": res.Decision,
		"effect":   res.Effect,
		"score":    res.Score,
		"digest":   res.Digest,
	})
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	return string(data)
}

func writeFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		log.Fatal(err)
	}
}

func printJSON(label string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%s:\n%s\n", label, data)
}
