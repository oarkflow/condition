package bcl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oarkflow/condition"
)

const testBCLPackage = `
module "fraud-intelligence" {
  version = "1"
  environment = "test"

  schema "fraud-review" {
    required customer.blacklisted
    required transaction.amount
    type transaction.amount number
  }

  policy "fraud-review" {
    default allow

    deny "deny-blacklisted" when {
      customer.blacklisted == true
    } reason "customer is blacklisted"

    require_review "review-market" when {
      customer.country == "NP"
    } reason "manual review market"
  }

  rule_set "fraud-review" {
    rule "1001" {
      when { transaction.amount > 100000 }
      then {
        decision = "require_review"
        score += 40
        action notify { team = "compliance" }
      }
      reason "high value transaction"
    }
  }

  ranking "fraud-review" {
    selection best
    priority_path = "provider.priority"
    cost_path = "provider.load"

    rule "active" {
      when { provider.active == true }
    }

    score "priority" {
      metric = "provider.priority"
      weight = 0.7
      normalize 1 10
    }
  }

  optimize "queue-optimization" {
    decision = "fraud-review"
    goal = "maximize reviewer readiness while minimizing queue load"
    ranking = "fraud-review"
    selection best
  }

  workflow "fraud-review-workflow" {
    start at "intake"

    stage "intake" {
      rule "2001" {
        when { transaction.amount > 100000 }
        then { decision = "escalate" }
      }
    }
  }

  test "blacklisted customers are denied" {
    decision = "fraud-review"
    input {
      customer.blacklisted = true
      customer.country = "NP"
      transaction.amount = 1000
    }
    expect {
      effect = "deny"
      allowed = false
    }
  }
}
`

const fullLanguageBCLPackage = `
module "full-bcl" {
  version = "1"
  environment = "test"
  const HIGH_AMOUNT = 100000
  vars { market = "NP" }
  metadata { owner = "platform" }

  schema "review" {
    field transaction.amount { required = true type = "number" min = 0 max = 1000000 }
    field customer.country { required = true type = "string" enum = ["NP", "US"] min_length = 2 max_length = 2 }
  }

  dataset "review-queues" {
    record "aml" { name = "AML" provider.active = true provider.priority = 9 provider.load = 0.2 }
    record "standard" { name = "Standard" provider.active = true provider.priority = 5 provider.load = 0.5 }
  }

  policy "review" {
    default allow
    require_review "market-review" when { customer.country == market } then {
      score += 10
      action notify { team = "risk" }
      event policy_hit { name = "market-review" }
      stop_on_match = true
    } reason "market requires review"
  }

  rule_set "review" {
    execution_mode = "highest_priority"
    rule "1001" {
      priority 10
      salience 5
      enabled = true
      stop_on_match = true
      valid_from 1
      valid_until 4102444800
      when { transaction.amount > HIGH_AMOUNT }
      group all {
        rule "1002" { when { customer.country == "NP" } }
      }
      then {
        decision = "require_review"
        score += 40
        event high_amount { threshold = HIGH_AMOUNT }
      }
      reason "high amount"
    }
  }

  ranking "review" {
    dataset = "review-queues"
    selection best
    priority_path = "provider.priority"
    cost_path = "provider.load"
    rule "open" { when { provider.active == true } }
    score "priority" { metric = "provider.priority" weight = 1 normalize 1 10 }
  }

  workflow "review-workflow" {
    start at "intake"
    stage "intake" {
      assign role "risk_manager"
      sla = "24h"
      on_timeout = "escalate"
      rule "2001" {
        when { transaction.amount > HIGH_AMOUNT }
        then { decision = "escalate" }
      }
    }
  }

  test "high amount routes to aml" {
    decision = "review"
    input {
      transaction.amount = 125000
      customer.country = "NP"
    }
    expect {
      effect = "require_review"
      allowed = false
      score_gte = 50
      rank.id = "aml"
      actions = ["assign"]
      events = ["policy_hit", "high_amount"]
      matched_rule_ids = [1001]
    }
  }
}
`

func TestParsePackageComplete(t *testing.T) {
	pkg, err := ParsePackage([]byte(testBCLPackage))
	if err != nil {
		t.Fatalf("ParsePackage returned error: %v", err)
	}
	if pkg.Name != "fraud-intelligence" || pkg.Version != "1" || pkg.Environment != "test" {
		t.Fatalf("unexpected package identity: %#v", pkg)
	}
	if len(pkg.Policies) != 1 || len(pkg.RuleSets) != 1 || len(pkg.Rankings) != 1 || len(pkg.Workflows) != 1 || len(pkg.Tests) != 1 {
		t.Fatalf("incomplete package parse: %#v", pkg)
	}
	if pkg.RuleSets[0].Rules[0].Actions[0].Payload["team"] != "compliance" {
		t.Fatalf("action payload not parsed: %#v", pkg.RuleSets[0].Rules[0].Actions)
	}
}

func TestBCLRoundTripDigest(t *testing.T) {
	pkg, err := ParsePackage([]byte(testBCLPackage))
	if err != nil {
		t.Fatalf("ParsePackage returned error: %v", err)
	}
	encoded, err := EncodePackage(pkg)
	if err != nil {
		t.Fatalf("EncodePackage returned error: %v", err)
	}
	again, err := ParsePackage(encoded)
	if err != nil {
		t.Fatalf("roundtrip ParsePackage returned error: %v\n%s", err, string(encoded))
	}
	a, _ := condition.PackageDigest(pkg)
	b, _ := condition.PackageDigest(again)
	if a != b {
		t.Fatalf("digest mismatch after roundtrip: %s != %s\n%s", a, b, string(encoded))
	}
}

func TestDecoderWithLoadDecisionPackage(t *testing.T) {
	pkg, err := condition.LoadDecisionPackage(context.Background(), condition.StringSource(testBCLPackage), Decoder())
	if err != nil {
		t.Fatalf("LoadDecisionPackage BCL returned error: %v", err)
	}
	if pkg.Name != "fraud-intelligence" {
		t.Fatalf("loaded package = %#v", pkg)
	}
}

func TestBCLNestedConditionBlocksCompileToExpressionDSL(t *testing.T) {
	src := `
module "nested" {
  rule_set "fraud-review" {
    rule "nested-risk" {
      when {
        all {
          transaction.amount > 100000
          any {
            customer.country == "NP"
            customer.tier == "vip"
          }
          not {
            customer.blacklisted == true
          }
        }
      }
      then { decision = "require_review" }
    }
  }
}
`
	pkg, err := ParsePackage([]byte(src))
	if err != nil {
		t.Fatalf("ParsePackage returned error: %v", err)
	}
	conditionText := pkg.RuleSets[0].Rules[0].Condition
	if !strings.Contains(conditionText, " and ") || !strings.Contains(conditionText, " or ") || !strings.Contains(conditionText, "not (") {
		t.Fatalf("condition was not normalized with conjunctions: %s", conditionText)
	}
	expr := condition.MustCompile(conditionText)
	matched, err := expr.EvalBool(context.Background(), condition.MapFacts{
		"transaction": map[string]any{"amount": 125000},
		"customer":    map[string]any{"country": "NP", "tier": "standard", "blacklisted": false},
	})
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatalf("expected nested condition to match: %s", conditionText)
	}
}

func TestBCLImplicitAndConditionLines(t *testing.T) {
	src := `
module "implicit" {
  rule_set "eligibility" {
    rule "adult-np" {
      when {
        user.age >= 18
        user.country == "NP"
      }
      then { decision = "allow" }
    }
  }
}
`
	pkg, err := ParsePackage([]byte(src))
	if err != nil {
		t.Fatalf("ParsePackage returned error: %v", err)
	}
	condition := pkg.RuleSets[0].Rules[0].Condition
	if condition != `(user.age >= 18) and (user.country == "NP")` {
		t.Fatalf("condition = %q", condition)
	}
}

func TestBCLSyntaxErrorHasLineColumn(t *testing.T) {
	_, err := ParsePackageWithName("bad.bcl", []byte(`module "bad" { policy`))
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "bad.bcl") || !strings.Contains(err.Error(), "line") || !strings.Contains(err.Error(), "col") || !strings.Contains(err.Error(), "near") {
		t.Fatalf("error lacks line/column: %v", err)
	}
}

func TestBCLGrammarCoverageCommentsArraysAndMalformedBlocks(t *testing.T) {
	src := `
// top-level comment
module "grammar" {
  governance {
    owner = "risk"
    approvers = ["alice", "bob"]
  }
  rule_set "review" {
    # inline comment style
    rule "r1" {
      when {
        any {
          account.segment == "vip"
          all {
            transaction.amount >= 500
            not { customer.blocked == true }
          }
        }
      }
      then {
        decision = "allow"
        action notify { teams = ["risk", "ops"] urgent = true }
      }
    }
  }
}
`
	pkg, err := ParsePackage([]byte(src))
	if err != nil {
		t.Fatalf("ParsePackage returned error: %v", err)
	}
	if len(pkg.Governance.Approvers) != 2 || len(pkg.RuleSets) != 1 {
		t.Fatalf("grammar package parse incomplete: %#v", pkg)
	}
	condition := pkg.RuleSets[0].Rules[0].Condition
	if !strings.Contains(condition, " or ") || !strings.Contains(condition, "not (") {
		t.Fatalf("nested condition not normalized: %s", condition)
	}
	if _, err := ParsePackage([]byte(`module "bad" { rule_set "x" { rule "r" { when { all { a == 1 } }`)); err == nil {
		t.Fatal("expected malformed block error")
	}
}

func TestValidateAndRunDecisionPackageTests(t *testing.T) {
	pkg, err := ParsePackage([]byte(testBCLPackage))
	if err != nil {
		t.Fatalf("ParsePackage returned error: %v", err)
	}
	if diags := condition.ValidateDecisionPackage(pkg); condition.DiagnosticsHaveErrors(diags) {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	result, err := condition.RunDecisionPackageTests(context.Background(), pkg)
	if err != nil {
		t.Fatalf("RunDecisionPackageTests returned error: %v", err)
	}
	if !result.Passed || result.Total != 1 {
		t.Fatalf("test result = %#v", result)
	}
}

func TestValidateDecisionPackageDiagnostics(t *testing.T) {
	pkg := DecisionPackage{
		Name: "bad",
		RuleSets: []RuleSet{{
			Name: "x",
			Rules: []Rule{
				{ID: "1", Condition: `user.age >`},
				{ID: "1", Condition: `true`},
			},
		}},
		Optimizations: []Optimization{{Name: "opt", Ranking: "missing"}},
	}
	diags := condition.ValidateDecisionPackage(pkg)
	if !condition.DiagnosticsHaveErrors(diags) {
		t.Fatalf("expected validation errors: %#v", diags)
	}
}

func TestBCLFullLanguageLayerRuntimeFeatures(t *testing.T) {
	pkg, err := ParsePackage([]byte(fullLanguageBCLPackage))
	if err != nil {
		t.Fatalf("ParsePackage returned error: %v", err)
	}
	if pkg.RuleSets[0].Rules[0].Condition != "transaction.amount > 100000" {
		t.Fatalf("constant was not compiled into condition: %q", pkg.RuleSets[0].Rules[0].Condition)
	}
	orchestrator := condition.NewDecisionOrchestrator()
	if err := orchestrator.AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage returned error: %v", err)
	}
	res, err := orchestrator.Evaluate(context.Background(), condition.DecisionRequest{
		PackageName: "full-bcl",
		Decision:    "review",
		Context: condition.MapFacts{
			"transaction": map[string]any{"amount": 125000},
			"customer":    map[string]any{"country": "NP"},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if res.Rank == nil || res.Rank.ID != "aml" {
		t.Fatalf("dataset-backed ranking did not select aml: %#v", res.Rank)
	}
	if len(res.Events) < 2 {
		t.Fatalf("expected policy/rule/workflow events: %#v", res.Events)
	}
	if !hasAction(res.Actions, "assign") {
		t.Fatalf("workflow assignment action missing: %#v", res.Actions)
	}
	if result, err := condition.RunDecisionPackageTests(context.Background(), pkg); err != nil || !result.Passed {
		t.Fatalf("package tests failed result=%#v err=%v", result, err)
	}
}

func TestBCLFileImportsAndCycleDetection(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.bcl")
	main := filepath.Join(dir, "main.bcl")
	if err := os.WriteFile(shared, []byte(`
module "shared" {
  const LIMIT = 100
  dataset "queues" {
    record "fast" { provider.active = true provider.priority = 10 }
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte(`
module "main" {
  import "shared.bcl"
  ranking "route" {
    dataset = "queues"
    rule "open" { when { provider.active == true } }
    score "priority" { metric = "provider.priority" }
  }
  rule_set "route" {
    rule "limit" {
      when { order.amount > LIMIT }
      then { decision = "require_review" }
    }
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := LoadPackageFile(main)
	if err != nil {
		t.Fatalf("LoadPackageFile returned error: %v", err)
	}
	if len(pkg.Datasets) != 1 || pkg.RuleSets[0].Rules[0].Condition != "order.amount > 100" {
		t.Fatalf("import did not merge dataset/constants: %#v", pkg)
	}
	if err := os.WriteFile(shared, []byte(`module "shared" { import "main.bcl" }`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPackageFile(main); err == nil {
		t.Fatal("expected import cycle error")
	}
}

func hasAction(actions []condition.Action, typ string) bool {
	for _, action := range actions {
		if action.Type == typ {
			return true
		}
	}
	return false
}

func BenchmarkBCLScanner(b *testing.B) {
	src := []byte(testBCLPackage)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sc := newBCLScanner(src)
		for {
			tok := sc.next()
			if tok.kind == bclEOF {
				break
			}
		}
	}
}

func BenchmarkBCLScannerNextToken(b *testing.B) {
	src := []byte("customer.country")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sc := newBCLScanner(src)
		if tok := sc.next(); tok.kind != bclIdent {
			b.Fatal(tok.kind)
		}
	}
}

func BenchmarkBCLScannerSmallModule(b *testing.B) {
	src := []byte(`module "x" { version = "1" }`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sc := newBCLScanner(src)
		for {
			tok := sc.next()
			if tok.kind == bclEOF {
				break
			}
		}
	}
}

func BenchmarkAppendPackage(b *testing.B) {
	pkg, err := ParsePackage([]byte(testBCLPackage))
	if err != nil {
		b.Fatal(err)
	}
	dst := make([]byte, 0, 8192)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := dst[:0]
		if _, err := AppendPackage(buf, pkg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAppendFullLanguagePackage(b *testing.B) {
	pkg, err := ParsePackage([]byte(fullLanguageBCLPackage))
	if err != nil {
		b.Fatal(err)
	}
	dst := make([]byte, 0, 8192)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := dst[:0]
		if _, err := AppendPackage(buf, pkg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAppendBCLValueInt(b *testing.B) {
	dst := make([]byte, 0, 32)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := dst[:0]
		buf = appendBCLValue(buf, int64(12345))
		if len(buf) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkAppendBCLStringSimple(b *testing.B) {
	dst := make([]byte, 0, 32)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := dst[:0]
		buf = appendBCLString(buf, "fraud-review")
		if len(buf) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkAppendMapInlineBlockFlat(b *testing.B) {
	dst := make([]byte, 0, 128)
	payload := map[string]any{"team": "risk", "urgent": true, "score": int64(40)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := dst[:0]
		buf = appendMapInlineBlock(buf, payload)
		if len(buf) == 0 {
			b.Fatal("empty")
		}
	}
}

func BenchmarkAppendFullLanguageComponents(b *testing.B) {
	pkg, err := ParsePackage([]byte(fullLanguageBCLPackage))
	if err != nil {
		b.Fatal(err)
	}
	dst := make([]byte, 0, 4096)
	b.Run("dataset", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := dst[:0]
			buf = appendDatasetBCL(buf, pkg.Datasets[0])
			if len(buf) == 0 {
				b.Fatal("empty")
			}
		}
	})
	b.Run("schema-rule", func(b *testing.B) {
		rule := pkg.Schemas["review"].Rules["customer.country"]
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := dst[:0]
			buf = appendSchemaRuleBCL(buf, "    ", "customer.country", rule)
			if len(buf) == 0 {
				b.Fatal("empty")
			}
		}
	})
	b.Run("metadata", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := dst[:0]
			buf = appendMapInlineBlock(buf, pkg.Metadata)
			if len(buf) == 0 {
				b.Fatal("empty")
			}
		}
	})
	b.Run("vars", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := dst[:0]
			buf = appendFactsBlockBCL(buf, "  ", "vars", pkg.Variables)
			if len(buf) == 0 {
				b.Fatal("empty")
			}
		}
	})
	b.Run("ruleset", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := dst[:0]
			buf = appendRuleSetBCL(buf, "  ", pkg.RuleSets[0])
			if len(buf) == 0 {
				b.Fatal("empty")
			}
		}
	})
	b.Run("ranking", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := dst[:0]
			buf = appendRankingBCL(buf, pkg.Rankings[0])
			if len(buf) == 0 {
				b.Fatal("empty")
			}
		}
	})
	b.Run("workflow", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := dst[:0]
			buf = appendWorkflowBCL(buf, pkg.Workflows[0])
			if len(buf) == 0 {
				b.Fatal("empty")
			}
		}
	})
	b.Run("test", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := dst[:0]
			buf = appendTestBCL(buf, pkg.Tests[0])
			if len(buf) == 0 {
				b.Fatal("empty")
			}
		}
	})
}
