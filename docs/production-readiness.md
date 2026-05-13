# Production Readiness

`condition` is production-ready in layers. Treat the layer boundary as an engineering contract.

## Readiness Matrix

| Layer | Status | What is included | What the host must provide |
| --- | --- | --- | --- |
| Core Go decision/routing engine | Ready for production integration | Compiled expressions, rule sets, ranker, bytecode runtime, file loaders, package validation, tests, benchmarks | Domain tests, package review, release gates |
| Single-node decision intelligence service | Ready with included production profile | SQLite `DecisionStore`, production server constructor, auth hooks, audit envelopes, Docker/Kubernetes example | Token/identity middleware, TLS, monitoring, backups, operational runbooks |
| Multi-tenant SaaS platform | Extension target | Tenant fields, authorizer seam, store interface, audit chain primitives | Tenant-scoped durable store, external IdP/OIDC, tenant-isolation tests, migrations, HA database |

## Included Production Primitives

- `NewSQLiteDecisionStore(path)` for durable single-node package, audit, and simulation storage.
- `StoreConformanceSuite` for custom store implementations.
- `ValidateProductionDecisionPackage` for validation, compile, digest, and embedded package tests.
- `NewProductionDecisionServer` for rejecting unsafe server configuration.
- `NewAuthzAuthorizerFromFile` for reviewed `.authz` policy files backed by `github.com/oarkflow/authz`.
- `NewAuthzAuthorizer` for callers that already construct an authz engine.
- `VerifyAuditChain` for tamper-evident audit envelope verification.

## Not Included As Turnkey Infrastructure

- Hosted multi-tenant control plane.
- Managed identity provider or OIDC middleware.
- PostgreSQL store implementation.
- Cross-node audit sequencing.
- Backup/restore automation.
- Compliance certification for regulated autonomous decisions.

## Recommended First Production Shape

Use the production server example for a single-node deployment:

```sh
go run ./examples/production_readiness_catalog
docker build -f examples/production_decision_server/Dockerfile -t production-decision-server:local .
docker run --rm \
  -e DECISION_ADMIN_TOKEN=change-me \
  -e DECISION_SQLITE_PATH=/data/decisions.db \
  -v "$PWD/.data:/data" \
  -p 8080:8080 \
  production-decision-server:local
```

`examples/production_readiness_catalog` is the shortest executable proof of the production baseline: file-backed package, file-backed `.authz`, JSON request payload, JSON candidate dataset, SQLite persistence, production validation, HTTP evaluation, audit chain verification, and reopen durability.

Then publish/evaluate packages through `/v1/*` endpoints with a bearer token and verify audit continuity through `/audit/verify`.
