package condition

import (
	"context"
	"fmt"
	"reflect"
)

type DiagnosticSeverity string

const (
	DiagnosticError   DiagnosticSeverity = "error"
	DiagnosticWarning DiagnosticSeverity = "warning"
)

type Diagnostic struct {
	Severity DiagnosticSeverity `json:"severity"`
	Path     string             `json:"path,omitempty"`
	Message  string             `json:"message"`
}

type PackageTestResult struct {
	Package string                  `json:"package"`
	Version string                  `json:"version,omitempty"`
	Passed  bool                    `json:"passed"`
	Total   int                     `json:"total"`
	Failed  int                     `json:"failed"`
	Cases   []PackageTestCaseResult `json:"cases"`
}

type PackageTestCaseResult struct {
	Name     string           `json:"name"`
	Passed   bool             `json:"passed"`
	Expected MapFacts         `json:"expected,omitempty"`
	Response DecisionResponse `json:"response"`
	Error    string           `json:"error,omitempty"`
}

type PackageTestOption func(*packageTestConfig)

type packageTestConfig struct {
	candidates []Candidate
}

func WithPackageTestCandidates(candidates []Candidate) PackageTestOption {
	return func(c *packageTestConfig) { c.candidates = candidates }
}

func ValidateDecisionPackage(pkg DecisionPackage) []Diagnostic {
	var out []Diagnostic
	if pkg.Name == "" {
		out = append(out, Diagnostic{Severity: DiagnosticError, Path: "name", Message: "package name is required"})
	}
	if pkg.Version == "" {
		out = append(out, Diagnostic{Severity: DiagnosticWarning, Path: "version", Message: "package version is recommended for production replay and rollback"})
	}
	seenRuleSets := map[string]bool{}
	for i, rs := range pkg.RuleSets {
		path := fmt.Sprintf("rule_sets[%d]", i)
		if rs.Name == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "rule set name is required"})
		}
		if seenRuleSets[rs.Name] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "duplicate rule set name"})
		}
		seenRuleSets[rs.Name] = true
		out = append(out, validateRules(path+".rules", rs.Rules)...)
	}
	seenPolicies := map[string]bool{}
	for i, policy := range pkg.Policies {
		path := fmt.Sprintf("policies[%d]", i)
		if policy.Name == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "policy name is required"})
		}
		if seenPolicies[policy.Name] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "duplicate policy name"})
		}
		seenPolicies[policy.Name] = true
		for j, rule := range policy.Rules {
			rulePath := fmt.Sprintf("%s.rules[%d]", path, j)
			if rule.ID == "" {
				out = append(out, Diagnostic{Severity: DiagnosticError, Path: rulePath + ".id", Message: "policy rule id is required"})
			}
			if rule.Effect == "" {
				out = append(out, Diagnostic{Severity: DiagnosticError, Path: rulePath + ".effect", Message: "policy rule effect is required"})
			}
			if rule.Effect != "" && normalizeEffect(string(rule.Effect)) != rule.Effect {
				out = append(out, Diagnostic{Severity: DiagnosticWarning, Path: rulePath + ".effect", Message: "policy rule effect will be normalized"})
			}
			if rule.Condition != "" {
				if _, err := Compile(rule.Condition); err != nil {
					out = append(out, Diagnostic{Severity: DiagnosticError, Path: rulePath + ".condition", Message: err.Error()})
				}
			}
		}
	}
	seenRankings := map[string]bool{}
	seenDatasets := map[string]bool{}
	for i, dataset := range pkg.Datasets {
		path := fmt.Sprintf("datasets[%d]", i)
		if dataset.Name == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "dataset name is required"})
		}
		if seenDatasets[dataset.Name] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "duplicate dataset name"})
		}
		seenDatasets[dataset.Name] = true
		for j, record := range dataset.Records {
			if record.ID == "" {
				out = append(out, Diagnostic{Severity: DiagnosticError, Path: fmt.Sprintf("%s.records[%d].id", path, j), Message: "dataset record id is required"})
			}
		}
	}
	for i, ranking := range pkg.Rankings {
		path := fmt.Sprintf("rankings[%d]", i)
		if ranking.Name == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "ranking name is required"})
		}
		if ranking.RuleSet.Name == "" && len(ranking.RuleSet.Rules) == 0 && len(ranking.RuleSet.ScoreRules) == 0 {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".rule_set", Message: "ranking rule set is required"})
		}
		if ranking.Selection != "" && ranking.Selection != SelectionRanked && ranking.Selection != SelectionBest && ranking.Selection != SelectionWeighted {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".selection", Message: "ranking selection is invalid"})
		}
		if ranking.Selection == SelectionWeighted && (ranking.WeightPath == "" || ranking.HashKeyPath == "") {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".weight_path", Message: "weighted ranking requires weight_path and hash_key_path"})
		}
		if seenRankings[ranking.Name] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "duplicate ranking name"})
		}
		if ranking.Dataset != "" && !seenDatasets[ranking.Dataset] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".dataset", Message: "referenced dataset does not exist"})
		}
		seenRankings[ranking.Name] = true
		out = append(out, validateRules(path+".rule_set.rules", ranking.RuleSet.Rules)...)
		for j, score := range ranking.RuleSet.ScoreRules {
			scorePath := fmt.Sprintf("%s.rule_set.score_rules[%d]", path, j)
			if score.ID == "" || score.Metric == "" {
				out = append(out, Diagnostic{Severity: DiagnosticError, Path: scorePath, Message: "score rule id and metric are required"})
			}
			if score.Condition != "" {
				if _, err := Compile(score.Condition); err != nil {
					out = append(out, Diagnostic{Severity: DiagnosticError, Path: scorePath + ".condition", Message: err.Error()})
				}
			}
		}
	}
	for i, opt := range pkg.Optimizations {
		path := fmt.Sprintf("optimizations[%d]", i)
		if opt.Name == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "optimization name is required"})
		}
		if opt.Selection != "" && opt.Selection != SelectionRanked && opt.Selection != SelectionBest && opt.Selection != SelectionWeighted {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".selection", Message: "optimization selection is invalid"})
		}
		if opt.Ranking == "" && len(opt.Constraints) == 0 && len(opt.Preferences) == 0 {
			out = append(out, Diagnostic{Severity: DiagnosticWarning, Path: path, Message: "optimization has no ranking, constraints, or preferences"})
		}
		if opt.Ranking != "" && !seenRankings[opt.Ranking] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".ranking", Message: "referenced ranking does not exist"})
		}
		if opt.Dataset != "" && !seenDatasets[opt.Dataset] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".dataset", Message: "referenced dataset does not exist"})
		}
	}
	for i, wf := range pkg.Workflows {
		path := fmt.Sprintf("workflows[%d]", i)
		if wf.Name == "" || wf.StartStage == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path, Message: "workflow name and start stage are required"})
		}
		stages := map[string]bool{}
		for j, stage := range wf.Stages {
			stagePath := fmt.Sprintf("%s.stages[%d]", path, j)
			stages[stage.Name] = true
			out = append(out, validateRules(stagePath+".rules", stage.Rules)...)
		}
		if wf.StartStage != "" && !stages[wf.StartStage] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".start_stage", Message: "start stage does not exist"})
		}
	}
	for name, schema := range pkg.Schemas {
		for i, required := range schema.Required {
			if required == "" {
				out = append(out, Diagnostic{Severity: DiagnosticError, Path: fmt.Sprintf("schemas.%s.required[%d]", name, i), Message: "required path cannot be empty"})
			}
		}
		for path, typ := range schema.Types {
			if path == "" || typ == "" {
				out = append(out, Diagnostic{Severity: DiagnosticError, Path: fmt.Sprintf("schemas.%s.types.%s", name, path), Message: "schema type path and type are required"})
			}
		}
		for path, rule := range schema.Rules {
			out = append(out, validateSchemaRuleDefinition(fmt.Sprintf("schemas.%s.rules.%s", name, path), path, rule)...)
		}
		for path, rule := range schema.Properties {
			out = append(out, validateSchemaRuleDefinition(fmt.Sprintf("schemas.%s.properties.%s", name, path), path, rule)...)
		}
	}
	for i, tc := range pkg.Tests {
		path := fmt.Sprintf("tests[%d]", i)
		if tc.Name == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "test name is required"})
		}
		if tc.Name != "" {
			for j := 0; j < i; j++ {
				if pkg.Tests[j].Name == tc.Name {
					out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".name", Message: "duplicate test name"})
					break
				}
			}
		}
		if len(tc.Expect) == 0 {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".expect", Message: "test expectation is required"})
		}
	}
	seenActions := map[string]bool{}
	for i, action := range pkg.Actions {
		path := fmt.Sprintf("actions[%d]", i)
		if action.Type == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".type", Message: "action type is required"})
		}
		if seenActions[action.Type] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: path + ".type", Message: "duplicate action type"})
		}
		seenActions[action.Type] = true
	}
	return out
}

func validateSchemaRuleDefinition(diagPath, factPath string, rule SchemaRule) []Diagnostic {
	var out []Diagnostic
	if factPath == "" {
		out = append(out, Diagnostic{Severity: DiagnosticError, Path: diagPath, Message: "schema rule path is required"})
	}
	if rule.Min != nil && rule.Max != nil && *rule.Min > *rule.Max {
		out = append(out, Diagnostic{Severity: DiagnosticError, Path: diagPath, Message: "schema min cannot be greater than max"})
	}
	if rule.MinLength != nil && rule.MaxLength != nil && *rule.MinLength > *rule.MaxLength {
		out = append(out, Diagnostic{Severity: DiagnosticError, Path: diagPath, Message: "schema min_length cannot be greater than max_length"})
	}
	if rule.MinItems != nil && rule.MaxItems != nil && *rule.MinItems > *rule.MaxItems {
		out = append(out, Diagnostic{Severity: DiagnosticError, Path: diagPath, Message: "schema min_items cannot be greater than max_items"})
	}
	for child, childRule := range rule.Properties {
		out = append(out, validateSchemaRuleDefinition(diagPath+".properties."+child, factPath+"."+child, childRule)...)
	}
	return out
}

func validateRules(path string, rules []Rule) []Diagnostic {
	var out []Diagnostic
	seen := map[string]bool{}
	for i, rule := range rules {
		rulePath := fmt.Sprintf("%s[%d]", path, i)
		if rule.ID == "" {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: rulePath + ".id", Message: "rule id is required"})
		}
		if seen[rule.ID] {
			out = append(out, Diagnostic{Severity: DiagnosticError, Path: rulePath + ".id", Message: "duplicate rule id"})
		}
		seen[rule.ID] = true
		if rule.Condition != "" {
			if _, err := Compile(rule.Condition); err != nil {
				out = append(out, Diagnostic{Severity: DiagnosticError, Path: rulePath + ".condition", Message: err.Error()})
			}
		}
		if rule.Group != nil {
			out = append(out, validateRules(rulePath+".group.rules", rule.Group.Rules)...)
		}
	}
	return out
}

func DiagnosticsHaveErrors(diags []Diagnostic) bool {
	for _, diag := range diags {
		if diag.Severity == DiagnosticError {
			return true
		}
	}
	return false
}

func RunDecisionPackageTests(ctx context.Context, pkg DecisionPackage, opts ...PackageTestOption) (PackageTestResult, error) {
	cfg := packageTestConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	result := PackageTestResult{Package: pkg.Name, Version: pkg.Version, Total: len(pkg.Tests), Passed: true}
	if diags := ValidateDecisionPackage(pkg); DiagnosticsHaveErrors(diags) {
		result.Passed = false
		return result, fmt.Errorf("decision package has validation errors")
	}
	orchestrator := NewDecisionOrchestrator()
	if err := orchestrator.AddPackage(pkg); err != nil {
		return result, err
	}
	for _, tc := range pkg.Tests {
		req := DecisionRequest{
			PackageName: pkg.Name,
			Version:     pkg.Version,
			Environment: pkg.Environment,
			Decision:    tc.Decision,
			Context:     tc.Context,
			VariableMap: tc.Variables,
			Candidates:  cfg.candidates,
		}
		res, err := orchestrator.Evaluate(ctx, req)
		item := PackageTestCaseResult{Name: tc.Name, Expected: tc.Expect, Response: res, Passed: err == nil}
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Passed = decisionResponseMatches(tc.Expect, res)
			if !item.Passed {
				item.Error = "expectation did not match response"
			}
		}
		if !item.Passed {
			result.Passed = false
			result.Failed++
		}
		result.Cases = append(result.Cases, item)
	}
	return result, nil
}

func decisionResponseMatches(expect MapFacts, res DecisionResponse) bool {
	for key, want := range expect {
		var got any
		switch key {
		case "decision":
			got = res.Decision
		case "effect":
			got = string(res.Effect)
		case "allowed":
			got = res.Allowed
		case "score":
			got = res.Score
		case "score_gte":
			if !compareNumber(res.Score, want, func(a, b float64) bool { return a >= b }) {
				return false
			}
			continue
		case "score_lte":
			if !compareNumber(res.Score, want, func(a, b float64) bool { return a <= b }) {
				return false
			}
			continue
		case "rank":
			if child, ok := want.(map[string]any); ok {
				for childKey, childWant := range child {
					if childKey == "id" {
						if res.Rank == nil || !reflect.DeepEqual(normalizeExpectedValue(childWant), normalizeExpectedValue(res.Rank.ID)) {
							return false
						}
					}
				}
				continue
			}
			if res.Rank != nil {
				got = res.Rank.ID
			}
		case "actions":
			if !containsActionTypes(res.Actions, want) {
				return false
			}
			continue
		case "events":
			if !containsEventTypes(res.Events, want) {
				return false
			}
			continue
		case "matched_rule_ids":
			if !containsRuleIDs(res.MatchedRuleIDs, want) {
				return false
			}
			continue
		default:
			continue
		}
		if !reflect.DeepEqual(normalizeExpectedValue(want), normalizeExpectedValue(got)) {
			return false
		}
	}
	return true
}

func compareNumber(got float64, want any, cmp func(float64, float64) bool) bool {
	switch x := normalizeExpectedValue(want).(type) {
	case float64:
		return cmp(got, x)
	default:
		return false
	}
}

func containsActionTypes(actions []Action, want any) bool {
	wanted := stringListExpectation(want)
	for _, item := range wanted {
		found := false
		for _, action := range actions {
			if action.Type == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsEventTypes(events []Event, want any) bool {
	wanted := stringListExpectation(want)
	for _, item := range wanted {
		found := false
		for _, event := range events {
			if event.Type == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsRuleIDs(ids []RuleID, want any) bool {
	wanted := stringListExpectation(want)
	for _, item := range wanted {
		found := false
		for _, id := range ids {
			if fmt.Sprint(id) == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func stringListExpectation(want any) []string {
	switch x := want.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, fmt.Sprint(normalizeExpectedValue(item)))
		}
		return out
	case []string:
		return x
	default:
		return []string{fmt.Sprint(normalizeExpectedValue(want))}
	}
}

func normalizeExpectedValue(v any) any {
	switch x := v.(type) {
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return x
	}
}
