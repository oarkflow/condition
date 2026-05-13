# TCPGuard — Complete Risk, Threat & Policy Detection Platform

TCPGuard should be designed as:

> **A policy-driven runtime risk and threat detection platform for Go/Fiber applications that protects APIs, sessions, users, tenants, devices, business workflows, and infrastructure using extensible rules, triggers, actions, risk scoring, and threat models.**

It is not just middleware. It is a complete **application security decision engine**.

---

# 1. Core Capabilities

TCPGuard should provide:

```txt
1. GoFiber middleware protection
2. Runtime threat detection
3. Business-aware anomaly detection
4. Session hijack detection
5. MITM and replay detection
6. DDoS and abuse protection
7. Risk scoring engine
8. Threat modeling engine
9. Policy engine
10. Rule DSL
11. Trigger engine
12. Action orchestration engine
13. Audit and incident engine
14. SIEM/webhook/event-bus integration
15. Multi-tenant policy support
16. Extensible plugin registry
17. No-code rule/config updates
18. Dashboard and policy simulator
```

---

# 2. High-Level Architecture

```txt
┌────────────────────────────┐
│ Incoming HTTP Request       │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ GoFiber Middleware          │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ Context Builder             │
│ - request                   │
│ - user                      │
│ - session                   │
│ - device                    │
│ - IP / geo / ASN            │
│ - tenant                    │
│ - business action           │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ Event Normalizer            │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ Trigger Engine              │
│ - request triggers          │
│ - session triggers          │
│ - auth triggers             │
│ - business triggers         │
│ - sequence triggers         │
│ - scheduled triggers        │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ Rule Engine                 │
│ - HSL-like DSL              │
│ - conditions                │
│ - windows                   │
│ - thresholds                │
│ - baselines                 │
│ - sequences                 │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ Risk & Threat Engine        │
│ - risk scoring              │
│ - STRIDE mapping            │
│ - MITRE mapping             │
│ - severity calculation      │
│ - confidence scoring        │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ Policy Decision Engine      │
│ - allow                     │
│ - monitor                   │
│ - challenge                 │
│ - throttle                  │
│ - block                     │
│ - revoke                    │
│ - escalate                  │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ Action Engine               │
│ - sequential actions        │
│ - parallel actions          │
│ - delayed actions           │
│ - retry actions             │
│ - conditional actions       │
│ - custom actions            │
└─────────────┬──────────────┘
              │
              ▼
┌────────────────────────────┐
│ Audit / Incident / SIEM     │
└────────────────────────────┘
```

---

# 3. Core Modules

## 3.1 Middleware Layer

Responsible for:

```txt
- Intercepting requests
- Extracting request metadata
- Attaching request ID
- Calling TCPGuard engine
- Applying final decision
- Returning challenge/block/throttle response
- Passing clean requests to GoFiber handlers
```

Example:

```go
app := fiber.New()

guard, err := tcpguard.New(
    tcpguard.WithPolicyDir("./policies"),
    tcpguard.WithRuleDir("./rules"),
    tcpguard.WithMode(tcpguard.Enforce),
)

if err != nil {
    panic(err)
}

app.Use(guard.Middleware())
```

---

## 3.2 Context Builder

Builds a normalized security context.

```txt
Context
 ├─ Request
 │   ├─ path
 │   ├─ method
 │   ├─ headers
 │   ├─ body size
 │   ├─ content type
 │   └─ protocol
 │
 ├─ Network
 │   ├─ IP
 │   ├─ country
 │   ├─ city
 │   ├─ ASN
 │   ├─ proxy/VPN/Tor
 │   └─ reputation
 │
 ├─ Identity
 │   ├─ user ID
 │   ├─ role
 │   ├─ tenant
 │   ├─ permissions
 │   └─ auth method
 │
 ├─ Session
 │   ├─ session ID
 │   ├─ device ID
 │   ├─ user agent
 │   ├─ fingerprint
 │   ├─ previous IP
 │   └─ previous country
 │
 ├─ Business
 │   ├─ action
 │   ├─ entity
 │   ├─ amount
 │   ├─ workflow
 │   ├─ approval level
 │   └─ sensitivity
 │
 └─ Runtime
     ├─ timestamp
     ├─ business hours
     ├─ holiday
     ├─ policy version
     └─ config hash
```

---

# 4. Feature List

## 4.1 Runtime API Protection

```txt
- Request anomaly detection
- Header anomaly detection
- Method anomaly detection
- Route abuse detection
- Query parameter abuse detection
- Payload size anomaly
- Content-Type mismatch detection
- Suspicious user-agent detection
- Host header tampering detection
- Origin/Referer mismatch detection
- API key misuse detection
- Token reuse detection
- Sensitive endpoint protection
```

---

## 4.2 Business-Aware Anomaly Detection

```txt
- Business-hours anomaly
- Holiday access anomaly
- Region-based access restriction
- Country-based access restriction
- Branch/location mismatch
- Department mismatch
- Role/action mismatch
- Suspicious approval behavior
- Workflow bypass detection
- High-value action anomaly
- Report/export anomaly
- Dormant user activity
- New admin activity
- Unusual tenant activity
- Unusual API usage by business function
```

---

## 4.3 Session Risk Detection

```txt
- Session hijack detection
- Session replay detection
- Session fixation detection
- Session country change
- Session ASN change
- Session device change
- Session user-agent change
- Concurrent location detection
- Impossible travel detection
- Token reuse
- Refresh token abuse
- Privilege escalation after login
- New device risk scoring
```

---

## 4.4 MITM / Replay Protection

```txt
- Signed request validation
- HMAC request verification
- Request nonce validation
- Timestamp validation
- Clock skew detection
- Body hash verification
- Header canonicalization
- Duplicate request detection
- Proxy header spoof detection
- Trusted proxy validation
- TLS fingerprint drift detection
- JA3/JA4 fingerprint support
- mTLS client verification
```

---

## 4.5 DDoS & Abuse Protection

```txt
- Per-IP rate limit
- Per-user rate limit
- Per-tenant rate limit
- Per-session rate limit
- Per-endpoint rate limit
- Global rate limit
- Burst control
- Slow request detection
- Slowloris detection
- Login flood detection
- Credential stuffing detection
- Password spraying detection
- Bot detection
- Distributed low-rate attack detection
- Adaptive throttling
- Temporary banning
- Progressive penalties
```

---

## 4.6 Risk & Threat Modeling

```txt
- User risk profile
- Session risk profile
- Device risk profile
- IP risk profile
- Tenant risk profile
- Endpoint risk profile
- API key risk profile
- Business action risk profile
- Threat confidence score
- Severity calculation
- Risk decay
- Entity baseline learning
- STRIDE mapping
- MITRE-style tactic/technique mapping
```

---

## 4.7 Policy & Rule Management

```txt
- HSL-like block DSL
- Rule hot reload
- Rule versioning
- Rule signing
- Rule checksum verification
- Draft rules
- Shadow rules
- Dry-run rules
- Rule approval workflow
- Per-tenant overrides
- Per-role overrides
- Per-endpoint overrides
- Rule priority
- Rule inheritance
- Rule rollback
- Rule simulation
- Rule testing
- False-positive review
```

---

## 4.8 Action Orchestration

```txt
- Allow
- Log
- Audit
- Add risk header
- Throttle
- Delay
- Tarpit
- CAPTCHA challenge
- MFA challenge
- Re-authenticate
- Block
- Revoke session
- Revoke all sessions
- Disable API key
- Lock user
- Ban IP
- Ban ASN
- Block country
- Notify admin
- Notify user
- Notify SOC
- Create incident
- Escalate incident
- Send webhook
- Send SIEM event
- Publish to NATS/Kafka/custom broker
- Execute custom Go action
```

---

# 5. TCPGuard HSL-Like DSL

Instead of YAML, TCPGuard can use a block-based DSL.

Example file:

```hsl
guard "tcpguard-main" {
    mode enforce
    version "2026.05.13"

    include "./rules/*.hsl"
    include "./actions/*.hsl"
    include "./triggers/*.hsl"

    timezone "Asia/Kathmandu"

    storage redis {
        address "127.0.0.1:6379"
        prefix "tcpguard:"
    }

    audit {
        mode tamper_evident
        hmac_key env("TCPGUARD_AUDIT_KEY")
    }
}
```

---

# 6. Rule DSL Structure

Every rule should follow this structure:

```hsl
rule "rule-id" {
    name "Human readable name"
    status active
    priority 100
    version 1

    scope {
        tenants ["*"]
        roles ["admin", "manager"]
        paths ["/admin/*", "/api/v1/*"]
    }

    trigger {
        ...
    }

    when {
        ...
    }

    risk {
        ...
    }

    severity {
        ...
    }

    actions {
        ...
    }

    cooldown {
        ...
    }

    audit {
        ...
    }
}
```

---

# 7. Example: After-Hours Admin Risk

```hsl
rule "after-hours-admin-access" {
    name "Admin access outside business hours"
    status active
    priority 100

    scope {
        roles ["admin", "super_admin"]
        paths ["/admin/*"]
    }

    trigger {
        on request.received
        on auth.login_success
    }

    when {
        all {
            request.path matches "/admin/*"
            user.role in ["admin", "super_admin"]
            business.outside_hours equals true
        }
    }

    risk {
        base 35

        add 20 when session.device.new equals true
        add 25 when network.country.changed equals true
        add 30 when network.ip.reputation greater_than 70
        add 20 when request.method in ["POST", "PUT", "DELETE"]

        max 100
    }

    severity {
        medium when risk.score greater_or_equal 50
        high when risk.score greater_or_equal 75
        critical when risk.score greater_or_equal 90
    }

    actions {
        medium {
            run audit
            run throttle
        }

        high {
            run audit
            run mfa_challenge
            run notify_admin
        }

        critical {
            run block
            run revoke_session
            run create_incident
        }
    }

    cooldown {
        key user.id
        duration 5m
    }

    audit {
        explain true
        include_context true
    }
}
```

---

# 8. Example: Suspicious Export After Risky Login

```hsl
rule "suspicious-export-after-risky-login" {
    name "Failed logins followed by successful login and large export"
    status active
    priority 200

    trigger {
        sequence within 10m {
            auth.login_failed count greater_or_equal 5
            auth.login_success
            business.export
        }
    }

    when {
        all {
            user.role in ["admin", "manager"]
            business.action equals "report.export"
            business.export.records greater_than 10000
        }

        any {
            session.device.new equals true
            network.country.changed equals true
            business.outside_hours equals true
        }
    }

    risk {
        base 50
        add 20 when session.device.new equals true
        add 25 when network.country.changed equals true
        add 30 when business.export.records greater_than 50000
        max 100
    }

    severity {
        high when risk.score greater_or_equal 75
        critical when risk.score greater_or_equal 90
    }

    actions {
        high {
            run mfa_challenge
            run throttle
            run notify_admin
            run create_incident
        }

        critical {
            run block
            run revoke_session
            run disable_api_key
            run notify_soc
        }
    }

    audit {
        explain true
        map_to_stride ["information_disclosure", "elevation_of_privilege"]
        map_to_mitre ["valid_accounts", "data_exfiltration"]
    }
}
```

---

# 9. Example: MITM / Replay Attack Detection

```hsl
rule "signed-request-replay-detection" {
    name "Detect duplicate nonce or invalid request signature"
    status active
    priority 300

    trigger {
        on request.received
    }

    when {
        any {
            security.signature.valid equals false
            security.nonce.reused equals true
            security.timestamp.skew_seconds greater_than 60
            security.body_hash.valid equals false
        }
    }

    risk {
        base 80
        add 20 when security.nonce.reused equals true
        add 15 when security.body_hash.valid equals false
        max 100
    }

    severity {
        high when risk.score greater_or_equal 75
        critical when risk.score greater_or_equal 90
    }

    actions {
        high {
            run block
            run audit
            run notify_admin
        }

        critical {
            run block
            run revoke_session
            run ban_ip temporary 30m
            run create_incident
        }
    }

    audit {
        explain true
        map_to_stride ["tampering", "spoofing", "repudiation"]
    }
}
```

---

# 10. Example: DDoS / Abuse Rule

```hsl
rule "adaptive-api-abuse" {
    name "Adaptive API abuse detection"
    status active
    priority 400

    trigger {
        on request.received
    }

    when {
        any {
            rate.ip.requests within 1m greater_than 120
            rate.user.requests within 1m greater_than 300
            rate.tenant.requests within 1m greater_than 5000
            rate.endpoint.errors within 1m greater_than 100
        }
    }

    risk {
        base 40

        add 20 when rate.ip.requests within 1m greater_than 120
        add 30 when rate.ip.requests within 1m greater_than 300
        add 25 when rate.endpoint.errors within 1m greater_than 100
        add 30 when network.ip.reputation greater_than 80

        max 100
    }

    severity {
        medium when risk.score greater_or_equal 50
        high when risk.score greater_or_equal 75
        critical when risk.score greater_or_equal 90
    }

    actions {
        medium {
            run throttle
            run audit
        }

        high {
            run throttle aggressive
            run delay 2s
            run notify_admin
        }

        critical {
            run block
            run ban_ip temporary 15m
            run publish_event "security.ddos"
        }
    }
}
```

---

# 11. Example: Impossible Travel Detection

```hsl
rule "impossible-travel" {
    name "Detect impossible travel between countries"
    status active
    priority 250

    trigger {
        on auth.login_success
        on session.country_changed
    }

    when {
        all {
            session.previous.country exists
            network.country not_equals session.previous.country
            geo.distance_km greater_than 1000
            session.last_seen_age less_than 30m
        }
    }

    risk {
        base 70
        add 20 when session.device.new equals true
        add 20 when network.ip.reputation greater_than 60
        max 100
    }

    severity {
        high when risk.score greater_or_equal 75
        critical when risk.score greater_or_equal 90
    }

    actions {
        high {
            run mfa_challenge
            run notify_user
            run notify_admin
        }

        critical {
            run block
            run revoke_session
            run create_incident
        }
    }
}
```

---

# 12. Trigger System

TCPGuard should allow new triggers without changing core code.

## Built-in Trigger Types

```txt
request.received
request.completed
request.failed
request.large_body
request.suspicious_header

auth.login_success
auth.login_failed
auth.logout
auth.password_reset
auth.mfa_failed
auth.token_refresh

session.created
session.changed
session.device_changed
session.country_changed
session.replayed
session.concurrent_location

business.action
business.export
business.approval
business.payment
business.role_change
business.workflow_bypass

threat.ddos
threat.bruteforce
threat.replay
threat.mitm
threat.bot
threat.exfiltration

system.cpu_pressure
system.memory_pressure
system.error_spike
system.policy_reload
```

---

# 13. Adding a New Trigger Without Code Change

A new trigger can be declared in DSL as an alias or derived trigger.

```hsl
trigger "business.high_value_payment" {
    source business.action

    when {
        all {
            business.action equals "payment.approve"
            business.amount greater_than 1000000
        }
    }

    emit "business.high_value_payment"
}
```

Then rules can use it:

```hsl
rule "high-value-payment-after-hours" {
    trigger {
        on business.high_value_payment
    }

    when {
        business.outside_hours equals true
    }

    risk {
        base 75
    }

    actions {
        high {
            run mfa_challenge
            run notify_admin
        }

        critical {
            run block
            run create_incident
        }
    }
}
```

No Go code change required because this trigger is derived from existing event fields.

---

# 14. Adding a New Action Without Core Code Change

TCPGuard should support action definitions as DSL wrappers around generic adapters.

## Webhook Action

```hsl
action "notify_fraud_team" {
    type webhook

    endpoint env("FRAUD_WEBHOOK_URL")
    method POST

    headers {
        "Content-Type" "application/json"
        "X-TCPGuard-Token" env("TCPGUARD_WEBHOOK_TOKEN")
    }

    body {
        template "incident_notification"
    }

    retry {
        attempts 3
        backoff exponential
    }

    timeout 5s
}
```

Usage:

```hsl
actions {
    high {
        run notify_fraud_team
    }
}
```

---

## Event Bus Action

```hsl
action "publish_security_event" {
    type event_bus
    provider nats
    subject "security.tcpguard.alert"

    payload {
        include request.id
        include user.id
        include tenant.id
        include risk.score
        include severity
        include findings
    }
}
```

---

## SQL Action

```hsl
action "insert_security_case" {
    type sql
    datasource "security_db"

    statement """
        INSERT INTO security_cases
        (request_id, user_id, tenant_id, severity, risk_score, reason)
        VALUES
        (:request_id, :user_id, :tenant_id, :severity, :risk_score, :reason)
    """
}
```

---

## Command Action

```hsl
action "call_external_blocker" {
    type command

    executable "/usr/local/bin/firewall-block"
    args [network.ip, severity]

    timeout 3s
}
```

---

# 15. Adding a New Detector Without Changing App Code

TCPGuard should support detector plugins.

## Option 1: DSL-Based Detector

```hsl
detector "sensitive-endpoint-detector" {
    input request

    finding "sensitive_endpoint_access" when {
        any {
            request.path matches "/admin/*"
            request.path matches "/api/v1/reports/export"
            request.path matches "/api/v1/users/*/permissions"
        }
    }

    output {
        field endpoint.sensitive true
        field endpoint.sensitivity_score 30
    }
}
```

---

## Option 2: External Detector via HTTP

```hsl
detector "ml-risk-detector" {
    type http

    endpoint "http://127.0.0.1:9090/score"
    method POST

    send {
        include request
        include user
        include session
        include network
        include business
    }

    receive {
        map "$.risk_score" to ml.risk_score
        map "$.label" to ml.label
        map "$.confidence" to ml.confidence
    }

    timeout 20ms
    fallback allow
}
```

Then use it:

```hsl
rule "ml-high-risk-request" {
    trigger {
        on request.received
    }

    when {
        ml.risk_score greater_than 85
    }

    risk {
        base 70
        add ml.risk_score scaled 0.3
    }

    actions {
        high {
            run throttle
            run notify_admin
        }

        critical {
            run block
        }
    }
}
```

---

# 16. Adding New Data Fields Without Code Change

Use context enrichment.

```hsl
enricher "office-branch-mapper" {
    type lookup

    source file "./data/office_branches.csv"

    key network.ip_prefix

    output {
        map "branch_id" to business.branch.id
        map "branch_name" to business.branch.name
        map "region" to business.branch.region
    }
}
```

Then rules can use:

```hsl
when {
    business.branch.region not_equals user.assigned_region
}
```

---

# 17. Adding New Threat Intelligence Without Code Change

```hsl
intel "bad-ip-feed" {
    type file
    path "./intel/bad_ips.txt"
    refresh 10m

    match network.ip

    output {
        field network.ip.blacklisted true
        field network.ip.reputation 90
    }
}
```

```hsl
intel "tor-exit-feed" {
    type http
    url "https://example.local/tor-exit-nodes.txt"
    refresh 1h

    match network.ip

    output {
        field network.ip.tor true
        field network.ip.reputation 80
    }
}
```

Rule usage:

```hsl
rule "block-blacklisted-ip" {
    trigger {
        on request.received
    }

    when {
        network.ip.blacklisted equals true
    }

    risk {
        base 95
    }

    actions {
        critical {
            run block
            run audit
        }
    }
}
```

---

# 18. Baseline and Usage-Aware Rules

TCPGuard should support learned baselines.

```hsl
baseline "user-normal-login-hours" {
    entity user.id
    observe auth.login_success

    fields {
        hour timestamp.hour
        country network.country
        device session.device.id
    }

    window 30d
    min_samples 20
}
```

Rule:

```hsl
rule "unusual-login-hour" {
    trigger {
        on auth.login_success
    }

    when {
        baseline.user-normal-login-hours.hour_zscore greater_than 3
    }

    risk {
        base 30
        add 30 when session.device.new equals true
    }

    actions {
        medium {
            run audit
        }

        high {
            run mfa_challenge
        }
    }
}
```

---

# 19. Threat Model Mapping

```hsl
threat_model "stride-default" {
    category spoofing {
        findings [
            "session_hijack",
            "api_key_forged",
            "invalid_signature",
            "token_reuse"
        ]
    }

    category tampering {
        findings [
            "body_hash_mismatch",
            "signature_mismatch",
            "header_tampering"
        ]
    }

    category repudiation {
        findings [
            "missing_audit_context",
            "unsigned_critical_action"
        ]
    }

    category information_disclosure {
        findings [
            "large_export",
            "unusual_download",
            "sensitive_endpoint_access"
        ]
    }

    category denial_of_service {
        findings [
            "ddos",
            "slowloris",
            "rate_limit_abuse"
        ]
    }

    category elevation_of_privilege {
        findings [
            "role_change_anomaly",
            "admin_endpoint_abuse",
            "privilege_escalation"
        ]
    }
}
```

---

# 20. Policy Decision Model

TCPGuard should separate:

```txt
Finding      = What happened
Risk         = How dangerous it is
Severity     = How serious it is
Policy       = What should be done
Action       = How to execute response
Audit        = Why decision was made
```

Example decision:

```txt
finding:
  session_country_changed
  new_device
  after_hours_admin_access

risk:
  score = 91
  confidence = 0.88

severity:
  critical

policy:
  block request
  revoke session
  create incident

actions:
  block
  revoke_session
  notify_soc
  audit

explanation:
  admin used new device from new country outside business hours
```

---

# 21. Recommended Internal Go Interfaces

```go
type Engine interface {
    Evaluate(ctx *Context, event Event) Decision
}

type Trigger interface {
    ID() string
    Match(ctx *Context, event Event) bool
}

type Rule interface {
    ID() string
    Evaluate(ctx *Context, event Event) RuleResult
}

type Detector interface {
    ID() string
    Detect(ctx *Context) ([]Finding, error)
}

type Scorer interface {
    Score(ctx *Context, findings []Finding) Risk
}

type Action interface {
    ID() string
    Execute(ctx *Context, decision Decision) error
}

type Enricher interface {
    ID() string
    Enrich(ctx *Context) error
}

type Store interface {
    Get(key string) ([]byte, error)
    Set(key string, value []byte, ttl time.Duration) error
    Delete(key string) error
}
```

---

# 22. Hot Reload Architecture

To add rules without code changes:

```txt
1. Write new .hsl file
2. Place it in /rules
3. TCPGuard watcher detects change
4. Parser validates syntax
5. Schema validates semantics
6. Simulator checks safety
7. Policy engine compiles rule
8. Rule becomes active
9. Old version remains rollbackable
```

Suggested flow:

```txt
/rules
  after_hours_admin.hsl
  impossible_travel.hsl
  suspicious_export.hsl

/actions
  notify_admin.hsl
  publish_security_event.hsl

/triggers
  high_value_payment.hsl

/intel
  bad_ips.hsl
  tor_exit_nodes.hsl

/enrichers
  office_branch_mapper.hsl
```

---

# 23. Safety Controls for No-Code Extensibility

To prevent dangerous policies:

```txt
- Rule validation
- Action allowlist
- Max action timeout
- Max retry count
- Max rule execution time
- Max memory usage
- Sandbox custom expressions
- Signed rule bundles
- Role-based rule publishing
- Approval workflow
- Dry-run mode
- Shadow mode
- Canary rollout
- Rollback
- Test fixtures
```

Example:

```hsl
policy_safety {
    max_rule_eval_time 2ms
    max_actions_per_rule 10
    max_webhook_timeout 5s
    require_signature true
    require_approval_for ["block", "disable_user", "ban_ip"]
}
```

---

# 24. Rule Lifecycle

```hsl
rule "new-risk-rule" {
    status shadow
    owner "security-team"
    version 1

    rollout {
        mode percentage
        value 10
    }

    approval {
        required true
        approvers ["security-admin", "platform-owner"]
    }

    expires never
}
```

Lifecycle states:

```txt
draft
shadow
testing
active
paused
deprecated
archived
```

---

# 25. Complete Example: Enterprise Bank Protection Pack

```hsl
pack "banking-protection-pack" {
    version "1.0.0"

    include rule "after-hours-admin-access"
    include rule "impossible-travel"
    include rule "signed-request-replay-detection"
    include rule "suspicious-export-after-risky-login"
    include rule "adaptive-api-abuse"

    defaults {
        mode enforce
        audit tamper_evident
        risk_decay 24h
    }
}
```

---

# 26. Final Product Architecture

```txt
TCPGuard
 ├─ Middleware Layer
 │   ├─ GoFiber middleware
 │   ├─ net/http adapter
 │   └─ gRPC gateway adapter
 │
 ├─ Event Layer
 │   ├─ request events
 │   ├─ auth events
 │   ├─ session events
 │   ├─ business events
 │   └─ system events
 │
 ├─ Context Layer
 │   ├─ request context
 │   ├─ identity context
 │   ├─ session context
 │   ├─ network context
 │   ├─ geo context
 │   └─ business context
 │
 ├─ Intelligence Layer
 │   ├─ GeoIP
 │   ├─ IP reputation
 │   ├─ ASN reputation
 │   ├─ Tor/VPN/proxy
 │   ├─ custom feeds
 │   └─ local offline DB
 │
 ├─ Detection Layer
 │   ├─ built-in detectors
 │   ├─ DSL detectors
 │   ├─ external ML detectors
 │   └─ plugin detectors
 │
 ├─ Rule Layer
 │   ├─ HSL parser
 │   ├─ trigger engine
 │   ├─ condition engine
 │   ├─ sequence engine
 │   ├─ baseline engine
 │   └─ rule compiler
 │
 ├─ Risk Layer
 │   ├─ risk score
 │   ├─ confidence score
 │   ├─ severity mapping
 │   ├─ risk decay
 │   └─ entity profiles
 │
 ├─ Threat Model Layer
 │   ├─ STRIDE mapping
 │   ├─ MITRE mapping
 │   ├─ attack chain detection
 │   └─ kill-chain timeline
 │
 ├─ Policy Layer
 │   ├─ policy decision
 │   ├─ tenant overrides
 │   ├─ endpoint overrides
 │   ├─ role overrides
 │   └─ emergency lockdown
 │
 ├─ Action Layer
 │   ├─ enforcement actions
 │   ├─ identity actions
 │   ├─ network actions
 │   ├─ notification actions
 │   ├─ incident actions
 │   └─ integration actions
 │
 ├─ Storage Layer
 │   ├─ memory
 │   ├─ Redis
 │   ├─ SQLite
 │   ├─ BadgerDB
 │   ├─ Postgres
 │   └─ custom store
 │
 ├─ Audit Layer
 │   ├─ structured logs
 │   ├─ tamper-evident logs
 │   ├─ decision traces
 │   ├─ config hash
 │   └─ policy version
 │
 ├─ Integration Layer
 │   ├─ webhook
 │   ├─ SIEM
 │   ├─ NATS
 │   ├─ Kafka
 │   ├─ email
 │   ├─ SMS
 │   └─ custom sink
 │
 └─ Management Layer
     ├─ dashboard
     ├─ CLI
     ├─ policy simulator
     ├─ rule tester
     ├─ false-positive review
     ├─ incident viewer
     └─ compliance reports
```

---

# 27. Best Final Design Principle

TCPGuard should follow this model:

```txt
Trigger decides when to evaluate.
Condition decides whether a rule matches.
Detector finds suspicious facts.
Risk decides how dangerous it is.
Severity decides how serious it is.
Policy decides what should happen.
Action performs the response.
Audit explains why it happened.
```

Final positioning:

> **TCPGuard is an extensible, HSL-configurable, business-aware security middleware and risk detection platform for Go applications, capable of detecting threats, modeling risk, enforcing policies, and automating response without requiring code changes for new rules, triggers, enrichers, threat intel, or actions.**
