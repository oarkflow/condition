package bcl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oarkflow/condition"
)

type DecisionPackage = condition.DecisionPackage
type Schema = condition.Schema
type SchemaRule = condition.SchemaRule
type Dataset = condition.Dataset
type DatasetRecord = condition.DatasetRecord
type Policy = condition.Policy
type PolicyRule = condition.PolicyRule
type RuleSet = condition.RuleSet
type Rule = condition.Rule
type Group = condition.Group
type GroupMode = condition.GroupMode
type Ranking = condition.Ranking
type SelectionMode = condition.SelectionMode
type Optimization = condition.Optimization
type ScoreRule = condition.ScoreRule
type ScoreDirection = condition.ScoreDirection
type Workflow = condition.Workflow
type Stage = condition.Stage
type Assignment = condition.Assignment
type Action = condition.Action
type Event = condition.Event
type ActionDefinition = condition.ActionDefinition
type Governance = condition.Governance
type DecisionTestCase = condition.DecisionTestCase
type MapFacts = condition.MapFacts
type Normalize = condition.Normalize

func init() {
	condition.RegisterDecisionPackageDecoder("bcl", Decoder())
}

var bclIdentByte = func() [256]bool {
	var table [256]bool
	table['_'], table['.'], table['-'], table['/'], table[':'] = true, true, true, true, true
	for ch := byte('a'); ch <= byte('z'); ch++ {
		table[ch] = true
	}
	for ch := byte('A'); ch <= byte('Z'); ch++ {
		table[ch] = true
	}
	for ch := byte('0'); ch <= byte('9'); ch++ {
		table[ch] = true
	}
	return table
}()

var bclNumberByte = func() [256]bool {
	var table [256]bool
	table['-'], table['.'] = true, true
	for ch := byte('0'); ch <= byte('9'); ch++ {
		table[ch] = true
	}
	return table
}()

type bclTokenKind uint8

const (
	bclEOF bclTokenKind = iota
	bclIdent
	bclString
	bclNumber
	bclLBrace
	bclRBrace
	bclLBracket
	bclRBracket
	bclComma
	bclEqual
	bclPlusEqual
	bclNewline
)

type bclToken struct {
	kind       bclTokenKind
	start, end int
	line, col  int
}

func (k bclTokenKind) String() string {
	switch k {
	case bclEOF:
		return "EOF"
	case bclIdent:
		return "identifier"
	case bclString:
		return "string"
	case bclNumber:
		return "number"
	case bclLBrace:
		return "{"
	case bclRBrace:
		return "}"
	case bclLBracket:
		return "["
	case bclRBracket:
		return "]"
	case bclComma:
		return ","
	case bclEqual:
		return "="
	case bclPlusEqual:
		return "+="
	case bclNewline:
		return "newline"
	default:
		return "unknown"
	}
}

type bclScanner struct {
	src       []byte
	pos       int
	line, col int
}

func newBCLScanner(src []byte) bclScanner { return bclScanner{src: src, line: 1, col: 1} }

func (s *bclScanner) next() bclToken {
	src := s.src
	pos, line, col := s.pos, s.line, s.col
	for pos < len(src) {
		ch := src[pos]
		if ch == ' ' || ch == '\t' || ch == '\r' {
			pos++
			col++
			continue
		}
		if ch == '#' || (ch == '/' && pos+1 < len(src) && src[pos+1] == '/') {
			for pos < len(src) && src[pos] != '\n' {
				pos++
				col++
			}
			continue
		}
		break
	}
	if pos >= len(src) {
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclEOF, start: pos, end: pos, line: line, col: col}
	}
	start, startLine, startCol := pos, line, col
	ch := src[pos]
	switch ch {
	case '\n':
		pos++
		line++
		col = 1
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclNewline, start: start, end: pos, line: startLine, col: startCol}
	case '{':
		pos++
		col++
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclLBrace, start: start, end: pos, line: startLine, col: startCol}
	case '}':
		pos++
		col++
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclRBrace, start: start, end: pos, line: startLine, col: startCol}
	case '[':
		pos++
		col++
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclLBracket, start: start, end: pos, line: startLine, col: startCol}
	case ']':
		pos++
		col++
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclRBracket, start: start, end: pos, line: startLine, col: startCol}
	case ',':
		pos++
		col++
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclComma, start: start, end: pos, line: startLine, col: startCol}
	case '=':
		pos++
		col++
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclEqual, start: start, end: pos, line: startLine, col: startCol}
	case '+':
		if pos+1 < len(src) && src[pos+1] == '=' {
			pos += 2
			col += 2
			s.pos, s.line, s.col = pos, line, col
			return bclToken{kind: bclPlusEqual, start: start, end: pos, line: startLine, col: startCol}
		}
	case '"':
		pos++
		col++
		for pos < len(src) {
			c := src[pos]
			pos++
			col++
			if c == '\\' && pos < len(src) {
				if src[pos] == '\n' {
					line++
					col = 1
				} else {
					col++
				}
				pos++
				continue
			}
			if c == '"' {
				break
			}
			if c == '\n' {
				line++
				col = 1
			}
		}
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: bclString, start: start, end: pos, line: startLine, col: startCol}
	}
	if bclIdentByte[ch] {
		kind := bclIdent
		if ch == '-' || ('0' <= ch && ch <= '9') {
			kind = bclNumber
		}
		pos++
		col++
		for pos < len(src) && bclIdentByte[src[pos]] {
			if kind == bclNumber && !bclNumberByte[src[pos]] {
				kind = bclIdent
			}
			pos++
			col++
		}
		s.pos, s.line, s.col = pos, line, col
		return bclToken{kind: kind, start: start, end: pos, line: startLine, col: startCol}
	}
	pos++
	col++
	s.pos, s.line, s.col = pos, line, col
	return bclToken{kind: bclIdent, start: start, end: pos, line: startLine, col: startCol}
}

func isBCLIdentPart(ch byte) bool {
	return bclIdentByte[ch]
}

func isBCLNumberPart(ch byte) bool {
	return bclNumberByte[ch]
}

type bclParser struct {
	src      []byte
	filename string
	baseDir  string
	imports  map[string]bool
	sc       bclScanner
	tok      bclToken
}

func ParsePackage(src []byte) (DecisionPackage, error) {
	return ParsePackageWithName("", src)
}

func ParsePackageWithName(filename string, src []byte) (DecisionPackage, error) {
	p := &bclParser{src: src, filename: filename, baseDir: filepath.Dir(filename), imports: map[string]bool{}, sc: newBCLScanner(src)}
	p.next()
	p.skipNewlines()
	if !p.isIdent("module") {
		return DecisionPackage{}, p.err("expected module block")
	}
	pkg, err := p.parseModule()
	if err != nil {
		return pkg, err
	}
	applyPackageConstants(&pkg)
	return pkg, nil
}

func Decoder() condition.Decoder[DecisionPackage] {
	return condition.DecoderFunc[DecisionPackage](ParsePackage)
}

func PackagesDecoder() condition.Decoder[[]DecisionPackage] {
	return condition.DecoderFunc[[]DecisionPackage](ParsePackages)
}

func ParsePackages(data []byte) ([]DecisionPackage, error) {
	p := &bclParser{src: data, sc: newBCLScanner(data)}
	p.next()
	var out []DecisionPackage
	for {
		p.skipNewlines()
		if p.tok.kind == bclEOF {
			return out, nil
		}
		if !p.isIdent("module") {
			return nil, p.err("expected module block")
		}
		pkg, err := p.parseModule()
		if err != nil {
			return nil, err
		}
		applyPackageConstants(&pkg)
		out = append(out, pkg)
	}
}

func LoadPackageFile(path string) (DecisionPackage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DecisionPackage{}, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return DecisionPackage{}, err
	}
	p := &bclParser{src: data, filename: abs, baseDir: filepath.Dir(abs), imports: map[string]bool{abs: true}, sc: newBCLScanner(data)}
	p.next()
	p.skipNewlines()
	if !p.isIdent("module") {
		return DecisionPackage{}, p.err("expected module block")
	}
	pkg, err := p.parseModule()
	if err != nil {
		return pkg, err
	}
	applyPackageConstants(&pkg)
	return pkg, nil
}

func (p *bclParser) parseModule() (DecisionPackage, error) {
	_ = p.expectIdent("module")
	name, err := p.expectStringLike()
	if err != nil {
		return DecisionPackage{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return DecisionPackage{}, err
	}
	pkg := DecisionPackage{Name: name}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return pkg, nil
		}
		if p.tok.kind == bclEOF {
			return pkg, p.err("unterminated module block")
		}
		switch p.text() {
		case "import":
			err = p.parseImport(&pkg)
		case "const":
			err = p.parseConst(&pkg)
		case "vars":
			pkg.Variables, err = p.parseFactsBlock("vars")
		case "metadata":
			pkg.Metadata, err = p.parseNamedMapBlock("metadata")
		case "version":
			pkg.Version, err = p.parseStringAttr("version")
		case "environment":
			pkg.Environment, err = p.parseStringAttr("environment")
		case "schema":
			var schema Schema
			var schemaName string
			schemaName, schema, err = p.parseSchema()
			if pkg.Schemas == nil {
				pkg.Schemas = map[string]Schema{}
			}
			pkg.Schemas[schemaName] = schema
		case "dataset":
			var dataset Dataset
			dataset, err = p.parseDataset()
			pkg.Datasets = append(pkg.Datasets, dataset)
		case "policy":
			var policy Policy
			policy, err = p.parsePolicy()
			pkg.Policies = append(pkg.Policies, policy)
		case "rule_set":
			var rs RuleSet
			rs, err = p.parseRuleSet()
			pkg.RuleSets = append(pkg.RuleSets, rs)
		case "ranking":
			var ranking Ranking
			ranking, err = p.parseRanking()
			pkg.Rankings = append(pkg.Rankings, ranking)
		case "optimize":
			var opt Optimization
			opt, err = p.parseOptimize()
			pkg.Optimizations = append(pkg.Optimizations, opt)
		case "workflow":
			var wf Workflow
			wf, err = p.parseWorkflow()
			pkg.Workflows = append(pkg.Workflows, wf)
		case "action":
			var action ActionDefinition
			action, err = p.parseActionDefinition()
			pkg.Actions = append(pkg.Actions, action)
		case "test":
			var tc DecisionTestCase
			tc, err = p.parseTest()
			pkg.Tests = append(pkg.Tests, tc)
		case "governance":
			pkg.Governance, err = p.parseGovernance()
		default:
			err = p.err("unexpected module item " + p.text())
		}
		if err != nil {
			return pkg, err
		}
	}
}

func (p *bclParser) parseImport(pkg *DecisionPackage) error {
	if err := p.expectIdent("import"); err != nil {
		return err
	}
	raw, err := p.expectStringLike()
	if err != nil {
		return err
	}
	pkg.Imports = append(pkg.Imports, raw)
	if p.baseDir == "" || filepath.IsAbs(raw) {
		return nil
	}
	path := filepath.Clean(filepath.Join(p.baseDir, raw))
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if p.imports[abs] {
		return p.err("BCL import cycle detected")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	imports := map[string]bool{}
	for k, v := range p.imports {
		imports[k] = v
	}
	imports[abs] = true
	imported := &bclParser{src: data, filename: abs, baseDir: filepath.Dir(abs), imports: imports, sc: newBCLScanner(data)}
	imported.next()
	imported.skipNewlines()
	if !imported.isIdent("module") {
		return imported.err("expected module block")
	}
	other, err := imported.parseModule()
	if err != nil {
		return err
	}
	mergeImportedPackage(pkg, other)
	return nil
}

func (p *bclParser) parseConst(pkg *DecisionPackage) error {
	if err := p.expectIdent("const"); err != nil {
		return err
	}
	name, err := p.expectStringLike()
	if err != nil {
		return err
	}
	if err := p.expect(bclEqual); err != nil {
		return err
	}
	value, err := p.parseValue()
	if err != nil {
		return err
	}
	if pkg.Constants == nil {
		pkg.Constants = map[string]any{}
	}
	pkg.Constants[name] = value
	return nil
}

func (p *bclParser) parseDataset() (Dataset, error) {
	_ = p.expectIdent("dataset")
	name, err := p.expectStringLike()
	if err != nil {
		return Dataset{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return Dataset{}, err
	}
	dataset := Dataset{Name: name}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return dataset, nil
		}
		if !p.isIdent("record") {
			return dataset, p.err("expected dataset record")
		}
		record, err := p.parseDatasetRecord()
		if err != nil {
			return dataset, err
		}
		dataset.Records = append(dataset.Records, record)
	}
}

func (p *bclParser) parseDatasetRecord() (DatasetRecord, error) {
	_ = p.expectIdent("record")
	id, err := p.expectStringLike()
	if err != nil {
		return DatasetRecord{}, err
	}
	facts, err := p.parseMapBlock()
	if err != nil {
		return DatasetRecord{}, err
	}
	record := DatasetRecord{ID: id, Facts: facts}
	if name, ok := facts["name"].(string); ok {
		record.Name = name
		delete(facts, "name")
	}
	return record, nil
}

func (p *bclParser) parseSchema() (string, Schema, error) {
	_ = p.expectIdent("schema")
	name, err := p.expectStringLike()
	if err != nil {
		return "", Schema{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return "", Schema{}, err
	}
	schema := Schema{}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return name, schema, nil
		}
		switch p.text() {
		case "required":
			p.next()
			path, err := p.expectStringLike()
			if err != nil {
				return "", schema, err
			}
			schema.Required = append(schema.Required, path)
		case "type":
			p.next()
			path, err := p.expectStringLike()
			if err != nil {
				return "", schema, err
			}
			typ, err := p.expectStringLike()
			if err != nil {
				return "", schema, err
			}
			if schema.Types == nil {
				schema.Types = map[string]string{}
			}
			schema.Types[path] = typ
		case "field":
			var path string
			var rule SchemaRule
			path, rule, err = p.parseSchemaField()
			if err != nil {
				return "", schema, err
			}
			if schema.Rules == nil {
				schema.Rules = map[string]SchemaRule{}
			}
			schema.Rules[path] = rule
		default:
			return "", schema, p.err("unexpected schema item " + p.text())
		}
	}
}

func (p *bclParser) parseSchemaField() (string, SchemaRule, error) {
	_ = p.expectIdent("field")
	path, err := p.expectStringLike()
	if err != nil {
		return "", SchemaRule{}, err
	}
	rule, err := p.parseSchemaRuleBlock()
	return path, rule, err
}

func (p *bclParser) parseSchemaRuleBlock() (SchemaRule, error) {
	if err := p.expect(bclLBrace); err != nil {
		return SchemaRule{}, err
	}
	var rule SchemaRule
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return rule, nil
		}
		key := p.text()
		p.next()
		if key == "property" {
			name, err := p.expectStringLike()
			if err != nil {
				return rule, err
			}
			child, err := p.parseSchemaRuleBlock()
			if err != nil {
				return rule, err
			}
			if rule.Properties == nil {
				rule.Properties = map[string]SchemaRule{}
			}
			rule.Properties[name] = child
			continue
		}
		if err := p.expect(bclEqual); err != nil {
			return rule, err
		}
		value, err := p.parseValue()
		if err != nil {
			return rule, err
		}
		if err := applySchemaRuleAttr(&rule, key, value); err != nil {
			return rule, err
		}
	}
}

func (p *bclParser) parsePolicy() (Policy, error) {
	_ = p.expectIdent("policy")
	name, err := p.expectStringLike()
	if err != nil {
		return Policy{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return Policy{}, err
	}
	policy := Policy{Name: name}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return policy, nil
		}
		if p.isIdent("default") {
			p.next()
			raw, err := p.expectStringLike()
			if err != nil {
				return policy, err
			}
			policy.DefaultEffect = normalizeEffect(raw)
			continue
		}
		effect := normalizeEffect(p.text())
		p.next()
		id, err := p.expectStringLike()
		if err != nil {
			return policy, err
		}
		rule := PolicyRule{ID: id, Effect: effect}
		if p.isIdent("when") {
			rule.Condition, err = p.parseExpressionBlock("when")
			if err != nil {
				return policy, err
			}
		}
		for {
			p.skipNewlines()
			switch p.text() {
			case "reason":
				p.next()
				rule.Reason, err = p.expectStringLike()
			case "then":
				err = p.parsePolicyThen(&rule)
			case "score":
				err = p.parsePolicyScore(&rule)
			case "action":
				var action Action
				action, err = p.parseAction()
				rule.Actions = append(rule.Actions, action)
			case "event":
				var event Event
				event, err = p.parseEvent()
				rule.Events = append(rule.Events, event)
			case "stop_on_match":
				rule.StopOnMatch, err = p.parseBoolAttr("stop_on_match")
			default:
				err = nil
				goto policyRuleDone
			}
			if err != nil {
				return policy, err
			}
		}
	policyRuleDone:
		policy.Rules = append(policy.Rules, rule)
	}
}

func (p *bclParser) parsePolicyThen(rule *PolicyRule) error {
	_ = p.expectIdent("then")
	if err := p.expect(bclLBrace); err != nil {
		return err
	}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return nil
		}
		var err error
		switch p.text() {
		case "score":
			err = p.parsePolicyScore(rule)
		case "action":
			var action Action
			action, err = p.parseAction()
			rule.Actions = append(rule.Actions, action)
		case "event":
			var event Event
			event, err = p.parseEvent()
			rule.Events = append(rule.Events, event)
		case "stop_on_match":
			rule.StopOnMatch, err = p.parseBoolAttr("stop_on_match")
		default:
			return p.err("unexpected policy then item " + p.text())
		}
		if err != nil {
			return err
		}
	}
}

func (p *bclParser) parsePolicyScore(rule *PolicyRule) error {
	p.next()
	if p.tok.kind != bclPlusEqual && p.tok.kind != bclEqual {
		return p.err("expected score assignment")
	}
	p.next()
	score, err := p.expectFloat()
	if err != nil {
		return err
	}
	rule.Score += score
	return nil
}

func (p *bclParser) parseRuleSet() (RuleSet, error) {
	_ = p.expectIdent("rule_set")
	name, err := p.expectStringLike()
	if err != nil {
		return RuleSet{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return RuleSet{}, err
	}
	rs := RuleSet{Name: name}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return rs, nil
		}
		if p.isIdent("execution_mode") {
			rs.ExecutionMode, err = p.parseExecutionModeAttr("execution_mode")
			if err != nil {
				return rs, err
			}
			continue
		}
		if !p.isIdent("rule") {
			return rs, p.err("expected rule")
		}
		rule, err := p.parseRule()
		if err != nil {
			return rs, err
		}
		rs.Rules = append(rs.Rules, rule)
	}
}

func (p *bclParser) parseRule() (Rule, error) {
	_ = p.expectIdent("rule")
	id, err := p.expectStringLike()
	if err != nil {
		return Rule{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return Rule{}, err
	}
	rule := Rule{ID: id}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return rule, nil
		}
		switch p.text() {
		case "when":
			rule.Condition, err = p.parseExpressionBlock("when")
		case "then":
			err = p.parseThen(&rule)
		case "group":
			rule.Group, err = p.parseGroup()
		case "reason":
			p.next()
			rule.Reason, err = p.expectStringLike()
		case "priority":
			p.next()
			rule.Priority, err = p.expectInt()
		case "salience":
			p.next()
			rule.Salience, err = p.expectInt()
		case "enabled":
			var enabled bool
			enabled, err = p.parseBoolAttr("enabled")
			rule.Enabled = &enabled
		case "stop_on_match":
			rule.StopOnMatch, err = p.parseBoolAttr("stop_on_match")
		case "valid_from":
			p.next()
			rule.ValidFrom, err = p.expectInt64()
		case "valid_until":
			p.next()
			rule.ValidUntil, err = p.expectInt64()
		case "next_stage":
			rule.NextStage, err = p.parseStringAttr("next_stage")
		default:
			err = p.err("unexpected rule item " + p.text())
		}
		if err != nil {
			return rule, err
		}
	}
}

func (p *bclParser) parseGroup() (*Group, error) {
	_ = p.expectIdent("group")
	raw, err := p.expectStringLike()
	if err != nil {
		return nil, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return nil, err
	}
	group := &Group{Mode: GroupMode(raw)}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return group, nil
		}
		if !p.isIdent("rule") {
			return group, p.err("expected rule in group")
		}
		rule, err := p.parseRule()
		if err != nil {
			return group, err
		}
		group.Rules = append(group.Rules, rule)
	}
}

func (p *bclParser) parseThen(rule *Rule) error {
	_ = p.expectIdent("then")
	if err := p.expect(bclLBrace); err != nil {
		return err
	}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return nil
		}
		switch p.text() {
		case "decision":
			rule.Decision, _ = p.parseStringAttr("decision")
		case "score":
			p.next()
			if p.tok.kind != bclPlusEqual && p.tok.kind != bclEqual {
				return p.err("expected score assignment")
			}
			p.next()
			score, err := p.expectFloat()
			if err != nil {
				return err
			}
			rule.Score += score
		case "action":
			action, err := p.parseAction()
			if err != nil {
				return err
			}
			rule.Actions = append(rule.Actions, action)
		case "event":
			event, err := p.parseEvent()
			if err != nil {
				return err
			}
			rule.Events = append(rule.Events, event)
		default:
			return p.err("unexpected then item " + p.text())
		}
	}
}

func (p *bclParser) parseAction() (Action, error) {
	_ = p.expectIdent("action")
	typ, err := p.expectStringLike()
	if err != nil {
		return Action{}, err
	}
	action := Action{Type: typ}
	if p.tok.kind == bclLBrace {
		action.Payload, err = p.parseMapBlock()
	}
	return action, err
}

func (p *bclParser) parseEvent() (Event, error) {
	_ = p.expectIdent("event")
	typ, err := p.expectStringLike()
	if err != nil {
		return Event{}, err
	}
	event := Event{Type: typ}
	if p.tok.kind == bclLBrace {
		event.Payload, err = p.parseMapBlock()
	}
	return event, err
}

func (p *bclParser) parseRanking() (Ranking, error) {
	_ = p.expectIdent("ranking")
	name, err := p.expectStringLike()
	if err != nil {
		return Ranking{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return Ranking{}, err
	}
	ranking := Ranking{Name: name, RuleSet: RuleSet{Name: name + "-ranking"}}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return ranking, nil
		}
		switch p.text() {
		case "selection":
			p.next()
			raw, err := p.expectStringLike()
			if err != nil {
				return ranking, err
			}
			ranking.Selection = SelectionMode(raw)
		case "dataset":
			ranking.Dataset, err = p.parseStringAttr("dataset")
		case "priority_path":
			ranking.PriorityPath, err = p.parseStringAttr("priority_path")
		case "specificity_path":
			ranking.SpecificityPath, err = p.parseStringAttr("specificity_path")
		case "fallback_path":
			ranking.FallbackPath, err = p.parseStringAttr("fallback_path")
		case "cost_path":
			ranking.CostPath, err = p.parseStringAttr("cost_path")
		case "weight_path":
			ranking.WeightPath, err = p.parseStringAttr("weight_path")
		case "hash_key_path":
			ranking.HashKeyPath, err = p.parseStringAttr("hash_key_path")
		case "rule":
			var rule Rule
			rule, err = p.parseRule()
			ranking.RuleSet.Rules = append(ranking.RuleSet.Rules, rule)
		case "score":
			var score ScoreRule
			score, err = p.parseScoreRule()
			ranking.RuleSet.ScoreRules = append(ranking.RuleSet.ScoreRules, score)
		default:
			err = p.err("unexpected ranking item " + p.text())
		}
		if err != nil {
			return ranking, err
		}
	}
}

func (p *bclParser) parseScoreRule() (ScoreRule, error) {
	_ = p.expectIdent("score")
	id, err := p.expectStringLike()
	if err != nil {
		return ScoreRule{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return ScoreRule{}, err
	}
	score := ScoreRule{ID: id}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return score, nil
		}
		switch p.text() {
		case "metric":
			score.Metric, err = p.parseStringAttr("metric")
		case "weight":
			p.next()
			if err = p.expect(bclEqual); err == nil {
				score.Weight, err = p.expectFloat()
			}
		case "direction":
			var raw string
			raw, err = p.parseStringAttr("direction")
			score.Direction = ScoreDirection(raw)
		case "normalize":
			p.next()
			score.Normalize.Min, err = p.expectFloat()
			if err == nil {
				score.Normalize.Max, err = p.expectFloat()
			}
		case "when":
			score.Condition, err = p.parseExpressionBlock("when")
		default:
			err = p.err("unexpected score item " + p.text())
		}
		if err != nil {
			return score, err
		}
	}
}

func (p *bclParser) parseOptimize() (Optimization, error) {
	_ = p.expectIdent("optimize")
	name, err := p.expectStringLike()
	if err != nil {
		return Optimization{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return Optimization{}, err
	}
	opt := Optimization{Name: name}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return opt, nil
		}
		switch p.text() {
		case "decision":
			opt.Decision, err = p.parseStringAttr("decision")
		case "dataset":
			opt.Dataset, err = p.parseStringAttr("dataset")
		case "goal":
			opt.Goal, err = p.parseStringAttr("goal")
		case "ranking":
			opt.Ranking, err = p.parseStringAttr("ranking")
		case "selection":
			p.next()
			var raw string
			raw, err = p.expectStringLike()
			opt.Selection = SelectionMode(raw)
		default:
			err = p.err("unexpected optimize item " + p.text())
		}
		if err != nil {
			return opt, err
		}
	}
}

func (p *bclParser) parseWorkflow() (Workflow, error) {
	_ = p.expectIdent("workflow")
	name, err := p.expectStringLike()
	if err != nil {
		return Workflow{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return Workflow{}, err
	}
	wf := Workflow{Name: name}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return wf, nil
		}
		switch p.text() {
		case "start":
			p.next()
			if err := p.expectIdent("at"); err != nil {
				return wf, err
			}
			wf.StartStage, err = p.expectStringLike()
		case "stage":
			var stage Stage
			stage, err = p.parseStage()
			wf.Stages = append(wf.Stages, stage)
		default:
			err = p.err("unexpected workflow item " + p.text())
		}
		if err != nil {
			return wf, err
		}
	}
}

func (p *bclParser) parseStage() (Stage, error) {
	_ = p.expectIdent("stage")
	name, err := p.expectStringLike()
	if err != nil {
		return Stage{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return Stage{}, err
	}
	stage := Stage{Name: name}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return stage, nil
		}
		switch p.text() {
		case "assign":
			stage.Assign, err = p.parseAssignment()
		case "sla":
			stage.SLA, err = p.parseStringAttr("sla")
		case "on_timeout":
			stage.OnTimeout, err = p.parseStringAttr("on_timeout")
		case "metadata":
			stage.Metadata, err = p.parseNamedMapBlock("metadata")
		case "rule":
			var rule Rule
			rule, err = p.parseRule()
			stage.Rules = append(stage.Rules, rule)
		default:
			return stage, p.err("expected rule in stage")
		}
		if err != nil {
			return stage, err
		}
	}
}

func (p *bclParser) parseAssignment() (*Assignment, error) {
	_ = p.expectIdent("assign")
	key, err := p.expectStringLike()
	if err != nil {
		return nil, err
	}
	value, err := p.expectStringLike()
	if err != nil {
		return nil, err
	}
	assignment := &Assignment{}
	switch key {
	case "role":
		assignment.Role = value
	case "user":
		assignment.User = value
	case "queue":
		assignment.Queue = value
	default:
		assignment.Metadata = map[string]any{key: value}
	}
	if p.tok.kind == bclLBrace {
		assignment.Metadata, err = p.parseMapBlock()
	}
	return assignment, err
}

func (p *bclParser) parseActionDefinition() (ActionDefinition, error) {
	_ = p.expectIdent("action")
	typ, err := p.expectStringLike()
	if err != nil {
		return ActionDefinition{}, err
	}
	def := ActionDefinition{Type: typ}
	if p.tok.kind == bclLBrace {
		payload, err := p.parseMapBlock()
		if err != nil {
			return def, err
		}
		if description, ok := payload["description"].(string); ok {
			def.Description = description
		}
		def.Metadata = payload
	}
	return def, nil
}

func (p *bclParser) parseGovernance() (Governance, error) {
	_ = p.expectIdent("governance")
	if err := p.expect(bclLBrace); err != nil {
		return Governance{}, err
	}
	var g Governance
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return g, nil
		}
		key := p.text()
		p.next()
		if err := p.expect(bclEqual); err != nil {
			return g, err
		}
		switch key {
		case "owner":
			g.Owner, _ = p.expectStringLike()
		case "maker":
			g.Maker, _ = p.expectStringLike()
		case "checker", "checkers":
			g.Checkers = append(g.Checkers, p.expectStringListOrOne()...)
		case "approver", "approvers":
			g.Approvers = append(g.Approvers, p.expectStringListOrOne()...)
		default:
			return g, p.err("unexpected governance item " + key)
		}
	}
}

func (p *bclParser) parseTest() (DecisionTestCase, error) {
	_ = p.expectIdent("test")
	name, err := p.expectStringLike()
	if err != nil {
		return DecisionTestCase{}, err
	}
	if err := p.expect(bclLBrace); err != nil {
		return DecisionTestCase{}, err
	}
	tc := DecisionTestCase{Name: name}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return tc, nil
		}
		switch p.text() {
		case "decision":
			tc.Decision, err = p.parseStringAttr("decision")
		case "input":
			tc.Context, err = p.parseFactsBlock("input")
		case "expect":
			tc.Expect, err = p.parseFactsBlock("expect")
		default:
			err = p.err("unexpected test item " + p.text())
		}
		if err != nil {
			return tc, err
		}
	}
}

func (p *bclParser) parseFactsBlock(name string) (MapFacts, error) {
	if err := p.expectIdent(name); err != nil {
		return nil, err
	}
	payload, err := p.parseMapBlock()
	return MapFacts(payload), err
}

func (p *bclParser) parseNamedMapBlock(name string) (map[string]any, error) {
	if err := p.expectIdent(name); err != nil {
		return nil, err
	}
	return p.parseMapBlock()
}

func (p *bclParser) parseMapBlock() (map[string]any, error) {
	if err := p.expect(bclLBrace); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for {
		p.skipNewlines()
		if p.tok.kind == bclRBrace {
			p.next()
			return out, nil
		}
		key, err := p.expectStringLike()
		if err != nil {
			return nil, err
		}
		if err := p.expect(bclEqual); err != nil {
			return nil, err
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		if strings.Contains(key, ".") {
			if err := setFactPath(MapFacts(out), strings.Split(key, "."), value); err != nil {
				return nil, err
			}
		} else {
			out[key] = value
		}
	}
}

func (p *bclParser) parseValue() (any, error) {
	switch p.tok.kind {
	case bclString:
		return p.expectStringLike()
	case bclNumber:
		raw := p.text()
		p.next()
		if strings.Contains(raw, ".") {
			return strconv.ParseFloat(raw, 64)
		}
		return strconv.ParseInt(raw, 10, 64)
	case bclIdent:
		raw := p.text()
		p.next()
		if raw == "true" {
			return true, nil
		}
		if raw == "false" {
			return false, nil
		}
		return raw, nil
	case bclLBracket:
		p.next()
		var out []any
		for {
			p.skipNewlines()
			if p.tok.kind == bclRBracket {
				p.next()
				return out, nil
			}
			value, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			out = append(out, value)
			if p.tok.kind == bclComma {
				p.next()
			}
		}
	default:
		return nil, p.err("expected value")
	}
}

func (p *bclParser) parseExpressionBlock(name string) (string, error) {
	if err := p.expectIdent(name); err != nil {
		return "", err
	}
	p.skipNewlines()
	if p.tok.kind != bclLBrace {
		return "", p.err("expected expression block")
	}
	start := p.tok.end
	depth := 1
	for depth > 0 {
		p.next()
		if p.tok.kind == bclEOF {
			return "", p.err("unterminated expression block")
		}
		if p.tok.kind == bclLBrace {
			depth++
		}
		if p.tok.kind == bclRBrace {
			depth--
		}
	}
	end := p.tok.start
	p.next()
	return normalizeBCLConditionBlock(strings.TrimSpace(string(p.src[start:end]))), nil
}

func normalizeBCLConditionBlock(raw string) string {
	lines := bclConditionLines(raw)
	if len(lines) == 0 {
		return raw
	}
	if !bclConditionLooksStructured(lines) {
		if len(lines) == 1 {
			return lines[0]
		}
		return joinBCLConditions(lines, "and")
	}
	expr, next := parseBCLConditionLines(lines, 0, "and")
	if next < len(lines) {
		rest := joinBCLConditions(lines[next:], "and")
		if rest != "" {
			if expr == "" {
				return rest
			}
			return "(" + expr + ") and (" + rest + ")"
		}
	}
	return expr
}

func bclConditionLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "{", "{\n")
	raw = strings.ReplaceAll(raw, "}", "\n}\n")
	parts := strings.Split(raw, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		line := strings.TrimSpace(part)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func bclConditionLooksStructured(lines []string) bool {
	if len(lines) > 1 {
		return true
	}
	for _, line := range lines {
		if line == "all {" || line == "any {" || line == "not {" {
			return true
		}
	}
	return false
}

func parseBCLConditionLines(lines []string, pos int, mode string) (string, int) {
	var exprs []string
	for pos < len(lines) {
		line := lines[pos]
		if line == "}" {
			return joinBCLConditions(exprs, mode), pos + 1
		}
		switch line {
		case "all {":
			var child string
			child, pos = parseBCLConditionLines(lines, pos+1, "and")
			if child != "" {
				exprs = append(exprs, child)
			}
		case "any {":
			var child string
			child, pos = parseBCLConditionLines(lines, pos+1, "or")
			if child != "" {
				exprs = append(exprs, child)
			}
		case "not {":
			var child string
			child, pos = parseBCLConditionLines(lines, pos+1, "and")
			if child != "" {
				exprs = append(exprs, "not ("+child+")")
			}
		default:
			exprs = append(exprs, line)
			pos++
		}
	}
	return joinBCLConditions(exprs, mode), pos
}

func joinBCLConditions(exprs []string, op string) string {
	filtered := exprs[:0]
	for _, expr := range exprs {
		expr = strings.TrimSpace(expr)
		if expr != "" {
			filtered = append(filtered, expr)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	var b strings.Builder
	for i, expr := range filtered {
		if i > 0 {
			b.WriteByte(' ')
			b.WriteString(op)
			b.WriteByte(' ')
		}
		b.WriteByte('(')
		b.WriteString(expr)
		b.WriteByte(')')
	}
	return b.String()
}

func (p *bclParser) parseStringAttr(name string) (string, error) {
	if err := p.expectIdent(name); err != nil {
		return "", err
	}
	if err := p.expect(bclEqual); err != nil {
		return "", err
	}
	return p.expectStringLike()
}

func (p *bclParser) next() { p.tok = p.sc.next() }

func (p *bclParser) skipNewlines() {
	for p.tok.kind == bclNewline {
		p.next()
	}
}

func (p *bclParser) isIdent(s string) bool {
	return p.tok.kind == bclIdent && bytes.Equal(p.src[p.tok.start:p.tok.end], []byte(s))
}

func (p *bclParser) text() string {
	if p.tok.kind == bclString {
		raw := p.src[p.tok.start:p.tok.end]
		if unquoted, err := strconv.Unquote(string(raw)); err == nil {
			return unquoted
		}
	}
	return string(p.src[p.tok.start:p.tok.end])
}

func (p *bclParser) expectIdent(s string) error {
	p.skipNewlines()
	if !p.isIdent(s) {
		return p.err("expected " + s)
	}
	p.next()
	return nil
}

func (p *bclParser) expect(kind bclTokenKind) error {
	p.skipNewlines()
	if p.tok.kind != kind {
		return p.err("unexpected token")
	}
	p.next()
	return nil
}

func (p *bclParser) expectStringLike() (string, error) {
	p.skipNewlines()
	if p.tok.kind != bclString && p.tok.kind != bclIdent && p.tok.kind != bclNumber {
		return "", p.err("expected string or identifier")
	}
	value := p.text()
	p.next()
	return value, nil
}

func (p *bclParser) expectFloat() (float64, error) {
	raw, err := p.expectStringLike()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(raw, 64)
}

func (p *bclParser) expectInt() (int, error) {
	raw, err := p.expectStringLike()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(raw)
}

func (p *bclParser) expectInt64() (int64, error) {
	raw, err := p.expectStringLike()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}

func (p *bclParser) parseBoolAttr(name string) (bool, error) {
	if err := p.expectIdent(name); err != nil {
		return false, err
	}
	if err := p.expect(bclEqual); err != nil {
		return false, err
	}
	raw, err := p.expectStringLike()
	if err != nil {
		return false, err
	}
	return strconv.ParseBool(raw)
}

func (p *bclParser) parseExecutionModeAttr(name string) (condition.ExecutionMode, error) {
	raw, err := p.parseStringAttr(name)
	if err != nil {
		return 0, err
	}
	switch raw {
	case "all_matches", "all":
		return condition.AllMatches, nil
	case "first_match", "first":
		return condition.FirstMatch, nil
	case "highest_priority":
		return condition.HighestPriority, nil
	case "deny_overrides":
		return condition.DenyOverrides, nil
	case "allow_overrides":
		return condition.AllowOverrides, nil
	case "score_based":
		return condition.ScoreBased, nil
	default:
		return 0, p.err("invalid execution_mode " + raw)
	}
}

func (p *bclParser) expectStringListOrOne() []string {
	if p.tok.kind != bclLBracket {
		value, _ := p.expectStringLike()
		return []string{value}
	}
	p.next()
	var out []string
	for p.tok.kind != bclRBracket && p.tok.kind != bclEOF {
		value, _ := p.expectStringLike()
		out = append(out, value)
		if p.tok.kind == bclComma {
			p.next()
		}
	}
	if p.tok.kind == bclRBracket {
		p.next()
	}
	return out
}

func setFactPath(root MapFacts, parts []string, value any) error {
	if len(parts) == 0 {
		return nil
	}
	current := map[string]any(root)
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("empty path segment")
		}
		if i == len(parts)-1 {
			current[part] = value
			return nil
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			if mf, ok := current[part].(MapFacts); ok {
				next = map[string]any(mf)
			} else {
				next = map[string]any{}
				current[part] = next
			}
		}
		current = next
	}
	return nil
}

func normalizeEffect(raw string) condition.DecisionEffect {
	switch condition.DecisionEffect(raw) {
	case condition.EffectAllow, condition.EffectDeny, condition.EffectRequireReview, condition.EffectEscalate, condition.EffectAbstain:
		return condition.DecisionEffect(raw)
	case "":
		return condition.EffectAbstain
	default:
		return condition.DecisionEffect(raw)
	}
}

func applySchemaRuleAttr(rule *SchemaRule, key string, value any) error {
	switch key {
	case "type":
		rule.Type = fmt.Sprint(value)
	case "required":
		v, ok := value.(bool)
		if !ok {
			return fmt.Errorf("required must be bool")
		}
		rule.Required = v
	case "enum":
		if values, ok := value.([]any); ok {
			rule.Enum = values
		} else {
			rule.Enum = []any{value}
		}
	case "min":
		v, err := numberValue(value)
		if err != nil {
			return err
		}
		rule.Min = &v
	case "max":
		v, err := numberValue(value)
		if err != nil {
			return err
		}
		rule.Max = &v
	case "min_length":
		v, err := intValue(value)
		if err != nil {
			return err
		}
		rule.MinLength = &v
	case "max_length":
		v, err := intValue(value)
		if err != nil {
			return err
		}
		rule.MaxLength = &v
	case "min_items":
		v, err := intValue(value)
		if err != nil {
			return err
		}
		rule.MinItems = &v
	case "max_items":
		v, err := intValue(value)
		if err != nil {
			return err
		}
		rule.MaxItems = &v
	default:
		return fmt.Errorf("unknown schema rule attribute %q", key)
	}
	return nil
}

func numberValue(value any) (float64, error) {
	switch v := value.(type) {
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("expected number")
	}
}

func intValue(value any) (int, error) {
	switch v := value.(type) {
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("expected integer")
	}
}

func mergeImportedPackage(dst *DecisionPackage, imported DecisionPackage) {
	if dst.Metadata == nil && len(imported.Metadata) > 0 {
		dst.Metadata = map[string]any{}
	}
	for k, v := range imported.Metadata {
		if _, exists := dst.Metadata[k]; !exists {
			dst.Metadata[k] = v
		}
	}
	if dst.Constants == nil && len(imported.Constants) > 0 {
		dst.Constants = map[string]any{}
	}
	for k, v := range imported.Constants {
		if _, exists := dst.Constants[k]; !exists {
			dst.Constants[k] = v
		}
	}
	if dst.Variables == nil && len(imported.Variables) > 0 {
		dst.Variables = MapFacts{}
	}
	for k, v := range imported.Variables {
		if _, exists := dst.Variables[k]; !exists {
			dst.Variables[k] = v
		}
	}
	if dst.Schemas == nil && len(imported.Schemas) > 0 {
		dst.Schemas = map[string]Schema{}
	}
	for k, v := range imported.Schemas {
		if _, exists := dst.Schemas[k]; !exists {
			dst.Schemas[k] = v
		}
	}
	dst.Datasets = append(imported.Datasets, dst.Datasets...)
	dst.RuleSets = append(imported.RuleSets, dst.RuleSets...)
	dst.Policies = append(imported.Policies, dst.Policies...)
	dst.Rankings = append(imported.Rankings, dst.Rankings...)
	dst.Workflows = append(imported.Workflows, dst.Workflows...)
	dst.Optimizations = append(imported.Optimizations, dst.Optimizations...)
	dst.Actions = append(imported.Actions, dst.Actions...)
}

func applyPackageConstants(pkg *DecisionPackage) {
	if len(pkg.Constants) == 0 {
		return
	}
	for i := range pkg.Policies {
		for j := range pkg.Policies[i].Rules {
			pkg.Policies[i].Rules[j].Condition = replaceConstants(pkg.Policies[i].Rules[j].Condition, pkg.Constants)
		}
	}
	for i := range pkg.RuleSets {
		applyConstantsToRules(pkg.RuleSets[i].Rules, pkg.Constants)
		for j := range pkg.RuleSets[i].ScoreRules {
			pkg.RuleSets[i].ScoreRules[j].Condition = replaceConstants(pkg.RuleSets[i].ScoreRules[j].Condition, pkg.Constants)
		}
	}
	for i := range pkg.Rankings {
		applyConstantsToRules(pkg.Rankings[i].RuleSet.Rules, pkg.Constants)
		for j := range pkg.Rankings[i].RuleSet.ScoreRules {
			pkg.Rankings[i].RuleSet.ScoreRules[j].Condition = replaceConstants(pkg.Rankings[i].RuleSet.ScoreRules[j].Condition, pkg.Constants)
		}
	}
	for i := range pkg.Workflows {
		for j := range pkg.Workflows[i].Stages {
			applyConstantsToRules(pkg.Workflows[i].Stages[j].Rules, pkg.Constants)
		}
	}
}

func applyConstantsToRules(rules []Rule, constants map[string]any) {
	for i := range rules {
		rules[i].Condition = replaceConstants(rules[i].Condition, constants)
		if rules[i].Group != nil {
			applyConstantsToRules(rules[i].Group.Rules, constants)
		}
	}
}

func replaceConstants(expr string, constants map[string]any) string {
	if expr == "" {
		return expr
	}
	out := expr
	for name, value := range constants {
		out = replaceIdentifier(out, name, literalValue(value))
	}
	return out
}

func replaceIdentifier(expr, name, value string) string {
	var b strings.Builder
	changed := false
	for i := 0; i < len(expr); {
		if i+len(name) <= len(expr) && expr[i:i+len(name)] == name && isIdentifierBoundary(expr, i-1) && isIdentifierBoundary(expr, i+len(name)) {
			if !changed {
				b.Grow(len(expr) + len(value))
				b.WriteString(expr[:i])
				changed = true
			}
			b.WriteString(value)
			i += len(name)
			continue
		}
		if changed {
			b.WriteByte(expr[i])
		}
		i++
	}
	if !changed {
		return expr
	}
	return b.String()
}

func isIdentifierBoundary(expr string, idx int) bool {
	if idx < 0 || idx >= len(expr) {
		return true
	}
	ch := expr[idx]
	return !(ch == '_' || ch == '.' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9'))
}

func literalValue(value any) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v)
	case bool:
		return strconv.FormatBool(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

func (p *bclParser) err(msg string) error {
	where := "BCL"
	if p.filename != "" {
		where = p.filename
	}
	token := p.text()
	if token == "" {
		token = p.tok.kind.String()
	}
	return fmt.Errorf("%s line %d col %d near %q: %s", where, p.tok.line, p.tok.col, token, msg)
}

func AppendPackage(dst []byte, pkg DecisionPackage) ([]byte, error) {
	dst = append(dst, "module "...)
	dst = appendBCLString(dst, pkg.Name)
	dst = append(dst, " {\n"...)
	dst = appendOptionalBCLAttr(dst, "  ", "version", pkg.Version)
	dst = appendOptionalBCLAttr(dst, "  ", "environment", pkg.Environment)
	for name, value := range pkg.Constants {
		dst = append(dst, "  const "...)
		dst = append(dst, name...)
		dst = append(dst, " = "...)
		dst = appendBCLValue(dst, value)
		dst = append(dst, '\n')
	}
	if len(pkg.Metadata) > 0 {
		dst = append(dst, "  metadata "...)
		dst = appendMapInlineBlock(dst, pkg.Metadata)
		dst = append(dst, '\n')
	}
	if len(pkg.Variables) > 0 {
		dst = appendFactsBlockBCL(dst, "  ", "vars", pkg.Variables)
	}
	for name, schema := range pkg.Schemas {
		dst = append(dst, "\n  schema "...)
		dst = appendBCLString(dst, name)
		dst = append(dst, " {\n"...)
		for _, path := range schema.Required {
			dst = append(dst, "    required "...)
			dst = append(dst, path...)
			dst = append(dst, '\n')
		}
		for path := range schema.Types {
			dst = append(dst, "    type "...)
			dst = append(dst, path...)
			dst = append(dst, ' ')
			dst = append(dst, schema.Types[path]...)
			dst = append(dst, '\n')
		}
		for path, rule := range schema.Rules {
			dst = appendSchemaRuleBCL(dst, "    ", path, rule)
		}
		dst = append(dst, "  }\n"...)
	}
	for _, dataset := range pkg.Datasets {
		dst = appendDatasetBCL(dst, dataset)
	}
	for _, policy := range pkg.Policies {
		dst = append(dst, "\n  policy "...)
		dst = appendBCLString(dst, policy.Name)
		dst = append(dst, " {\n"...)
		if policy.DefaultEffect != "" {
			dst = append(dst, "    default "...)
			dst = append(dst, string(policy.DefaultEffect)...)
			dst = append(dst, '\n')
		}
		for _, rule := range policy.Rules {
			dst = append(dst, "\n    "...)
			dst = append(dst, string(rule.Effect)...)
			dst = append(dst, ' ')
			dst = appendBCLString(dst, rule.ID)
			dst = append(dst, " when {\n      "...)
			dst = append(dst, rule.Condition...)
			dst = append(dst, "\n    }"...)
			if rule.Reason != "" {
				dst = append(dst, " reason "...)
				dst = appendBCLString(dst, rule.Reason)
			}
			if rule.Score != 0 || len(rule.Actions) > 0 || len(rule.Events) > 0 || rule.StopOnMatch {
				dst = append(dst, " then {\n"...)
				if rule.Score != 0 {
					dst = append(dst, "      score += "...)
					dst = strconv.AppendFloat(dst, rule.Score, 'f', -1, 64)
					dst = append(dst, '\n')
				}
				for _, action := range rule.Actions {
					dst = appendActionBCL(dst, "      ", action)
				}
				for _, event := range rule.Events {
					dst = appendEventBCL(dst, "      ", event)
				}
				if rule.StopOnMatch {
					dst = append(dst, "      stop_on_match = true\n"...)
				}
				dst = append(dst, "    }"...)
			}
			dst = append(dst, '\n')
		}
		dst = append(dst, "  }\n"...)
	}
	for _, rs := range pkg.RuleSets {
		dst = appendRuleSetBCL(dst, "  ", rs)
	}
	for _, ranking := range pkg.Rankings {
		dst = appendRankingBCL(dst, ranking)
	}
	for _, opt := range pkg.Optimizations {
		dst = appendOptimizeBCL(dst, opt)
	}
	for _, wf := range pkg.Workflows {
		dst = appendWorkflowBCL(dst, wf)
	}
	for _, tc := range pkg.Tests {
		dst = appendTestBCL(dst, tc)
	}
	dst = append(dst, "}\n"...)
	return dst, nil
}

func EncodePackage(pkg DecisionPackage) ([]byte, error) {
	return AppendPackage(make([]byte, 0, 4096), pkg)
}

func appendSchemaRuleBCL(dst []byte, indent, path string, rule SchemaRule) []byte {
	dst = append(dst, indent...)
	dst = append(dst, "field "...)
	dst = append(dst, path...)
	dst = append(dst, " { "...)
	if rule.Required {
		dst = append(dst, "required = true "...)
	}
	if rule.Type != "" {
		dst = append(dst, "type = "...)
		dst = appendBCLString(dst, rule.Type)
		dst = append(dst, ' ')
	}
	if len(rule.Enum) > 0 {
		dst = append(dst, "enum = "...)
		dst = appendBCLAnySlice(dst, rule.Enum)
		dst = append(dst, ' ')
	}
	if rule.Min != nil {
		dst = append(dst, "min = "...)
		dst = strconv.AppendFloat(dst, *rule.Min, 'f', -1, 64)
		dst = append(dst, ' ')
	}
	if rule.Max != nil {
		dst = append(dst, "max = "...)
		dst = strconv.AppendFloat(dst, *rule.Max, 'f', -1, 64)
		dst = append(dst, ' ')
	}
	if rule.MinLength != nil {
		dst = append(dst, "min_length = "...)
		dst = strconv.AppendInt(dst, int64(*rule.MinLength), 10)
		dst = append(dst, ' ')
	}
	if rule.MaxLength != nil {
		dst = append(dst, "max_length = "...)
		dst = strconv.AppendInt(dst, int64(*rule.MaxLength), 10)
		dst = append(dst, ' ')
	}
	if rule.MinItems != nil {
		dst = append(dst, "min_items = "...)
		dst = strconv.AppendInt(dst, int64(*rule.MinItems), 10)
		dst = append(dst, ' ')
	}
	if rule.MaxItems != nil {
		dst = append(dst, "max_items = "...)
		dst = strconv.AppendInt(dst, int64(*rule.MaxItems), 10)
		dst = append(dst, ' ')
	}
	dst = append(dst, "}\n"...)
	return dst
}

func appendDatasetBCL(dst []byte, dataset Dataset) []byte {
	dst = append(dst, "\n  dataset "...)
	dst = appendBCLString(dst, dataset.Name)
	dst = append(dst, " {\n"...)
	for _, record := range dataset.Records {
		dst = append(dst, "    record "...)
		dst = appendBCLString(dst, record.ID)
		dst = append(dst, ' ')
		dst = appendDatasetRecordBlock(dst, record)
		dst = append(dst, '\n')
	}
	dst = append(dst, "  }\n"...)
	return dst
}

func appendRankingBCL(dst []byte, ranking Ranking) []byte {
	dst = append(dst, "\n  ranking "...)
	dst = appendBCLString(dst, ranking.Name)
	dst = append(dst, " {\n"...)
	if ranking.Selection != "" {
		dst = append(dst, "    selection "...)
		dst = append(dst, string(ranking.Selection)...)
		dst = append(dst, '\n')
	}
	dst = appendOptionalBCLAttr(dst, "    ", "dataset", ranking.Dataset)
	dst = appendOptionalBCLAttr(dst, "    ", "priority_path", ranking.PriorityPath)
	dst = appendOptionalBCLAttr(dst, "    ", "specificity_path", ranking.SpecificityPath)
	dst = appendOptionalBCLAttr(dst, "    ", "fallback_path", ranking.FallbackPath)
	dst = appendOptionalBCLAttr(dst, "    ", "cost_path", ranking.CostPath)
	dst = appendOptionalBCLAttr(dst, "    ", "weight_path", ranking.WeightPath)
	dst = appendOptionalBCLAttr(dst, "    ", "hash_key_path", ranking.HashKeyPath)
	for _, rule := range ranking.RuleSet.Rules {
		dst = appendRuleBCL(dst, "    ", rule)
	}
	for _, score := range ranking.RuleSet.ScoreRules {
		dst = append(dst, "\n    score "...)
		dst = appendBCLString(dst, score.ID)
		dst = append(dst, " {\n"...)
		dst = appendBCLAttr(dst, "      ", "metric", score.Metric)
		if score.Weight != 0 {
			dst = append(dst, "      weight = "...)
			dst = strconv.AppendFloat(dst, score.Weight, 'f', -1, 64)
			dst = append(dst, '\n')
		}
		if score.Normalize.Min != 0 || score.Normalize.Max != 0 {
			dst = append(dst, "      normalize "...)
			dst = strconv.AppendFloat(dst, score.Normalize.Min, 'f', -1, 64)
			dst = append(dst, ' ')
			dst = strconv.AppendFloat(dst, score.Normalize.Max, 'f', -1, 64)
			dst = append(dst, '\n')
		}
		dst = append(dst, "    }\n"...)
	}
	dst = append(dst, "  }\n"...)
	return dst
}

func appendOptimizeBCL(dst []byte, opt Optimization) []byte {
	dst = append(dst, "\n  optimize "...)
	dst = appendBCLString(dst, opt.Name)
	dst = append(dst, " {\n"...)
	dst = appendOptionalBCLAttr(dst, "    ", "decision", opt.Decision)
	dst = appendOptionalBCLAttr(dst, "    ", "dataset", opt.Dataset)
	dst = appendOptionalBCLAttr(dst, "    ", "goal", opt.Goal)
	dst = appendOptionalBCLAttr(dst, "    ", "ranking", opt.Ranking)
	if opt.Selection != "" {
		dst = append(dst, "    selection "...)
		dst = append(dst, string(opt.Selection)...)
		dst = append(dst, '\n')
	}
	dst = append(dst, "  }\n"...)
	return dst
}

func appendWorkflowBCL(dst []byte, wf Workflow) []byte {
	dst = append(dst, "\n  workflow "...)
	dst = appendBCLString(dst, wf.Name)
	dst = append(dst, " {\n    start at "...)
	dst = appendBCLString(dst, wf.StartStage)
	dst = append(dst, '\n')
	for _, stage := range wf.Stages {
		dst = append(dst, "\n    stage "...)
		dst = appendBCLString(dst, stage.Name)
		dst = append(dst, " {\n"...)
		if stage.Assign != nil {
			dst = append(dst, "      assign "...)
			switch {
			case stage.Assign.Role != "":
				dst = append(dst, "role "...)
				dst = appendBCLString(dst, stage.Assign.Role)
			case stage.Assign.User != "":
				dst = append(dst, "user "...)
				dst = appendBCLString(dst, stage.Assign.User)
			case stage.Assign.Queue != "":
				dst = append(dst, "queue "...)
				dst = appendBCLString(dst, stage.Assign.Queue)
			}
			dst = append(dst, '\n')
		}
		dst = appendOptionalBCLAttr(dst, "      ", "sla", stage.SLA)
		dst = appendOptionalBCLAttr(dst, "      ", "on_timeout", stage.OnTimeout)
		for _, rule := range stage.Rules {
			dst = appendRuleBCL(dst, "      ", rule)
		}
		dst = append(dst, "    }\n"...)
	}
	dst = append(dst, "  }\n"...)
	return dst
}

func appendTestBCL(dst []byte, tc DecisionTestCase) []byte {
	dst = append(dst, "\n  test "...)
	dst = appendBCLString(dst, tc.Name)
	dst = append(dst, " {\n"...)
	dst = appendOptionalBCLAttr(dst, "    ", "decision", tc.Decision)
	dst = appendFactsBlockBCL(dst, "    ", "input", tc.Context)
	dst = appendFactsBlockBCL(dst, "    ", "expect", tc.Expect)
	dst = append(dst, "  }\n"...)
	return dst
}

func appendRuleSetBCL(dst []byte, indent string, rs RuleSet) []byte {
	dst = append(dst, '\n')
	dst = append(dst, indent...)
	dst = append(dst, "rule_set "...)
	dst = appendBCLString(dst, rs.Name)
	dst = append(dst, " {\n"...)
	if rs.ExecutionMode != 0 {
		dst = appendBCLAttr(dst, indent+"  ", "execution_mode", executionModeString(rs.ExecutionMode))
	}
	for _, rule := range rs.Rules {
		dst = appendRuleBCL(dst, indent+"  ", rule)
	}
	dst = append(dst, indent...)
	dst = append(dst, "}\n"...)
	return dst
}

func appendRuleBCL(dst []byte, indent string, rule Rule) []byte {
	dst = append(dst, '\n')
	dst = append(dst, indent...)
	dst = append(dst, "rule "...)
	dst = appendBCLString(dst, rule.ID)
	dst = append(dst, " {\n"...)
	if rule.Enabled != nil {
		dst = append(dst, indent...)
		dst = append(dst, "  enabled = "...)
		dst = strconv.AppendBool(dst, *rule.Enabled)
		dst = append(dst, '\n')
	}
	if rule.Priority != 0 {
		dst = append(dst, indent...)
		dst = append(dst, "  priority "...)
		dst = strconv.AppendInt(dst, int64(rule.Priority), 10)
		dst = append(dst, '\n')
	}
	if rule.Salience != 0 {
		dst = append(dst, indent...)
		dst = append(dst, "  salience "...)
		dst = strconv.AppendInt(dst, int64(rule.Salience), 10)
		dst = append(dst, '\n')
	}
	if rule.StopOnMatch {
		dst = append(dst, indent...)
		dst = append(dst, "  stop_on_match = true\n"...)
	}
	if rule.ValidFrom != 0 {
		dst = append(dst, indent...)
		dst = append(dst, "  valid_from "...)
		dst = strconv.AppendInt(dst, rule.ValidFrom, 10)
		dst = append(dst, '\n')
	}
	if rule.ValidUntil != 0 {
		dst = append(dst, indent...)
		dst = append(dst, "  valid_until "...)
		dst = strconv.AppendInt(dst, rule.ValidUntil, 10)
		dst = append(dst, '\n')
	}
	dst = appendOptionalBCLAttr(dst, indent+"  ", "next_stage", rule.NextStage)
	if rule.Condition != "" {
		dst = append(dst, indent...)
		dst = append(dst, "  when {\n"...)
		dst = append(dst, indent...)
		dst = append(dst, "    "...)
		dst = append(dst, rule.Condition...)
		dst = append(dst, '\n')
		dst = append(dst, indent...)
		dst = append(dst, "  }\n"...)
	}
	if rule.Group != nil {
		dst = appendGroupBCL(dst, indent, *rule.Group)
	}
	if rule.Decision != "" || rule.Score != 0 || len(rule.Actions) > 0 || len(rule.Events) > 0 {
		dst = append(dst, indent...)
		dst = append(dst, "  then {\n"...)
		if rule.Decision != "" {
			dst = append(dst, indent...)
			dst = appendBCLAttr(dst, "    ", "decision", rule.Decision)
		}
		if rule.Score != 0 {
			dst = append(dst, indent...)
			dst = append(dst, "    score += "...)
			dst = strconv.AppendFloat(dst, rule.Score, 'f', -1, 64)
			dst = append(dst, '\n')
		}
		for _, action := range rule.Actions {
			dst = append(dst, indent...)
			dst = appendActionBCL(dst, "    ", action)
		}
		for _, event := range rule.Events {
			dst = append(dst, indent...)
			dst = appendEventBCL(dst, "    ", event)
		}
		dst = append(dst, indent...)
		dst = append(dst, "  }\n"...)
	}
	if rule.Reason != "" {
		dst = append(dst, indent...)
		dst = append(dst, "  reason "...)
		dst = appendBCLString(dst, rule.Reason)
		dst = append(dst, '\n')
	}
	dst = append(dst, indent...)
	dst = append(dst, "}\n"...)
	return dst
}

func appendGroupBCL(dst []byte, indent string, group Group) []byte {
	dst = append(dst, indent...)
	dst = append(dst, "group "...)
	dst = append(dst, string(group.Mode)...)
	dst = append(dst, " {\n"...)
	for _, rule := range group.Rules {
		dst = appendRuleBCL(dst, indent, rule)
	}
	dst = append(dst, indent...)
	dst = append(dst, "}\n"...)
	return dst
}

func appendActionBCL(dst []byte, indent string, action Action) []byte {
	dst = append(dst, indent...)
	dst = append(dst, "action "...)
	dst = append(dst, action.Type...)
	dst = appendMapInlineBlock(dst, action.Payload)
	dst = append(dst, '\n')
	return dst
}

func appendEventBCL(dst []byte, indent string, event Event) []byte {
	dst = append(dst, indent...)
	dst = append(dst, "event "...)
	dst = append(dst, event.Type...)
	dst = appendMapInlineBlock(dst, event.Payload)
	dst = append(dst, '\n')
	return dst
}

func executionModeString(mode condition.ExecutionMode) string {
	switch mode {
	case condition.AllMatches:
		return "all_matches"
	case condition.FirstMatch:
		return "first_match"
	case condition.HighestPriority:
		return "highest_priority"
	case condition.DenyOverrides:
		return "deny_overrides"
	case condition.AllowOverrides:
		return "allow_overrides"
	case condition.ScoreBased:
		return "score_based"
	default:
		return ""
	}
}

func appendBCLAttr(dst []byte, indent, key, value string) []byte {
	dst = append(dst, indent...)
	dst = append(dst, key...)
	dst = append(dst, " = "...)
	dst = appendBCLString(dst, value)
	dst = append(dst, '\n')
	return dst
}

func appendOptionalBCLAttr(dst []byte, indent, key, value string) []byte {
	if value == "" {
		return dst
	}
	return appendBCLAttr(dst, indent, key, value)
}

func appendFactsBlockBCL(dst []byte, indent, name string, facts MapFacts) []byte {
	if len(facts) == 0 {
		return dst
	}
	dst = append(dst, indent...)
	dst = append(dst, name...)
	dst = append(dst, " {\n"...)
	var pathScratch [256]byte
	dst = appendFactsEntriesBCL(dst, indent+"  ", pathScratch[:0], facts)
	dst = append(dst, indent...)
	dst = append(dst, "}\n"...)
	return dst
}

func appendFactsEntriesBCL(dst []byte, indent string, prefix []byte, facts map[string]any) []byte {
	for key := range facts {
		oldLen := len(prefix)
		if oldLen > 0 {
			prefix = append(prefix, '.')
		}
		prefix = append(prefix, key...)
		switch child := facts[key].(type) {
		case map[string]any:
			dst = appendFactsEntriesBCL(dst, indent, prefix, child)
		case MapFacts:
			dst = appendFactsEntriesBCL(dst, indent, prefix, map[string]any(child))
		default:
			dst = append(dst, indent...)
			dst = append(dst, prefix...)
			dst = append(dst, " = "...)
			dst = appendBCLValue(dst, child)
			dst = append(dst, '\n')
		}
		prefix = prefix[:oldLen]
	}
	return dst
}

func appendMapInlineBlock(dst []byte, m map[string]any) []byte {
	dst = append(dst, " {"...)
	var scratch [256]byte
	dst, _ = appendMapInlineEntries(dst, scratch[:0], m, 0)
	dst = append(dst, "}"...)
	return dst
}

func appendMapInlineEntries(dst []byte, prefix []byte, in map[string]any, count int) ([]byte, int) {
	for k, v := range in {
		oldLen := len(prefix)
		if oldLen > 0 {
			prefix = append(prefix, '.')
		}
		prefix = append(prefix, k...)
		switch child := v.(type) {
		case map[string]any:
			dst, count = appendMapInlineEntries(dst, prefix, child, count)
		case MapFacts:
			dst, count = appendMapInlineEntries(dst, prefix, map[string]any(child), count)
		default:
			if count > 0 {
				dst = append(dst, ' ')
			}
			dst = append(dst, prefix...)
			dst = append(dst, " = "...)
			dst = appendBCLValue(dst, child)
			count++
		}
		prefix = prefix[:oldLen]
	}
	return dst, count
}

func appendDatasetRecordBlock(dst []byte, record DatasetRecord) []byte {
	dst = append(dst, " {"...)
	count := 0
	if record.Name != "" {
		dst = append(dst, "name = "...)
		dst = appendBCLString(dst, record.Name)
		count = 1
	}
	var scratch [256]byte
	dst, _ = appendMapInlineEntries(dst, scratch[:0], map[string]any(record.Facts), count)
	dst = append(dst, "}"...)
	return dst
}

func appendBCLValue(dst []byte, v any) []byte {
	switch x := v.(type) {
	case string:
		return appendBCLString(dst, x)
	case bool:
		return strconv.AppendBool(dst, x)
	case int:
		return strconv.AppendInt(dst, int64(x), 10)
	case int64:
		return strconv.AppendInt(dst, x, 10)
	case float64:
		return strconv.AppendFloat(dst, x, 'f', -1, 64)
	case []string:
		return appendBCLStringSlice(dst, x)
	case []any:
		return appendBCLAnySlice(dst, x)
	default:
		return appendBCLString(dst, fmt.Sprint(v))
	}
}

func appendBCLAnySlice(dst []byte, items []any) []byte {
	dst = append(dst, '[')
	for i, it := range items {
		if i > 0 {
			dst = append(dst, ", "...)
		}
		dst = appendBCLValue(dst, it)
	}
	return append(dst, ']')
}

func appendBCLStringSlice(dst []byte, items []string) []byte {
	dst = append(dst, '[')
	for i, it := range items {
		if i > 0 {
			dst = append(dst, ", "...)
		}
		dst = appendBCLString(dst, it)
	}
	return append(dst, ']')
}

func appendBCLString(dst []byte, s string) []byte {
	if bclStringNeedsQuoteSlowPath(s) {
		return strconv.AppendQuote(dst, s)
	}
	dst = append(dst, '"')
	dst = append(dst, s...)
	dst = append(dst, '"')
	return dst
}

func bclStringNeedsQuoteSlowPath(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == '"' || s[i] == '\\' || s[i] >= 0x80 {
			return true
		}
	}
	return false
}
