# condition

`condition` is a lightweight Go library for building native condition evaluators, rule engines, and candidate ranking systems.

It is designed for use cases such as:

- Routing: SMS providers, payment gateways, shipping carriers, notification channels
- Policy checks: eligibility, access, fraud, risk, limits
- Rule engines: decisions, scores, actions, events
- Matching: nullable route tables, default fallbacks, pattern matching
- Ranking: choose the best candidate from many possible rows

The core idea is simple: compile conditions once, evaluate them many times against facts.

## Features

- SPL expression parser/evaluator powered by `github.com/oarkflow/interpreter`
- Boolean logic: `and`, `or`, `not`
- Comparisons: `==`, `!=`, `<`, `<=`, `>`, `>=`
- Matching operators and helpers: `in`, `not in`, string membership, `regex_match`, `starts_with`, `ends_with`
- Literals: strings, numbers, booleans, `null`, arrays
- Dot paths: `user.id`, `route.priority`, `quality.success_rate`
- Variables as ordinary identifiers: `minAge`, `tenantID`, `threshold`
- Conditional/default helpers: SPL ternary/if expressions, `coalesce`, `defaultValue`, `isNull`, `isNotNull`
- String, numeric, and time helpers: `lower`, `trim`, `clamp`, `between`, `now`, `date`, `before`, `after`, `age`
- Aggregations: `count`, `sum`, `avg`, `min`, `max`, `filter`, `pluck`, `top`, `bottom`, `percentile`, `sortBy`, `take`, `skip`, `slice`, `reverse`, `distinct`, `groupBy`, `groupCount`, `groupSum`, `groupAvg`, `groupMin`, `groupMax`, `groupCountWhere`, `groupSumWhere`, `groupAvgWhere`, `distinctCount`, `any`, `all`, `none`, `countWhere`
- Route-table helpers: `nullableMatch`, `rangeMatch`, `active`, `validNow`, `specificity`, `stableBucket`
- Map, struct, function, and chained fact providers
- Fluent builder API
- Rule sets with groups, priorities, scores, decisions, actions, events
- Generic candidate ranker with weighted scoring and deterministic tie-breakers
- Decision packages with policies, schemas, rankings, workflows, optimizations, governance metadata, and tests
- Decision orchestrator with policy effect resolution, package fingerprints, explainability, audit records, and simulation diffs
- Typed facts, field registry, string interning, bytecode programs, and lock-free runtime reload
- Indexed bytecode rule evaluation for high-volume rule sets
- Zero-allocation hot paths for compiled boolean evaluation and best-candidate selection
- Interpreter-backed SPL syntax with bytecode and ranker native paths for supported rule shapes

## Install

```sh
go get github.com/oarkflow/condition
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	"github.com/oarkflow/condition"
)

func main() {
	expr := condition.MustCompile(`user.age >= 18 and recipient.country == "NP"`)

	matched, err := expr.EvalBool(context.Background(), condition.MapFacts{
		"user":      map[string]any{"age": 29},
		"recipient": map[string]any{"country": "NP"},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(matched)
}
```

For the lowest-overhead expression path, compile once and use `EvalBool`.

## Decision Intelligence Platform

For full decision intelligence use cases, define a `DecisionPackage` and evaluate it through `DecisionOrchestrator`. A package is the immutable unit for versioning, explainability, audit, simulation, and rollback.

```go
pkg := condition.DecisionPackage{
	Name:        "fraud-intelligence",
	Version:     "1",
	Environment: "production",
	Schemas: map[string]condition.Schema{"fraud-review": {
		Required: []string{"customer.blacklisted", "transaction.amount"},
		Types:    map[string]string{"transaction.amount": "number"},
	}},
	Policies: []condition.Policy{{
		Name:          "fraud-review",
		DefaultEffect: condition.EffectAllow,
		Rules: []condition.PolicyRule{
			{ID: "deny-blacklisted", Effect: condition.EffectDeny, Condition: `customer.blacklisted == true`},
			{ID: "review-market", Effect: condition.EffectRequireReview, Condition: `customer.country == "NP"`},
		},
	}},
	RuleSets: []condition.RuleSet{{
		Name: "fraud-review",
		Rules: []condition.Rule{
			{ID: "1001", Condition: `transaction.amount > 100000`, Score: 40, Decision: "require_review"},
		},
	}},
}

orchestrator := condition.NewDecisionOrchestrator()
_ = orchestrator.AddPackage(pkg)

res, err := orchestrator.Evaluate(ctx, condition.DecisionRequest{
	PackageName: "fraud-intelligence",
	Decision:    "fraud-review",
	Context: condition.MapFacts{
		"customer":    map[string]any{"blacklisted": false, "country": "NP"},
		"transaction": map[string]any{"amount": 125000},
	},
})
```

`DecisionResponse` includes:

- `Decision`, `Allowed`, `Effect`, `Score`, `Rank`, actions, and events
- package `Version` and canonical `Digest`
- `Explanation` with matched rules, failed rules, policy effects, score deltas, ranking reasons, workflow path, fact evidence, counterfactual hints, and evidence graph nodes
- `AuditRecord` with package digest, request/result fingerprints, timestamps, duration, and trace summary

Run the complete platform examples:

```sh
go run ./examples/decision_intelligence
go run ./examples/bcl_decision_platform
```

### BCL Package DSL

BCL is the canonical block language for decision packages. The same package can also be represented by Go structs, JSON, or YAML. Conditions inside `when { ... }` use the expression DSL documented below.

```txt
module "fraud-intelligence" {
  version = "1"
  environment = "production"

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
  }

  rule_set "fraud-review" {
    rule "1001" {
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
      then {
        decision = "require_review"
        score += 40
        action notify { team = "compliance" }
      }
    }
  }
}
```

Load packages with existing source/decoder patterns:

```go
import "github.com/oarkflow/condition/bcl"

pkg, err := condition.LoadDecisionPackage(ctx, condition.FileSource{Path: "decision.bcl"}, bcl.Decoder())
```

Use `bcl.PackagesDecoder` for multi-module files, `bcl.LoadPackageFile` for relative `import` resolution, `bcl.AppendPackage` for append-style encoding into caller-owned buffers, and `bcl.EncodePackage` for convenience.

`when` blocks accept normal expression DSL, multiple condition lines with implicit `and`, or nested condition sets with `all`, `any`, and `not`. Nested sets compile to the same deterministic expression runtime used by the rest of the engine. BCL also supports package constants, variables, metadata, relative imports, rich schema `field` rules, reusable `dataset` candidates, policy actions/events/scores, rule groups, execution modes, validity windows, workflow assignment/SLA metadata, and richer embedded test expectations.

### File-Based Live Runtime

Use `LiveDecisionRuntime` when decision packages live in BCL, JSON, or YAML files and operators need safe realtime refresh. The runtime validates and compiles changed packages before publishing them. Invalid edits are rejected and the last known-good package remains active.

```go
import _ "github.com/oarkflow/condition/bcl" // registers the .bcl decoder

runtime, err := condition.NewLiveDecisionRuntime(condition.LiveDecisionRuntimeConfig{
	Packages: []condition.DecisionPackageFile{{Path: "fraud.bcl"}},
	PollInterval:          500 * time.Millisecond,
	ValidateBeforePublish: true,
	RunPackageTests:       true,
	KeepLastGood:          true,
})
if err != nil {
	return err
}

events, errs := runtime.Watch(ctx)
_ = events
_ = errs
```

File helpers are available for examples and application-owned configuration:

```go
pkg, _ := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: "fraud.bcl"})
req, _ := condition.LoadDecisionRequestFile(ctx, "request.json")
cases, _ := condition.LoadDecisionRequestsFile(ctx, "simulation.yaml")
```

### Production Server

`DecisionServer` provides a standard-library HTTP surface over the embeddable orchestrator:

- `POST /v1/packages`
- `GET /v1/packages`
- `GET /v1/packages/{name}`
- `GET /v1/packages/{name}/versions/{version}`
- `POST /v1/decision/{package}/{decision}`
- `POST /v1/packages/{package}/simulate`
- `POST /v1/packages/compare`
- `POST /v1/packages/{package}/tests`
- `GET /v1/audit`
- `GET /v1/audit/{id}`

The server uses storage interfaces (`PackageStore`, `AuditStore`, `SimulationStore`, and combined `DecisionStore`) and ships with `MemoryDecisionStore` for examples, tests, and local development. `MemoryDecisionStore` is not durable. Production deployments can use `NewSQLiteDecisionStore` for a single-node durable store or inject another durable store that passes `StoreConformanceSuite`.

Hardening hooks are built in:

- `DecisionServerConfig` for request body limits, request timeouts, request IDs, and optional content-type enforcement
- `Principal`, `Permission`, `Authorizer`, and `WithDecisionServerAuthorizer` for endpoint RBAC
- `NewProductionDecisionServer` to reject unsafe production configuration such as memory storage, missing authorizer, missing request limits, or disabled content-type enforcement
- `NewAuthzAuthorizerFromFile` for `.authz` policy files backed by `github.com/oarkflow/authz`, plus `NewAuthzAuthorizer` for prebuilt authz engines
- `AuditEnvelope`, `AuditChainVerifier`, and `VerifyAuditChain` for tamper-evident audit continuity
- `DecisionMetrics`, `DecisionLogger`, and rate-limit hooks for observability and abuse control

See [docs/security-threat-model.md](docs/security-threat-model.md), [docs/production-checklist.md](docs/production-checklist.md), and [docs/production-readiness.md](docs/production-readiness.md). Run [examples/production_readiness_catalog](examples/production_readiness_catalog) for the compact production smoke test. The production server example includes a [Dockerfile](examples/production_decision_server/Dockerfile) and [Kubernetes manifest](examples/production_decision_server/k8s.yaml).

For a business-facing catalog of applicable domains and a cautious patent-candidate analysis of the platform mechanisms, see [docs/use-cases-and-patent-candidates.md](docs/use-cases-and-patent-candidates.md).

### Validation And Testing

Use `ValidateDecisionPackage` before publish and `RunDecisionPackageTests` for embedded `test` blocks. Schemas support required paths, type checks, enum values, numeric min/max, string length, array length, and nested property rules while preserving the lightweight `Required`/`Types` model. BCL scanner tokenization is allocation-free; `bcl.AppendPackage` can encode into a pre-sized caller buffer without allocating on its hot path. Parsing necessarily allocates the resulting package structs, maps, slices, and strings.

```sh
go test ./...
go test ./bcl -bench 'BenchmarkBCL|BenchmarkAppend'
```

## Bytecode Runtime

For the highest-performance path, compile rules into bytecode and evaluate typed facts. This avoids `map[string]any`, reflection, JSON parsing, and per-evaluation allocation.

```go
registry := condition.NewRegistry()

program, err := condition.CompileRuleSet(condition.RuleSet{
	Name: "workflow.approval",
	Rules: []condition.Rule{{
		ID:        "1001",
		Priority: 100,
		Condition: `amount > 100000 and department in ["finance", "procurement"]`,
		Decision:  "require_approval",
	}},
}, registry)

facts := condition.NewTypedFacts(registry.Field("department"))
facts.SetInt(registry.Field("amount"), 150000)
facts.SetString(registry.Field("department"), registry.Intern("finance"))

ctx := condition.EvalContext{
	Matched: make([]condition.RuleID, 0, 4),
	Actions: make([]condition.CompiledAction, 0, 4),
	Stack:   make([]bool, 0, 8),
}

runtime := condition.NewRuntime(program)
result := runtime.Evaluate(registry.Namespace("workflow.approval"), facts, &ctx)
```

`TypedFacts` is the zero-allocation hot path. When convenience matters at system boundaries, use `EvaluateAny` or the adapters:

```go
result, err := runtime.EvaluateAny(registry.Namespace("workflow.approval"), map[string]any{
	"amount":     150000,
	"department": "finance",
}, &ctx)
```

`EvaluateAny` supports maps, structs, slices/arrays of maps, and slices/arrays of structs. Nested slices use bracket paths in DSL, for example `orders[0].amount`; root slices are exposed under `items[0]`.

## DSL

Examples:

```txt
user.age >= 18
user.tier == "enterprise" and message.category == "otp"
recipient.country in ["NP", "IN", "US"]
route.user_id == null or route.user_id == user.id
regex_match(message.body, route.message_pattern)
minCount <= message.count and message.count <= maxCount
```

Supported operators:

```txt
== != < <= > >=
in not in
and or not
regex_match starts_with ends_with
```

`null` is useful for route-table style defaults:

```txt
route.carrier_code == null or route.carrier_code == recipient.carrier
```

## Facts

Use `MapFacts` for the lowest-overhead common path:

```go
facts := condition.MapFacts{
	"user": map[string]any{
		"id":   15,
		"tier": "enterprise",
	},
	"message": map[string]any{
		"category": "otp",
		"count":    500,
	},
}
```

Struct facts are supported:

```go
type User struct {
	ID   int    `json:"id"`
	Tier string `json:"tier"`
}

facts := condition.NewStructFacts(struct {
	User User `json:"user"`
}{
	User: User{ID: 15, Tier: "enterprise"},
})
```

You can also provide a custom getter:

```go
facts := condition.FactFunc(func(path string) (any, bool) {
	switch path {
	case "user.id":
		return 15, true
	case "message.category":
		return "otp", true
	default:
		return nil, false
	}
})
```

## Variables

Variables can be used on either side of a condition as ordinary SPL identifiers:

```go
expr := condition.MustCompile(
	`minCount <= message.count and message.count <= maxCount`,
	condition.WithVariables(condition.MapFacts{
		"minCount": 100,
		"maxCount": 1000,
	}),
)
```

Per-evaluation variables are also supported:

```go
result, err := expr.EvalWithVariables(ctx, facts, condition.MapFacts{
	"minCount": 10,
	"maxCount": 500,
})
```

Template-style placeholders are normalized before parsing, so rule strings loaded from config can use `{{name}}` or `${name}`:

```go
expr := condition.MustCompile(
	`{{minCount}} <= message.count and message.country == {{required.country}}`,
	condition.WithVariables(condition.MapFacts{
		"minCount": 100,
		"required": map[string]any{"country": "NP"},
	}),
)
```

For compile-time interpolation, pass values with `WithInterpolationMap`; values are rendered as DSL literals, which is useful for static thresholds and bytecode compilation:

```go
expr := condition.MustCompile(
	`message.count >= {{minCount}} and message.country in {{countries}}`,
	condition.WithInterpolationMap(map[string]any{
		"minCount": 100,
		"countries": []string{"NP", "US"},
	}),
)
```

## DSL Helpers

The DSL stays expression-based. Helpers are ordinary functions, so they compose with all existing operators:

```txt
user.tier == "vip" ? 10 : 1
coalesce(user.nickname, user.name, "guest") == "Sujit"
defaultValue(order.discount, 0) == 0
isNull(route.carrier_code)
ends_with(lower(trim(user.email)), "@example.com")
clamp(order.risk, 0, 1) < 0.75
between(order.total, 100, 500)
before(order.created_at, now())
betweenTime(route.valid_from, "2026-01-01T00:00:00Z", "2026-12-31T23:59:59Z")
age(user.created_at, "day") >= 30
```

Run the helper example:

```sh
go run ./examples/advanced_dsl
```

## Aggregations

Aggregation functions work on arrays, slices, maps, and slices of rows/maps/structs.

Numeric aggregations:

```txt
count(items)
sum(items)
avg(items)
min(items)
max(items)
sum(items, "amount")
avg(routes, "quality.success_rate")
```

Predicate aggregations:

```txt
any(items, "status", "==", "active")
all(items, "amount", ">", 0)
none(routes, "failure_rate", ">", 0.10)
countWhere(messages, "category", "==", "otp") >= 2
count(filter(messages, "country", "==", "NP")) >= 2
sum(filter(messages, "status", "==", "sent"), "cost") < 0.05
```

Grouped aggregations:

```txt
"NP" in groupBy(messages, "country")
groupCount(messages, "country", "NP") >= 2
groupSum(messages, "country", "NP", "cost") < 0.05
groupAvg(messages, "country", "NP", "success_rate") >= 0.98
groupMin(routes, "carrier", "NCELL", "cost_per_sms") <= 0.01
groupMax(routes, "country", "NP", "success_rate") > 0.99
groupCountWhere(messages, "country", "NP", "cost", "<", 0.01) >= 2
groupSumWhere(messages, "country", "NP", "cost", "<", 0.01, "cost") < 0.05
groupAvgWhere(messages, "country", "NP", "success_rate", ">", 0.95, "success_rate") >= 0.98
distinctCount(messages, "country") == 2
```

Collection helpers:

```txt
pluck(messages, "country")
distinct(messages, "country")
first(messages)
last(messages)
top(routes, "quality.success_rate")
bottom(routes, "cost_per_sms")
percentile(messages, "delivery_ms", 95) < 5000
take(sortBy(routes, "quality.success_rate", "desc"), 3)
slice(reverse(messages), 0, 10)
```

Two-argument predicate forms check truthiness or equality:

```txt
any(users, "active")
any(routes, "country", "NP")
```

Example:

```go
expr := condition.MustCompile(`
	count(messages) >= 3 and
	avg(messages, "success_rate") >= 0.98 and
	sum(messages, "cost") < 0.05 and
	any(messages, "country", "==", "NP") and
	groupCount(messages, "country", "NP") >= 2
`)
```

## Builder API

```go
builder := condition.
	When("user.age").Gte(18).
	And("country").In("US", "CA", "NP").
	And("plan").Ne("free")

expr, err := builder.Compile()
```

Variables can be placed on the left-hand side:

```go
builder := condition.Var("minAge").LtePath("user.age")
```

Builder fragments can also be grouped and combined:

```go
eligibility := condition.
	When("user.age").Between(18, 65).
	And("country").InPath("allowed.countries").
	And("user.email").Exists()

override := condition.
	Expr(`account.status == "trusted"`).
	AndVar("manualOverride").Eq(true)

expr, err := eligibility.OrExpr(override).Compile(
	condition.WithVariables(condition.MapFacts{"manualOverride": false}),
)
```

## Rule Engine

Use `RuleSet` when you need decisions, scores, actions, events, groups, or workflows.

```go
engine := condition.NewEngine()

err := engine.AddRuleSet(condition.RuleSet{
	Name: "checkout",
	Rules: []condition.Rule{
		{
			ID:        "vip-approval",
			Priority: 100,
			Condition: `user.tier == "vip" and order.total >= 100`,
			Decision:  "approve",
			Score:     10,
			Actions: []condition.Action{
				{Type: "notify", Payload: map[string]any{"channel": "risk"}},
			},
		},
	},
})
```

Evaluate:

```go
result, err := engine.Evaluate(ctx, facts, "checkout")
```

Classic engine evaluation records matched rule IDs, honors `ValidFrom`/`ValidUntil`, sorts by `Priority` then `Salience`, and applies allow/deny decisions or actions to `Result.Allowed`. Non-zero `ExecutionMode` values are honored for top-level rules and workflow stages: `HighestPriority`, `DenyOverrides`, `AllowOverrides`, and `ScoreBased`. The zero value is kept as the backward-compatible default `AllMatches`; use `StopOnMatch` for first-match behavior in JSON/YAML rulesets.

## Generic Candidate Ranking

Use `Ranker` when you need to choose the best candidate from many matching rows.

Examples:

- Best SMS route
- Best payment gateway
- Best warehouse
- Best shipping carrier
- Best model/provider
- Best feature variant

Each candidate is just a row of facts:

```go
candidate := condition.Candidate{
	ID:   "route-1",
	Name: "Provider A",
	Facts: condition.MapFacts{
		"route": map[string]any{
			"user_id":           15,
			"message_category":  "otp",
			"recipient_country": "NP",
			"carrier_code":      "NCELL",
			"priority":          100,
			"specificity":       38,
			"fallback_order":    1,
			"status":            "active",
		},
		"provider": map[string]any{
			"name":   "Provider A",
			"status": "active",
		},
		"quality": map[string]any{
			"success_rate":         0.991,
			"avg_delivery_seconds": 2.0,
			"failure_rate":         0.004,
			"cost_per_sms":         0.010,
		},
	},
}
```

Rules decide eligibility:

```go
rules := condition.RuleSet{
	Name: "routing",
	Rules: []condition.Rule{
		{ID: "route-active", Condition: `active(route.status)`},
		{ID: "provider-active", Condition: `active(provider.status)`},
		{ID: "valid-window", Condition: `validNow(route.valid_from, route.valid_to)`},
		{ID: "user", Condition: `nullableMatch(route.user_id, user.id)`},
		{ID: "country", Condition: `nullableMatch(route.recipient_country, recipient.country)`},
		{ID: "count", Condition: `rangeMatch(route.min_message_count, route.max_message_count, message.count)`},
	},
	ScoreRules: []condition.ScoreRule{
		{ID: "quality", Metric: "quality.success_rate", Weight: 0.70, Direction: condition.HigherBetter, Normalize: condition.Normalize{Min: 0.90, Max: 1}},
		{ID: "cost", Metric: "quality.cost_per_sms", Weight: 0.30, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 0.003, Max: 0.03}},
	},
}
```

Create a ranker:

```go
ranker, err := condition.NewRanker(
	rules,
	condition.WithRoutingTieBreakerPaths(
		"route.priority",
		"route.specificity",
		"route.fallback_order",
		"quality.cost_per_sms",
	),
)
```

Nativeest selection path:

```go
selected, ok, err := ranker.SelectBestID(ctx, condition.RankingRequest{
	Facts:      requestFacts,
	Candidates: candidates,
})
```

Full explanation path:

```go
result, err := ranker.Rank(ctx, condition.RankingRequest{
	Facts:      requestFacts,
	Candidates: candidates,
})
```

Use `SelectBestID` in hot production paths. Use `Rank` for debugging, audit, admin UI, or explainability.

Fallback chain and stable weighted selection:

```go
fallbacks, err := ranker.SelectFallbacks(ctx, request, 3)

weightedRanker, err := condition.NewRanker(
	rules,
	condition.WithWeightedSelection("route.weight", "recipient.number"),
)
selected, ok, err := weightedRanker.SelectWeightedID(ctx, request)
```

## Route-Table Pattern

The recommended routing-table pattern is:

```sql
sms_routes
----------
user_id              nullable
message_category     nullable
message_type         nullable
delivery_quality     nullable
recipient_country    nullable
carrier_code         nullable
min_message_count    nullable
max_message_count    nullable
message_pattern      nullable
sender_id            nullable
tenant_id            nullable
priority
fallback_order
status
valid_from
valid_to
weight
```

`NULL` means “match anything.” In DSL:

```txt
nullableMatch(route.user_id, user.id)
nullableMatch(route.message_category, message.category)
nullableMatch(route.recipient_country, recipient.country)
nullableMatch(route.carrier_code, recipient.carrier)
rangeMatch(route.min_message_count, route.max_message_count, message.count)
active(route.status)
validNow(route.valid_from, route.valid_to)
```

Order candidates by:

```txt
priority desc
specificity desc
quality score desc
cost asc
fallback_order asc
```

The SMS example implements this pattern:

```sh
go run ./examples/sms_routing
```

## TCPGuard

TCPGuard has been moved into its own nested Go module under `./tcpguard`. The root `condition` module does not import TCPGuard. Run TCPGuard examples, CLI commands, and TCPGuard BCL parsing from that module:

```sh
cd tcpguard
go run ./examples/tcpguard_banking_protection_pack
go run ./examples/tcpguard_fiber_server
go run ./cmd/tcpguard validate -dir ./examples/tcpguard_multi_file_policy_pack
```

See [`tcpguard/README.md`](./tcpguard/README.md) for TCPGuard middleware, BCL, policy packs, and runtime operations.

## Performance

The library has two styles of API:

- Hot path APIs avoid allocations and return compact results.
- Explanation APIs return reasons, traces, and candidate lists, so they allocate by design.

Recommended hot paths:

```go
matched, err := expr.EvalBool(ctx, facts)
selected, ok, err := ranker.SelectBestID(ctx, request)
```

Representative local benchmark results:

```txt
BenchmarkBytecodeSingleComparison          ~9 ns/op      0 B/op    0 allocs/op
BenchmarkBytecodeBooleanExpression         ~27 ns/op     0 B/op    0 allocs/op
BenchmarkBytecodeProgram25Rules            ~880 ns/op    0 B/op    0 allocs/op
BenchmarkBytecodeProgram1000RulesIndexed   ~126 ns/op    0 B/op    0 allocs/op
BenchmarkEvalBoolMapFacts                 ~132 ns/op    0 B/op    0 allocs/op
BenchmarkEvalWithVariables                ~161 ns/op    0 B/op    0 allocs/op
BenchmarkRankerSelectBestID25Candidates   ~9.8 us/op    0 B/op    0 allocs/op
```

Run benchmarks:

```sh
go test -bench=. -benchmem -run '^$'
```

## Examples

```sh
go run ./examples/simple
go run ./examples/aggregations
go run ./examples/advanced_dsl
go run ./examples/bytecode_runtime
go run ./examples/builder
go run ./examples/placeholders
go run ./examples/struct_facts
go run ./examples/ruleset
go run ./examples/sms_routing
```

## Testing

```sh
go test ./...
go test -race ./...
```

## Design Notes

- Compile once, evaluate many times.
- Keep conditions domain-neutral.
- Model domain data as facts.
- For routing, model rows as candidates.
- Use nullable candidate dimensions for defaults and fallbacks.
- Use `SelectBestID` for the lowest-overhead production selector.
- Use `Rank` when you need explanations.
