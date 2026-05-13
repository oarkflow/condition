package condition

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
)

type EngineOption func(*engineConfig)

type engineConfig struct {
	exprOptions []Option
	handlers    map[string]ActionHandler
}

type ActionHandler func(context.Context, Action, Facts) (Event, error)

func WithExpressionOptions(opts ...Option) EngineOption {
	return func(c *engineConfig) { c.exprOptions = append(c.exprOptions, opts...) }
}

func WithActionHandler(actionType string, h ActionHandler) EngineOption {
	return func(c *engineConfig) {
		if c.handlers == nil {
			c.handlers = map[string]ActionHandler{}
		}
		c.handlers[actionType] = h
	}
}

type Engine struct {
	mu       sync.RWMutex
	ruleSets map[string]*compiledRuleSet
	cfg      engineConfig
}

func NewEngine(opts ...EngineOption) *Engine {
	cfg := engineConfig{handlers: map[string]ActionHandler{}}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Engine{ruleSets: make(map[string]*compiledRuleSet), cfg: cfg}
}

type RuleSet struct {
	Name          string        `json:"name"`
	Namespace     string        `json:"namespace,omitempty"`
	ExecutionMode ExecutionMode `json:"execution_mode,omitempty"`
	Rules         []Rule        `json:"rules,omitempty"`
	ScoreRules    []ScoreRule   `json:"score_rules,omitempty"`
	Workflows     []Workflow    `json:"workflows,omitempty"`
}

type Rule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
	Priority    int      `json:"priority,omitempty"`
	Salience    int      `json:"salience,omitempty"`
	Condition   string   `json:"condition,omitempty"`
	Actions     []Action `json:"actions,omitempty"`
	Events      []Event  `json:"events,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Score       float64  `json:"score,omitempty"`
	Decision    string   `json:"decision,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
	StopOnMatch bool     `json:"stop_on_match,omitempty"`
	Group       *Group   `json:"group,omitempty"`
	NextStage   string   `json:"next_stage,omitempty"`
	ValidFrom   int64    `json:"valid_from,omitempty"`
	ValidUntil  int64    `json:"valid_until,omitempty"`
	Version     uint32   `json:"version,omitempty"`
}

type Group struct {
	Mode  GroupMode `json:"mode"`
	Rules []Rule    `json:"rules"`
}

type GroupMode string

const (
	GroupAll  GroupMode = "all"
	GroupAny  GroupMode = "any"
	GroupNone GroupMode = "none"
)

type Workflow struct {
	Name       string  `json:"name"`
	StartStage string  `json:"start_stage"`
	Stages     []Stage `json:"stages"`
}

type Stage struct {
	Name      string         `json:"name"`
	Assign    *Assignment    `json:"assign,omitempty"`
	SLA       string         `json:"sla,omitempty"`
	OnTimeout string         `json:"on_timeout,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Rules     []Rule         `json:"rules"`
}

type Assignment struct {
	Role     string         `json:"role,omitempty"`
	User     string         `json:"user,omitempty"`
	Queue    string         `json:"queue,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type compiledRuleSet struct {
	def       RuleSet
	rules     []compiledRule
	workflows map[string]compiledWorkflow
	mode      ExecutionMode
}

type compiledWorkflow struct {
	start  string
	stages map[string]compiledStage
	mode   ExecutionMode
}

type compiledStage struct {
	def   Stage
	rules []compiledRule
}

type compiledRule struct {
	rule       Rule
	expr       *Expression
	group      *compiledGroup
	rankNative nativeBool
}

type compiledGroup struct {
	mode  GroupMode
	rules []compiledRule
}

func (e *Engine) AddRuleSet(rs RuleSet) error {
	if rs.Name == "" {
		return newError(ErrRuleSet, 0, "ruleset name is required")
	}
	mode := ruleSetMode(rs.ExecutionMode)
	crs := &compiledRuleSet{def: rs, workflows: map[string]compiledWorkflow{}, mode: mode}
	var err error
	crs.rules, err = e.compileRules(rs.Rules)
	if err != nil {
		return err
	}
	sortRules(crs.rules)
	for _, wf := range rs.Workflows {
		if wf.Name == "" || wf.StartStage == "" {
			return newError(ErrRuleSet, 0, "workflow name and start_stage are required")
		}
		cw := compiledWorkflow{start: wf.StartStage, stages: map[string]compiledStage{}, mode: mode}
		for _, st := range wf.Stages {
			if st.Name == "" {
				return newError(ErrRuleSet, 0, "workflow stage name is required")
			}
			rules, err := e.compileRules(st.Rules)
			if err != nil {
				return err
			}
			sortRules(rules)
			cw.stages[st.Name] = compiledStage{def: st, rules: rules}
		}
		crs.workflows[wf.Name] = cw
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ruleSets[rs.Name] = crs
	return nil
}

func (e *Engine) AddRuleSetFrom(ctx context.Context, source Source, decoder Decoder[RuleSet]) error {
	rs, err := LoadRuleSet(ctx, source, decoder)
	if err != nil {
		return err
	}
	return e.AddRuleSet(rs)
}

func (e *Engine) ReloadRuleSetFrom(ctx context.Context, source Source, decoder Decoder[RuleSet]) error {
	return e.AddRuleSetFrom(ctx, source, decoder)
}

func (e *Engine) Evaluate(ctx context.Context, facts Facts, ruleSetName string) (Result, error) {
	e.mu.RLock()
	rs, ok := e.ruleSets[ruleSetName]
	e.mu.RUnlock()
	if !ok {
		return Result{}, newError(ErrRuleSet, 0, "ruleset %q not found", ruleSetName)
	}
	res, err := e.evalRules(ctx, facts, rs.rules, rs.mode)
	if err != nil {
		return res, err
	}
	for name, wf := range rs.workflows {
		wr, err := e.evalWorkflow(ctx, facts, name, wf)
		res = mergeResults(res, wr)
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

func (e *Engine) compileRules(rules []Rule) ([]compiledRule, error) {
	out := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		if r.ID == "" {
			return nil, newError(ErrRule, 0, "rule id is required")
		}
		cr := compiledRule{rule: r}
		if r.Condition != "" {
			ex, err := Compile(r.Condition, e.cfg.exprOptions...)
			if err != nil {
				return nil, err
			}
			cr.expr = ex
		}
		if r.Group != nil {
			cg, err := e.compileGroup(*r.Group)
			if err != nil {
				return nil, err
			}
			cr.group = cg
		}
		out = append(out, cr)
	}
	return out, nil
}

func (e *Engine) compileGroup(g Group) (*compiledGroup, error) {
	if g.Mode == "" {
		g.Mode = GroupAll
	}
	if g.Mode != GroupAll && g.Mode != GroupAny && g.Mode != GroupNone {
		return nil, newError(ErrRule, 0, "invalid group mode %q", g.Mode)
	}
	rules, err := e.compileRules(g.Rules)
	if err != nil {
		return nil, err
	}
	sortRules(rules)
	return &compiledGroup{mode: g.Mode, rules: rules}, nil
}

func (e *Engine) evalWorkflow(ctx context.Context, facts Facts, name string, wf compiledWorkflow) (Result, error) {
	seen := map[string]bool{}
	stage := wf.start
	var all Result
	for stage != "" {
		if seen[stage] {
			return all, newError(ErrRuleSet, 0, "workflow %q loop at stage %q", name, stage)
		}
		seen[stage] = true
		compiled, ok := wf.stages[stage]
		if !ok {
			return all, newError(ErrRuleSet, 0, "workflow %q missing stage %q", name, stage)
		}
		if compiled.def.Assign != nil {
			all.Actions = append(all.Actions, workflowAssignAction(name, compiled.def))
		}
		if compiled.def.SLA != "" || compiled.def.OnTimeout != "" {
			all.Events = append(all.Events, workflowStageEvent(name, compiled.def))
		}
		res, next, err := e.evalRulesWithNext(ctx, facts, compiled.rules, wf.mode)
		all = mergeResults(all, res)
		if err != nil {
			return all, err
		}
		stage = next
	}
	return all, nil
}

func (e *Engine) evalRules(ctx context.Context, facts Facts, rules []compiledRule, mode ExecutionMode) (Result, error) {
	res, _, err := e.evalRulesWithNext(ctx, facts, rules, mode)
	return res, err
}

func (e *Engine) evalRulesWithNext(ctx context.Context, facts Facts, rules []compiledRule, mode ExecutionMode) (Result, string, error) {
	var res Result
	res.Trace.Enabled = true
	start := time.Now()
	now := start.Unix()
	for _, r := range rules {
		matched, rr, err := e.evalRule(ctx, facts, r, now)
		res.Trace.Steps = append(res.Trace.Steps, rr.Trace.Steps...)
		if err != nil {
			return res, "", err
		}
		if !matched {
			continue
		}
		if err := e.applyMatchedRule(ctx, facts, &res, r.rule); err != nil {
			return res, "", err
		}
		if r.rule.StopOnMatch {
			res.Trace.Duration = time.Since(start).String()
			return res, r.rule.NextStage, nil
		}
		if r.rule.NextStage != "" {
			res.Trace.Duration = time.Since(start).String()
			return res, r.rule.NextStage, nil
		}
		if shouldStopForMode(mode, r.rule) {
			res.Trace.Duration = time.Since(start).String()
			return res, "", nil
		}
	}
	res.Trace.Duration = time.Since(start).String()
	return res, "", nil
}

func (e *Engine) evalRule(ctx context.Context, facts Facts, r compiledRule, now int64) (bool, Result, error) {
	if r.rule.Enabled != nil && !*r.rule.Enabled {
		return false, Result{}, nil
	}
	if !ruleActiveAt(r.rule, now) {
		return false, Result{}, nil
	}
	matched := true
	var res Result
	if r.expr != nil {
		rr, err := r.expr.Eval(ctx, facts)
		res = mergeResults(res, rr)
		if err != nil {
			return false, res, err
		}
		matched = matched && rr.Matched
	}
	if r.group != nil {
		gm, gr, err := e.evalGroup(ctx, facts, r.group)
		res = mergeResults(res, gr)
		if err != nil {
			return false, res, err
		}
		matched = matched && gm
	}
	return matched, res, nil
}

func (e *Engine) evalGroup(ctx context.Context, facts Facts, g *compiledGroup) (bool, Result, error) {
	switch g.mode {
	case GroupAll:
		return allMatched(ctx, facts, e, g.rules)
	case GroupAny:
		res, err := e.evalRules(ctx, facts, g.rules, AllMatches)
		if err != nil {
			return false, res, err
		}
		return res.Matched, res, nil
	case GroupNone:
		res, err := e.evalRules(ctx, facts, g.rules, AllMatches)
		if err != nil {
			return false, res, err
		}
		return !res.Matched, res, nil
	default:
		return false, Result{}, newError(ErrRule, 0, "invalid group mode %q", g.mode)
	}
}

func allMatched(ctx context.Context, facts Facts, e *Engine, rules []compiledRule) (bool, Result, error) {
	var res Result
	now := time.Now().Unix()
	for _, r := range rules {
		m, rr, err := e.evalRule(ctx, facts, r, now)
		res = mergeResults(res, rr)
		if err != nil {
			return false, res, err
		}
		if !m {
			return false, res, nil
		}
	}
	return true, res, nil
}

func sortRules(rules []compiledRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		a := rules[i].rule
		b := rules[j].rule
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		return a.Salience > b.Salience
	})
}

func mergeResults(a, b Result) Result {
	a.Matched = a.Matched || b.Matched
	a.MatchedRuleIDs = append(a.MatchedRuleIDs, b.MatchedRuleIDs...)
	a.Score += b.Score
	if b.Decision != "" {
		a.Decision = b.Decision
	}
	if resultAllows(b) {
		a.Allowed = true
	}
	if resultDenies(b) {
		a.Allowed = false
	}
	a.Actions = append(a.Actions, b.Actions...)
	a.CompiledActions = append(a.CompiledActions, b.CompiledActions...)
	a.Events = append(a.Events, b.Events...)
	a.Trace.Enabled = a.Trace.Enabled || b.Trace.Enabled
	a.Trace.Steps = append(a.Trace.Steps, b.Trace.Steps...)
	if b.Trace.Duration != "" {
		a.Trace.Duration = b.Trace.Duration
	}
	return a
}

func (rs RuleSet) JSON() ([]byte, error) { return json.Marshal(rs) }

func (e *Engine) applyMatchedRule(ctx context.Context, facts Facts, res *Result, rule Rule) error {
	res.Matched = true
	res.MatchedRuleIDs = append(res.MatchedRuleIDs, parseRuleID(rule.ID))
	res.Score += rule.Score
	if rule.Decision != "" {
		res.Decision = rule.Decision
		applyDecision(res, rule.Decision)
	}
	res.Actions = append(res.Actions, rule.Actions...)
	for _, a := range rule.Actions {
		applyAction(res, a)
		if h := e.cfg.handlers[a.Type]; h != nil {
			ev, err := h(ctx, a, facts)
			if err != nil {
				return err
			}
			if ev.Type != "" {
				res.Events = append(res.Events, ev)
			}
		}
	}
	res.Events = append(res.Events, rule.Events...)
	return nil
}

func workflowAssignAction(workflow string, stage Stage) Action {
	payload := map[string]any{"workflow": workflow, "stage": stage.Name}
	if stage.Assign != nil {
		if stage.Assign.Role != "" {
			payload["role"] = stage.Assign.Role
		}
		if stage.Assign.User != "" {
			payload["user"] = stage.Assign.User
		}
		if stage.Assign.Queue != "" {
			payload["queue"] = stage.Assign.Queue
		}
		for k, v := range stage.Assign.Metadata {
			payload[k] = v
		}
	}
	if stage.SLA != "" {
		payload["sla"] = stage.SLA
	}
	if stage.OnTimeout != "" {
		payload["on_timeout"] = stage.OnTimeout
	}
	return Action{Type: "assign", Payload: payload}
}

func workflowStageEvent(workflow string, stage Stage) Event {
	payload := map[string]any{"workflow": workflow, "stage": stage.Name}
	if stage.SLA != "" {
		payload["sla"] = stage.SLA
	}
	if stage.OnTimeout != "" {
		payload["on_timeout"] = stage.OnTimeout
	}
	return Event{Type: "workflow_stage", Payload: payload}
}

func ruleSetMode(mode ExecutionMode) ExecutionMode {
	if mode == FirstMatch {
		return AllMatches
	}
	return mode
}

func ruleActiveAt(rule Rule, now int64) bool {
	if rule.ValidFrom > 0 && now < rule.ValidFrom {
		return false
	}
	if rule.ValidUntil > 0 && now > rule.ValidUntil {
		return false
	}
	return true
}

func shouldStopForMode(mode ExecutionMode, rule Rule) bool {
	switch mode {
	case FirstMatch, HighestPriority:
		return true
	case DenyOverrides:
		return ruleDenies(rule)
	case AllowOverrides:
		return ruleAllows(rule)
	default:
		return false
	}
}

func ruleAllows(rule Rule) bool {
	if strings.EqualFold(rule.Decision, "allow") {
		return true
	}
	for _, action := range rule.Actions {
		if strings.EqualFold(action.Type, "allow") {
			return true
		}
	}
	return false
}

func ruleDenies(rule Rule) bool {
	if strings.EqualFold(rule.Decision, "deny") {
		return true
	}
	for _, action := range rule.Actions {
		if strings.EqualFold(action.Type, "deny") {
			return true
		}
	}
	return false
}

func applyDecision(res *Result, decision string) {
	switch {
	case strings.EqualFold(decision, "allow"):
		res.Allowed = true
	case strings.EqualFold(decision, "deny"):
		res.Allowed = false
	}
}

func applyAction(res *Result, action Action) {
	switch {
	case strings.EqualFold(action.Type, "allow"):
		res.Allowed = true
	case strings.EqualFold(action.Type, "deny"):
		res.Allowed = false
	}
}

func resultAllows(res Result) bool {
	if strings.EqualFold(res.Decision, "allow") || res.Allowed {
		return true
	}
	for _, action := range res.Actions {
		if strings.EqualFold(action.Type, "allow") {
			return true
		}
	}
	return false
}

func resultDenies(res Result) bool {
	if strings.EqualFold(res.Decision, "deny") {
		return true
	}
	for _, action := range res.Actions {
		if strings.EqualFold(action.Type, "deny") {
			return true
		}
	}
	return false
}
