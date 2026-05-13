package condition

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"time"
)

type Candidate struct {
	ID       string         `json:"id"`
	Name     string         `json:"name,omitempty"`
	Facts    Facts          `json:"-"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type RankingRequest struct {
	Facts      Facts       `json:"-"`
	Variables  Facts       `json:"-"`
	Candidates []Candidate `json:"candidates"`
}

type RankingResult struct {
	Winner     *CandidateResult  `json:"winner,omitempty"`
	Candidates []CandidateResult `json:"candidates"`
	Trace      Trace             `json:"trace,omitempty"`
}

type CandidateResult struct {
	ID       string   `json:"id"`
	Name     string   `json:"name,omitempty"`
	Eligible bool     `json:"eligible"`
	Score    float64  `json:"score"`
	Decision string   `json:"decision,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
	Actions  []Action `json:"actions,omitempty"`
	Events   []Event  `json:"events,omitempty"`

	priority    float64
	specificity float64
	fallback    float64
	cost        float64
}

type Selection struct {
	ID    string  `json:"id"`
	Name  string  `json:"name,omitempty"`
	Score float64 `json:"score"`
}

type FallbackResult struct {
	Primary   *CandidateResult  `json:"primary,omitempty"`
	Fallbacks []CandidateResult `json:"fallbacks,omitempty"`
}

type ScoreRule struct {
	ID        string         `json:"id"`
	Condition string         `json:"condition,omitempty"`
	Metric    string         `json:"metric"`
	Weight    float64        `json:"weight,omitempty"`
	Direction ScoreDirection `json:"direction,omitempty"`
	Normalize Normalize      `json:"normalize,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

type ScoreDirection string

const (
	HigherBetter ScoreDirection = "higher_better"
	LowerBetter  ScoreDirection = "lower_better"
)

type Normalize struct {
	Min float64 `json:"min,omitempty"`
	Max float64 `json:"max,omitempty"`
	Raw bool    `json:"raw,omitempty"`
}

type RankerOption func(*rankerConfig)

type rankerConfig struct {
	exprOptions     []Option
	strict          bool
	trace           bool
	explain         bool
	priorityPath    string
	specificityPath string
	fallbackPath    string
	costPath        string
	weightPath      string
	hashKeyPath     string
}

func WithRankerExpressionOptions(opts ...Option) RankerOption {
	return func(c *rankerConfig) { c.exprOptions = append(c.exprOptions, opts...) }
}

func WithRankerStrict(v bool) RankerOption {
	return func(c *rankerConfig) { c.strict = v }
}

func WithRankerTrace(v bool) RankerOption {
	return func(c *rankerConfig) { c.trace = v }
}

func WithRankerExplain(v bool) RankerOption {
	return func(c *rankerConfig) { c.explain = v }
}

func WithTieBreakerPaths(priorityPath, costPath string) RankerOption {
	return func(c *rankerConfig) {
		if priorityPath != "" {
			c.priorityPath = priorityPath
		}
		if costPath != "" {
			c.costPath = costPath
		}
	}
}

func WithRoutingTieBreakerPaths(priorityPath, specificityPath, fallbackPath, costPath string) RankerOption {
	return func(c *rankerConfig) {
		if priorityPath != "" {
			c.priorityPath = priorityPath
		}
		if specificityPath != "" {
			c.specificityPath = specificityPath
		}
		if fallbackPath != "" {
			c.fallbackPath = fallbackPath
		}
		if costPath != "" {
			c.costPath = costPath
		}
	}
}

func WithWeightedSelection(weightPath, hashKeyPath string) RankerOption {
	return func(c *rankerConfig) {
		if weightPath != "" {
			c.weightPath = weightPath
		}
		if hashKeyPath != "" {
			c.hashKeyPath = hashKeyPath
		}
	}
}

type Ranker struct {
	ruleSet          RuleSet
	rules            []compiledRule
	scores           []compiledScoreRule
	cfg              rankerConfig
	priorityParts    []string
	specificityParts []string
	fallbackParts    []string
	costParts        []string
	weightParts      []string
	hashKeyParts     []string
	priorityGet      nativePathGetter
	specificityGet   nativePathGetter
	fallbackGet      nativePathGetter
	costGet          nativePathGetter
	weightGet        nativePathGetter
}

type compiledScoreRule struct {
	rule            ScoreRule
	condition       *Expression
	conditionNative nativeBool
	metric          []string
	metricGet       nativePathGetter
}

func NewRanker(ruleSet RuleSet, opts ...RankerOption) (*Ranker, error) {
	cfg := rankerConfig{explain: true, priorityPath: "provider.priority", costPath: "provider.cost", weightPath: "route.weight"}
	for _, opt := range opts {
		opt(&cfg)
	}
	exprOpts := append([]Option(nil), cfg.exprOptions...)
	if cfg.strict {
		exprOpts = append(exprOpts, Strict(true))
	}
	engine := NewEngine(WithExpressionOptions(exprOpts...))
	rules, err := engine.compileRules(ruleSet.Rules)
	if err != nil {
		return nil, err
	}
	sortRules(rules)
	prepareRankNativeRules(rules)
	scores := make([]compiledScoreRule, 0, len(ruleSet.ScoreRules))
	for _, sr := range ruleSet.ScoreRules {
		if sr.ID == "" {
			return nil, newError(ErrRule, 0, "score rule id is required")
		}
		if sr.Metric == "" {
			return nil, newError(ErrRule, 0, "score rule %q metric is required", sr.ID)
		}
		if sr.Weight == 0 {
			sr.Weight = 1
		}
		if sr.Direction == "" {
			sr.Direction = HigherBetter
		}
		if sr.Direction != HigherBetter && sr.Direction != LowerBetter {
			return nil, newError(ErrRule, 0, "score rule %q has invalid direction %q", sr.ID, sr.Direction)
		}
		var cond *Expression
		var condNative nativeBool
		if sr.Condition != "" {
			cond, err = Compile(sr.Condition, exprOpts...)
			if err != nil {
				return nil, err
			}
			condNative = compileNativeBool(cond.expr)
		}
		metricParts := splitPath(sr.Metric)
		scores = append(scores, compiledScoreRule{rule: sr, condition: cond, conditionNative: condNative, metric: metricParts, metricGet: compileNativePathGetter(metricParts)})
	}
	priorityParts := splitPath(cfg.priorityPath)
	specificityParts := splitPath(cfg.specificityPath)
	fallbackParts := splitPath(cfg.fallbackPath)
	costParts := splitPath(cfg.costPath)
	weightParts := splitPath(cfg.weightPath)
	return &Ranker{
		ruleSet:          ruleSet,
		rules:            rules,
		scores:           scores,
		cfg:              cfg,
		priorityParts:    priorityParts,
		specificityParts: specificityParts,
		fallbackParts:    fallbackParts,
		costParts:        costParts,
		weightParts:      weightParts,
		hashKeyParts:     splitPath(cfg.hashKeyPath),
		priorityGet:      compileNativePathGetter(priorityParts),
		specificityGet:   compileNativePathGetter(specificityParts),
		fallbackGet:      compileNativePathGetter(fallbackParts),
		costGet:          compileNativePathGetter(costParts),
		weightGet:        compileNativePathGetter(weightParts),
	}, nil
}

func (r *Ranker) Rank(ctx context.Context, req RankingRequest) (RankingResult, error) {
	var out RankingResult
	err := r.RankInto(ctx, req, &out)
	return out, err
}

func (r *Ranker) RankInto(ctx context.Context, req RankingRequest, out *RankingResult) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Facts == nil {
		req.Facts = MapFacts(nil)
	}
	if out == nil {
		return newError(ErrEval, 0, "ranking result output is required")
	}
	start := time.Now()
	oldCandidates := out.Candidates
	oldTraceSteps := out.Trace.Steps
	*out = RankingResult{}
	if cap(oldCandidates) < len(req.Candidates) {
		out.Candidates = make([]CandidateResult, len(req.Candidates))
	} else {
		out.Candidates = oldCandidates[:len(req.Candidates)]
	}
	if r.cfg.trace {
		out.Trace.Enabled = true
		out.Trace.Steps = oldTraceSteps[:0]
	}
	cf := rankingFactsPool.Get().(*rankingFacts)
	native := r.canRankNative()
	for i, candidate := range req.Candidates {
		cf.reset(req.Facts, candidate)
		var err error
		if native {
			err = r.rankCandidateIntoNative(ctx, cf, req.Variables, candidate, &out.Candidates[i])
		} else {
			err = r.rankCandidateInto(ctx, cf, req.Variables, candidate, &out.Candidates[i])
		}
		if err != nil {
			cf.clear()
			rankingFactsPool.Put(cf)
			return err
		}
	}
	cf.clear()
	rankingFactsPool.Put(cf)
	sortCandidateResults(out.Candidates)
	if len(out.Candidates) > 0 && out.Candidates[0].Eligible {
		out.Winner = &out.Candidates[0]
	}
	if out.Trace.Enabled {
		out.Trace.Duration = time.Since(start).String()
		for _, c := range out.Candidates {
			out.Trace.Steps = append(out.Trace.Steps, TraceStep{
				Path:   c.ID,
				Result: map[string]any{"eligible": c.Eligible, "score": c.Score},
			})
		}
	}
	return nil
}

func (r *Ranker) SelectBest(ctx context.Context, req RankingRequest) (CandidateResult, bool, error) {
	var out CandidateResult
	ok, err := r.SelectBestInto(ctx, req, &out)
	return out, ok, err
}

func (r *Ranker) SelectBestID(ctx context.Context, req RankingRequest) (Selection, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Facts == nil {
		req.Facts = MapFacts(nil)
	}
	cf := rankingFactsPool.Get().(*rankingFacts)
	var best Selection
	var bestCmp candidateCompare
	found := false
	for _, candidate := range req.Candidates {
		cf.reset(req.Facts, candidate)
		score, cmp, eligible, err := r.rankCandidateNative(ctx, cf, req.Variables)
		if err != nil {
			cf.clear()
			rankingFactsPool.Put(cf)
			return Selection{}, false, err
		}
		if !eligible {
			continue
		}
		if !found || betterSelection(candidate.ID, score, cmp, best.ID, best.Score, bestCmp) {
			best = Selection{ID: candidate.ID, Name: candidate.Name, Score: score}
			bestCmp = cmp
			found = true
		}
	}
	cf.clear()
	rankingFactsPool.Put(cf)
	return best, found, nil
}

func (r *Ranker) SelectBestInto(ctx context.Context, req RankingRequest, out *CandidateResult) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Facts == nil {
		req.Facts = MapFacts(nil)
	}
	if out == nil {
		return false, newError(ErrEval, 0, "candidate result output is required")
	}
	reasons := out.Reasons[:0]
	actions := out.Actions[:0]
	events := out.Events[:0]
	*out = CandidateResult{Reasons: reasons, Actions: actions, Events: events}
	cf := rankingFactsPool.Get().(*rankingFacts)
	var current CandidateResult
	native := r.canRankNative()
	found := false
	for _, candidate := range req.Candidates {
		cf.reset(req.Facts, candidate)
		var err error
		if native {
			err = r.rankCandidateIntoNative(ctx, cf, req.Variables, candidate, &current)
		} else {
			err = r.rankCandidateInto(ctx, cf, req.Variables, candidate, &current)
		}
		if err != nil {
			cf.clear()
			rankingFactsPool.Put(cf)
			return false, err
		}
		if !current.Eligible {
			continue
		}
		if !found || betterCandidate(current, *out) {
			*out = current
			found = true
		}
	}
	cf.clear()
	rankingFactsPool.Put(cf)
	return found, nil
}

func (r *Ranker) SelectFallbacks(ctx context.Context, req RankingRequest, limit int) (FallbackResult, error) {
	var ranked RankingResult
	if err := r.RankInto(ctx, req, &ranked); err != nil {
		return FallbackResult{}, err
	}
	var out FallbackResult
	if ranked.Winner != nil {
		out.Primary = ranked.Winner
	}
	if limit <= 0 {
		limit = len(ranked.Candidates)
	}
	for i := 0; i < len(ranked.Candidates) && len(out.Fallbacks) < limit; i++ {
		if !ranked.Candidates[i].Eligible {
			continue
		}
		if out.Primary != nil && ranked.Candidates[i].ID == out.Primary.ID {
			continue
		}
		out.Fallbacks = append(out.Fallbacks, ranked.Candidates[i])
	}
	return out, nil
}

func (r *Ranker) SelectWeightedID(ctx context.Context, req RankingRequest) (Selection, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Facts == nil {
		req.Facts = MapFacts(nil)
	}
	var hashKey any
	if len(r.hashKeyParts) > 0 {
		hashKey, _ = getPathParts(req.Facts, r.hashKeyParts)
	}
	if hashKey == nil {
		hashKey = "default"
	}
	cf := rankingFactsPool.Get().(*rankingFacts)
	type weightedCandidate struct {
		selection Selection
		cmp       candidateCompare
		weight    float64
	}
	var bucket []weightedCandidate
	bestPriority := 0.0
	bestSpecificity := 0.0
	found := false
	totalWeight := 0.0
	for _, candidate := range req.Candidates {
		cf.reset(req.Facts, candidate)
		score, cmp, eligible, err := r.rankCandidateNative(ctx, cf, req.Variables)
		if err != nil {
			cf.clear()
			rankingFactsPool.Put(cf)
			return Selection{}, false, err
		}
		if !eligible {
			continue
		}
		if !found || cmp.priority > bestPriority || (cmp.priority == bestPriority && cmp.specificity > bestSpecificity) {
			bucket = bucket[:0]
			bestPriority = cmp.priority
			bestSpecificity = cmp.specificity
			totalWeight = 0
			found = true
		}
		if cmp.priority == bestPriority && cmp.specificity == bestSpecificity {
			weight, ok := nativeFloat(r.weightGet, cf)
			if !ok || weight <= 0 {
				weight = 1
			}
			bucket = append(bucket, weightedCandidate{selection: Selection{ID: candidate.ID, Name: candidate.Name, Score: score}, cmp: cmp, weight: weight})
			totalWeight += weight
		}
	}
	cf.clear()
	rankingFactsPool.Put(cf)
	if !found || len(bucket) == 0 {
		return Selection{}, false, nil
	}
	point := stableHashFloat(hashKey, totalWeight)
	running := 0.0
	for _, candidate := range bucket {
		running += candidate.weight
		if point < running {
			return candidate.selection, true, nil
		}
	}
	return bucket[len(bucket)-1].selection, true, nil
}

type candidateCompare struct {
	priority    float64
	specificity float64
	fallback    float64
	cost        float64
	priorityOK  bool
	costOK      bool
}

func (r *Ranker) compareValues(facts *rankingFacts) candidateCompare {
	priority, priorityOK := nativeFloat(r.priorityGet, facts)
	specificity, _ := nativeFloat(r.specificityGet, facts)
	fallback, ok := nativeFloat(r.fallbackGet, facts)
	if !ok {
		fallback = math.MaxFloat64
	}
	cost, costOK := nativeFloat(r.costGet, facts)
	return candidateCompare{priority: priority, specificity: specificity, fallback: fallback, cost: cost, priorityOK: priorityOK, costOK: costOK}
}

func (r *Ranker) rankCandidateNative(ctx context.Context, facts *rankingFacts, vars Facts) (score float64, cmp candidateCompare, eligible bool, err error) {
	eligible = true
	cmp = r.compareValues(facts)
	for _, rule := range r.rules {
		var matched bool
		matched, err = r.evalEligibilityNative(ctx, facts, vars, rule)
		if err != nil {
			return 0, cmp, false, err
		}
		if !matched {
			return 0, cmp, false, nil
		}
		score += rule.rule.Score
	}
	for _, scoreRule := range r.scores {
		applies := true
		if scoreRule.condition != nil {
			if scoreRule.conditionNative != nil {
				applies, err = scoreRule.conditionNative(facts, vars, r.cfg.strict)
			} else if vars != nil {
				rr, evalErr := scoreRule.condition.EvalWithVariables(ctx, facts, vars)
				applies, err = rr.Matched, evalErr
			} else {
				applies, err = scoreRule.condition.EvalBool(ctx, facts)
			}
			if err != nil {
				return 0, cmp, false, err
			}
		}
		if !applies {
			continue
		}
		value, ok := r.scoreMetricValue(scoreRule, facts, cmp)
		if !ok {
			if r.cfg.strict {
				return 0, cmp, false, nil
			}
			continue
		}
		score += normalizeScore(value, scoreRule.rule) * scoreRule.rule.Weight
	}
	return score, cmp, true, nil
}

func (r *Ranker) canRankNative() bool {
	for _, rule := range r.rules {
		if !canRuleRankNative(rule) {
			return false
		}
	}
	for _, score := range r.scores {
		if score.condition != nil && score.conditionNative == nil {
			return false
		}
	}
	return true
}

func canRuleRankNative(rule compiledRule) bool {
	if rule.expr != nil && rule.rankNative == nil {
		return false
	}
	if rule.group == nil {
		return true
	}
	for _, child := range rule.group.rules {
		if !canRuleRankNative(child) {
			return false
		}
	}
	return true
}

func (r *Ranker) scoreMetricValue(score compiledScoreRule, facts *rankingFacts, cmp candidateCompare) (float64, bool) {
	if score.rule.Metric == r.cfg.priorityPath && cmp.priorityOK {
		return cmp.priority, true
	}
	if score.rule.Metric == r.cfg.costPath && cmp.costOK {
		return cmp.cost, true
	}
	return nativeFloat(score.metricGet, facts)
}

func (r *Ranker) evalEligibilityNative(ctx context.Context, facts *rankingFacts, vars Facts, rule compiledRule) (bool, error) {
	if rule.rule.Enabled != nil && !*rule.rule.Enabled {
		return true, nil
	}
	matched := true
	if rule.expr != nil {
		var ok bool
		var err error
		if rule.rankNative != nil {
			ok, err = rule.rankNative(facts, vars, r.cfg.strict)
		} else if vars != nil {
			rr, evalErr := rule.expr.EvalWithVariables(ctx, facts, vars)
			ok, err = rr.Matched, evalErr
		} else {
			ok, err = rule.expr.EvalBool(ctx, facts)
		}
		if err != nil {
			return false, err
		}
		matched = matched && ok
	}
	if rule.group != nil {
		ok, err := r.evalEligibilityGroupNative(ctx, facts, vars, rule.group)
		if err != nil {
			return false, err
		}
		matched = matched && ok
	}
	return matched, nil
}

func (r *Ranker) evalEligibilityGroupNative(ctx context.Context, facts *rankingFacts, vars Facts, group *compiledGroup) (bool, error) {
	switch group.mode {
	case GroupAll:
		for _, rule := range group.rules {
			ok, err := r.evalEligibilityNative(ctx, facts, vars, rule)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	case GroupAny:
		for _, rule := range group.rules {
			ok, err := r.evalEligibilityNative(ctx, facts, vars, rule)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case GroupNone:
		for _, rule := range group.rules {
			ok, err := r.evalEligibilityNative(ctx, facts, vars, rule)
			if err != nil {
				return false, err
			}
			if ok {
				return false, nil
			}
		}
		return true, nil
	default:
		return false, newError(ErrRule, 0, "invalid group mode %q", group.mode)
	}
}

func betterCandidate(a, b CandidateResult) bool {
	if a.Eligible != b.Eligible {
		return a.Eligible
	}
	if a.priority != b.priority {
		return a.priority > b.priority
	}
	if a.specificity != b.specificity {
		return a.specificity > b.specificity
	}
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.cost != b.cost {
		return a.cost < b.cost
	}
	if a.fallback != b.fallback {
		return a.fallback < b.fallback
	}
	return a.ID < b.ID
}

func sortCandidateResults(candidates []CandidateResult) {
	if len(candidates) < 12 {
		insertionSortCandidateResults(candidates)
		return
	}
	quickSortCandidateResults(candidates, 0, len(candidates)-1)
}

func quickSortCandidateResults(candidates []CandidateResult, lo, hi int) {
	for lo < hi {
		if hi-lo < 12 {
			insertionSortCandidateResults(candidates[lo : hi+1])
			return
		}
		mid := lo + (hi-lo)/2
		if betterCandidate(candidates[mid], candidates[lo]) {
			candidates[lo], candidates[mid] = candidates[mid], candidates[lo]
		}
		if betterCandidate(candidates[hi], candidates[lo]) {
			candidates[lo], candidates[hi] = candidates[hi], candidates[lo]
		}
		if betterCandidate(candidates[hi], candidates[mid]) {
			candidates[mid], candidates[hi] = candidates[hi], candidates[mid]
		}
		pivot := candidates[mid]
		i, j := lo, hi
		for i <= j {
			for betterCandidate(candidates[i], pivot) {
				i++
			}
			for betterCandidate(pivot, candidates[j]) {
				j--
			}
			if i <= j {
				candidates[i], candidates[j] = candidates[j], candidates[i]
				i++
				j--
			}
		}
		if j-lo < hi-i {
			if lo < j {
				quickSortCandidateResults(candidates, lo, j)
			}
			lo = i
		} else {
			if i < hi {
				quickSortCandidateResults(candidates, i, hi)
			}
			hi = j
		}
	}
}

func insertionSortCandidateResults(candidates []CandidateResult) {
	for i := 1; i < len(candidates); i++ {
		current := candidates[i]
		j := i - 1
		for ; j >= 0 && betterCandidate(current, candidates[j]); j-- {
			candidates[j+1] = candidates[j]
		}
		candidates[j+1] = current
	}
}

func betterSelection(aID string, aScore float64, a candidateCompare, bID string, bScore float64, b candidateCompare) bool {
	if a.priority != b.priority {
		return a.priority > b.priority
	}
	if a.specificity != b.specificity {
		return a.specificity > b.specificity
	}
	if aScore != bScore {
		return aScore > bScore
	}
	if a.cost != b.cost {
		return a.cost < b.cost
	}
	if a.fallback != b.fallback {
		return a.fallback < b.fallback
	}
	return aID < bID
}

func stableHashFloat(key any, modulo float64) float64 {
	if modulo <= 0 {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprint(key)))
	return math.Mod(float64(h.Sum64()), modulo)
}

func (r *Ranker) rankCandidateInto(ctx context.Context, facts *rankingFacts, vars Facts, candidate Candidate, cr *CandidateResult) error {
	reasons := cr.Reasons[:0]
	actions := cr.Actions[:0]
	events := cr.Events[:0]
	*cr = CandidateResult{ID: candidate.ID, Name: candidate.Name, Eligible: true}
	if r.cfg.explain {
		cr.Reasons = reasons
	}
	cr.Actions = actions
	cr.Events = events
	cmp := r.compareValues(facts)
	cr.priority = cmp.priority
	cr.specificity = cmp.specificity
	cr.fallback = cmp.fallback
	cr.cost = cmp.cost
	for _, rule := range r.rules {
		matched, rr, err := r.evalEligibility(ctx, facts, vars, rule)
		if err != nil {
			return err
		}
		cr.Actions = append(cr.Actions, rr.Actions...)
		cr.Events = append(cr.Events, rr.Events...)
		if rr.Decision != "" {
			cr.Decision = rr.Decision
		}
		if matched {
			cr.Score += rule.rule.Score
			continue
		}
		cr.Eligible = false
		if r.cfg.explain {
			cr.Reasons = append(cr.Reasons, reasonForRule(rule.rule))
		}
		return nil
	}
	for _, score := range r.scores {
		applies := true
		if score.condition != nil {
			var err error
			if vars != nil {
				rr, evalErr := score.condition.EvalWithVariables(ctx, facts, vars)
				applies, err = rr.Matched, evalErr
			} else {
				applies, err = score.condition.EvalBool(ctx, facts)
			}
			if err != nil {
				return err
			}
		}
		if !applies {
			continue
		}
		value, ok := r.scoreMetricValue(score, facts, cmp)
		if !ok {
			reason := "missing metric: " + score.rule.Metric
			if r.cfg.strict {
				cr.Eligible = false
				if r.cfg.explain {
					cr.Reasons = append(cr.Reasons, reason)
				}
				return nil
			}
			if r.cfg.explain {
				cr.Reasons = append(cr.Reasons, reason)
			}
			continue
		}
		points := normalizeScore(value, score.rule) * score.rule.Weight
		cr.Score += points
		if r.cfg.explain && score.rule.Reason != "" {
			cr.Reasons = append(cr.Reasons, score.rule.Reason)
		}
	}
	return nil
}

func (r *Ranker) rankCandidateIntoNative(ctx context.Context, facts *rankingFacts, vars Facts, candidate Candidate, cr *CandidateResult) error {
	reasons := cr.Reasons[:0]
	actions := cr.Actions[:0]
	events := cr.Events[:0]
	*cr = CandidateResult{ID: candidate.ID, Name: candidate.Name, Eligible: true}
	if r.cfg.explain {
		cr.Reasons = reasons
	}
	cr.Actions = actions
	cr.Events = events
	cmp := r.compareValues(facts)
	cr.priority = cmp.priority
	cr.specificity = cmp.specificity
	cr.fallback = cmp.fallback
	cr.cost = cmp.cost
	for _, rule := range r.rules {
		matched, err := r.evalEligibilityNative(ctx, facts, vars, rule)
		if err != nil {
			return err
		}
		if matched {
			cr.Score += rule.rule.Score
			if rule.rule.Decision != "" {
				cr.Decision = rule.rule.Decision
			}
			cr.Actions = append(cr.Actions, rule.rule.Actions...)
			cr.Events = append(cr.Events, rule.rule.Events...)
			continue
		}
		cr.Eligible = false
		if r.cfg.explain {
			cr.Reasons = append(cr.Reasons, reasonForRule(rule.rule))
		}
		return nil
	}
	for _, score := range r.scores {
		applies := true
		if score.conditionNative != nil {
			var err error
			applies, err = score.conditionNative(facts, vars, r.cfg.strict)
			if err != nil {
				return err
			}
		}
		if !applies {
			continue
		}
		value, ok := r.scoreMetricValue(score, facts, cmp)
		if !ok {
			reason := "missing metric: " + score.rule.Metric
			if r.cfg.strict {
				cr.Eligible = false
				if r.cfg.explain {
					cr.Reasons = append(cr.Reasons, reason)
				}
				return nil
			}
			if r.cfg.explain {
				cr.Reasons = append(cr.Reasons, reason)
			}
			continue
		}
		points := normalizeScore(value, score.rule) * score.rule.Weight
		cr.Score += points
		if r.cfg.explain && score.rule.Reason != "" {
			cr.Reasons = append(cr.Reasons, score.rule.Reason)
		}
	}
	return nil
}

func (r *Ranker) evalEligibility(ctx context.Context, facts Facts, vars Facts, rule compiledRule) (bool, Result, error) {
	if rule.rule.Enabled != nil && !*rule.rule.Enabled {
		return true, Result{}, nil
	}
	matched := true
	var res Result
	if rule.expr != nil {
		var ok bool
		var err error
		if vars != nil {
			rr, err := rule.expr.EvalWithVariables(ctx, facts, vars)
			res = mergeResults(res, rr)
			ok = rr.Matched
			if err != nil {
				return false, res, err
			}
		} else {
			ok, err = rule.expr.EvalBool(ctx, facts)
			if err != nil {
				return false, res, err
			}
		}
		matched = matched && ok
	}
	if rule.group != nil {
		ok, gr, err := r.evalEligibilityGroup(ctx, facts, vars, rule.group)
		res = mergeResults(res, gr)
		if err != nil {
			return false, res, err
		}
		matched = matched && ok
	}
	if matched {
		res.Score += rule.rule.Score
		res.Decision = rule.rule.Decision
		res.Actions = append(res.Actions, rule.rule.Actions...)
		res.Events = append(res.Events, rule.rule.Events...)
	}
	return matched, res, nil
}

func (r *Ranker) evalEligibilityGroup(ctx context.Context, facts Facts, vars Facts, group *compiledGroup) (bool, Result, error) {
	var res Result
	switch group.mode {
	case GroupAll:
		for _, rule := range group.rules {
			ok, rr, err := r.evalEligibility(ctx, facts, vars, rule)
			res = mergeResults(res, rr)
			if err != nil || !ok {
				return ok, res, err
			}
		}
		return true, res, nil
	case GroupAny:
		for _, rule := range group.rules {
			ok, rr, err := r.evalEligibility(ctx, facts, vars, rule)
			res = mergeResults(res, rr)
			if err != nil {
				return false, res, err
			}
			if ok {
				return true, res, nil
			}
		}
		return false, res, nil
	case GroupNone:
		for _, rule := range group.rules {
			ok, rr, err := r.evalEligibility(ctx, facts, vars, rule)
			res = mergeResults(res, rr)
			if err != nil {
				return false, res, err
			}
			if ok {
				return false, res, nil
			}
		}
		return true, res, nil
	default:
		return false, res, newError(ErrRule, 0, "invalid group mode %q", group.mode)
	}
}

func normalizeScore(value float64, rule ScoreRule) float64 {
	n := rule.Normalize
	if !n.Raw && n.Max > n.Min {
		value = (value - n.Min) / (n.Max - n.Min)
		if value < 0 {
			value = 0
		} else if value > 1 {
			value = 1
		}
		if rule.Direction == LowerBetter {
			value = 1 - value
		}
		return value
	}
	if rule.Direction == LowerBetter {
		return -value
	}
	return value
}

func reasonForRule(rule Rule) string {
	if rule.Reason != "" {
		return rule.Reason
	}
	if rule.Name != "" {
		return "not matched: " + rule.Name
	}
	return "not matched: " + rule.ID
}

func getFloatPath(f Facts, parts []string) (float64, bool) {
	v, ok := getPathParts(f, parts)
	if !ok {
		return 0, false
	}
	return asFloat(v)
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

type rankingFacts struct {
	global   Facts
	provider Facts
	metadata MapFacts
}

var rankingFactsPool = sync.Pool{
	New: func() any { return new(rankingFacts) },
}

func newRankingFacts(global Facts, candidate Candidate) *rankingFacts {
	f := &rankingFacts{}
	f.reset(global, candidate)
	return f
}

func (f *rankingFacts) reset(global Facts, candidate Candidate) {
	provider := candidate.Facts
	if provider == nil {
		provider = MapFacts(nil)
	}
	f.global = global
	f.provider = provider
	f.metadata = MapFacts(candidate.Metadata)
}

func (f *rankingFacts) clear() {
	f.global = nil
	f.provider = nil
	f.metadata = nil
}

func (f *rankingFacts) Get(path string) (any, bool) {
	return f.GetPath(strings.Split(path, "."))
}

func (f *rankingFacts) GetPath(parts []string) (any, bool) {
	if len(parts) == 0 {
		return nil, false
	}
	switch parts[0] {
	case "route", "candidate":
		return f.domainValue(parts[0], parts[1:], true)
	case "provider", "quality":
		return f.domainValue(parts[0], parts[1:], false)
	default:
		return getPathParts(f.global, parts)
	}
}

func (f *rankingFacts) domainValue(domain string, parts []string, fallbackRoot bool) (any, bool) {
	root, hasRoot := getTopLevel(f.provider, domain)
	if len(parts) == 0 {
		if hasRoot {
			return root, true
		}
		return f.provider, true
	}
	if hasRoot {
		if v, ok := lookupParts(root, parts); ok {
			return v, true
		}
	}
	if fallbackRoot || domain == "provider" || domain == "quality" {
		if v, ok := getPathParts(f.provider, parts); ok {
			return v, true
		}
	}
	if f.metadata != nil {
		if root, ok := f.metadata[domain]; ok {
			if v, ok := lookupParts(root, parts); ok {
				return v, true
			}
		}
	}
	if fallbackRoot && f.metadata != nil {
		if v, ok := f.metadata.GetPath(parts); ok {
			return v, true
		}
	}
	return nil, false
}

func getTopLevel(f Facts, key string) (any, bool) {
	switch m := f.(type) {
	case MapFacts:
		v, ok := m[key]
		return v, ok
	default:
		var one [1]string
		one[0] = key
		return getPathParts(f, one[:])
	}
}

func (f *rankingFacts) providerValue(parts []string) (any, bool) {
	if len(parts) == 0 {
		return f.provider, true
	}
	if v, ok := f.domainValue("provider", parts, true); ok {
		return v, true
	}
	if f.metadata != nil {
		return f.metadata.GetPath(parts)
	}
	return nil, false
}

func getPathParts(f Facts, parts []string) (any, bool) {
	if pf, ok := f.(PathFacts); ok {
		return pf.GetPath(parts)
	}
	return f.Get(strings.Join(parts, "."))
}

func getVariableParts(f Facts, parts []string) (any, bool) {
	if f == nil {
		return nil, false
	}
	return getPathParts(f, parts)
}
