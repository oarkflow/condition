package condition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type PackageRecord struct {
	Package     DecisionPackage `json:"package"`
	Digest      string          `json:"digest"`
	PublishedAt string          `json:"published_at"`
}

type PackageStore interface {
	SavePackage(context.Context, DecisionPackage, string) error
	ListPackages(context.Context) ([]PackageRecord, error)
	GetPackage(ctx context.Context, name, version, environment string) (PackageRecord, bool, error)
}

type AuditStore interface {
	SaveAudit(context.Context, AuditRecord) (string, error)
	ListAudits(context.Context) ([]AuditRecord, error)
	GetAudit(context.Context, string) (AuditRecord, bool, error)
	SaveAuditEnvelope(context.Context, AuditRecord) (AuditEnvelope, error)
	ListAuditEnvelopes(context.Context) ([]AuditEnvelope, error)
	GetAuditEnvelope(context.Context, string) (AuditEnvelope, bool, error)
}

type SimulationStore interface {
	SaveSimulation(context.Context, SimulationResult) (string, error)
}

type DecisionStore interface {
	PackageStore
	AuditStore
	SimulationStore
}

type AuditEnvelope struct {
	ID           string      `json:"id"`
	Sequence     uint64      `json:"sequence"`
	Timestamp    string      `json:"timestamp"`
	PreviousHash string      `json:"previous_hash,omitempty"`
	PayloadHash  string      `json:"payload_hash"`
	ChainHash    string      `json:"chain_hash"`
	Signature    string      `json:"signature,omitempty"`
	Record       AuditRecord `json:"record"`
}

type AuditChainVerifier struct{}

func (AuditChainVerifier) Verify(envelopes []AuditEnvelope) error {
	return VerifyAuditChain(envelopes)
}

func VerifyAuditChain(envelopes []AuditEnvelope) error {
	var previous string
	var sequence uint64
	for i, envelope := range envelopes {
		if envelope.Sequence == 0 {
			return fmt.Errorf("audit envelope %d has zero sequence", i)
		}
		if sequence != 0 && envelope.Sequence != sequence+1 {
			return fmt.Errorf("audit envelope %s has non-contiguous sequence", envelope.ID)
		}
		if envelope.PreviousHash != previous {
			return fmt.Errorf("audit envelope %s previous hash mismatch", envelope.ID)
		}
		payloadHash, err := auditPayloadHash(envelope.Record)
		if err != nil {
			return err
		}
		if payloadHash != envelope.PayloadHash {
			return fmt.Errorf("audit envelope %s payload hash mismatch", envelope.ID)
		}
		if auditChainHash(envelope.Sequence, envelope.Timestamp, envelope.ID, envelope.PreviousHash, envelope.PayloadHash) != envelope.ChainHash {
			return fmt.Errorf("audit envelope %s chain hash mismatch", envelope.ID)
		}
		previous = envelope.ChainHash
		sequence = envelope.Sequence
	}
	return nil
}

type MemoryDecisionStore struct {
	mu            sync.RWMutex
	packages      map[string]PackageRecord
	latest        map[string]string
	audits        map[string]AuditRecord
	envelopes     map[string]AuditEnvelope
	auditOrder    []string
	simulations   map[string]SimulationResult
	nextAudit     int64
	nextSim       int64
	lastAuditHash string
}

func NewMemoryDecisionStore() *MemoryDecisionStore {
	return &MemoryDecisionStore{
		packages:    map[string]PackageRecord{},
		latest:      map[string]string{},
		audits:      map[string]AuditRecord{},
		envelopes:   map[string]AuditEnvelope{},
		simulations: map[string]SimulationResult{},
	}
}

func (s *MemoryDecisionStore) SavePackage(ctx context.Context, pkg DecisionPackage, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := packageKey(pkg.Name, pkg.Version, pkg.Environment)
	s.packages[key] = PackageRecord{Package: pkg, Digest: digest, PublishedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	s.latest[packageLatestKey(pkg.Name, pkg.Environment)] = key
	s.latest[pkg.Name] = key
	return nil
}

func (s *MemoryDecisionStore) ListPackages(ctx context.Context) ([]PackageRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PackageRecord, 0, len(s.packages))
	for _, record := range s.packages {
		out = append(out, record)
	}
	return out, nil
}

func (s *MemoryDecisionStore) GetPackage(ctx context.Context, name, version, environment string) (PackageRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return PackageRecord{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if version != "" {
		record, ok := s.packages[packageKey(name, version, environment)]
		return record, ok, nil
	}
	key := s.latest[packageLatestKey(name, environment)]
	if key == "" {
		key = s.latest[name]
	}
	record, ok := s.packages[key]
	return record, ok, nil
}

func (s *MemoryDecisionStore) SaveAudit(ctx context.Context, audit AuditRecord) (string, error) {
	envelope, err := s.SaveAuditEnvelope(ctx, audit)
	if err != nil {
		return "", err
	}
	return envelope.ID, nil
}

func (s *MemoryDecisionStore) SaveAuditEnvelope(ctx context.Context, audit AuditRecord) (AuditEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return AuditEnvelope{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextAudit++
	id := "audit_" + strconvFormatInt(s.nextAudit)
	payloadHash, err := auditPayloadHash(audit)
	if err != nil {
		return AuditEnvelope{}, err
	}
	envelope := AuditEnvelope{
		ID:           id,
		Sequence:     uint64(s.nextAudit),
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		PreviousHash: s.lastAuditHash,
		PayloadHash:  payloadHash,
		Record:       audit,
	}
	envelope.ChainHash = auditChainHash(envelope.Sequence, envelope.Timestamp, envelope.ID, envelope.PreviousHash, envelope.PayloadHash)
	s.audits[id] = audit
	s.envelopes[id] = envelope
	s.auditOrder = append(s.auditOrder, id)
	s.lastAuditHash = envelope.ChainHash
	return envelope, nil
}

func (s *MemoryDecisionStore) ListAudits(ctx context.Context) ([]AuditRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AuditRecord, 0, len(s.auditOrder))
	for _, id := range s.auditOrder {
		out = append(out, s.audits[id])
	}
	return out, nil
}

func (s *MemoryDecisionStore) ListAuditEnvelopes(ctx context.Context) ([]AuditEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AuditEnvelope, 0, len(s.auditOrder))
	for _, id := range s.auditOrder {
		out = append(out, s.envelopes[id])
	}
	return out, nil
}

func (s *MemoryDecisionStore) GetAudit(ctx context.Context, id string) (AuditRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return AuditRecord{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	audit, ok := s.audits[id]
	return audit, ok, nil
}

func (s *MemoryDecisionStore) GetAuditEnvelope(ctx context.Context, id string) (AuditEnvelope, bool, error) {
	if err := ctx.Err(); err != nil {
		return AuditEnvelope{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	envelope, ok := s.envelopes[id]
	return envelope, ok, nil
}

func (s *MemoryDecisionStore) SaveSimulation(ctx context.Context, sim SimulationResult) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSim++
	id := "sim_" + strconvFormatInt(s.nextSim)
	s.simulations[id] = sim
	return id, nil
}

type DecisionServer struct {
	orchestrator *DecisionOrchestrator
	packages     PackageStore
	audits       AuditStore
	simulations  SimulationStore
	config       DecisionServerConfig
	auth         DecisionServerAuth
	metrics      DecisionMetrics
	logger       DecisionLogger
	rateLimiter  DecisionRateLimiter
}

type DecisionServerOption func(*DecisionServer)

type DecisionServerConfig struct {
	MaxBodyBytes        int64
	RequestTimeout      time.Duration
	RequireContentType  bool
	AllowedContentTypes []string
}

type DecisionServerAuth struct {
	Authorizer Authorizer
}

type Permission string
type Role string

const (
	PermissionPackagePublish  Permission = "package:publish"
	PermissionPackageList     Permission = "package:list"
	PermissionPackageRead     Permission = "package:read"
	PermissionDecisionEval    Permission = "decision:evaluate"
	PermissionPackageSimulate Permission = "package:simulate"
	PermissionPackageCompare  Permission = "package:compare"
	PermissionPackageTest     Permission = "package:test"
	PermissionAuditRead       Permission = "audit:read"
)

type Principal struct {
	ID          string         `json:"id"`
	Tenant      string         `json:"tenant,omitempty"`
	Roles       []Role         `json:"roles,omitempty"`
	Permissions []Permission   `json:"permissions,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Authorizer interface {
	Authorize(context.Context, Principal, Permission, string) error
}

type AuthorizerFunc func(context.Context, Principal, Permission, string) error

func (f AuthorizerFunc) Authorize(ctx context.Context, p Principal, perm Permission, resource string) error {
	return f(ctx, p, perm, resource)
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

type DecisionRateLimiter interface {
	Allow(context.Context, *http.Request) bool
}

type DecisionMetrics interface {
	IncCounter(name string, labels map[string]string)
	ObserveDuration(name string, duration time.Duration, labels map[string]string)
}

type DecisionLogger interface {
	Log(ctx context.Context, level, message string, fields map[string]any)
}

type requestIDContextKey struct{}

var decisionServerRequestCounter uint64

const defaultDecisionServerMaxBodyBytes int64 = 10 << 20

func defaultDecisionServerConfig() DecisionServerConfig {
	return DecisionServerConfig{MaxBodyBytes: defaultDecisionServerMaxBodyBytes, RequestTimeout: 30 * time.Second}
}

func WithDecisionServerStore(store interface {
	PackageStore
	AuditStore
	SimulationStore
}) DecisionServerOption {
	return func(s *DecisionServer) {
		s.packages = store
		s.audits = store
		s.simulations = store
	}
}

func WithDecisionServerConfig(config DecisionServerConfig) DecisionServerOption {
	return func(s *DecisionServer) {
		if config.MaxBodyBytes <= 0 {
			config.MaxBodyBytes = defaultDecisionServerMaxBodyBytes
		}
		if config.RequestTimeout <= 0 {
			config.RequestTimeout = defaultDecisionServerConfig().RequestTimeout
		}
		s.config = config
	}
}

func WithDecisionServerAuthorizer(authorizer Authorizer) DecisionServerOption {
	return func(s *DecisionServer) {
		s.auth.Authorizer = authorizer
	}
}

func WithDecisionServerMetrics(metrics DecisionMetrics) DecisionServerOption {
	return func(s *DecisionServer) { s.metrics = metrics }
}

func WithDecisionServerLogger(logger DecisionLogger) DecisionServerOption {
	return func(s *DecisionServer) { s.logger = logger }
}

func WithDecisionServerRateLimiter(rateLimiter DecisionRateLimiter) DecisionServerOption {
	return func(s *DecisionServer) { s.rateLimiter = rateLimiter }
}

func WithDecisionServerRuntime(runtime *LiveDecisionRuntime) DecisionServerOption {
	return func(s *DecisionServer) {
		if runtime != nil && runtime.Orchestrator() != nil {
			s.orchestrator = runtime.Orchestrator()
		}
	}
}

func NewDecisionServer(opts ...DecisionServerOption) *DecisionServer {
	store := NewMemoryDecisionStore()
	s := &DecisionServer{
		orchestrator: NewDecisionOrchestrator(),
		packages:     store,
		audits:       store,
		simulations:  store,
		config:       defaultDecisionServerConfig(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *DecisionServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = "req_" + strconvFormatInt(int64(atomic.AddUint64(&decisionServerRequestCounter, 1)))
	}
	w.Header().Set("X-Request-ID", requestID)
	ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
	var cancel context.CancelFunc
	if s.config.RequestTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, s.config.RequestTimeout)
		defer cancel()
	}
	r = r.WithContext(ctx)
	if s.config.MaxBodyBytes > 0 && r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxBodyBytes)
	}
	if s.rateLimiter != nil && !s.rateLimiter.Allow(ctx, r) {
		s.writeError(w, r, http.StatusTooManyRequests, "rate_limited", "request rate limit exceeded")
		return
	}
	defer func() {
		if s.metrics != nil {
			s.metrics.ObserveDuration("decision_server_request_duration", time.Since(start), map[string]string{"method": r.Method, "path": r.URL.Path})
		}
		if s.logger != nil {
			s.logger.Log(ctx, "info", "decision server request", map[string]any{"method": r.Method, "path": r.URL.Path, "request_id": requestID, "duration": time.Since(start).String()})
		}
	}()
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "v1" {
		s.writeError(w, r, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	switch {
	case r.Method == http.MethodPost && path == "v1/packages":
		if !s.allow(w, r, PermissionPackagePublish, "packages") || !s.requireContentType(w, r, "application/json", "application/bcl", "text/plain") {
			return
		}
		s.handlePublishPackage(w, r)
	case r.Method == http.MethodGet && path == "v1/packages":
		if !s.allow(w, r, PermissionPackageList, "packages") {
			return
		}
		s.handleListPackages(w, r)
	case r.Method == http.MethodPost && path == "v1/packages/compare":
		if !s.allow(w, r, PermissionPackageCompare, "packages") || !s.requireContentType(w, r, "application/json") {
			return
		}
		s.handleComparePackages(w, r)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "packages":
		if !s.allow(w, r, PermissionPackageRead, parts[2]) {
			return
		}
		s.handleGetPackage(w, r, parts[2], "", "")
	case r.Method == http.MethodGet && len(parts) == 5 && parts[1] == "packages" && parts[3] == "versions":
		if !s.allow(w, r, PermissionPackageRead, parts[2]) {
			return
		}
		s.handleGetPackage(w, r, parts[2], parts[4], r.URL.Query().Get("environment"))
	case r.Method == http.MethodPost && len(parts) == 4 && parts[1] == "decision":
		if !s.allow(w, r, PermissionDecisionEval, parts[2]+"/"+parts[3]) || !s.requireContentType(w, r, "application/json") {
			return
		}
		s.handleEvaluate(w, r, parts[2], parts[3])
	case r.Method == http.MethodPost && len(parts) == 4 && parts[1] == "packages" && parts[3] == "simulate":
		if !s.allow(w, r, PermissionPackageSimulate, parts[2]) || !s.requireContentType(w, r, "application/json") {
			return
		}
		s.handleSimulate(w, r, parts[2])
	case r.Method == http.MethodPost && len(parts) == 4 && parts[1] == "packages" && parts[3] == "tests":
		if !s.allow(w, r, PermissionPackageTest, parts[2]) {
			return
		}
		s.handleTests(w, r, parts[2])
	case r.Method == http.MethodGet && path == "v1/audit":
		if !s.allow(w, r, PermissionAuditRead, "audit") {
			return
		}
		s.handleListAudits(w, r)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "audit":
		if !s.allow(w, r, PermissionAuditRead, parts[2]) {
			return
		}
		s.handleGetAudit(w, r, parts[2])
	default:
		s.writeError(w, r, http.StatusNotFound, "not_found", "endpoint not found")
	}
}

func (s *DecisionServer) handlePublishPackage(w http.ResponseWriter, r *http.Request) {
	pkg, err := decodePackageRequest(r)
	if err != nil {
		writeDecisionError(w, http.StatusBadRequest, "invalid_package", err.Error())
		return
	}
	record, err := s.PublishPackage(r.Context(), pkg)
	if err != nil {
		status := http.StatusBadRequest
		code := "publish_failed"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusRequestTimeout
			code = "request_timeout"
		}
		writeDecisionError(w, status, code, err.Error())
		return
	}
	w.Header().Set("X-Decision-Package-Digest", record.Digest)
	writeJSON(w, http.StatusCreated, record)
}

func (s *DecisionServer) PublishPackage(ctx context.Context, pkg DecisionPackage) (PackageRecord, error) {
	if diags := ValidateDecisionPackage(pkg); DiagnosticsHaveErrors(diags) {
		return PackageRecord{}, fmt.Errorf("decision package validation failed: %v", diags)
	}
	compiled, err := CompileDecisionPackage(pkg)
	if err != nil {
		return PackageRecord{}, err
	}
	if err := s.orchestrator.AddPackage(pkg); err != nil {
		return PackageRecord{}, err
	}
	if err := s.packages.SavePackage(ctx, pkg, compiled.Digest); err != nil {
		return PackageRecord{}, err
	}
	return PackageRecord{Package: pkg, Digest: compiled.Digest, PublishedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func (s *DecisionServer) handleListPackages(w http.ResponseWriter, r *http.Request) {
	records, err := s.packages.ListPackages(r.Context())
	if err != nil {
		writeDecisionError(w, http.StatusInternalServerError, "store_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *DecisionServer) handleGetPackage(w http.ResponseWriter, r *http.Request, name, version, environment string) {
	record, ok, err := s.packages.GetPackage(r.Context(), name, version, environment)
	if err != nil {
		writeDecisionError(w, http.StatusInternalServerError, "store_failed", err.Error())
		return
	}
	if !ok {
		writeDecisionError(w, http.StatusNotFound, "not_found", "package not found")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *DecisionServer) handleEvaluate(w http.ResponseWriter, r *http.Request, packageName, decision string) {
	req, err := decodeDecisionHTTPBody(r.Body)
	if err != nil && !errors.Is(err, io.EOF) {
		writeDecisionError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.PackageName = packageName
	req.Decision = decision
	res, err := s.orchestrator.Evaluate(r.Context(), req)
	if err != nil {
		writeDecisionError(w, http.StatusBadRequest, "evaluate_failed", err.Error())
		return
	}
	if _, err := s.audits.SaveAuditEnvelope(r.Context(), res.Audit); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "audit_persist_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type decisionHTTPRequest struct {
	Version     string                  `json:"version,omitempty"`
	Environment string                  `json:"environment,omitempty"`
	Context     MapFacts                `json:"context,omitempty"`
	Variables   MapFacts                `json:"variables,omitempty"`
	Candidates  []decisionHTTPCandidate `json:"candidates,omitempty"`
	Options     DecisionOptions         `json:"options,omitempty"`
}

type decisionHTTPCandidate struct {
	ID       string         `json:"id"`
	Name     string         `json:"name,omitempty"`
	Facts    MapFacts       `json:"facts,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func decodeDecisionHTTPBody(r io.Reader) (DecisionRequest, error) {
	var in decisionHTTPRequest
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return DecisionRequest{}, err
	}
	req := DecisionRequest{
		Version:     in.Version,
		Environment: in.Environment,
		Context:     in.Context,
		VariableMap: in.Variables,
		Options:     in.Options,
		Candidates:  make([]Candidate, 0, len(in.Candidates)),
	}
	for _, candidate := range in.Candidates {
		req.Candidates = append(req.Candidates, Candidate{
			ID:       candidate.ID,
			Name:     candidate.Name,
			Facts:    candidate.Facts,
			Metadata: candidate.Metadata,
		})
	}
	return req, nil
}

func (s *DecisionServer) handleSimulate(w http.ResponseWriter, r *http.Request, packageName string) {
	var req SimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDecisionError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.PackageName = packageName
	res, err := s.orchestrator.Simulate(r.Context(), req)
	if err != nil {
		writeDecisionError(w, http.StatusBadRequest, "simulate_failed", err.Error())
		return
	}
	_, _ = s.simulations.SaveSimulation(r.Context(), res)
	writeJSON(w, http.StatusOK, res)
}

func (s *DecisionServer) handleComparePackages(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Left  DecisionPackage `json:"left"`
		Right DecisionPackage `json:"right"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDecisionError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	diff, err := s.orchestrator.ComparePackages(req.Left, req.Right)
	if err != nil {
		writeDecisionError(w, http.StatusBadRequest, "compare_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (s *DecisionServer) handleTests(w http.ResponseWriter, r *http.Request, packageName string) {
	record, ok, err := s.packages.GetPackage(r.Context(), packageName, r.URL.Query().Get("version"), r.URL.Query().Get("environment"))
	if err != nil {
		writeDecisionError(w, http.StatusInternalServerError, "store_failed", err.Error())
		return
	}
	if !ok {
		writeDecisionError(w, http.StatusNotFound, "not_found", "package not found")
		return
	}
	res, err := RunDecisionPackageTests(r.Context(), record.Package)
	if err != nil {
		writeDecisionError(w, http.StatusBadRequest, "tests_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *DecisionServer) handleListAudits(w http.ResponseWriter, r *http.Request) {
	records, err := s.audits.ListAuditEnvelopes(r.Context())
	if err != nil {
		writeDecisionError(w, http.StatusInternalServerError, "store_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *DecisionServer) handleGetAudit(w http.ResponseWriter, r *http.Request, id string) {
	audit, ok, err := s.audits.GetAuditEnvelope(r.Context(), id)
	if err != nil {
		writeDecisionError(w, http.StatusInternalServerError, "store_failed", err.Error())
		return
	}
	if !ok {
		writeDecisionError(w, http.StatusNotFound, "not_found", "audit not found")
		return
	}
	writeJSON(w, http.StatusOK, audit)
}

func decodePackageRequest(r *http.Request) (DecisionPackage, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return DecisionPackage{}, err
	}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "bcl") || strings.Contains(ct, "text/plain") || bytesHasPrefixNonSpace(data, []byte("module")) {
		return DecisionPackage{}, errors.New("BCL upload moved to github.com/oarkflow/condition/bcl; parse BCL and publish JSON or call PublishPackage")
	}
	var pkg DecisionPackage
	err = json.Unmarshal(data, &pkg)
	return pkg, err
}

func bytesHasPrefixNonSpace(data, prefix []byte) bool {
	data = []byte(strings.TrimSpace(string(data)))
	return len(data) >= len(prefix) && string(data[:len(prefix)]) == string(prefix)
}

func (s *DecisionServer) allow(w http.ResponseWriter, r *http.Request, permission Permission, resource string) bool {
	if s.auth.Authorizer == nil {
		return true
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok || principal.ID == "" {
		s.writeError(w, r, http.StatusUnauthorized, "unauthorized", "principal is required")
		return false
	}
	if err := s.auth.Authorizer.Authorize(r.Context(), principal, permission, resource); err != nil {
		s.writeError(w, r, http.StatusForbidden, "forbidden", err.Error())
		return false
	}
	return true
}

func (s *DecisionServer) requireContentType(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if !s.config.RequireContentType || r.Body == nil {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if ct == "" {
		s.writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_content_type", "content-type is required")
		return false
	}
	for _, allow := range allowed {
		if ct == strings.ToLower(allow) {
			return true
		}
	}
	for _, allow := range s.config.AllowedContentTypes {
		if ct == strings.ToLower(allow) {
			return true
		}
	}
	s.writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_content_type", "content-type is not supported")
	return false
}

func (s *DecisionServer) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if s.metrics != nil {
		s.metrics.IncCounter("decision_server_errors_total", map[string]string{"code": code, "status": strconvFormatInt(int64(status))})
	}
	if s.logger != nil {
		s.logger.Log(r.Context(), "warn", "decision server error", map[string]any{"code": code, "status": status, "message": message, "request_id": requestIDFromContext(r.Context())})
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestIDFromContext(r.Context())}})
}

func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeDecisionError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func auditPayloadHash(record AuditRecord) (string, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func auditChainHash(sequence uint64, timestamp, id, previousHash, payloadHash string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d\n%s\n%s\n%s\n%s", sequence, timestamp, id, previousHash, payloadHash)
	return hex.EncodeToString(h.Sum(nil))
}

func strconvFormatInt(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
