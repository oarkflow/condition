# Decision Intelligence Example

This example demonstrates the package-based platform API:

```sh
go run ./examples/decision_intelligence
```

It builds a `DecisionPackage` named `fraud-intelligence` and evaluates the `fraud-review` decision.

## Requirement Coverage

| Requirement | Covered by |
| --- | --- |
| BCL block language | `examples/bcl_decision_platform/fraud.bcl` |
| Decision package versioning | `DecisionPackage{Name, Version, Environment}` and package digest |
| Schemas / validation | `Schemas["fraud-review"]` with required facts and type checks |
| Policies | `Policies` with `deny`, `require_review`, default `allow` |
| Policy effect lattice | `deny > require_review > escalate > allow > custom > abstain` |
| Rules | `RuleSets["fraud-review"]` with conditions, scores, actions, reasons |
| Scoring | rule scores plus ranking score contribution |
| Ranking | `Rankings["fraud-review"]` over review queues |
| Optimization hook | `Optimizations["queue-optimization"]` reuses the ranking objective |
| Workflow | `Workflows["fraud-review-workflow"]` records escalation path evidence |
| Actions | `ActionDefinition` plus runtime `notify` action |
| Governance | `Governance` maker/checker/approver metadata |
| Tests | embedded `DecisionTestCase` metadata |
| Explainability | `Explanation` evidence graph, matched/failed rules, score deltas, facts |
| Counterfactuals | failed conditions produce lightweight change hints |
| Audit | request/result fingerprints, package digest, timestamps, trace summary |
| Simulation | baseline vs candidate package comparison |
| Server APIs | `DecisionServer` in `examples/bcl_decision_platform` |
| Storage interfaces | `PackageStore`, `AuditStore`, `SimulationStore`, and `MemoryDecisionStore` |

## Package DSL Shape

The canonical platform DSL is BCL. The same package can also be represented as Go structs, JSON, or YAML. Conditions inside rules and policies use the existing expression DSL.

```yaml
name: fraud-intelligence
version: "1"
environment: production
schemas:
  fraud-review:
    required:
      - customer.blacklisted
      - transaction.amount
      - device.is_new
    types:
      transaction.amount: number
      device.is_new: bool
policies:
  - name: fraud-review
    default_effect: allow
    rules:
      - id: deny-blacklisted
        effect: deny
        condition: customer.blacklisted == true
        reason: customer is blacklisted
      - id: review-high-risk-market
        effect: require_review
        condition: customer.country == "NP"
        reason: manual review market
rule_sets:
  - name: fraud-review
    rules:
      - id: "1001"
        condition: (transaction.amount > 100000) and ((customer.country == "NP") or (customer.tier == "vip")) and (not (customer.blacklisted == true))
        decision: require_review
        score: 40
        reason: high value transaction
        actions:
          - type: notify
            payload:
              team: compliance
rankings:
  - name: fraud-review
    selection: best
    priority_path: provider.priority
    specificity_path: provider.sla
    cost_path: provider.load
    rule_set:
      name: review-queue-ranking
      rules:
        - id: active
          condition: provider.active == true
      score_rules:
        - id: priority
          metric: provider.priority
          weight: 0.7
          normalize:
            min: 1
            max: 10
optimizations:
  - name: queue-optimization
    decision: fraud-review
    goal: maximize reviewer readiness while minimizing queue load
    ranking: fraud-review
    selection: best
```

Use `LoadDecisionPackage` with `bcl.Decoder()`, `JSONDecoder[DecisionPackage]()`, or the `yamlx.JSONTagDecoder[DecisionPackage]()` helper to load packages from files or HTTP sources. Use `bcl.LoadPackageFile` when the BCL package uses relative `import` statements.

BCL `when` blocks can use raw expression DSL or nested condition sets. Multiple condition lines are joined with implicit `and`; `all`, `any`, and `not` blocks compile to expression DSL conjunctions:

```txt
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
```

## Production Hardening

The embeddable server now exposes production hooks without forcing database dependencies into the core module:

- Durable storage contract: implement `DecisionStore` and run `StoreConformanceSuite` against it.
- Development store: `MemoryDecisionStore` is concurrency-safe but non-durable.
- Audit integrity: evaluation writes `AuditEnvelope` records with sequence, previous hash, payload hash, and chain hash; verify exports with `VerifyAuditChain`.
- RBAC: configure `WithDecisionServerAuthorizer`; requests without a `Principal` are denied when an authorizer is installed.
- HTTP hardening: `DecisionServerConfig` controls body limits, request timeout, request IDs, and content-type enforcement.
- Observability: plug in `DecisionMetrics`, `DecisionLogger`, and a rate-limit hook.
- Deployment examples: see `examples/bcl_decision_platform/Dockerfile` and `examples/bcl_decision_platform/k8s.yaml`.

Production deployments must still provide a durable external store and tenant-aware auth middleware. See `docs/security-threat-model.md` and `docs/production-checklist.md`.
