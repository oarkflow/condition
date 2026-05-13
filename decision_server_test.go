package condition

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDecisionServerPublishEvaluateTestAndAudit(t *testing.T) {
	server := NewDecisionServer()
	publishBody, _ := json.Marshal(testDecisionPackage())
	publish := httptest.NewRequest(http.MethodPost, "/v1/packages", bytes.NewReader(publishBody))
	publish.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, publish)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish status = %d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Decision-Package-Digest") == "" {
		t.Fatal("missing package digest header")
	}

	body := `{"context":{"customer":{"blacklisted":false,"country":"NP"},"transaction":{"amount":125000}},"candidates":[{"id":"aml","facts":{"active":true,"priority":9,"load":0.2}}]}`
	eval := httptest.NewRequest(http.MethodPost, "/v1/decision/fraud-intelligence/fraud-review", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	server.ServeHTTP(w, eval)
	if w.Code != http.StatusOK {
		t.Fatalf("evaluate status = %d body=%s", w.Code, w.Body.String())
	}
	var res DecisionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.Effect != EffectRequireReview || res.Audit.PackageDigest == "" {
		t.Fatalf("unexpected decision response: %#v", res)
	}

	tests := httptest.NewRequest(http.MethodPost, "/v1/packages/fraud-intelligence/tests", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, tests)
	if w.Code != http.StatusOK {
		t.Fatalf("tests status = %d body=%s", w.Code, w.Body.String())
	}

	audits := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, audits)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("package_digest")) {
		t.Fatalf("audit status = %d body=%s", w.Code, w.Body.String())
	}
	var envelopes []AuditEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelopes); err != nil {
		t.Fatalf("decode audit envelopes: %v", err)
	}
	if err := VerifyAuditChain(envelopes); err != nil {
		t.Fatalf("audit chain should verify: %v", err)
	}
}

func TestDecisionServerCompareSimulateAndConcurrentEvaluate(t *testing.T) {
	server := NewDecisionServer()
	w := httptest.NewRecorder()
	pkg := testDecisionPackage()
	publishBody, _ := json.Marshal(pkg)
	req := httptest.NewRequest(http.MethodPost, "/v1/packages", bytes.NewReader(publishBody))
	req.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish status = %d body=%s", w.Code, w.Body.String())
	}

	next := pkg
	next.Version = "2"
	next.RuleSets[0].Rules[0].Condition = `transaction.amount > 200000`
	compareBody, _ := json.Marshal(map[string]any{"left": pkg, "right": next})
	w = httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/packages/compare", bytes.NewReader(compareBody)))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("changed")) {
		t.Fatalf("compare status = %d body=%s", w.Code, w.Body.String())
	}

	simBody, _ := json.Marshal(SimulationRequest{
		Baseline:  &pkg,
		Candidate: &next,
		Cases: []DecisionRequest{{
			Decision: "fraud-review",
			Context:  MapFacts{"customer": map[string]any{"blacklisted": false, "country": "NP"}, "transaction": map[string]any{"amount": 125000}},
		}},
	})
	w = httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/packages/fraud-intelligence/simulate", bytes.NewReader(simBody)))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("score_changes")) {
		t.Fatalf("simulate status = %d body=%s", w.Code, w.Body.String())
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := `{"context":{"customer":{"blacklisted":false,"country":"NP"},"transaction":{"amount":125000}}}`
			w := httptest.NewRecorder()
			server.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/decision/fraud-intelligence/fraud-review", bytes.NewBufferString(body)))
			if w.Code != http.StatusOK {
				t.Errorf("concurrent evaluate status = %d body=%s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
}

func TestMemoryDecisionStoreConformance(t *testing.T) {
	StoreConformanceSuite(t, NewMemoryDecisionStore())
}

func TestAuditChainVerifierDetectsTampering(t *testing.T) {
	store := NewMemoryDecisionStore()
	_, _ = store.SaveAuditEnvelope(context.Background(), AuditRecord{Package: "a"})
	_, _ = store.SaveAuditEnvelope(context.Background(), AuditRecord{Package: "b"})
	envelopes, err := store.ListAuditEnvelopes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyAuditChain(envelopes); err != nil {
		t.Fatalf("valid chain rejected: %v", err)
	}
	envelopes[1].Record.Package = "tampered"
	if err := VerifyAuditChain(envelopes); err == nil {
		t.Fatal("expected tampered chain to fail")
	}
}

func TestDecisionServerRBAC(t *testing.T) {
	server := NewDecisionServer(WithDecisionServerAuthorizer(AuthorizerFunc(func(ctx context.Context, p Principal, perm Permission, resource string) error {
		for _, allowed := range p.Permissions {
			if allowed == perm {
				return nil
			}
		}
		return errors.New("permission denied")
	})))
	req := httptest.NewRequest(http.MethodGet, "/v1/packages", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing principal status = %d body=%s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/packages", nil).WithContext(WithPrincipal(context.Background(), Principal{ID: "u1", Permissions: []Permission{PermissionPackageList}}))
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("authorized list status = %d body=%s", w.Code, w.Body.String())
	}
	publishBody, _ := json.Marshal(testDecisionPackage())
	req = httptest.NewRequest(http.MethodPost, "/v1/packages", bytes.NewReader(publishBody)).WithContext(WithPrincipal(context.Background(), Principal{ID: "u1", Permissions: []Permission{PermissionPackageList}}))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("forbidden publish status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestDecisionServerRBACMissingPrincipalAllEndpoints(t *testing.T) {
	server := NewDecisionServer(WithDecisionServerAuthorizer(AuthorizerFunc(func(context.Context, Principal, Permission, string) error {
		return nil
	})))
	publishBody, _ := json.Marshal(testDecisionPackage())
	cases := []struct {
		method string
		path   string
		body   string
		ct     string
	}{
		{http.MethodPost, "/v1/packages", string(publishBody), "application/json"},
		{http.MethodGet, "/v1/packages", "", ""},
		{http.MethodGet, "/v1/packages/fraud-intelligence", "", ""},
		{http.MethodGet, "/v1/packages/fraud-intelligence/versions/1", "", ""},
		{http.MethodPost, "/v1/packages/compare", `{}`, "application/json"},
		{http.MethodPost, "/v1/decision/fraud-intelligence/fraud-review", `{}`, "application/json"},
		{http.MethodPost, "/v1/packages/fraud-intelligence/simulate", `{}`, "application/json"},
		{http.MethodPost, "/v1/packages/fraud-intelligence/tests", "", ""},
		{http.MethodGet, "/v1/audit", "", ""},
		{http.MethodGet, "/v1/audit/audit_1", "", ""},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		if tc.ct != "" {
			req.Header.Set("Content-Type", tc.ct)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func TestDecisionServerHardeningAndObservability(t *testing.T) {
	metrics := &fakeDecisionMetrics{}
	logger := &fakeDecisionLogger{}
	publishBody, _ := json.Marshal(testDecisionPackage())
	server := NewDecisionServer(
		WithDecisionServerConfig(DecisionServerConfig{MaxBodyBytes: 8, RequestTimeout: time.Second, RequireContentType: true}),
		WithDecisionServerMetrics(metrics),
		WithDecisionServerLogger(logger),
	)
	req := httptest.NewRequest(http.MethodPost, "/v1/packages", bytes.NewReader(publishBody))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("missing content type status = %d body=%s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/packages", bytes.NewReader(publishBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || w.Header().Get("X-Request-ID") == "" {
		t.Fatalf("oversized body status = %d request_id=%q body=%s", w.Code, w.Header().Get("X-Request-ID"), w.Body.String())
	}
	if metrics.durations == 0 || logger.entries == 0 {
		t.Fatalf("observability hooks not called: metrics=%#v logger=%#v", metrics, logger)
	}
}

type fakeDecisionMetrics struct {
	counters  int
	durations int
}

func (f *fakeDecisionMetrics) IncCounter(string, map[string]string) {
	f.counters++
}

func (f *fakeDecisionMetrics) ObserveDuration(string, time.Duration, map[string]string) {
	f.durations++
}

type fakeDecisionLogger struct {
	entries int
}

func (f *fakeDecisionLogger) Log(context.Context, string, string, map[string]any) {
	f.entries++
}
