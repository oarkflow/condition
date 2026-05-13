# Use Cases And Patent-Candidate Mechanisms

`condition` is an embeddable Go decision intelligence platform. It can be used as a fast condition evaluator, a rule engine, a ranking system, a policy platform, a workflow decision layer, or a full package-based decision runtime with BCL authoring, simulation, explainability, audit fingerprints, and a production-oriented HTTP server surface.

This document is both a business use-case catalog and a technical differentiation memo. It is not legal advice. Patentability depends on novelty, non-obviousness, claim drafting, jurisdiction, public disclosure timing, and prior art. Treat the invention notes below as patent-candidate material for review by a qualified patent attorney.

## Executive Overview

Most businesses repeatedly answer questions like:

- Is this customer eligible?
- Should this transaction be allowed, denied, reviewed, or escalated?
- Which provider, queue, offer, carrier, or workflow path should be selected?
- Why did the system make this decision?
- What changed between policy versions?
- Can we replay and audit the exact result later?

`condition` addresses these as one deterministic flow:

```txt
Context + package version -> policies + rules + scoring + ranking + workflow -> explainable decision + audit record
```

The project can run in several deployment shapes:

- Embedded SDK inside Go services for low-latency local decisions.
- BCL-authored decision packages loaded from files, object storage, or custom sources.
- HTTP decision server with package publish, evaluate, simulate, test, compare, and audit endpoints.
- Batch simulation or replay engine for historical records.
- On-premise or air-gapped binary when the host application provides durable storage and authentication middleware.

## Capability Map

| Capability | What it solves | Project feature |
| --- | --- | --- |
| Boolean conditions | Fast yes/no checks over request facts | Expression compiler, `EvalBool`, bytecode runtime |
| Business rules | Deterministic decisions, actions, events, scores | `Rule`, `RuleSet`, groups, priorities, execution modes |
| Policy effects | Allow, deny, review, escalate, abstain, and custom effects | `Policy`, `PolicyRule`, policy effect lattice |
| Scoring | Fraud score, priority score, quality score, risk score | Rule score deltas and ranking score rules |
| Ranking | Choose best candidate from providers, queues, offers, vendors | `Ranker`, `Ranking`, deterministic tie-breakers |
| Optimization | Select best option against objective metadata and constraints | `Optimization` with ranking-backed selection |
| Workflows | Human review, approval, escalation, assignment metadata | `Workflow`, stages, assignment, SLA metadata, workflow evidence |
| Package authoring | Business-readable decision configuration | BCL package DSL, JSON/YAML/Go struct equivalents |
| Versioning | Immutable decision package identity and rollback | `DecisionPackage` name, version, environment, digest |
| Explainability | Matched/failed rules, fact evidence, score deltas, pathing | `Explanation`, evidence graph nodes, counterfactual hints |
| Simulation | Compare behavior before publishing | `Simulate`, `ComparePackages`, embedded package tests |
| Audit | Replay support, request/result fingerprints, tamper-evident chain | `AuditRecord`, `AuditEnvelope`, audit verifier |
| Production API | Remote publish/evaluate/test/audit flows | `DecisionServer` over `net/http` |
| Storage | Database-neutral persistence boundary | `PackageStore`, `AuditStore`, `SimulationStore`, `DecisionStore` |
| Governance hooks | Maker/checker metadata and endpoint authorization | Governance metadata, `Authorizer`, permissions |
| Observability hooks | Metrics, logs, tracing, abuse controls | `DecisionMetrics`, `DecisionLogger`, rate-limit hook |

## Business Use Cases

The BCL examples below are illustrative documentation examples. Runnable examples are linked in [Existing Examples To Review](#existing-examples-to-review).

### Banking And Fintech

Banking systems need deterministic, explainable decisions for regulated workflows.

Useful requirements:

- Real-time fraud review for card, wallet, ACH, and wire transactions.
- AML flags based on amount, country, velocity, sanctions, and customer risk.
- KYC completeness checks and customer onboarding eligibility.
- Credit approval and manual-review routing.
- Transaction limits by customer tier, country, account age, and device risk.
- Payment processor routing by approval rate, cost, currency, country, card brand, and 3DS capability.
- Hold, freeze, notify, or escalate actions with audit evidence.

Why `condition` fits:

- Policies can produce `deny`, `require_review`, or `escalate`.
- Rule scores can accumulate risk.
- Ranking can select a review queue or payment processor.
- Audit fingerprints and package digests support replay and compliance investigation.
- Simulation can compare new fraud thresholds against historical transactions.

Representative BCL:

```txt
module "fraud-review" {
  version = "1"
  environment = "production"

  dataset "review-queues" {
    record "aml" { provider.active = true provider.skills = ["payments", "aml"] provider.priority = 10 provider.sla = 1 provider.load = 0.42 provider.fallback_order = 1 }
    record "card-risk" { provider.active = true provider.skills = ["cards", "payments"] provider.priority = 8 provider.sla = 2 provider.load = 0.35 provider.fallback_order = 2 }
    record "standard-review" { provider.active = true provider.skills = ["payments"] provider.priority = 5 provider.sla = 4 provider.load = 0.20 provider.fallback_order = 3 }
  }

  policy "transaction" {
    default allow
    deny "blacklisted" when { customer.blacklisted == true } reason "customer is blacklisted"
    require_review "high-risk-market" when { customer.country in ["NP", "IR"] } reason "manual review market"
    escalate "sanctions-hit" when { customer.sanctions_match == true } reason "sanctions match requires escalation"
  }

  rule_set "transaction" {
    rule "high-value" {
      when {
        transaction.amount > 100000
        device.is_new == true
      }
      then {
        decision = "manual_review"
        score += 50
        action notify { team = "compliance" }
      }
      reason "high value transaction from new device"
    }
  }

  ranking "review-queue" {
    dataset = "review-queues"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.sla"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.load"
    rule "active" { when { provider.active == true } }
    rule "skill" { when { "payments" in provider.skills } }
    score "priority" { metric = "provider.priority" weight = 0.5 normalize 1 10 }
    score "sla" { metric = "provider.sla" weight = 0.2 direction = "lower_better" normalize 1 8 }
    score "load" { metric = "provider.load" weight = 0.3 direction = "lower_better" normalize 0 1 }
  }

  optimize "fraud-review-routing" {
    decision = "transaction"
    ranking = "review-queue"
    selection best
    goal = "route risky payments to the most qualified available queue"
  }
}
```

### Insurance

Insurance decisions combine eligibility, risk, scoring, document checks, and human review.

Useful requirements:

- Claim fraud scoring based on claim amount, prior claims, location, repair provider, and policy status.
- Underwriting eligibility by applicant profile, asset type, coverage request, and exclusions.
- Renewal pricing bands using deterministic risk rules and score deltas.
- Document routing to claims adjusters or specialist teams.
- Escalation when policy terms, claim type, or reserve amount exceed thresholds.

Why `condition` fits:

- Rich schemas validate required claim facts.
- Policies enforce hard exclusions.
- Scores explain why a claim moved to special investigation.
- Workflow evidence records assignment and escalation path.

Representative BCL:

```txt
module "claim-review" {
  version = "2026.05"

  dataset "claims-teams" {
    record "siu-auto" { provider.active = true provider.lines = ["auto"] provider.skills = ["fraud", "repair"] provider.priority = 10 provider.capacity = 0.74 provider.load = 0.48 provider.fallback_order = 1 }
    record "property-large-loss" { provider.active = true provider.lines = ["property"] provider.skills = ["large_loss", "fraud"] provider.priority = 8 provider.capacity = 0.62 provider.load = 0.56 provider.fallback_order = 2 }
    record "standard-adjusters" { provider.active = true provider.lines = ["auto", "property"] provider.skills = ["standard"] provider.priority = 5 provider.capacity = 0.90 provider.load = 0.31 provider.fallback_order = 3 }
  }

  policy "claim" {
    default allow
    deny "inactive-policy" when { policy.active == false } reason "policy is inactive"
    require_review "high-prior-claims" when { claimant.prior_claims_12m >= 3 } reason "claimant has recent claim velocity"
    escalate "large-reserve" when { claim.reserve_amount > 250000 } reason "large reserve requires senior review"
  }

  ranking "claim-team" {
    dataset = "claims-teams"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.capacity"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.load"
    rule "active" { when { provider.active == true } }
    rule "line" { when { claim.line in provider.lines } }
    rule "fraud-skill" { when { claim.fraud_score < 60 or "fraud" in provider.skills } }
    score "capacity" { metric = "provider.capacity" weight = 0.4 normalize 0 1 }
    score "priority" { metric = "provider.priority" weight = 0.35 normalize 1 10 }
    score "load" { metric = "provider.load" weight = 0.25 direction = "lower_better" normalize 0 1 }
  }

  optimize "claim-routing" {
    decision = "claim"
    ranking = "claim-team"
    selection best
    goal = "send claims to an eligible team with capacity and fraud expertise"
  }
}
```

### Government And Public Sector

Government systems often need traceable eligibility and policy automation.

Useful requirements:

- Benefit eligibility by income, residency, age, family size, disability status, and document completeness.
- Permit approval routing by applicant type, zone, risk class, and missing documents.
- Tax exemption qualification.
- Grant scoring and ranking by criteria, priority groups, and available budget.
- Citizen service routing to the correct department or queue.

Why `condition` fits:

- BCL gives non-application teams a reviewable package format.
- Package version and digest identify the exact policy in force.
- Counterfactual hints can show what data or threshold would change an outcome.
- Embedded tests can verify statutory examples before publishing.

Representative BCL:

```txt
module "permit-eligibility" {
  version = "2026.05"

  dataset "permit-queues" {
    record "residential-fast-track" { provider.active = true provider.zones = ["R1", "R2"] provider.skills = ["residential"] provider.priority = 9 provider.review_days = 2 provider.load = 0.40 provider.fallback_order = 1 }
    record "zoning-board" { provider.active = true provider.zones = ["R1", "R2", "C1"] provider.skills = ["variance", "zoning"] provider.priority = 8 provider.review_days = 7 provider.load = 0.55 provider.fallback_order = 2 }
    record "general-permits" { provider.active = true provider.zones = ["R1", "R2", "C1", "I1"] provider.skills = ["standard"] provider.priority = 5 provider.review_days = 10 provider.load = 0.30 provider.fallback_order = 3 }
  }

  schema "permit" {
    field applicant.age { required = true type = "number" min = 18 }
    field property.zone { required = true type = "string" }
  }

  policy "permit" {
    default deny
    allow "residential-low-risk" when {
      applicant.age >= 18
      property.zone in ["R1", "R2"]
      documents.complete == true
    } reason "meets residential permit criteria"
    require_review "missing-documents" when { documents.complete == false } reason "missing required documents"
    escalate "variance-needed" when { property.requires_variance == true } reason "variance requires board review"
  }

  ranking "permit-queue" {
    dataset = "permit-queues"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.review_days"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.load"
    rule "active" { when { provider.active == true } }
    rule "zone" { when { property.zone in provider.zones } }
    rule "variance" { when { property.requires_variance == false or "variance" in provider.skills } }
    score "priority" { metric = "provider.priority" weight = 0.45 normalize 1 10 }
    score "review-time" { metric = "provider.review_days" weight = 0.35 direction = "lower_better" normalize 1 14 }
    score "load" { metric = "provider.load" weight = 0.20 direction = "lower_better" normalize 0 1 }
  }

  optimize "permit-routing" {
    decision = "permit"
    ranking = "permit-queue"
    selection best
    goal = "route eligible permit applications to the fastest qualified public queue"
  }
}
```

### Telecom And Messaging

Messaging platforms must route traffic across providers while balancing cost, delivery, compliance, and reliability.

Useful requirements:

- SMS provider selection by country, sender type, route availability, delivery quality, latency, and cost.
- Email provider routing by tenant, region, suppression status, deliverability, latency, and price.
- Campaign compliance checks for opt-in, quiet hours, country restrictions, and content category.
- Stable weighted routing during provider migrations.
- Fallback when primary providers are unhealthy.

Why `condition` fits:

- Candidate ranking handles provider choice.
- `stableBucket` and deterministic tie-breakers support reproducible weighted routing.
- Optimization metadata explains why a provider was selected.
- Rules can emit provider failover events.

Representative BCL:

```txt
module "telecom-routing" {
  version = "1"

  dataset "sms-providers" {
    record "primary" { provider.active = true provider.countries = ["US", "NP"] provider.sender_types = ["transactional"] provider.priority = 10 provider.cost = 0.7 provider.quality = 0.98 provider.latency_ms = 180 provider.fallback_order = 1 }
    record "backup" { provider.active = true provider.countries = ["US", "NP", "GB"] provider.sender_types = ["transactional", "marketing"] provider.priority = 7 provider.cost = 0.5 provider.quality = 0.91 provider.latency_ms = 260 provider.fallback_order = 2 }
    record "compliance-route" { provider.active = true provider.countries = ["NP"] provider.sender_types = ["transactional"] provider.priority = 8 provider.cost = 0.9 provider.quality = 0.95 provider.latency_ms = 220 provider.fallback_order = 3 }
  }

  policy "campaign" {
    default allow
    deny "no-opt-in" when { recipient.opted_in == false } reason "recipient has not opted in"
    require_review "quiet-hours" when { campaign.local_hour < 8 or campaign.local_hour > 20 } reason "campaign falls outside normal sending hours"
    deny "blocked-country" when { recipient.country in ["IR", "KP"] } reason "country is blocked for this campaign"
  }

  ranking "sms" {
    dataset = "sms-providers"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.latency_ms"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.cost"
    rule "active" { when { provider.active == true } }
    rule "country" { when { recipient.country in provider.countries } }
    rule "sender-type" { when { campaign.sender_type in provider.sender_types } }
    score "quality" { metric = "provider.quality" weight = 0.5 normalize 0 1 }
    score "latency" { metric = "provider.latency_ms" weight = 0.2 direction = "lower_better" normalize 100 1000 }
    score "cost" { metric = "provider.cost" weight = 0.3 direction = "lower_better" normalize 0.1 1.5 }
  }

  optimize "sms-provider-selection" {
    decision = "sms"
    ranking = "sms"
    selection best
    goal = "maximize delivery quality while minimizing route cost"
  }
}
```

### Support And Operations

Operations teams need repeatable triage and routing across queues, people, regions, and SLAs.

Useful requirements:

- Ticket priority by severity, customer tier, age, sentiment, product, and SLA.
- Queue assignment by language, skill, availability, load, and region.
- Escalation when SLA is near breach.
- Incident routing to on-call teams.
- Manual review assignment with workflow evidence.

Why `condition` fits:

- Ranking selects the best queue.
- Rules and policies can add priority scores.
- Workflow assignment emits evidence and actions.
- Simulation shows how a new routing policy changes queue load.

Representative BCL:

```txt
module "support-priority" {
  version = "1"

  dataset "support-queues" {
    record "enterprise-billing" { provider.active = true provider.customer_tiers = ["enterprise"] provider.skills = ["billing", "payments"] provider.languages = ["en", "es"] provider.priority = 9 provider.availability = 0.92 provider.queue_depth = 12 provider.fallback_order = 1 }
    record "urgent-swat" { provider.active = true provider.customer_tiers = ["enterprise"] provider.skills = ["incident", "urgent"] provider.languages = ["en"] provider.priority = 8 provider.availability = 0.70 provider.queue_depth = 8 provider.fallback_order = 2 }
    record "general-billing" { provider.active = true provider.customer_tiers = ["free", "business", "enterprise"] provider.skills = ["billing"] provider.languages = ["en"] provider.priority = 5 provider.availability = 0.84 provider.queue_depth = 25 provider.fallback_order = 3 }
  }

  policy "ticket" {
    default allow
    require_review "negative-sentiment" when { ticket.sentiment < -0.5 } reason "customer sentiment is poor"
    escalate "sla-near-breach" when { ticket.sla_minutes <= 30 } reason "ticket is near SLA breach"
  }

  rule_set "ticket" {
    rule "critical-enterprise" {
      when {
        ticket.severity == "critical"
        customer.plan == "enterprise"
      }
      then {
        decision = "priority_support"
        score += 80
        event priority_escalated { source = "rules" }
      }
    }
  }

  ranking "support-queue" {
    dataset = "support-queues"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.availability"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.queue_depth"
    rule "active" { when { provider.active == true } }
    rule "tier" { when { customer.plan in provider.customer_tiers } }
    rule "skill" { when { ticket.topic in provider.skills } }
    rule "language" { when { customer.language in provider.languages } }
    score "availability" { metric = "provider.availability" weight = 0.45 normalize 0.5 1 }
    score "queue-depth" { metric = "provider.queue_depth" weight = 0.35 direction = "lower_better" normalize 0 50 }
    score "priority" { metric = "provider.priority" weight = 0.20 normalize 1 10 }
  }

  optimize "support-routing" {
    decision = "ticket"
    ranking = "support-queue"
    selection best
    goal = "route tickets to the best qualified queue before SLA breach"
  }

  workflow "ticket-review" {
    start at "triage"
    stage "triage" {
      assign role "support_lead"
      sla = "2h"
      on_timeout = "escalate"
    }
  }
}
```

### Ecommerce And Marketplaces

Commerce decisions often mix eligibility, risk, ranking, routing, and experimentation.

Useful requirements:

- Promotion eligibility by customer segment, cart value, product category, date, and abuse checks.
- Seller ranking by delivery quality, price, stock, rating, region, and return rate.
- Shipping carrier routing by country, package attributes, SLA, cost, and carrier health.
- Checkout fraud review by risk score, velocity, account age, and device reputation.
- Buy-box or offer selection with deterministic tie-breakers.

Why `condition` fits:

- Policies enforce hard eligibility.
- Ranking selects carriers, sellers, offers, or payment routes.
- Rule actions can hold orders, notify teams, or add review events.
- Embedded package tests protect high-value promotions from accidental broadening.

Representative BCL:

```txt
module "marketplace-shipping" {
  version = "1"

  dataset "shipping-carriers" {
    record "ups-2day" { provider.active = true provider.countries = ["US", "CA"] provider.handling = ["fragile"] provider.priority = 8 provider.delivery_days = 2 provider.cost = 16.8 provider.on_time = 0.971 provider.fallback_order = 1 }
    record "fedex-ground" { provider.active = true provider.countries = ["US"] provider.handling = ["fragile"] provider.priority = 5 provider.delivery_days = 3 provider.cost = 9.4 provider.on_time = 0.955 provider.fallback_order = 2 }
    record "dhl-express" { provider.active = true provider.countries = ["US", "GB", "NP"] provider.handling = ["fragile"] provider.priority = 7 provider.delivery_days = 1 provider.cost = 22.0 provider.on_time = 0.980 provider.fallback_order = 3 }
  }

  policy "checkout" {
    default allow
    deny "abuse-block" when { customer.abuse_flag == true } reason "customer is blocked for promotion abuse"
    require_review "high-risk-order" when { order.risk_score >= 70 } reason "checkout risk requires review"
    require_review "high-value-fragile" when { shipment.fragile == true and order.value > 1000 } reason "fragile high-value shipment needs review"
  }

  ranking "shipping-carrier" {
    dataset = "shipping-carriers"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.delivery_days"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.cost"
    rule "active" { when { provider.active == true } }
    rule "country" { when { shipment.country in provider.countries } }
    rule "fragile" { when { shipment.fragile == false or "fragile" in provider.handling } }
    rule "delivery-sla" { when { provider.delivery_days <= shipment.max_delivery_days } }
    score "on-time" { metric = "provider.on_time" weight = 0.45 normalize 0.90 1 }
    score "speed" { metric = "provider.delivery_days" weight = 0.25 direction = "lower_better" normalize 1 7 }
    score "cost" { metric = "provider.cost" weight = 0.30 direction = "lower_better" normalize 5 40 }
  }

  optimize "shipping-selection" {
    decision = "checkout"
    ranking = "shipping-carrier"
    selection best
    goal = "select the lowest-risk carrier that meets delivery promise and cost goals"
  }
}
```

### Healthcare And Administration

Healthcare administrative systems need explainable, auditable routing and eligibility decisions.

Useful requirements:

- Appointment prioritization by symptoms, wait time, location, and provider availability.
- Prior authorization checks using policy, coverage, diagnosis, and documentation.
- Claims routing to specialists by procedure, amount, payer, and exception flags.
- Care-management queue selection for high-risk patients.
- Compliance gates for privacy-sensitive workflows.

Why `condition` fits:

- Rich schema validation makes missing facts visible.
- Policies can require human review instead of making autonomous clinical judgments.
- Audit and explanation support administrative review.

Important boundary: clinical diagnosis or treatment recommendations may require medical-device, safety, and regulatory review. `condition` is best positioned for administrative decisions unless a regulated deployment adds the required controls.

Representative BCL:

```txt
module "prior-auth-admin" {
  version = "2026.05"

  dataset "admin-review-queues" {
    record "payer-authorization" { provider.active = true provider.payers = ["acme-health"] provider.skills = ["prior_auth", "documentation"] provider.priority = 9 provider.turnaround_hours = 12 provider.load = 0.52 provider.fallback_order = 1 }
    record "claims-specialist" { provider.active = true provider.payers = ["acme-health", "northstar"] provider.skills = ["claims", "exceptions"] provider.priority = 7 provider.turnaround_hours = 18 provider.load = 0.44 provider.fallback_order = 2 }
    record "privacy-review" { provider.active = true provider.payers = ["acme-health", "northstar"] provider.skills = ["privacy", "documentation"] provider.priority = 8 provider.turnaround_hours = 24 provider.load = 0.36 provider.fallback_order = 3 }
  }

  policy "prior-auth" {
    default allow
    require_review "missing-documentation" when { request.documentation_complete == false } reason "administrative documentation is incomplete"
    require_review "coverage-check" when { coverage.active == false } reason "coverage status requires administrative review"
    escalate "privacy-sensitive" when { request.privacy_sensitive == true } reason "privacy-sensitive workflow requires specialized review"
  }

  ranking "admin-queue" {
    dataset = "admin-review-queues"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.turnaround_hours"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.load"
    rule "active" { when { provider.active == true } }
    rule "payer" { when { request.payer in provider.payers } }
    rule "documentation" { when { request.documentation_complete == true or "documentation" in provider.skills } }
    rule "privacy" { when { request.privacy_sensitive == false or "privacy" in provider.skills } }
    score "turnaround" { metric = "provider.turnaround_hours" weight = 0.35 direction = "lower_better" normalize 4 72 }
    score "priority" { metric = "provider.priority" weight = 0.35 normalize 1 10 }
    score "load" { metric = "provider.load" weight = 0.30 direction = "lower_better" normalize 0 1 }
  }

  optimize "admin-routing" {
    decision = "prior-auth"
    ranking = "admin-queue"
    selection best
    goal = "route administrative prior-authorization work without making clinical recommendations"
  }
}
```

### AI Safety And Guardrails

AI applications need deterministic controls around probabilistic systems.

Useful requirements:

- Block credential extraction, malware generation, regulated advice, and data exfiltration attempts.
- Require review for low-confidence or high-risk AI outputs.
- Route prompts to specialized models or moderation queues.
- Enforce tenant-specific AI policies.
- Produce an audit trail for why a prompt or output was allowed or denied.

Why `condition` fits:

- AI signals become facts; deterministic policies make the final decision.
- BCL can encode guardrails independent of model prompts.
- Explanation shows which policy matched and which facts were used.
- Package versioning supports controlled rollout of safety policy changes.

Representative BCL:

```txt
module "ai-guardrails" {
  version = "1"

  dataset "moderation-queues" {
    record "security-review" { provider.active = true provider.risks = ["credential_extraction", "malware", "data_exfiltration"] provider.priority = 10 provider.sla_minutes = 15 provider.load = 0.62 provider.fallback_order = 1 }
    record "policy-review" { provider.active = true provider.risks = ["regulated_advice", "low_confidence"] provider.priority = 8 provider.sla_minutes = 30 provider.load = 0.40 provider.fallback_order = 2 }
    record "tenant-review" { provider.active = true provider.risks = ["tenant_policy"] provider.priority = 7 provider.sla_minutes = 45 provider.load = 0.35 provider.fallback_order = 3 }
  }

  policy "response" {
    default allow
    deny "credential-extraction" when {
      ai.intent == "credential_extraction"
    } reason "credential extraction is blocked"

    require_review "low-confidence" when {
      ai.confidence < 0.75
      ai.risk_score >= 40
    } reason "low confidence high risk response"

    require_review "tenant-policy" when {
      tenant.policy_mode == "review"
    } reason "tenant policy requires review"
  }

  ranking "moderation-queue" {
    dataset = "moderation-queues"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.sla_minutes"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.load"
    rule "active" { when { provider.active == true } }
    rule "risk-fit" { when { ai.intent in provider.risks or ai.risk_label in provider.risks } }
    score "priority" { metric = "provider.priority" weight = 0.45 normalize 1 10 }
    score "sla" { metric = "provider.sla_minutes" weight = 0.30 direction = "lower_better" normalize 5 120 }
    score "load" { metric = "provider.load" weight = 0.25 direction = "lower_better" normalize 0 1 }
  }

  optimize "moderation-routing" {
    decision = "response"
    ranking = "moderation-queue"
    selection best
    goal = "route risky AI interactions to the most appropriate moderation queue"
  }
}
```

### HR, Procurement, And Legal

Internal corporate workflows often require policy checks, approvals, and defensible routing.

Useful requirements:

- Procurement approval by amount, department, vendor risk, budget, and category.
- Vendor ranking by cost, compliance, quality, region, and availability.
- Contract routing by value, clause risk, jurisdiction, and urgency.
- HR eligibility checks for benefits, leave, promotion workflows, and access requests.
- Maker/checker metadata for regulated internal decisions.

Why `condition` fits:

- Workflows represent staged approval.
- Governance metadata can identify owners, makers, checkers, approvers, and signatures.
- Package tests can lock expected approval outcomes.

Representative BCL:

```txt
module "procurement" {
  version = "1"

  dataset "vendor-review-queues" {
    record "preferred-vendor" { provider.active = true provider.categories = ["software", "cloud"] provider.regions = ["US", "GB"] provider.priority = 9 provider.compliance = 0.98 provider.cost_index = 0.70 provider.fallback_order = 1 }
    record "legal-review" { provider.active = true provider.categories = ["software", "services"] provider.regions = ["US", "EU", "GB"] provider.priority = 8 provider.compliance = 0.95 provider.cost_index = 0.85 provider.fallback_order = 2 }
    record "finance-review" { provider.active = true provider.categories = ["software", "hardware", "services"] provider.regions = ["US", "EU", "GB"] provider.priority = 6 provider.compliance = 0.90 provider.cost_index = 0.60 provider.fallback_order = 3 }
  }

  policy "purchase-request" {
    default allow
    deny "budget-unavailable" when { budget.available < request.amount } reason "budget is unavailable"
    require_review "vendor-risk" when { vendor.risk_score >= 60 } reason "vendor risk requires review"
    escalate "large-contract" when { request.amount > 250000 } reason "large contract requires executive approval"
  }

  ranking "vendor-review" {
    dataset = "vendor-review-queues"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.compliance"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.cost_index"
    rule "active" { when { provider.active == true } }
    rule "category" { when { request.category in provider.categories } }
    rule "region" { when { request.region in provider.regions } }
    score "compliance" { metric = "provider.compliance" weight = 0.45 normalize 0.7 1 }
    score "priority" { metric = "provider.priority" weight = 0.30 normalize 1 10 }
    score "cost" { metric = "provider.cost_index" weight = 0.25 direction = "lower_better" normalize 0 1 }
  }

  optimize "vendor-review-routing" {
    decision = "purchase-request"
    ranking = "vendor-review"
    selection best
    goal = "route purchase requests to the best approval path for risk, cost, and compliance"
  }

  workflow "purchase-request" {
    start at "department_review"

    stage "department_review" {
      assign role "department_head"
      rule "small-request" {
        when { request.amount <= 50000 }
        then { decision = "approve" }
      }
    }

    stage "finance_review" {
      assign role "finance_manager"
      sla = "24h"
      on_timeout = "escalate"
      rule "budget-check" {
        when { budget.available >= request.amount }
        then { decision = "approve" }
      }
    }
  }
}
```

### SaaS And Product Platforms

SaaS platforms need tenant-aware, low-latency decisions that change faster than application deployments.

Useful requirements:

- Feature flags by tenant, cohort, plan, region, stable bucket, and allow/deny list.
- Entitlement checks by subscription, contract, usage, and account status.
- Rate-limit tier decisions.
- Plan upgrade offers and in-product recommendation eligibility.
- Regional data residency or compliance gates.

Why `condition` fits:

- Compile once and evaluate many times.
- Variables and package metadata support tenant-specific configuration.
- Package digests identify exactly which rollout configuration was active.
- Embedded runtime avoids a network hop when decisions must be extremely fast.

Representative BCL:

```txt
module "saas-entitlements" {
  version = "1"

  dataset "upgrade-offers" {
    record "ai-workflows" { provider.active = true provider.plans = ["business", "enterprise"] provider.regions = ["US", "CA", "GB"] provider.priority = 9 provider.expected_value = 0.82 provider.cost = 0.30 provider.fallback_order = 1 }
    record "audit-pack" { provider.active = true provider.plans = ["enterprise"] provider.regions = ["US", "EU", "GB"] provider.priority = 8 provider.expected_value = 0.74 provider.cost = 0.25 provider.fallback_order = 2 }
    record "support-plus" { provider.active = true provider.plans = ["free", "business"] provider.regions = ["US", "CA", "GB", "AU"] provider.priority = 6 provider.expected_value = 0.68 provider.cost = 0.20 provider.fallback_order = 3 }
  }

  policy "feature-access" {
    default deny
    deny "account-inactive" when { account.status != "active" } reason "account is not active"
    allow "allow-list" when { account.id in lists.allow_accounts } reason "account is explicitly allowed"
    allow "eligible-rollout" when {
      account.plan in ["business", "enterprise"]
      account.region in ["US", "CA", "GB"]
      stableBucket(account.id, 100) < feature.rollout_percent
    } reason "account is eligible for rollout"
  }

  ranking "upgrade-offer" {
    dataset = "upgrade-offers"
    selection best
    priority_path = "provider.priority"
    specificity_path = "provider.expected_value"
    fallback_path = "provider.fallback_order"
    cost_path = "provider.cost"
    rule "active" { when { provider.active == true } }
    rule "plan" { when { account.plan in provider.plans } }
    rule "region" { when { account.region in provider.regions } }
    score "expected-value" { metric = "provider.expected_value" weight = 0.50 normalize 0 1 }
    score "priority" { metric = "provider.priority" weight = 0.30 normalize 1 10 }
    score "cost" { metric = "provider.cost" weight = 0.20 direction = "lower_better" normalize 0 1 }
  }

  optimize "offer-selection" {
    decision = "feature-access"
    ranking = "upgrade-offer"
    selection best
    goal = "select the best eligible in-product offer for the tenant"
  }
}
```

## Requirement-To-Feature Examples

| Business requirement | Recommended `condition` approach |
| --- | --- |
| Hard compliance block | `Policy` with `deny` effect and reason |
| Manual review trigger | `Policy` or `Rule` with `require_review` or `escalate` |
| Risk score | Rule score deltas plus `Explanation.ScoreDeltas` |
| Best provider/queue/vendor | `Ranking` with eligibility rules and weighted score rules |
| Objective-based route selection | `Optimization` backed by a ranking and objective metadata |
| Human assignment | `Workflow` stage with `assign role/user/queue` |
| SLA escalation metadata | Workflow stage `sla` and `on_timeout` |
| Package publish safety | `ValidateDecisionPackage` and embedded BCL `test` blocks |
| Historical replay | Store package digest, request fingerprint, and result fingerprint |
| Version comparison | `Simulate` and `ComparePackages` |
| Audit integrity | `AuditEnvelope` chain verification plus durable `AuditStore` |
| Multi-tenant authorization | `DecisionServer` with `Authorizer` middleware |
| Business-readable authoring | BCL package DSL |
| Lowest latency runtime | Compiled expressions, typed facts, bytecode runtime, ranker hot paths |

## Deployment Fit

### Embedded SDK

Use when the host service needs fast local decisions with no network hop. This is a strong fit for feature flags, eligibility checks, provider routing, fraud prechecks, and tenant-specific gates.

### Package-Based Runtime

Use when policy/rule changes need versioning, testing, simulation, explanation, and rollback. This is a strong fit for fraud, compliance, procurement, AI safety, and regulated workflows.

### File-Based Live Runtime

Use when operators want to update BCL, JSON, or YAML decision packages without restarting the host process. The live runtime polls configured files, waits for stable writes, validates and compiles the new package, optionally runs embedded tests, then publishes only successful reloads. Syntax errors, broken tests, and invalid packages are rejected while the last known-good package remains active.

### HTTP Server

Use when multiple services need a shared decision API. The built-in server exposes package publish, list, get, evaluate, simulate, compare, tests, and audit endpoints. Production deployments should inject durable stores and authentication middleware.

### Batch And Replay

Use simulation for historical impact analysis: false-positive estimation, score threshold tuning, queue load changes, provider routing changes, and package comparison before publish.

### On-Premise And Air-Gapped

Use BCL files, embedded package tests, a durable local store, and operator-controlled deployment artifacts. This works well for banks, government, and high-security institutions, assuming the deployment adds required operational controls.

## Patent-Candidate Analysis

Again: this section does not say the project is patented or guaranteed patentable. It identifies technical mechanisms that may be worth reviewing with patent counsel.

### Investor-Facing Differentiation

The differentiated product idea is not merely a rule engine. It is a deterministic decision operating layer that combines:

- Business-readable policy packages.
- Deterministic execution across policies, rules, scores, rankings, optimizations, and workflows.
- Explainability as a first-class output.
- Simulation and version comparison before publish.
- Audit fingerprints and tamper-evident envelopes.
- Low-latency embedded execution.
- Production server contracts without forcing a database dependency.

This convergence may be more defensible than any single feature. The strongest technical story is the end-to-end chain from package authoring to deterministic decision evidence to replayable audit identity.

### 1. Evidence Graph Ledger

Problem solved:

Enterprises need to know why a decision occurred, not only what result was returned.

Technical approach:

Each policy, rule, score delta, ranking decision, workflow transition, action, event, and fact dependency can be represented as evidence attached to the response. The response includes matched rules, failed rules, fact evidence, score deltas, ranking reasons, workflow path, counterfactual hints, and evidence graph nodes.

Why it may be differentiated:

Many rule engines return a boolean or matched rule list. A ledger-style evidence graph across multiple decision subsystems gives a more complete replay and audit model.

Business value:

Compliance review, customer dispute handling, model-risk review, policy debugging, and regulated audit support.

Prior-art questions for counsel:

- Existing rule-engine agenda traces and explanation systems.
- Business rule management system audit logs.
- Policy-as-code explanation and tracing systems.
- Decision-model notation audit outputs.

Possible claim-direction language, non-legal draft:

A deterministic decision execution system that records heterogeneous decision artifacts as linked evidence nodes, where the nodes include policy effects, rule matches, score changes, ranking selection reasons, workflow transitions, actions, events, and fact dependencies, and where the evidence graph is returned with the decision result for replay and audit.

### 2. Policy Effect Lattice

Problem solved:

Real decisions can produce conflicting effects such as allow, deny, require review, escalate, or abstain.

Technical approach:

Policy rules emit normalized effects. The runtime resolves them through deterministic precedence and records the contributing policy effects and conflict handling in the explanation.

Why it may be differentiated:

The lattice combines access-control style decisions with business workflow effects and explainable conflict resolution.

Business value:

Predictable governance behavior, no ambiguous policy conflicts, and easier audit of why a deny or review overrode an allow.

Prior-art questions for counsel:

- XACML combining algorithms.
- Open Policy Agent policy decisions.
- Access-control policy lattices.
- Business rules conflict-resolution systems.

Possible claim-direction language, non-legal draft:

A policy evaluation method that maps heterogeneous business policy outcomes into a deterministic effect lattice and produces a resolved effect with an explanation of all contributing and overridden effects.

### 3. Counterfactual Delta Engine

Problem solved:

Users often ask what would need to change for a denied, reviewed, or failed decision to pass.

Technical approach:

Failed rule comparisons are inspected to produce lightweight hints identifying missing facts, target thresholds, or fact values that could change the outcome.

Why it may be differentiated:

The hints are generated from deterministic rule evaluation rather than from a probabilistic explanation model. They can be tied to exact package version, rule ID, and fact path.

Business value:

Faster remediation, clearer customer support, better analyst productivity, and safer pre-publish simulation.

Prior-art questions for counsel:

- Counterfactual explanations in ML.
- Rule-engine failed-condition diagnostics.
- Eligibility screening systems that list missing requirements.

Possible claim-direction language, non-legal draft:

A deterministic rule evaluation system that derives counterfactual change hints from failed comparison conditions and associates each hint with a versioned rule, fact path, current value, target value, and decision package digest.

### 4. Canonical Decision Fingerprint

Problem solved:

Audit systems need to replay decisions and detect whether package, input, or output changed.

Technical approach:

The platform computes canonical package digests plus request and result fingerprints. Audit records include package digest, request fingerprint, result fingerprint, timestamps, duration, and trace summary. Audit envelopes can chain payload hashes.

Why it may be differentiated:

The fingerprint covers the decision package and the decision result envelope, connecting authoring, runtime, explanation, and audit.

Business value:

Tamper detection, replay confidence, regulated audit trails, rollback validation, and dispute resolution.

Prior-art questions for counsel:

- Content-addressed configuration systems.
- Blockchain/hash-chained audit logs.
- Rule package signing and policy bundle digests.
- Event sourcing and replay fingerprints.

Possible claim-direction language, non-legal draft:

A decision replay integrity method that produces canonical hashes for a decision package, request shape, evidence-bearing result envelope, and audit payload, and links the hashes to a versioned decision response.

### 5. Deterministic Multi-Objective Ranking

Problem solved:

Businesses need to select the best candidate while balancing eligibility, priority, cost, quality, specificity, fallback behavior, and stable routing.

Technical approach:

Rankings evaluate eligibility rules and score rules, then apply deterministic ordering and tie-breakers. Optimizations can attach objective metadata while using ranking mechanics for reproducible selection.

Why it may be differentiated:

The same mechanism can serve provider routing, queue assignment, offer selection, vendor ranking, and fraud queue prioritization while remaining explainable and deterministic.

Business value:

Lower cost, better quality, predictable routing, safer experiments, and reproducible incident analysis.

Prior-art questions for counsel:

- Multi-criteria decision analysis systems.
- Load-balancing and weighted routing algorithms.
- Ranking engines and recommender systems.
- Deterministic experimentation and bucketing systems.

Possible claim-direction language, non-legal draft:

A deterministic candidate selection method that combines eligibility conditions, weighted scores, priority paths, cost paths, fallback paths, specificity paths, and stable selection keys into a reproducible ranking result with explanation metadata.

### 6. Explainable Workflow Pathing

Problem solved:

Approval workflows need to explain why a request moved, stopped, escalated, or was assigned.

Technical approach:

Workflow stages can include assignment metadata, SLA intent, timeout behavior, and rules. Evaluation emits workflow path evidence and assignment actions when stages are reached.

Why it may be differentiated:

The workflow path is part of the same decision evidence model as policies, scores, rankings, and rules.

Business value:

Human-in-the-loop governance, approval auditability, queue management, and SLA review.

Prior-art questions for counsel:

- BPMN workflow engines.
- Human-task workflow audit trails.
- Approval routing systems.
- Rule-driven workflow engines.

Possible claim-direction language, non-legal draft:

A workflow decision method that records stage transitions, assignment metadata, timeout intent, and rule outcomes as explainable evidence nodes in the same decision response as policy and ranking evidence.

### 7. BCL Zero-Allocation Scanner And Append Encoder Hot Paths

Problem solved:

Decision packages need human-readable authoring without making tooling slow or allocation-heavy.

Technical approach:

BCL tokenization references source byte spans rather than allocating token strings. Append-style encoding writes to caller-provided buffers and uses fast paths for simple strings, inline maps, schema enums, and primitive values.

Why it may be differentiated:

The BCL layer is tailored to decision packages, not general-purpose configuration, and its scanner/encoder hot paths are designed for predictable low allocation behavior.

Business value:

Fast package loading tooling, efficient tests and simulation, better behavior in embedded systems, and lower garbage collection pressure.

Prior-art questions for counsel:

- Zero-copy parsers and scanners.
- Append-style encoders in standard libraries.
- Configuration language parsers.
- Domain-specific decision DSLs.

Possible claim-direction language, non-legal draft:

A domain-specific decision package language processor that scans token spans without allocating token strings and encodes decision packages into caller-owned buffers while preserving canonical decision semantics.

## Attorney-Prep Invention Disclosure Checklist

For each patent-candidate mechanism, prepare:

- Earliest conception date and contributors.
- First implementation date and commit references.
- Public disclosures, demos, docs, or releases.
- Concrete technical problem and why ordinary systems were insufficient.
- Specific data structures and algorithms used.
- Inputs, outputs, and execution flow.
- Performance, audit, or correctness advantages.
- Known competing products or open-source projects.
- Claims that should be broad, and claims that should be narrow fallback positions.
- Jurisdictions of commercial interest.

High-priority prior-art areas:

- Business rule management systems.
- Policy-as-code engines.
- Decision Model and Notation tools.
- Workflow/BPM systems.
- Multi-objective ranking and routing.
- Audit-log hashing and event sourcing.
- Explainable AI and counterfactual explanation.
- Configuration DSL parsers and zero-copy scanners.

## Practical Positioning

Best public positioning:

```txt
condition is an embeddable decision intelligence platform for deterministic, explainable, auditable business decisions.
```

Avoid positioning it only as:

```txt
rule engine
```

The broader value is the convergence of rules, policies, scoring, ranking, workflows, optimization metadata, simulation, BCL authoring, and audit-ready explanations.

## Existing Examples To Review

The `usecase_*` Go examples are file-backed: each one loads a `package.yaml` decision package and a `request.json` evaluation request rather than embedding the package definition in Go code. The `routing_file_backed_*` examples are the routing-focused catalog, with candidates, policies, rankings, tie-breakers, and optimization metadata in files.

- Banking and fintech: [Go use-case package](../examples/usecase_banking_fintech), [Decision intelligence example](../examples/decision_intelligence), [BCL decision platform server example](../examples/bcl_decision_platform), and [Fraud review](../examples/fraud_review).
- Insurance: [Go use-case package](../examples/usecase_insurance) and [file-backed claims routing](../examples/routing_file_backed_claims).
- Government and public sector: [Go use-case package](../examples/usecase_government_public_sector) and [file-backed permit routing](../examples/routing_file_backed_permits).
- Telecom and messaging: [Go use-case package](../examples/usecase_telecom_messaging), [file-backed SMS routing](../examples/routing_file_backed_sms), [file-backed email routing](../examples/routing_file_backed_email), [SMS routing](../examples/sms_routing), [Email routing](../examples/email_routing), and [telecom BCL package](../examples/bcl/packages/telecom_optimization.bcl).
- Support and operations: [Go use-case package](../examples/usecase_support_operations), [file-backed support routing](../examples/routing_file_backed_support), [Support routing](../examples/support_routing), and [support priority BCL package](../examples/bcl/packages/support_priority.bcl).
- Ecommerce and marketplaces: [Go use-case package](../examples/usecase_ecommerce_marketplaces), [file-backed shipping routing](../examples/routing_file_backed_shipping), [file-backed payment routing](../examples/routing_file_backed_payment), [Shipping routing](../examples/shipping_routing), [Payment routing](../examples/payment_routing), and [complete checkout condition](../examples/complete_condition).
- Healthcare administration: [Go use-case package](../examples/usecase_healthcare_administration) and [file-backed healthcare admin routing](../examples/routing_file_backed_healthcare_admin).
- AI safety guardrails: [Go use-case package](../examples/usecase_ai_safety_guardrails), [file-backed AI moderation routing](../examples/routing_file_backed_ai_moderation), and [AI guardrails BCL package](../examples/bcl/packages/ai_guardrails.bcl).
- HR, procurement, and legal: [Go use-case package](../examples/usecase_hr_procurement_legal), [file-backed vendor/procurement routing](../examples/routing_file_backed_vendor_procurement), and [procurement workflow BCL package](../examples/bcl/packages/procurement_workflow.bcl).
- SaaS and product platforms: [Go use-case package](../examples/usecase_saas_product_platforms), [file-backed SaaS offer routing](../examples/routing_file_backed_saas_offers), and [Feature flags](../examples/feature_flags).
- General BCL loading and package authoring: [BCL packages](../examples/bcl/packages), [BCL example runner](../examples/bcl), and [load sources](../examples/load_sources).
