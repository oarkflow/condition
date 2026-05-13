# Decision Platform Threat Model

## Assets

- Published decision packages and package digests.
- Decision requests, facts, candidate data, and results.
- Audit envelopes and hash-chain continuity.
- Package tests, simulation inputs, and governance metadata.
- Tenant or environment boundaries supplied by the host application.

## Primary Risks

- Unauthorized package publishing can change production decisions.
- Policy bypass can occur if a server is exposed without an `Authorizer` or upstream authentication.
- Payload abuse can exhaust memory or CPU through oversized JSON/BCL bodies or expensive simulations.
- Audit tampering can hide changed inputs, results, or package digests.
- Multi-tenant deployments can leak package metadata or audit records without tenant-aware stores and authorizers.

## Built-In Controls

- `DecisionServerConfig` adds request body limits, per-request timeout, request IDs, and optional content-type enforcement.
- `NewProductionDecisionServer` rejects unsafe production wiring such as memory-backed storage, missing authorization, missing body limits, missing request timeout, or disabled JSON content-type enforcement.
- `Authorizer` and `Principal` enforce endpoint permissions when configured. Default behavior is deny for missing principals once an authorizer is installed.
- `NewAuthzAuthorizerFromFile` loads reviewed `.authz` policy files through `github.com/oarkflow/authz` and adapts them to the stable `Authorizer` interface.
- `AuditEnvelope` records sequence, previous hash, payload hash, and chain hash. Use `VerifyAuditChain` to detect missing links or payload edits.
- `SQLiteDecisionStore` provides single-node durable storage for packages, latest package resolution, audit records, audit envelopes, and simulations.
- `DecisionStore` splits durable storage behavior behind interfaces. `StoreConformanceSuite` verifies versioning, latest resolution, audit ordering, simulation writes, reopen persistence, and concurrent access.
- BCL parser diagnostics include filename, line, column, and nearest token.

## Deployment Requirements

- Use `NewSQLiteDecisionStore` or another durable `DecisionStore` implementation for production. `MemoryDecisionStore` is intentionally non-durable and intended for tests, examples, and local development.
- Put authentication in middleware or context injection and configure `WithDecisionServerAuthorizer`.
- Configure load balancer and `http.Server` read/write/idle timeouts.
- Keep `MaxBodyBytes` small for synchronous endpoints; use batch infrastructure for large simulations.
- Verify audit chains during export, archival, incident review, and store migrations.
- Use tenant-aware keys, authorization checks, and audit queries in shared environments.
- Treat the bundled production server example as the single-node baseline. Multi-node or SaaS deployments should add an external database, stronger tenant indexes, centralized identity, rate limiting, and operational backup/restore procedures.
