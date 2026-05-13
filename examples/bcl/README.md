# BCL Examples

This directory contains standalone BCL decision packages that demonstrate the canonical block language:

- `packages/loan_eligibility.bcl`: constants, vars, rich schemas, policy lattice, scoring, events, actions, governance, and embedded tests.
- `packages/support_priority.bcl`: dataset-backed support queue ranking.
- `packages/procurement_workflow.bcl`: workflow stages with assignment, SLA, timeout intent, approval, rejection, and escalation.
- `packages/ai_guardrails.bcl`: LLM safety policy with imported constants.
- `packages/telecom_optimization.bcl`: weighted SMS provider ranking with reusable provider datasets.
- `packages/shared_ai_constants.bcl`: importable constants and metadata.

Run every package through parse, validation, digesting, and embedded package tests:

```sh
go run ./examples/bcl
```

Each `when { ... }` block uses the expression DSL. Multiple condition lines are joined with implicit `and`; nested `all`, `any`, and `not` blocks compile into deterministic expression conditions.
