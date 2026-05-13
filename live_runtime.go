package condition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type DecisionPackageFile struct {
	Path        string `json:"path" yaml:"path"`
	Format      string `json:"format,omitempty" yaml:"format,omitempty"`
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Version     string `json:"version,omitempty" yaml:"version,omitempty"`
	Environment string `json:"environment,omitempty" yaml:"environment,omitempty"`
}

type LiveDecisionRuntimeConfig struct {
	Packages              []DecisionPackageFile
	PollInterval          time.Duration
	StableInterval        time.Duration
	RunPackageTests       bool
	ValidateBeforePublish bool
	KeepLastGood          bool
	Logger                DecisionLogger
	Metrics               DecisionMetrics
}

type ReloadEvent struct {
	Path        string    `json:"path"`
	Package     string    `json:"package,omitempty"`
	Version     string    `json:"version,omitempty"`
	Environment string    `json:"environment,omitempty"`
	OldDigest   string    `json:"old_digest,omitempty"`
	NewDigest   string    `json:"new_digest,omitempty"`
	Reloaded    bool      `json:"reloaded"`
	Rejected    bool      `json:"rejected"`
	Error       string    `json:"error,omitempty"`
	Err         error     `json:"-"`
	At          time.Time `json:"at"`
}

type LiveDecisionRuntime struct {
	mu           sync.RWMutex
	config       LiveDecisionRuntimeConfig
	orchestrator *DecisionOrchestrator
	files        map[string]livePackageState
	opts         []EngineOption
}

type livePackageState struct {
	file        DecisionPackageFile
	identity    string
	digest      string
	fingerprint string
	stat        liveFileStat
}

type liveFileStat struct {
	modTime time.Time
	size    int64
	hash    string
}

var (
	decisionPackageDecodersMu sync.RWMutex
	decisionPackageDecoders   = map[string]Decoder[DecisionPackage]{}
)

func init() {
	RegisterDecisionPackageDecoder("json", JSONDecoder[DecisionPackage]())
	RegisterDecisionPackageDecoder("yaml", DecoderFunc[DecisionPackage](decodeYAMLJSONTags[DecisionPackage]))
	RegisterDecisionPackageDecoder("yml", DecoderFunc[DecisionPackage](decodeYAMLJSONTags[DecisionPackage]))
}

func RegisterDecisionPackageDecoder(format string, decoder Decoder[DecisionPackage]) {
	format = normalizeDecisionFileFormat(format)
	if format == "" || decoder == nil {
		return
	}
	decisionPackageDecodersMu.Lock()
	defer decisionPackageDecodersMu.Unlock()
	decisionPackageDecoders[format] = decoder
}

func NewLiveDecisionRuntime(config LiveDecisionRuntimeConfig, opts ...EngineOption) (*LiveDecisionRuntime, error) {
	if config.PollInterval <= 0 {
		config.PollInterval = 500 * time.Millisecond
	}
	if config.StableInterval <= 0 {
		config.StableInterval = config.PollInterval
	}
	if !config.ValidateBeforePublish {
		config.ValidateBeforePublish = true
	}
	if !config.KeepLastGood {
		config.KeepLastGood = true
	}
	r := &LiveDecisionRuntime{
		config:       config,
		orchestrator: NewDecisionOrchestrator(opts...),
		files:        map[string]livePackageState{},
		opts:         opts,
	}
	events, err := r.Reload(context.Background())
	if err != nil {
		return nil, err
	}
	loaded := 0
	for _, event := range events {
		if event.Reloaded {
			loaded++
		}
	}
	if len(config.Packages) > 0 && loaded == 0 {
		return nil, errors.New("condition: live runtime loaded no packages")
	}
	return r, nil
}

func (r *LiveDecisionRuntime) Evaluate(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	return r.orchestrator.Evaluate(ctx, req)
}

func (r *LiveDecisionRuntime) Orchestrator() *DecisionOrchestrator {
	if r == nil {
		return nil
	}
	return r.orchestrator
}

func (r *LiveDecisionRuntime) Reload(ctx context.Context) ([]ReloadEvent, error) {
	if r == nil {
		return nil, errors.New("condition: live runtime is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	events := make([]ReloadEvent, 0, len(r.config.Packages))
	var firstErr error
	for _, file := range r.config.Packages {
		event := r.reloadFile(ctx, file)
		events = append(events, event)
		if event.Err != nil && firstErr == nil {
			firstErr = event.Err
		}
	}
	return events, firstErr
}

func (r *LiveDecisionRuntime) Watch(ctx context.Context) (<-chan ReloadEvent, <-chan error) {
	events := make(chan ReloadEvent, 16)
	errs := make(chan error, 4)
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		defer close(events)
		defer close(errs)
		poll := r.config.PollInterval
		if poll <= 0 {
			poll = 500 * time.Millisecond
		}
		stable := r.config.StableInterval
		if stable <= 0 {
			stable = poll
		}
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		pending := map[string]struct {
			stat  liveFileStat
			since time.Time
		}{}
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				for _, file := range r.config.Packages {
					stat, err := statDecisionFile(file.Path)
					if err != nil {
						sendReloadError(ctx, errs, err)
						continue
					}
					current := r.stateForPath(file.Path)
					if liveFileStatEqual(stat, current.stat) {
						delete(pending, file.Path)
						continue
					}
					p, ok := pending[file.Path]
					if !ok || !liveFileStatEqual(p.stat, stat) {
						pending[file.Path] = struct {
							stat  liveFileStat
							since time.Time
						}{stat: stat, since: now}
						continue
					}
					if now.Sub(p.since) < stable {
						continue
					}
					event := r.reloadFile(ctx, file)
					delete(pending, file.Path)
					if event.Err != nil {
						sendReloadError(ctx, errs, event.Err)
					}
					select {
					case events <- event:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return events, errs
}

func sendReloadError(ctx context.Context, errs chan<- error, err error) {
	select {
	case errs <- err:
	case <-ctx.Done():
	default:
	}
}

func (r *LiveDecisionRuntime) reloadFile(ctx context.Context, file DecisionPackageFile) ReloadEvent {
	event := ReloadEvent{Path: file.Path, At: time.Now().UTC()}
	pkg, err := LoadDecisionPackageFile(ctx, file)
	if err != nil {
		return rejectedReloadEvent(event, err)
	}
	event.Package = pkg.Name
	event.Version = pkg.Version
	event.Environment = pkg.Environment
	if r.config.ValidateBeforePublish {
		if diags := ValidateDecisionPackage(pkg); DiagnosticsHaveErrors(diags) {
			return rejectedReloadEvent(event, fmt.Errorf("decision package validation failed: %v", diags))
		}
	}
	if r.config.RunPackageTests && len(pkg.Tests) > 0 {
		result, err := RunDecisionPackageTests(ctx, pkg)
		if err != nil {
			return rejectedReloadEvent(event, err)
		}
		if !result.Passed {
			return rejectedReloadEvent(event, fmt.Errorf("decision package tests failed: %d/%d failed", result.Failed, result.Total))
		}
	}
	compiled, err := CompileDecisionPackage(pkg, r.opts...)
	if err != nil {
		return rejectedReloadEvent(event, err)
	}
	stat, err := statDecisionFile(file.Path)
	if err != nil {
		return rejectedReloadEvent(event, err)
	}
	identity := packageLatestKey(pkg.Name, pkg.Environment)
	old := r.stateForPath(file.Path)
	event.OldDigest = old.digest
	event.NewDigest = compiled.Digest
	if err := r.orchestrator.AddPackage(pkg); err != nil {
		return rejectedReloadEvent(event, err)
	}
	r.mu.Lock()
	r.files[file.Path] = livePackageState{file: file, identity: identity, digest: compiled.Digest, fingerprint: stat.hash, stat: stat}
	r.mu.Unlock()
	event.Reloaded = true
	r.log(ctx, "info", "live decision package reloaded", map[string]any{"path": file.Path, "package": pkg.Name, "version": pkg.Version, "digest": compiled.Digest})
	if r.config.Metrics != nil {
		r.config.Metrics.IncCounter("live_decision_runtime_reload_total", map[string]string{"status": "success", "package": pkg.Name})
	}
	return event
}

func rejectedReloadEvent(event ReloadEvent, err error) ReloadEvent {
	event.Rejected = true
	event.Err = err
	if err != nil {
		event.Error = err.Error()
	}
	return event
}

func (r *LiveDecisionRuntime) stateForPath(path string) livePackageState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.files[path]
}

func (r *LiveDecisionRuntime) log(ctx context.Context, level, message string, fields map[string]any) {
	if r != nil && r.config.Logger != nil {
		r.config.Logger.Log(ctx, level, message, fields)
	}
}

func LoadDecisionPackageFile(ctx context.Context, file DecisionPackageFile) (DecisionPackage, error) {
	var zero DecisionPackage
	if file.Path == "" {
		return zero, errors.New("condition: decision package file path is required")
	}
	format := normalizeDecisionFileFormat(file.Format)
	if format == "" {
		format = inferDecisionFileFormat(file.Path)
	}
	decoder := decisionPackageDecoder(format)
	if decoder == nil {
		if format == "bcl" {
			return zero, errors.New(`condition: bcl decoder is not registered; import "github.com/oarkflow/condition/bcl"`)
		}
		return zero, fmt.Errorf("condition: unsupported decision package format %q", format)
	}
	pkg, err := LoadDecisionPackage(ctx, FileSource{Path: file.Path}, decoder)
	if err != nil {
		return zero, err
	}
	if file.Name != "" {
		pkg.Name = file.Name
	}
	if file.Version != "" {
		pkg.Version = file.Version
	}
	if file.Environment != "" {
		pkg.Environment = file.Environment
	}
	return pkg, nil
}

func LoadDecisionRequestFile(ctx context.Context, path string) (DecisionRequest, error) {
	var req decisionRequestFile
	if err := loadJSONOrYAMLFile(ctx, path, &req); err != nil {
		return DecisionRequest{}, err
	}
	return req.toDecisionRequest(), nil
}

func LoadDecisionRequestsFile(ctx context.Context, path string) ([]DecisionRequest, error) {
	var reqs []decisionRequestFile
	if err := loadJSONOrYAMLFile(ctx, path, &reqs); err != nil {
		var wrapper struct {
			Cases []decisionRequestFile `json:"cases" yaml:"cases"`
		}
		if err2 := loadJSONOrYAMLFile(ctx, path, &wrapper); err2 != nil {
			return nil, err
		}
		reqs = wrapper.Cases
	}
	out := make([]DecisionRequest, 0, len(reqs))
	for _, req := range reqs {
		out = append(out, req.toDecisionRequest())
	}
	return out, nil
}

func LoadCandidatesFile(ctx context.Context, path string) ([]Candidate, error) {
	var candidates []candidateFile
	if err := loadJSONOrYAMLFile(ctx, path, &candidates); err != nil {
		var wrapper struct {
			Candidates []candidateFile `json:"candidates" yaml:"candidates"`
		}
		if err2 := loadJSONOrYAMLFile(ctx, path, &wrapper); err2 != nil {
			return nil, err
		}
		candidates = wrapper.Candidates
	}
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.toCandidate())
	}
	return out, nil
}

type candidateFile struct {
	ID       string         `json:"id" yaml:"id"`
	Name     string         `json:"name,omitempty" yaml:"name,omitempty"`
	Facts    MapFacts       `json:"facts,omitempty" yaml:"facts,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (c candidateFile) toCandidate() Candidate {
	return Candidate{ID: c.ID, Name: c.Name, Facts: c.Facts, Metadata: c.Metadata}
}

type decisionRequestFile struct {
	PackageName string          `json:"package_name,omitempty" yaml:"package_name,omitempty"`
	Version     string          `json:"version,omitempty" yaml:"version,omitempty"`
	Environment string          `json:"environment,omitempty" yaml:"environment,omitempty"`
	Decision    string          `json:"decision,omitempty" yaml:"decision,omitempty"`
	Context     MapFacts        `json:"context,omitempty" yaml:"context,omitempty"`
	Variables   MapFacts        `json:"variables,omitempty" yaml:"variables,omitempty"`
	Candidates  []candidateFile `json:"candidates,omitempty" yaml:"candidates,omitempty"`
	Options     DecisionOptions `json:"options,omitempty" yaml:"options,omitempty"`
}

func (r decisionRequestFile) toDecisionRequest() DecisionRequest {
	req := DecisionRequest{
		PackageName: r.PackageName,
		Version:     r.Version,
		Environment: r.Environment,
		Decision:    r.Decision,
		Context:     r.Context,
		Facts:       r.Context,
		VariableMap: r.Variables,
		Variables:   r.Variables,
		Options:     r.Options,
		Candidates:  make([]Candidate, 0, len(r.Candidates)),
	}
	for _, candidate := range r.Candidates {
		req.Candidates = append(req.Candidates, candidate.toCandidate())
	}
	return req
}

func loadJSONOrYAMLFile(ctx context.Context, path string, out any) error {
	if path == "" {
		return errors.New("condition: file path is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	switch inferDecisionFileFormat(path) {
	case "json":
		return json.Unmarshal(data, out)
	case "yaml", "yml":
		return decodeYAMLJSONTagsInto(data, out)
	default:
		return fmt.Errorf("condition: unsupported file format %q", filepath.Ext(path))
	}
}

func decisionPackageDecoder(format string) Decoder[DecisionPackage] {
	decisionPackageDecodersMu.RLock()
	defer decisionPackageDecodersMu.RUnlock()
	return decisionPackageDecoders[normalizeDecisionFileFormat(format)]
}

func normalizeDecisionFileFormat(format string) string {
	format = strings.TrimSpace(strings.ToLower(format))
	format = strings.TrimPrefix(format, ".")
	return format
}

func inferDecisionFileFormat(path string) string {
	return normalizeDecisionFileFormat(filepath.Ext(path))
}

func statDecisionFile(path string) (liveFileStat, error) {
	info, err := os.Stat(path)
	if err != nil {
		return liveFileStat{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return liveFileStat{}, err
	}
	sum := sha256.Sum256(data)
	return liveFileStat{modTime: info.ModTime(), size: info.Size(), hash: hex.EncodeToString(sum[:])}, nil
}

func liveFileStatEqual(a, b liveFileStat) bool {
	return a.size == b.size && a.hash == b.hash && a.modTime.Equal(b.modTime)
}

func decodeYAMLJSONTags[T any](data []byte) (T, error) {
	var out T
	if err := decodeYAMLJSONTagsInto(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func decodeYAMLJSONTagsInto(data []byte, out any) error {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	normalized := normalizeYAMLValue(raw)
	jsonData, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonData, out)
}

func normalizeYAMLValue(v any) any {
	switch x := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			if key, ok := k.(string); ok {
				out[key] = normalizeYAMLValue(v)
			}
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = normalizeYAMLValue(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = normalizeYAMLValue(v)
		}
		return out
	default:
		return x
	}
}
