package condition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DecisionEffect string

const (
	EffectAbstain       DecisionEffect = "abstain"
	EffectAllow         DecisionEffect = "allow"
	EffectDeny          DecisionEffect = "deny"
	EffectRequireReview DecisionEffect = "require_review"
	EffectEscalate      DecisionEffect = "escalate"
)

type DecisionPackage struct {
	Name          string             `json:"name"`
	Version       string             `json:"version,omitempty"`
	Environment   string             `json:"environment,omitempty"`
	Metadata      map[string]any     `json:"metadata,omitempty"`
	Constants     map[string]any     `json:"constants,omitempty"`
	Variables     MapFacts           `json:"variables,omitempty"`
	Imports       []string           `json:"imports,omitempty"`
	Schemas       map[string]Schema  `json:"schemas,omitempty"`
	Datasets      []Dataset          `json:"datasets,omitempty"`
	RuleSets      []RuleSet          `json:"rule_sets,omitempty"`
	Policies      []Policy           `json:"policies,omitempty"`
	Rankings      []Ranking          `json:"rankings,omitempty"`
	Workflows     []Workflow         `json:"workflows,omitempty"`
	Optimizations []Optimization     `json:"optimizations,omitempty"`
	Actions       []ActionDefinition `json:"actions,omitempty"`
	Tests         []DecisionTestCase `json:"tests,omitempty"`
	Governance    Governance         `json:"governance,omitempty"`
}

type Dataset struct {
	Name    string          `json:"name"`
	Records []DatasetRecord `json:"records,omitempty"`
}

type DatasetRecord struct {
	ID       string         `json:"id"`
	Name     string         `json:"name,omitempty"`
	Facts    MapFacts       `json:"facts,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Schema struct {
	Required   []string              `json:"required,omitempty"`
	Types      map[string]string     `json:"types,omitempty"`
	Rules      map[string]SchemaRule `json:"rules,omitempty"`
	Properties map[string]SchemaRule `json:"properties,omitempty"`
}

type SchemaRule struct {
	Type       string                `json:"type,omitempty"`
	Required   bool                  `json:"required,omitempty"`
	Enum       []any                 `json:"enum,omitempty"`
	Min        *float64              `json:"min,omitempty"`
	Max        *float64              `json:"max,omitempty"`
	MinLength  *int                  `json:"min_length,omitempty"`
	MaxLength  *int                  `json:"max_length,omitempty"`
	MinItems   *int                  `json:"min_items,omitempty"`
	MaxItems   *int                  `json:"max_items,omitempty"`
	Properties map[string]SchemaRule `json:"properties,omitempty"`
}

type Governance struct {
	Owner      string   `json:"owner,omitempty"`
	Maker      string   `json:"maker,omitempty"`
	Checkers   []string `json:"checkers,omitempty"`
	Approvers  []string `json:"approvers,omitempty"`
	Signatures []string `json:"signatures,omitempty"`
	Published  int64    `json:"published,omitempty"`
}

type Policy struct {
	Name          string         `json:"name"`
	Decision      string         `json:"decision,omitempty"`
	DefaultEffect DecisionEffect `json:"default_effect,omitempty"`
	Rules         []PolicyRule   `json:"rules,omitempty"`
}

type PolicyRule struct {
	ID          string         `json:"id"`
	Name        string         `json:"name,omitempty"`
	Effect      DecisionEffect `json:"effect,omitempty"`
	Condition   string         `json:"condition,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Score       float64        `json:"score,omitempty"`
	Actions     []Action       `json:"actions,omitempty"`
	Events      []Event        `json:"events,omitempty"`
	StopOnMatch bool           `json:"stop_on_match,omitempty"`
}

type Ranking struct {
	Name            string        `json:"name"`
	Decision        string        `json:"decision,omitempty"`
	Dataset         string        `json:"dataset,omitempty"`
	RuleSet         RuleSet       `json:"rule_set"`
	PriorityPath    string        `json:"priority_path,omitempty"`
	SpecificityPath string        `json:"specificity_path,omitempty"`
	FallbackPath    string        `json:"fallback_path,omitempty"`
	CostPath        string        `json:"cost_path,omitempty"`
	WeightPath      string        `json:"weight_path,omitempty"`
	HashKeyPath     string        `json:"hash_key_path,omitempty"`
	Selection       SelectionMode `json:"selection,omitempty"`
}

type SelectionMode string

const (
	SelectionRanked   SelectionMode = "ranked"
	SelectionBest     SelectionMode = "best"
	SelectionWeighted SelectionMode = "weighted"
)

type Optimization struct {
	Name        string         `json:"name"`
	Decision    string         `json:"decision,omitempty"`
	Dataset     string         `json:"dataset,omitempty"`
	Goal        string         `json:"goal,omitempty"`
	Constraints []Rule         `json:"constraints,omitempty"`
	Preferences []ScoreRule    `json:"preferences,omitempty"`
	Ranking     string         `json:"ranking,omitempty"`
	Selection   SelectionMode  `json:"selection,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ActionDefinition struct {
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type DecisionTestCase struct {
	Name      string   `json:"name"`
	Decision  string   `json:"decision,omitempty"`
	Context   MapFacts `json:"context,omitempty"`
	Variables MapFacts `json:"variables,omitempty"`
	Expect    MapFacts `json:"expect,omitempty"`
}

type DecisionRequest struct {
	PackageName string          `json:"package_name,omitempty"`
	Version     string          `json:"version,omitempty"`
	Environment string          `json:"environment,omitempty"`
	Decision    string          `json:"decision,omitempty"`
	Facts       Facts           `json:"-"`
	Context     MapFacts        `json:"context,omitempty"`
	Variables   Facts           `json:"-"`
	VariableMap MapFacts        `json:"variables,omitempty"`
	Candidates  []Candidate     `json:"candidates,omitempty"`
	Options     DecisionOptions `json:"options,omitempty"`
}

type DecisionOptions struct {
	Explain       bool `json:"explain,omitempty"`
	Trace         bool `json:"trace,omitempty"`
	IncludeFailed bool `json:"include_failed,omitempty"`
}

type DecisionResponse struct {
	Decision       string           `json:"decision,omitempty"`
	Allowed        bool             `json:"allowed"`
	Effect         DecisionEffect   `json:"effect,omitempty"`
	Score          float64          `json:"score,omitempty"`
	Rank           *CandidateResult `json:"rank,omitempty"`
	Ranking        *RankingResult   `json:"ranking,omitempty"`
	Actions        []Action         `json:"actions,omitempty"`
	Events         []Event          `json:"events,omitempty"`
	MatchedRuleIDs []RuleID         `json:"matched_rule_ids,omitempty"`
	Version        string           `json:"version,omitempty"`
	Digest         string           `json:"digest,omitempty"`
	Explanation    Explanation      `json:"explanation,omitempty"`
	Audit          AuditRecord      `json:"audit"`
}

type Explanation struct {
	Package         string               `json:"package,omitempty"`
	Version         string               `json:"version,omitempty"`
	Digest          string               `json:"digest,omitempty"`
	MatchedRules    []EvidenceNode       `json:"matched_rules,omitempty"`
	FailedRules     []EvidenceNode       `json:"failed_rules,omitempty"`
	PolicyEffects   []PolicyEffectRecord `json:"policy_effects,omitempty"`
	ScoreDeltas     []ScoreDelta         `json:"score_deltas,omitempty"`
	RankingReasons  []RankingEvidence    `json:"ranking_reasons,omitempty"`
	WorkflowPath    []WorkflowEvidence   `json:"workflow_path,omitempty"`
	FactEvidence    []FactEvidence       `json:"fact_evidence,omitempty"`
	Counterfactuals []CounterfactualHint `json:"counterfactual_hints,omitempty"`
	Evidence        []EvidenceNode       `json:"evidence_graph,omitempty"`
}

type EvidenceNode struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Source    string         `json:"source,omitempty"`
	RuleID    string         `json:"rule_id,omitempty"`
	Condition string         `json:"condition,omitempty"`
	Matched   bool           `json:"matched"`
	Effect    DecisionEffect `json:"effect,omitempty"`
	Decision  string         `json:"decision,omitempty"`
	Score     float64        `json:"score,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	FactPaths []string       `json:"fact_paths,omitempty"`
	Parents   []string       `json:"parents,omitempty"`
}

type PolicyEffectRecord struct {
	Policy string         `json:"policy"`
	RuleID string         `json:"rule_id,omitempty"`
	Effect DecisionEffect `json:"effect"`
	Reason string         `json:"reason,omitempty"`
	Score  float64        `json:"score,omitempty"`
}

type ScoreDelta struct {
	Source string  `json:"source"`
	RuleID string  `json:"rule_id,omitempty"`
	Delta  float64 `json:"delta"`
	Reason string  `json:"reason,omitempty"`
}

type RankingEvidence struct {
	Ranking   string         `json:"ranking"`
	Winner    string         `json:"winner,omitempty"`
	Selection SelectionMode  `json:"selection,omitempty"`
	Score     float64        `json:"score,omitempty"`
	Reasons   []string       `json:"reasons,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type WorkflowEvidence struct {
	Workflow string `json:"workflow"`
	Stage    string `json:"stage,omitempty"`
	RuleID   string `json:"rule_id,omitempty"`
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type FactEvidence struct {
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

type CounterfactualHint struct {
	Source  string `json:"source"`
	RuleID  string `json:"rule_id,omitempty"`
	Path    string `json:"path,omitempty"`
	Current any    `json:"current,omitempty"`
	Target  any    `json:"target,omitempty"`
	Hint    string `json:"hint"`
}

type AuditRecord struct {
	Package            string `json:"package,omitempty"`
	Version            string `json:"version,omitempty"`
	Environment        string `json:"environment,omitempty"`
	PackageDigest      string `json:"package_digest,omitempty"`
	RequestFingerprint string `json:"request_fingerprint,omitempty"`
	ResultFingerprint  string `json:"result_fingerprint,omitempty"`
	StartedAt          string `json:"started_at,omitempty"`
	CompletedAt        string `json:"completed_at,omitempty"`
	Duration           string `json:"duration,omitempty"`
	TraceSummary       string `json:"trace_summary,omitempty"`
}

type SimulationRequest struct {
	PackageName string            `json:"package_name,omitempty"`
	Version     string            `json:"version,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Cases       []DecisionRequest `json:"cases"`
	Baseline    *DecisionPackage  `json:"baseline,omitempty"`
	Candidate   *DecisionPackage  `json:"candidate,omitempty"`
}

type SimulationResult struct {
	Cases   []SimulationCaseResult `json:"cases"`
	Summary SimulationSummary      `json:"summary"`
}

type SimulationCaseResult struct {
	Name      string           `json:"name,omitempty"`
	Baseline  DecisionResponse `json:"baseline,omitempty"`
	Candidate DecisionResponse `json:"candidate"`
	Diff      DecisionDiff     `json:"diff"`
}

type SimulationSummary struct {
	Total           int `json:"total"`
	DecisionChanges int `json:"decision_changes"`
	EffectChanges   int `json:"effect_changes"`
	ScoreChanges    int `json:"score_changes"`
	ActionChanges   int `json:"action_changes"`
	RankChanges     int `json:"rank_changes"`
}

type DecisionDiff struct {
	DecisionChanged bool     `json:"decision_changed,omitempty"`
	EffectChanged   bool     `json:"effect_changed,omitempty"`
	ScoreChanged    bool     `json:"score_changed,omitempty"`
	ActionsChanged  bool     `json:"actions_changed,omitempty"`
	RankChanged     bool     `json:"rank_changed,omitempty"`
	MatchedAdded    []RuleID `json:"matched_added,omitempty"`
	MatchedRemoved  []RuleID `json:"matched_removed,omitempty"`
}

type PackageDiff struct {
	LeftDigest   string   `json:"left_digest"`
	RightDigest  string   `json:"right_digest"`
	Changed      bool     `json:"changed"`
	ChangedParts []string `json:"changed_parts,omitempty"`
}

type DecisionOrchestrator struct {
	mu       sync.RWMutex
	packages map[string]*CompiledDecisionPackage
	latest   map[string]string
	opts     []EngineOption
}

type CompiledDecisionPackage struct {
	Package       DecisionPackage
	Digest        string
	engine        *Engine
	rankings      map[string]*compiledRanking
	optimizations map[string]*compiledOptimization
	policies      []compiledPolicy
	datasets      map[string][]Candidate
}

type compiledPolicy struct {
	def   Policy
	rules []compiledPolicyRule
}

type compiledPolicyRule struct {
	def  PolicyRule
	expr *Expression
}

type compiledRanking struct {
	def    Ranking
	ranker *Ranker
}

type compiledOptimization struct {
	def    Optimization
	ranker *Ranker
}

func NewDecisionOrchestrator(opts ...EngineOption) *DecisionOrchestrator {
	return &DecisionOrchestrator{
		packages: make(map[string]*CompiledDecisionPackage),
		latest:   make(map[string]string),
		opts:     opts,
	}
}

func (o *DecisionOrchestrator) AddPackage(pkg DecisionPackage) error {
	compiled, err := CompileDecisionPackage(pkg, o.opts...)
	if err != nil {
		return err
	}
	key := packageKey(pkg.Name, pkg.Version, pkg.Environment)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.packages[key] = compiled
	o.latest[packageLatestKey(pkg.Name, pkg.Environment)] = key
	return nil
}

func CompileDecisionPackage(pkg DecisionPackage, opts ...EngineOption) (*CompiledDecisionPackage, error) {
	if pkg.Name == "" {
		return nil, newError(ErrRuleSet, 0, "decision package name is required")
	}
	digest, err := PackageDigest(pkg)
	if err != nil {
		return nil, err
	}
	engine := NewEngine(opts...)
	for _, rs := range pkg.RuleSets {
		if err := engine.AddRuleSet(rs); err != nil {
			return nil, err
		}
	}
	if len(pkg.Workflows) > 0 {
		if err := engine.AddRuleSet(RuleSet{Name: platformWorkflowRuleSetName(pkg), Workflows: pkg.Workflows}); err != nil {
			return nil, err
		}
	}
	cp := &CompiledDecisionPackage{
		Package:       pkg,
		Digest:        digest,
		engine:        engine,
		rankings:      make(map[string]*compiledRanking),
		optimizations: make(map[string]*compiledOptimization),
		datasets:      compileDatasets(pkg.Datasets),
	}
	for _, policy := range pkg.Policies {
		compiled, err := compilePolicy(policy)
		if err != nil {
			return nil, err
		}
		cp.policies = append(cp.policies, compiled)
	}
	for _, ranking := range pkg.Rankings {
		name := ranking.Name
		if name == "" {
			name = ranking.RuleSet.Name
		}
		if name == "" {
			return nil, newError(ErrRuleSet, 0, "ranking name is required")
		}
		rs := ranking.RuleSet
		if rs.Name == "" {
			rs.Name = name
		}
		ranker, err := NewRanker(rs, rankingOptions(ranking)...)
		if err != nil {
			return nil, err
		}
		ranking.Name = name
		cp.rankings[name] = &compiledRanking{def: ranking, ranker: ranker}
	}
	for _, opt := range pkg.Optimizations {
		name := opt.Name
		if name == "" {
			return nil, newError(ErrRuleSet, 0, "optimization name is required")
		}
		rs := RuleSet{Name: name, Rules: opt.Constraints, ScoreRules: opt.Preferences}
		if opt.Ranking != "" {
			if ranked := cp.rankings[opt.Ranking]; ranked != nil {
				cp.optimizations[name] = &compiledOptimization{def: opt, ranker: ranked.ranker}
				continue
			}
		}
		ranker, err := NewRanker(rs)
		if err != nil {
			return nil, err
		}
		cp.optimizations[name] = &compiledOptimization{def: opt, ranker: ranker}
	}
	return cp, nil
}

func (o *DecisionOrchestrator) Evaluate(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	cp, err := o.resolve(req)
	if err != nil {
		return DecisionResponse{}, err
	}
	return cp.Evaluate(ctx, req)
}

func (cp *CompiledDecisionPackage) Evaluate(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now().UTC()
	facts := requestFacts(req)
	vars := cp.requestVars(req)
	if err := cp.validate(req.Decision, facts); err != nil {
		return DecisionResponse{}, err
	}
	explain := Explanation{Package: cp.Package.Name, Version: cp.Package.Version, Digest: cp.Digest}
	var response DecisionResponse
	response.Version = cp.Package.Version
	response.Digest = cp.Digest
	response.Effect = EffectAbstain
	response.Decision = string(EffectAbstain)

	policyResult, err := cp.evaluatePolicies(ctx, req.Decision, facts, vars, &explain)
	if err != nil {
		return DecisionResponse{}, err
	}
	response.Score += policyResult.score
	response.Actions = append(response.Actions, policyResult.actions...)
	response.Events = append(response.Events, policyResult.events...)
	response.Effect = policyResult.effect
	response.Decision = string(policyResult.effect)
	response.Allowed = policyResult.effect == EffectAllow

	for _, rs := range cp.Package.RuleSets {
		if !decisionMatches(req.Decision, rs.Name, rs.Namespace) {
			continue
		}
		res, err := cp.engine.Evaluate(ctx, facts, rs.Name)
		if err != nil {
			return response, err
		}
		cp.explainRuleSet(ctx, rs, facts, vars, res, &explain)
		response.MatchedRuleIDs = append(response.MatchedRuleIDs, res.MatchedRuleIDs...)
		response.Score += res.Score
		response.Actions = append(response.Actions, res.Actions...)
		response.Events = append(response.Events, res.Events...)
		if res.Decision != "" {
			response.Decision = res.Decision
			eff := normalizeEffect(res.Decision)
			if eff != EffectAbstain {
				response.Effect = resolveEffect(response.Effect, eff)
			}
		}
	}
	if len(cp.Package.Workflows) > 0 {
		res, err := cp.engine.Evaluate(ctx, facts, platformWorkflowRuleSetName(cp.Package))
		if err != nil {
			return response, err
		}
		response.MatchedRuleIDs = append(response.MatchedRuleIDs, res.MatchedRuleIDs...)
		response.Score += res.Score
		response.Actions = append(response.Actions, res.Actions...)
		response.Events = append(response.Events, res.Events...)
		if res.Decision != "" {
			response.Decision = res.Decision
			response.Effect = resolveEffect(response.Effect, normalizeEffect(res.Decision))
		}
		cp.explainWorkflows(res, &explain)
	}
	req.Candidates = cp.resolveRequestCandidates(req.Candidates, req.Decision)
	if len(req.Candidates) > 0 {
		if err := cp.evaluateRankings(ctx, req, facts, vars, &response, &explain); err != nil {
			return response, err
		}
		if err := cp.evaluateOptimizations(ctx, req, facts, vars, &response, &explain); err != nil {
			return response, err
		}
	}
	response.Effect = resolveEffect(response.Effect, policyResult.effect)
	applyEffect(&response)
	if response.Decision == "" || response.Decision == string(EffectAbstain) {
		response.Decision = string(response.Effect)
	}
	explain.FactEvidence = collectFactEvidence(facts, explain.Evidence)
	response.Explanation = explain
	completed := time.Now().UTC()
	response.Audit = AuditRecord{
		Package:            cp.Package.Name,
		Version:            cp.Package.Version,
		Environment:        cp.Package.Environment,
		PackageDigest:      cp.Digest,
		RequestFingerprint: fingerprint(requestFingerprintPayload(req)),
		StartedAt:          start.Format(time.RFC3339Nano),
		CompletedAt:        completed.Format(time.RFC3339Nano),
		Duration:           completed.Sub(start).String(),
		TraceSummary:       traceSummary(response),
	}
	response.Audit.ResultFingerprint = fingerprint(resultFingerprintPayload(response))
	return response, nil
}

func (o *DecisionOrchestrator) Simulate(ctx context.Context, req SimulationRequest) (SimulationResult, error) {
	var baseline *CompiledDecisionPackage
	var candidate *CompiledDecisionPackage
	var err error
	if req.Baseline != nil {
		baseline, err = CompileDecisionPackage(*req.Baseline, o.opts...)
		if err != nil {
			return SimulationResult{}, err
		}
	}
	if req.Candidate != nil {
		candidate, err = CompileDecisionPackage(*req.Candidate, o.opts...)
		if err != nil {
			return SimulationResult{}, err
		}
	} else {
		candidate, err = o.resolve(DecisionRequest{PackageName: req.PackageName, Version: req.Version, Environment: req.Environment})
		if err != nil {
			return SimulationResult{}, err
		}
	}
	out := SimulationResult{Cases: make([]SimulationCaseResult, 0, len(req.Cases))}
	for i, c := range req.Cases {
		if c.Decision == "" {
			c.Decision = req.Cases[i].Decision
		}
		cand, err := candidate.Evaluate(ctx, c)
		if err != nil {
			return out, err
		}
		item := SimulationCaseResult{Name: c.Decision, Candidate: cand}
		if baseline != nil {
			base, err := baseline.Evaluate(ctx, c)
			if err != nil {
				return out, err
			}
			item.Baseline = base
			item.Diff = diffDecision(base, cand)
		}
		out.Cases = append(out.Cases, item)
		applySimulationSummary(&out.Summary, item.Diff)
	}
	out.Summary.Total = len(out.Cases)
	return out, nil
}

func (o *DecisionOrchestrator) ComparePackages(left, right DecisionPackage) (PackageDiff, error) {
	leftDigest, err := PackageDigest(left)
	if err != nil {
		return PackageDiff{}, err
	}
	rightDigest, err := PackageDigest(right)
	if err != nil {
		return PackageDiff{}, err
	}
	diff := PackageDiff{LeftDigest: leftDigest, RightDigest: rightDigest, Changed: leftDigest != rightDigest}
	if !diff.Changed {
		return diff, nil
	}
	if !reflect.DeepEqual(left.RuleSets, right.RuleSets) {
		diff.ChangedParts = append(diff.ChangedParts, "rule_sets")
	}
	if !reflect.DeepEqual(left.Policies, right.Policies) {
		diff.ChangedParts = append(diff.ChangedParts, "policies")
	}
	if !reflect.DeepEqual(left.Rankings, right.Rankings) {
		diff.ChangedParts = append(diff.ChangedParts, "rankings")
	}
	if !reflect.DeepEqual(left.Workflows, right.Workflows) {
		diff.ChangedParts = append(diff.ChangedParts, "workflows")
	}
	if !reflect.DeepEqual(left.Optimizations, right.Optimizations) {
		diff.ChangedParts = append(diff.ChangedParts, "optimizations")
	}
	if !reflect.DeepEqual(left.Datasets, right.Datasets) {
		diff.ChangedParts = append(diff.ChangedParts, "datasets")
	}
	if !reflect.DeepEqual(left.Constants, right.Constants) || !reflect.DeepEqual(left.Variables, right.Variables) {
		diff.ChangedParts = append(diff.ChangedParts, "configuration")
	}
	sort.Strings(diff.ChangedParts)
	return diff, nil
}

func LoadDecisionPackage(ctx context.Context, source Source, decoder Decoder[DecisionPackage]) (DecisionPackage, error) {
	return LoadValue(ctx, source, decoder)
}

func LoadDecisionPackages(ctx context.Context, source Source, decoder Decoder[[]DecisionPackage]) ([]DecisionPackage, error) {
	return LoadValue(ctx, source, decoder)
}

func PackageDigest(pkg DecisionPackage) (string, error) {
	clone := pkg
	clone.Metadata = cloneMap(clone.Metadata)
	if clone.Metadata == nil {
		clone.Metadata = map[string]any{}
	}
	delete(clone.Metadata, "digest")
	data, err := json.Marshal(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type policyEvalResult struct {
	effect  DecisionEffect
	score   float64
	actions []Action
	events  []Event
}

func compilePolicy(policy Policy) (compiledPolicy, error) {
	if policy.Name == "" {
		return compiledPolicy{}, newError(ErrRuleSet, 0, "policy name is required")
	}
	out := compiledPolicy{def: policy}
	for _, rule := range policy.Rules {
		if rule.ID == "" {
			return out, newError(ErrRule, 0, "policy %q rule id is required", policy.Name)
		}
		var expr *Expression
		var err error
		if rule.Condition != "" {
			expr, err = Compile(rule.Condition, WithTrace(true))
			if err != nil {
				return out, err
			}
		}
		out.rules = append(out.rules, compiledPolicyRule{def: rule, expr: expr})
	}
	return out, nil
}

func (cp *CompiledDecisionPackage) evaluatePolicies(ctx context.Context, decision string, facts, vars Facts, ex *Explanation) (policyEvalResult, error) {
	out := policyEvalResult{effect: EffectAbstain}
	for _, policy := range cp.policies {
		if !decisionMatches(decision, policy.def.Name, policy.def.Decision) {
			continue
		}
		matchedAny := false
		for _, rule := range policy.rules {
			matched := true
			var err error
			if rule.expr != nil {
				var res Result
				res, err = rule.expr.EvalWithVariables(ctx, facts, vars)
				matched = res.Matched
			}
			if err != nil {
				return out, err
			}
			node := evidenceForPolicy(policy.def, rule, matched)
			ex.Evidence = append(ex.Evidence, node)
			if !matched {
				ex.FailedRules = append(ex.FailedRules, node)
				if hint := counterfactualFromCondition("policy:"+policy.def.Name, rule.def.ID, rule.def.Condition, facts); hint.Hint != "" {
					ex.Counterfactuals = append(ex.Counterfactuals, hint)
				}
				continue
			}
			matchedAny = true
			effect := normalizeEffect(string(rule.def.Effect))
			if effect == EffectAbstain && rule.def.Effect == "" {
				effect = EffectAbstain
			}
			out.effect = resolveEffect(out.effect, effect)
			out.score += rule.def.Score
			out.actions = append(out.actions, rule.def.Actions...)
			out.events = append(out.events, rule.def.Events...)
			record := PolicyEffectRecord{Policy: policy.def.Name, RuleID: rule.def.ID, Effect: effect, Reason: rule.def.Reason, Score: rule.def.Score}
			ex.PolicyEffects = append(ex.PolicyEffects, record)
			ex.MatchedRules = append(ex.MatchedRules, node)
			if rule.def.Score != 0 {
				ex.ScoreDeltas = append(ex.ScoreDeltas, ScoreDelta{Source: "policy:" + policy.def.Name, RuleID: rule.def.ID, Delta: rule.def.Score, Reason: rule.def.Reason})
			}
			if rule.def.StopOnMatch {
				break
			}
		}
		if !matchedAny && policy.def.DefaultEffect != "" {
			effect := normalizeEffect(string(policy.def.DefaultEffect))
			out.effect = resolveEffect(out.effect, effect)
			ex.PolicyEffects = append(ex.PolicyEffects, PolicyEffectRecord{Policy: policy.def.Name, Effect: effect, Reason: "default effect"})
		}
	}
	return out, nil
}

func (cp *CompiledDecisionPackage) explainRuleSet(ctx context.Context, rs RuleSet, facts, vars Facts, res Result, ex *Explanation) {
	matched := make(map[string]bool, len(res.MatchedRuleIDs))
	for _, id := range res.MatchedRuleIDs {
		matched[strconv.FormatUint(uint64(id), 10)] = true
	}
	for _, rule := range rs.Rules {
		ruleMatched := matched[rule.ID] || matched[strconv.FormatUint(uint64(parseRuleID(rule.ID)), 10)]
		if !ruleMatched && rule.Condition != "" {
			expr, err := Compile(rule.Condition)
			if err == nil {
				eval, evalErr := expr.EvalWithVariables(ctx, facts, vars)
				if evalErr == nil {
					ruleMatched = eval.Matched
				}
			}
		}
		node := evidenceForRule(rs.Name, rule, ruleMatched)
		ex.Evidence = append(ex.Evidence, node)
		if ruleMatched {
			ex.MatchedRules = append(ex.MatchedRules, node)
			if rule.Score != 0 {
				ex.ScoreDeltas = append(ex.ScoreDeltas, ScoreDelta{Source: "ruleset:" + rs.Name, RuleID: rule.ID, Delta: rule.Score, Reason: rule.Reason})
			}
		} else {
			ex.FailedRules = append(ex.FailedRules, node)
			if hint := counterfactualFromCondition("ruleset:"+rs.Name, rule.ID, rule.Condition, facts); hint.Hint != "" {
				ex.Counterfactuals = append(ex.Counterfactuals, hint)
			}
		}
	}
}

func (cp *CompiledDecisionPackage) evaluateRankings(ctx context.Context, req DecisionRequest, facts, vars Facts, response *DecisionResponse, ex *Explanation) error {
	names := sortedRankingNames(cp.rankings)
	for _, name := range names {
		ranking := cp.rankings[name]
		if !decisionMatches(req.Decision, ranking.def.Name, ranking.def.Decision) {
			continue
		}
		rankReq := RankingRequest{Facts: facts, Variables: vars, Candidates: req.Candidates}
		result, err := ranking.ranker.Rank(ctx, rankReq)
		if err != nil {
			return err
		}
		response.Ranking = &result
		if result.Winner != nil {
			winner := *result.Winner
			response.Rank = &winner
			response.Score += winner.Score
			response.Actions = append(response.Actions, winner.Actions...)
			response.Events = append(response.Events, winner.Events...)
			ex.RankingReasons = append(ex.RankingReasons, RankingEvidence{Ranking: name, Winner: winner.ID, Selection: ranking.def.Selection, Score: winner.Score, Reasons: winner.Reasons})
		}
	}
	return nil
}

func (cp *CompiledDecisionPackage) resolveRequestCandidates(candidates []Candidate, decision string) []Candidate {
	if len(candidates) > 0 {
		return candidates
	}
	for _, ranking := range cp.Package.Rankings {
		if !decisionMatches(decision, ranking.Name, ranking.Decision) || ranking.Dataset == "" {
			continue
		}
		if ds := cp.datasets[ranking.Dataset]; len(ds) > 0 {
			return cloneCandidates(ds)
		}
	}
	for _, opt := range cp.Package.Optimizations {
		if !decisionMatches(decision, opt.Name, opt.Decision) || opt.Dataset == "" {
			continue
		}
		if ds := cp.datasets[opt.Dataset]; len(ds) > 0 {
			return cloneCandidates(ds)
		}
	}
	return nil
}

func compileDatasets(datasets []Dataset) map[string][]Candidate {
	out := make(map[string][]Candidate, len(datasets))
	for _, dataset := range datasets {
		if dataset.Name == "" {
			continue
		}
		candidates := make([]Candidate, 0, len(dataset.Records))
		for _, record := range dataset.Records {
			facts := record.Facts
			if facts == nil {
				facts = MapFacts{}
			}
			candidates = append(candidates, Candidate{
				ID:       record.ID,
				Name:     record.Name,
				Facts:    facts,
				Metadata: cloneMap(record.Metadata),
			})
		}
		out[dataset.Name] = candidates
	}
	return out
}

func cloneCandidates(in []Candidate) []Candidate {
	out := make([]Candidate, len(in))
	copy(out, in)
	return out
}

type mergedFacts struct {
	primary   MapFacts
	fallback  Facts
	secondary Facts
}

func (m mergedFacts) Get(path string) (any, bool) {
	if v, ok := m.primary.Get(path); ok {
		return v, true
	}
	if m.fallback == nil {
		if m.secondary == nil {
			return nil, false
		}
		return m.secondary.Get(path)
	}
	if v, ok := m.fallback.Get(path); ok {
		return v, true
	}
	if m.secondary == nil {
		return nil, false
	}
	return m.secondary.Get(path)
}

func (cp *CompiledDecisionPackage) evaluateOptimizations(ctx context.Context, req DecisionRequest, facts, vars Facts, response *DecisionResponse, ex *Explanation) error {
	names := sortedOptimizationNames(cp.optimizations)
	for _, name := range names {
		opt := cp.optimizations[name]
		if !decisionMatches(req.Decision, opt.def.Name, opt.def.Decision) {
			continue
		}
		rankReq := RankingRequest{Facts: facts, Variables: vars, Candidates: req.Candidates}
		selection := opt.def.Selection
		if selection == "" {
			selection = SelectionBest
		}
		var winner Selection
		var ok bool
		var err error
		switch selection {
		case SelectionWeighted:
			winner, ok, err = opt.ranker.SelectWeightedID(ctx, rankReq)
		default:
			winner, ok, err = opt.ranker.SelectBestID(ctx, rankReq)
		}
		if err != nil {
			return err
		}
		if ok {
			ex.RankingReasons = append(ex.RankingReasons, RankingEvidence{
				Ranking:   name,
				Winner:    winner.ID,
				Selection: selection,
				Score:     winner.Score,
				Metadata:  map[string]any{"goal": opt.def.Goal},
			})
		}
	}
	return nil
}

func (cp *CompiledDecisionPackage) explainWorkflows(res Result, ex *Explanation) {
	for _, id := range res.MatchedRuleIDs {
		ex.WorkflowPath = append(ex.WorkflowPath, WorkflowEvidence{Workflow: "package_workflows", RuleID: strconv.FormatUint(uint64(id), 10)})
	}
}

func (cp *CompiledDecisionPackage) validate(decision string, facts Facts) error {
	if len(cp.Package.Schemas) == 0 {
		return nil
	}
	schema, ok := cp.Package.Schemas[decision]
	if !ok {
		schema = cp.Package.Schemas["default"]
	}
	for _, path := range schema.Required {
		if _, ok := facts.Get(path); !ok {
			return newError(ErrMissing, 0, "decision schema requires fact %q", path)
		}
	}
	for path, want := range schema.Types {
		value, ok := facts.Get(path)
		if !ok {
			continue
		}
		if !schemaTypeMatches(value, want) {
			return newError(ErrType, 0, "decision schema fact %q must be %s", path, want)
		}
	}
	for path, rule := range schema.Rules {
		if err := validateSchemaRule(path, rule, facts); err != nil {
			return err
		}
	}
	for path, rule := range schema.Properties {
		if err := validateSchemaRule(path, rule, facts); err != nil {
			return err
		}
	}
	return nil
}

func (o *DecisionOrchestrator) resolve(req DecisionRequest) (*CompiledDecisionPackage, error) {
	name := req.PackageName
	o.mu.RLock()
	defer o.mu.RUnlock()
	if name == "" && len(o.latest) == 1 {
		for _, key := range o.latest {
			return o.packages[key], nil
		}
	}
	key := packageKey(name, req.Version, req.Environment)
	if pkg := o.packages[key]; pkg != nil {
		return pkg, nil
	}
	if req.Version == "" {
		if latest := o.latest[packageLatestKey(name, req.Environment)]; latest != "" {
			return o.packages[latest], nil
		}
		if req.Environment == "" {
			prefix := name + "\x00"
			for latestKey, key := range o.latest {
				if strings.HasPrefix(latestKey, prefix) {
					return o.packages[key], nil
				}
			}
		}
	}
	return nil, newError(ErrRuleSet, 0, "decision package %q version %q environment %q not found", name, req.Version, req.Environment)
}

func rankingOptions(r Ranking) []RankerOption {
	var opts []RankerOption
	if r.PriorityPath != "" || r.SpecificityPath != "" || r.FallbackPath != "" || r.CostPath != "" {
		opts = append(opts, WithRoutingTieBreakerPaths(r.PriorityPath, r.SpecificityPath, r.FallbackPath, r.CostPath))
	}
	if r.WeightPath != "" || r.HashKeyPath != "" {
		opts = append(opts, WithWeightedSelection(r.WeightPath, r.HashKeyPath))
	}
	return opts
}

func requestFacts(req DecisionRequest) Facts {
	if req.Facts != nil {
		return req.Facts
	}
	return req.Context
}

func (cp *CompiledDecisionPackage) requestVars(req DecisionRequest) Facts {
	var base Facts
	if len(cp.Package.Variables) > 0 {
		base = cp.Package.Variables
	}
	if req.Variables != nil {
		if len(req.VariableMap) > 0 {
			return mergedFacts{primary: req.VariableMap, fallback: req.Variables, secondary: base}
		}
		return mergedFacts{fallback: req.Variables, secondary: base}
	}
	if len(req.VariableMap) > 0 {
		return mergedFacts{primary: req.VariableMap, fallback: base}
	}
	return base
}

func decisionMatches(requested string, names ...string) bool {
	if requested == "" {
		return true
	}
	for _, name := range names {
		if name != "" && requested == name {
			return true
		}
	}
	return false
}

func workflowMatches(workflows []Workflow, decision string) bool {
	for _, wf := range workflows {
		if wf.Name == decision {
			return true
		}
	}
	return false
}

func normalizeEffect(raw string) DecisionEffect {
	switch DecisionEffect(strings.ToLower(strings.TrimSpace(raw))) {
	case EffectAllow:
		return EffectAllow
	case EffectDeny:
		return EffectDeny
	case EffectRequireReview:
		return EffectRequireReview
	case EffectEscalate:
		return EffectEscalate
	case EffectAbstain, "":
		return EffectAbstain
	default:
		return DecisionEffect(strings.ToLower(strings.TrimSpace(raw)))
	}
}

func effectRank(effect DecisionEffect) int {
	switch effect {
	case EffectDeny:
		return 500
	case EffectRequireReview:
		return 400
	case EffectEscalate:
		return 300
	case EffectAllow:
		return 200
	case EffectAbstain, "":
		return 0
	default:
		return 100
	}
}

func resolveEffect(current, next DecisionEffect) DecisionEffect {
	if effectRank(next) >= effectRank(current) {
		return next
	}
	return current
}

func applyEffect(res *DecisionResponse) {
	switch res.Effect {
	case EffectAllow:
		res.Allowed = true
	case EffectDeny, EffectRequireReview, EffectEscalate:
		res.Allowed = false
	default:
		if strings.EqualFold(res.Decision, "allow") {
			res.Allowed = true
		}
	}
}

func evidenceForPolicy(policy Policy, rule compiledPolicyRule, matched bool) EvidenceNode {
	return EvidenceNode{
		ID:        evidenceID("policy", policy.Name, rule.def.ID),
		Kind:      "policy",
		Source:    policy.Name,
		RuleID:    rule.def.ID,
		Condition: rule.def.Condition,
		Matched:   matched,
		Effect:    normalizeEffect(string(rule.def.Effect)),
		Score:     rule.def.Score,
		Reason:    rule.def.Reason,
		FactPaths: expressionPaths(rule.expr),
	}
}

func evidenceForRule(source string, rule Rule, matched bool) EvidenceNode {
	var paths []string
	if rule.Condition != "" {
		if expr, err := Compile(rule.Condition); err == nil {
			paths = append(paths, expr.paths...)
		}
	}
	return EvidenceNode{
		ID:        evidenceID("rule", source, rule.ID),
		Kind:      "rule",
		Source:    source,
		RuleID:    rule.ID,
		Condition: rule.Condition,
		Matched:   matched,
		Decision:  rule.Decision,
		Effect:    normalizeEffect(rule.Decision),
		Score:     rule.Score,
		Reason:    rule.Reason,
		FactPaths: paths,
	}
}

func expressionPaths(expr *Expression) []string {
	if expr == nil {
		return nil
	}
	out := append([]string(nil), expr.paths...)
	sort.Strings(out)
	return out
}

func evidenceID(parts ...string) string {
	return fingerprint(parts)[:16]
}

func counterfactualFromCondition(source, ruleID, condition string, facts Facts) CounterfactualHint {
	fields := strings.Fields(condition)
	if len(fields) < 3 {
		return CounterfactualHint{}
	}
	path, op, rawTarget := fields[0], fields[1], strings.Trim(fields[2], `"'`)
	if !strings.Contains(" == != > >= < <= ", " "+op+" ") {
		return CounterfactualHint{}
	}
	current, ok := facts.Get(path)
	if !ok {
		return CounterfactualHint{Source: source, RuleID: ruleID, Path: path, Hint: "provide missing fact " + path}
	}
	target := parseHintValue(rawTarget)
	return CounterfactualHint{
		Source:  source,
		RuleID:  ruleID,
		Path:    path,
		Current: current,
		Target:  target,
		Hint:    fmt.Sprintf("change %s so `%s %s %s` can match", path, path, op, fields[2]),
	}
}

func parseHintValue(raw string) any {
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}
	if b, err := strconv.ParseBool(raw); err == nil {
		return b
	}
	return raw
}

func collectFactEvidence(facts Facts, nodes []EvidenceNode) []FactEvidence {
	seen := map[string]bool{}
	var out []FactEvidence
	for _, node := range nodes {
		for _, path := range node.FactPaths {
			if seen[path] {
				continue
			}
			seen[path] = true
			if value, ok := facts.Get(path); ok {
				out = append(out, FactEvidence{Path: path, Value: value})
			} else {
				out = append(out, FactEvidence{Path: path})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func schemaTypeMatches(value any, want string) bool {
	if value == nil {
		return false
	}
	switch strings.ToLower(want) {
	case "string":
		_, ok := value.(string)
		return ok
	case "bool", "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := asFloat(value)
		return ok
	case "int", "integer":
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		default:
			return false
		}
	case "array", "slice":
		k := reflect.ValueOf(value).Kind()
		return k == reflect.Slice || k == reflect.Array
	case "object", "map":
		return reflect.ValueOf(value).Kind() == reflect.Map
	default:
		return true
	}
}

func validateSchemaRule(path string, rule SchemaRule, facts Facts) error {
	value, ok := facts.Get(path)
	if rule.Required && !ok {
		return newError(ErrMissing, 0, "decision schema requires fact %q", path)
	}
	if !ok {
		return nil
	}
	if rule.Type != "" && !schemaTypeMatches(value, rule.Type) {
		return newError(ErrType, 0, "decision schema fact %q must be %s", path, rule.Type)
	}
	if len(rule.Enum) > 0 && !schemaEnumContains(rule.Enum, value) {
		return newError(ErrType, 0, "decision schema fact %q must be one of allowed enum values", path)
	}
	if rule.Min != nil || rule.Max != nil {
		n, ok := asFloat(value)
		if !ok {
			return newError(ErrType, 0, "decision schema fact %q must be numeric", path)
		}
		if rule.Min != nil && n < *rule.Min {
			return newError(ErrType, 0, "decision schema fact %q must be >= %v", path, *rule.Min)
		}
		if rule.Max != nil && n > *rule.Max {
			return newError(ErrType, 0, "decision schema fact %q must be <= %v", path, *rule.Max)
		}
	}
	if rule.MinLength != nil || rule.MaxLength != nil {
		s, ok := value.(string)
		if !ok {
			return newError(ErrType, 0, "decision schema fact %q must be string", path)
		}
		if rule.MinLength != nil && len(s) < *rule.MinLength {
			return newError(ErrType, 0, "decision schema fact %q length must be >= %d", path, *rule.MinLength)
		}
		if rule.MaxLength != nil && len(s) > *rule.MaxLength {
			return newError(ErrType, 0, "decision schema fact %q length must be <= %d", path, *rule.MaxLength)
		}
	}
	if rule.MinItems != nil || rule.MaxItems != nil {
		v := reflect.ValueOf(value)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return newError(ErrType, 0, "decision schema fact %q must be array", path)
		}
		if rule.MinItems != nil && v.Len() < *rule.MinItems {
			return newError(ErrType, 0, "decision schema fact %q item count must be >= %d", path, *rule.MinItems)
		}
		if rule.MaxItems != nil && v.Len() > *rule.MaxItems {
			return newError(ErrType, 0, "decision schema fact %q item count must be <= %d", path, *rule.MaxItems)
		}
	}
	for child, childRule := range rule.Properties {
		childPath := path + "." + child
		if err := validateSchemaRule(childPath, childRule, facts); err != nil {
			return err
		}
	}
	return nil
}

func schemaEnumContains(values []any, got any) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, got) {
			return true
		}
		if gf, gok := asFloat(got); gok {
			if vf, vok := asFloat(value); vok && gf == vf {
				return true
			}
		}
	}
	return false
}

func isZeroNumber(v any) bool {
	switch x := v.(type) {
	case int:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	case float32:
		return x == 0
	default:
		return false
	}
}

func packageKey(name, version, environment string) string {
	return name + "\x00" + version + "\x00" + environment
}

func packageLatestKey(name, environment string) string {
	return name + "\x00" + environment
}

func platformWorkflowRuleSetName(pkg DecisionPackage) string {
	return pkg.Name + ".__workflows"
}

func sortedRankingNames(m map[string]*compiledRanking) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sortedOptimizationNames(m map[string]*compiledOptimization) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func fingerprint(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		data = []byte(fmt.Sprintf("%#v", v))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func requestFingerprintPayload(req DecisionRequest) any {
	return map[string]any{
		"package":     req.PackageName,
		"version":     req.Version,
		"environment": req.Environment,
		"decision":    req.Decision,
		"context":     req.Context,
		"facts_type":  fmt.Sprintf("%T", req.Facts),
		"variables":   req.VariableMap,
		"candidates":  req.Candidates,
	}
}

func resultFingerprintPayload(res DecisionResponse) any {
	return map[string]any{
		"decision":         res.Decision,
		"allowed":          res.Allowed,
		"effect":           res.Effect,
		"score":            res.Score,
		"rank":             res.Rank,
		"actions":          res.Actions,
		"events":           res.Events,
		"matched_rule_ids": res.MatchedRuleIDs,
		"version":          res.Version,
		"digest":           res.Digest,
	}
}

func traceSummary(res DecisionResponse) string {
	return fmt.Sprintf("matched=%d actions=%d events=%d evidence=%d", len(res.MatchedRuleIDs), len(res.Actions), len(res.Events), len(res.Explanation.Evidence))
}

func diffDecision(a, b DecisionResponse) DecisionDiff {
	diff := DecisionDiff{
		DecisionChanged: a.Decision != b.Decision,
		EffectChanged:   a.Effect != b.Effect,
		ScoreChanged:    a.Score != b.Score,
		ActionsChanged:  fingerprint(a.Actions) != fingerprint(b.Actions),
	}
	if a.Rank != nil || b.Rank != nil {
		var ar, br string
		if a.Rank != nil {
			ar = a.Rank.ID
		}
		if b.Rank != nil {
			br = b.Rank.ID
		}
		diff.RankChanged = ar != br
	}
	diff.MatchedAdded, diff.MatchedRemoved = ruleIDSetDiff(a.MatchedRuleIDs, b.MatchedRuleIDs)
	return diff
}

func ruleIDSetDiff(a, b []RuleID) (added, removed []RuleID) {
	am := map[RuleID]bool{}
	bm := map[RuleID]bool{}
	for _, id := range a {
		am[id] = true
	}
	for _, id := range b {
		bm[id] = true
		if !am[id] {
			added = append(added, id)
		}
	}
	for _, id := range a {
		if !bm[id] {
			removed = append(removed, id)
		}
	}
	return added, removed
}

func applySimulationSummary(summary *SimulationSummary, diff DecisionDiff) {
	if diff.DecisionChanged {
		summary.DecisionChanges++
	}
	if diff.EffectChanged {
		summary.EffectChanges++
	}
	if diff.ScoreChanged {
		summary.ScoreChanges++
	}
	if diff.ActionsChanged {
		summary.ActionChanges++
	}
	if diff.RankChanged {
		summary.RankChanges++
	}
}
