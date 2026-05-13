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
	"path/filepath"
	"runtime"

	"github.com/oarkflow/condition"
	_ "modernc.org/sqlite"
)

func main() {
	ctx := context.Background()
	dir := exampleDir()

	pkg, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{
		Path: filepath.Join(dir, "package.yaml"),
	})
	must("load package", err)
	routes, err := condition.LoadValue(ctx, condition.FileSource{Path: filepath.Join(dir, "route_candidates.json")}, condition.JSONDecoder[condition.Dataset]())
	must("load route candidate dataset json", err)
	pkg.Datasets = []condition.Dataset{routes}

	validation, err := condition.ValidateProductionDecisionPackage(ctx, pkg)
	if err != nil && validation.Tests != nil {
		out, marshalErr := json.MarshalIndent(validation.Tests, "", "  ")
		must("marshal failed package tests", marshalErr)
		log.Printf("package tests:\n%s", out)
	}
	must("production package validation", err)

	storePath := filepath.Join(os.TempDir(), "condition-production-readiness.sqlite")
	_ = os.Remove(storePath)
	store, err := condition.NewSQLiteDecisionStore(storePath)
	must("open sqlite store", err)
	defer store.Close()

	authorizer, err := condition.NewAuthzAuthorizerFromFile(ctx, filepath.Join(dir, "access.authz"))
	must("load .authz authorizer", err)

	server, err := condition.NewProductionDecisionServer(condition.ProductionDecisionServerConfig{
		Store:      store,
		Authorizer: authorizer,
		Config:     condition.DefaultProductionDecisionServerConfig(),
	})
	must("create production decision server", err)

	record, err := server.PublishPackage(ctx, pkg)
	must("publish package to durable store", err)

	req, err := condition.LoadDecisionRequestFile(ctx, filepath.Join(dir, "request.json"))
	must("load request", err)
	response := evaluateViaHTTP(server, req)

	envelopes, err := store.ListAuditEnvelopes(ctx)
	must("list audit envelopes", err)
	must("verify audit chain", condition.VerifyAuditChain(envelopes))

	must("close sqlite store", store.Close())
	reopened, err := condition.NewSQLiteDecisionStore(storePath)
	must("reopen sqlite store", err)
	defer reopened.Close()
	_, found, err := reopened.GetPackage(ctx, pkg.Name, pkg.Version, pkg.Environment)
	must("read package after reopen", err)
	if !found {
		log.Fatal("read package after reopen: package was not found")
	}

	summary := map[string]any{
		"production_ready_layer": "single-node core/server baseline",
		"decision_intelligence_ready": []string{
			"datasets",
			"policies",
			"rankings",
			"optimization metadata",
			"package tests",
			"explainability",
			"audit chain",
			"durable package storage",
			".authz authorization",
		},
		"package":          record.Package.Name,
		"version":          record.Package.Version,
		"digest":           validation.Digest,
		"package_tests":    validation.Tests,
		"winner":           rankID(response),
		"effect":           response.Effect,
		"audit_envelopes":  len(envelopes),
		"sqlite_path":      storePath,
		"dataset_source":   "route_candidates.json",
		"request_source":   "request.json",
		"fact_source":      "request.json context",
		"reopen_verified":  found,
		"production_notes": "Use this as the single-node baseline; multi-node SaaS still needs external DB, identity provider integration, backups, tenant indexes, and SLO monitoring.",
	}
	out, err := json.MarshalIndent(summary, "", "  ")
	must("marshal summary", err)
	fmt.Println(string(out))
}

func evaluateViaHTTP(server http.Handler, req condition.DecisionRequest) condition.DecisionResponse {
	body, err := json.Marshal(req)
	must("marshal request", err)
	url := fmt.Sprintf("/v1/decision/%s/%s", req.PackageName, req.Decision)
	httpReq := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	principal := condition.Principal{
		ID:     "platform-manager",
		Tenant: "default",
		Roles:  []condition.Role{"manager"},
	}
	httpReq = httpReq.WithContext(condition.WithPrincipal(httpReq.Context(), principal))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		log.Fatalf("evaluate via production HTTP failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response condition.DecisionResponse
	must("decode response", json.NewDecoder(rec.Body).Decode(&response))
	return response
}

func rankID(response condition.DecisionResponse) string {
	if response.Rank == nil {
		return ""
	}
	return response.Rank.ID
}

func mergeFacts(items ...condition.MapFacts) condition.MapFacts {
	out := condition.MapFacts{}
	for _, item := range items {
		for key, value := range item {
			out[key] = value
		}
	}
	return out
}

func exampleDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("resolve example directory")
	}
	return filepath.Dir(file)
}

func must(label string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", label, err)
	}
}
