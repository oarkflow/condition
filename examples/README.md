# Runnable Examples

Run these from the repository root:

```sh
go run ./examples/simple
go run ./examples/aggregations
go run ./examples/advanced_dsl
go run ./examples/bcl
go run ./examples/bcl_decision_platform
go run ./examples/bytecode_runtime
go run ./examples/builder
go run ./examples/complete_condition
go run ./examples/decision_intelligence
go run ./examples/email_routing
go run ./examples/feature_flags
go run ./examples/fraud_review
go run ./examples/live_reload_decision_runtime
go run ./examples/payment_routing
go run ./examples/placeholders
go run ./examples/production_readiness_catalog
go run ./examples/routing_file_backed_ai_moderation
go run ./examples/routing_file_backed_claims
go run ./examples/routing_file_backed_email
go run ./examples/routing_file_backed_healthcare_admin
go run ./examples/routing_file_backed_payment
go run ./examples/routing_file_backed_permits
go run ./examples/routing_file_backed_saas_offers
go run ./examples/routing_file_backed_shipping
go run ./examples/routing_file_backed_sms
go run ./examples/routing_file_backed_support
go run ./examples/routing_file_backed_vendor_procurement
go run ./examples/shipping_routing
go run ./examples/struct_facts
go run ./examples/ruleset
go run ./examples/sms_routing
go run ./examples/support_routing
go run ./examples/usecase_ai_safety_guardrails
go run ./examples/usecase_banking_fintech
go run ./examples/usecase_ecommerce_marketplaces
go run ./examples/usecase_government_public_sector
go run ./examples/usecase_healthcare_administration
go run ./examples/usecase_hr_procurement_legal
go run ./examples/usecase_insurance
go run ./examples/usecase_saas_product_platforms
go run ./examples/usecase_support_operations
go run ./examples/usecase_telecom_messaging
```

The examples cover direct DSL evaluation, advanced DSL helpers, typed bytecode runtime usage, aggregations and grouped aggregations, fluent builder usage, placeholders/variables, struct-backed facts, rule sets with groups/workflows/actions, production-style routing, package-based decision intelligence, and business use-case packages. `complete_condition` is the end-to-end marketplace checkout example that combines evaluation, filters, nested conditions, ranking, fallbacks, and variables.

The `usecase_*` examples are file-backed: each directory keeps the decision package in `package.yaml`, the request facts in `request.json`, and a complete `main.go` that loads, validates, publishes, evaluates, and prints the result.

The `routing_file_backed_*` examples are a routing-focused file-backed catalog. Each directory keeps candidates, policies, rankings, scoring, tie-breakers, and optimization metadata in `package.yaml`; request facts and variables live in `request.json`; the runner prints the decision winner, fallback chain, weighted selection when configured, and the full JSON response.

`production_decision_server` is a long-running server profile rather than a short demo. Start it with explicit durable storage and an admin token:

```sh
DECISION_ADMIN_TOKEN=dev-admin-token DECISION_SQLITE_PATH=/tmp/condition-decisions.db DECISION_AUTHZ_FILE=examples/production_decision_server/production.authz go run ./examples/production_decision_server
```

`production_readiness_catalog` is the runnable smoke test for the production primitives. It loads `package.yaml` for decision logic, `request.json` for the SMS routing payload, `route_candidates.json` for the JSON candidate dataset, and `access.authz` for authorization; then it validates package tests, publishes to SQLite, evaluates through the production HTTP server path, verifies the audit chain, and reopens the SQLite store.

Use-case index:

- `bcl`: standalone BCL packages for loan eligibility, support priority, procurement workflow, AI guardrails, telecom optimization, imports, datasets, rich schemas, events, and workflow assignment.
- `bcl_decision_platform`: load a block-language `.bcl` decision package, publish it to the HTTP server surface, evaluate file-backed requests, simulate, and run embedded package tests.
- `production_readiness_catalog`: runnable production-readiness catalog for SQLite durability, `.authz` authorization, production validation, audit verification, JSON request payloads, JSON datasets, and dataset-backed SMS provider routing.
- `production_decision_server`: single-node production profile with SQLite durability, strict server config, `.authz` authorization, health/readiness checks, graceful shutdown, and audit chain verification.
- `live_reload_decision_runtime`: watch file-backed BCL packages, reject invalid edits, retain the last known-good runtime, and publish valid changes in realtime.
- `email_routing`: choose SES, SendGrid, Mailgun, or Postmark-style providers by tenant, region, suppression status, provider health, deliverability, latency, and cost.
- `decision_intelligence`: file-backed package orchestration with BCL/JSON/YAML packages, JSON/YAML requests, policies, rules, ranking, workflow evidence, audit fingerprints, tests, and simulation diffs.
- `payment_routing`: choose processors by currency, country, card brand, 3DS support, risk score, approval rate, fees, and settlement speed.
- `shipping_routing`: choose carriers by country, postal blocks, package handling, delivery SLA, on-time rate, and shipping cost.
- `feature_flags`: evaluate rollout eligibility with allow/deny lists, account tier, region, stable buckets, and nested groups.
- `fraud_review`: approve, decline, or queue orders using risk score, velocity, cart risk, account age, and device reputation.
- `support_routing`: rank support queues by language, skill, customer tier, queue load, availability, and SLA urgency.
- `sms_routing`: deep SMS route-table ranking with fallback chains and stable weighted selection.
- `routing_file_backed_sms`: file-backed SMS route-table ranking with policies, fallbacks, and weighted selection.
- `routing_file_backed_email`: file-backed email provider routing.
- `routing_file_backed_shipping`: file-backed shipping carrier routing.
- `routing_file_backed_payment`: file-backed payment processor routing.
- `routing_file_backed_support`: file-backed support queue routing.
- `routing_file_backed_claims`: file-backed insurance claims team routing.
- `routing_file_backed_permits`: file-backed permit queue routing.
- `routing_file_backed_healthcare_admin`: file-backed administrative healthcare queue routing.
- `routing_file_backed_vendor_procurement`: file-backed procurement and legal review routing.
- `routing_file_backed_ai_moderation`: file-backed AI moderation queue routing.
- `routing_file_backed_saas_offers`: file-backed SaaS offer routing.
- `usecase_banking_fintech`: fraud transaction policy, review-queue dataset, ranking, and optimization.
- `usecase_insurance`: claim review policy with claims-team routing.
- `usecase_government_public_sector`: permit eligibility and public-sector queue routing.
- `usecase_telecom_messaging`: campaign compliance with SMS provider routing.
- `usecase_support_operations`: support triage policy with queue ranking.
- `usecase_ecommerce_marketplaces`: checkout risk policy with shipping carrier selection.
- `usecase_healthcare_administration`: administrative prior-authorization routing without clinical recommendations.
- `usecase_ai_safety_guardrails`: AI guardrail policy with moderation queue routing.
- `usecase_hr_procurement_legal`: purchase request policy with procurement/legal review routing.
- `usecase_saas_product_platforms`: feature access policy with upgrade-offer selection.

Decision intelligence requirement map:

| Requirement | Example |
| --- | --- |
| Package DSL/config | `bcl`, `bcl_decision_platform`, `decision_intelligence` |
| Policies and effect lattice | `decision_intelligence` |
| Explainability and audit fingerprints | `decision_intelligence` |
| Simulation and package comparison | `bcl_decision_platform`, `decision_intelligence` |
| File-backed realtime refresh | `live_reload_decision_runtime`, `decision_intelligence` |
| HTTP server surface and memory stores | `bcl_decision_platform` |
| Durable single-node production server | `production_decision_server` |
| Production readiness primitives | `production_readiness_catalog` |
| JSON request payloads, JSON datasets, and SQL sources | `production_readiness_catalog`, `load_sources` |
| Workflow pathing | `decision_intelligence`, `ruleset` |
| Ranking and deterministic winner selection | `decision_intelligence`, `sms_routing`, `email_routing`, `support_routing` |
| Weighted routing / optimization-style selection | `sms_routing`, `decision_intelligence`, `routing_file_backed_sms`, `routing_file_backed_email`, `routing_file_backed_shipping`, `routing_file_backed_payment` |
| DSL helpers and temporal/string/numeric functions | `advanced_dsl` |
| Aggregations and collection scoring | `aggregations` |
| Bytecode runtime | `bytecode_runtime` |
| Dynamic loading from sources | `load_sources` |
| Business use-case catalog | `usecase_banking_fintech`, `usecase_insurance`, `usecase_government_public_sector`, `usecase_telecom_messaging`, `usecase_support_operations`, `usecase_ecommerce_marketplaces`, `usecase_healthcare_administration`, `usecase_ai_safety_guardrails`, `usecase_hr_procurement_legal`, `usecase_saas_product_platforms` |
| File-backed routing catalog | `routing_file_backed_sms`, `routing_file_backed_email`, `routing_file_backed_shipping`, `routing_file_backed_payment`, `routing_file_backed_support`, `routing_file_backed_claims`, `routing_file_backed_permits`, `routing_file_backed_healthcare_admin`, `routing_file_backed_vendor_procurement`, `routing_file_backed_ai_moderation`, `routing_file_backed_saas_offers` |

For the lowest-latency hot path, compile once and evaluate with `TypedFacts` plus the bytecode runtime; `MapFacts` plus `EvalBool` remains the ergonomic zero-allocation expression path.
