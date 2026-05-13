package condition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oarkflow/condition"
	_ "github.com/oarkflow/condition/bcl"
)

func TestDecisionPackageFileLoadersBCLJSONYAML(t *testing.T) {
	dir := t.TempDir()
	bclPath := filepath.Join(dir, "pkg.bcl")
	jsonPath := filepath.Join(dir, "pkg.json")
	yamlPath := filepath.Join(dir, "pkg.yaml")
	writeTestFile(t, bclPath, liveTestBCL("1", "100"))
	pkg, err := condition.LoadDecisionPackageFile(context.Background(), condition.DecisionPackageFile{Path: bclPath})
	if err != nil {
		t.Fatalf("LoadDecisionPackageFile BCL returned error: %v", err)
	}
	data, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, jsonPath, string(data))
	writeTestFile(t, yamlPath, `name: live-test
version: "1"
environment: test
rule_sets:
  - name: review
    rules:
      - id: "1"
        condition: transaction.amount > 100
        decision: review
`)
	for _, path := range []string{jsonPath, yamlPath} {
		got, err := condition.LoadDecisionPackageFile(context.Background(), condition.DecisionPackageFile{Path: path})
		if err != nil {
			t.Fatalf("LoadDecisionPackageFile(%s) returned error: %v", path, err)
		}
		if got.Name != "live-test" || got.RuleSets[0].Rules[0].ID != "1" {
			t.Fatalf("unexpected package from %s: %#v", path, got)
		}
	}
}

func TestDecisionRequestAndCandidateLoaders(t *testing.T) {
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "request.json")
	candidatesPath := filepath.Join(dir, "candidates.yaml")
	writeTestFile(t, reqPath, `{
  "package_name": "live-test",
  "decision": "review",
  "context": {"transaction": {"amount": 150}},
  "candidates": [{"id": "queue-a", "facts": {"active": true, "priority": 10}}]
}`)
	req, err := condition.LoadDecisionRequestFile(context.Background(), reqPath)
	if err != nil {
		t.Fatalf("LoadDecisionRequestFile returned error: %v", err)
	}
	if req.PackageName != "live-test" || len(req.Candidates) != 1 {
		t.Fatalf("unexpected request: %#v", req)
	}
	if _, ok := req.Candidates[0].Facts.(condition.MapFacts)["priority"]; !ok {
		t.Fatalf("candidate facts were not loaded: %#v", req.Candidates[0])
	}
	writeTestFile(t, candidatesPath, `candidates:
  - id: queue-a
    facts:
      active: true
      priority: 10
`)
	candidates, err := condition.LoadCandidatesFile(context.Background(), candidatesPath)
	if err != nil {
		t.Fatalf("LoadCandidatesFile returned error: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != "queue-a" {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestLiveDecisionRuntimeReloadKeepsLastGood(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg.bcl")
	writeTestFile(t, path, liveTestBCL("1", "100"))
	runtime, err := condition.NewLiveDecisionRuntime(condition.LiveDecisionRuntimeConfig{
		Packages: []condition.DecisionPackageFile{{Path: path}},
	})
	if err != nil {
		t.Fatalf("NewLiveDecisionRuntime returned error: %v", err)
	}
	req := condition.DecisionRequest{PackageName: "live-test", Decision: "review", Context: condition.MapFacts{"transaction": map[string]any{"amount": 150}}}
	before, err := runtime.Evaluate(ctx, req)
	if err != nil {
		t.Fatalf("Evaluate before reload returned error: %v", err)
	}
	writeTestFile(t, path, `module "broken" { rule_set "review" { rule "x" { when {`)
	events, err := runtime.Reload(ctx)
	if err == nil {
		t.Fatal("Reload returned nil error for malformed BCL")
	}
	if len(events) != 1 || !events[0].Rejected {
		t.Fatalf("expected rejected event, got %#v", events)
	}
	afterInvalid, err := runtime.Evaluate(ctx, req)
	if err != nil {
		t.Fatalf("Evaluate after invalid reload returned error: %v", err)
	}
	if afterInvalid.Digest != before.Digest {
		t.Fatalf("invalid reload changed digest: before=%s after=%s", before.Digest, afterInvalid.Digest)
	}
	writeTestFile(t, path, liveTestBCL("1", "200"))
	events, err = runtime.Reload(ctx)
	if err != nil {
		t.Fatalf("valid Reload returned error: %v", err)
	}
	if len(events) != 1 || !events[0].Reloaded || events[0].NewDigest == before.Digest {
		t.Fatalf("expected changed reload event, got %#v", events)
	}
	afterValid, err := runtime.Evaluate(ctx, req)
	if err != nil {
		t.Fatalf("Evaluate after valid reload returned error: %v", err)
	}
	if afterValid.Decision == "review" {
		t.Fatalf("expected changed behavior after threshold reload, got %#v", afterValid)
	}
}

func TestLiveDecisionRuntimeWatchDetectsChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg.bcl")
	writeTestFile(t, path, liveTestBCL("1", "100"))
	runtime, err := condition.NewLiveDecisionRuntime(condition.LiveDecisionRuntimeConfig{
		Packages:       []condition.DecisionPackageFile{{Path: path}},
		PollInterval:   20 * time.Millisecond,
		StableInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLiveDecisionRuntime returned error: %v", err)
	}
	events, errs := runtime.Watch(ctx)
	writeTestFile(t, path, liveTestBCL("2", "200"))
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Reloaded && event.Version == "2" {
				return
			}
		case err := <-errs:
			t.Fatalf("watch error: %v", err)
		case <-timeout:
			t.Fatal("timed out waiting for reload event")
		}
	}
}

func TestLiveDecisionRuntimePackageTestsCanBlockPublish(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg.bcl")
	writeTestFile(t, path, liveTestBCLWithTest("1", "100", "deny"))
	_, err := condition.NewLiveDecisionRuntime(condition.LiveDecisionRuntimeConfig{
		Packages:        []condition.DecisionPackageFile{{Path: path}},
		RunPackageTests: true,
	})
	if err == nil {
		t.Fatal("NewLiveDecisionRuntime returned nil error for failing embedded test")
	}
}

func TestDecisionServerWithLiveRuntime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg.bcl")
	writeTestFile(t, path, liveTestBCL("1", "100"))
	runtime, err := condition.NewLiveDecisionRuntime(condition.LiveDecisionRuntimeConfig{Packages: []condition.DecisionPackageFile{{Path: path}}})
	if err != nil {
		t.Fatalf("NewLiveDecisionRuntime returned error: %v", err)
	}
	server := condition.NewDecisionServer(condition.WithDecisionServerRuntime(runtime))
	req := condition.DecisionRequest{PackageName: "live-test", Decision: "review", Context: condition.MapFacts{"transaction": map[string]any{"amount": 150}}}
	res, err := serverDecisionEvaluate(server, req)
	if err != nil {
		t.Fatalf("server evaluate returned error: %v", err)
	}
	if res.Decision != "review" {
		t.Fatalf("unexpected decision before reload: %#v", res)
	}
	writeTestFile(t, path, liveTestBCL("1", "200"))
	if _, err := runtime.Reload(context.Background()); err != nil {
		t.Fatalf("Reload returned error: %v", err)
	}
	res, err = serverDecisionEvaluate(server, req)
	if err != nil {
		t.Fatalf("server evaluate after reload returned error: %v", err)
	}
	if res.Decision == "review" {
		t.Fatalf("expected server behavior to change after live reload: %#v", res)
	}
}

func serverDecisionEvaluate(server *condition.DecisionServer, req condition.DecisionRequest) (condition.DecisionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return condition.DecisionResponse{}, err
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/decision/live-test/review", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, httpReq)
	if w.Code < 200 || w.Code >= 300 {
		return condition.DecisionResponse{}, fmt.Errorf("server returned %d: %s", w.Code, w.Body.String())
	}
	var res condition.DecisionResponse
	err = json.Unmarshal(w.Body.Bytes(), &res)
	return res, err
}

func liveTestBCL(version, threshold string) string {
	src := strings.ReplaceAll(`module "live-test" {
  version = "__VERSION__"
  environment = "test"
  rule_set "review" {
    rule "1" {
      when { transaction.amount > __THRESHOLD__ }
      then { decision = "review" }
    }
  }
}`, "__VERSION__", version)
	return strings.ReplaceAll(src, "__THRESHOLD__", threshold)
}

func liveTestBCLWithTest(version, threshold, expectedDecision string) string {
	base := liveTestBCL(version, threshold)
	testBlock := `
test "decision expectation" {
  decision = "review"
  input { transaction.amount = 150 }
  expect { decision = "` + expectedDecision + `" }
}
`
	idx := strings.LastIndex(base, "}")
	if idx < 0 {
		return base
	}
	return base[:idx] + testBlock + base[idx:]
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) returned error: %v", path, err)
	}
}
