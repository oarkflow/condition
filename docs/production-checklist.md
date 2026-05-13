# Production Checklist

This checklist is the release gate for using `condition` as a production Go decision/routing engine or a single-node decision intelligence service.

## Core Library Gate

- Run `go test ./...`.
- Run BCL fuzzing for changed parser/loader code:

```sh
go test ./bcl -fuzz FuzzParsePackage -fuzztime 30s
```

- Run BCL and hot-path benchmarks before performance-sensitive releases:

```sh
go test ./bcl -bench 'BenchmarkBCL|BenchmarkAppend'
go test -bench 'BenchmarkRanker|BenchmarkBytecode'
```

- Validate every decision package before publish:

```go
result, err := condition.ValidateProductionDecisionPackage(ctx, pkg)
```

- Keep package files in versioned durable storage. Local files are a deployment transport, not an audit store.

## Single-Node Platform Gate

- Use `NewSQLiteDecisionStore` or another durable `DecisionStore`; never use `MemoryDecisionStore` in production.
- Run `StoreConformanceSuite` against custom stores.
- Create servers with `NewProductionDecisionServer` so unsafe production settings are rejected.
- Configure:
  - `MaxBodyBytes`
  - request timeout
  - content-type enforcement
  - `Authorizer`
  - `DecisionLogger`
  - `DecisionMetrics`
  - upstream TLS and rate limiting
- Persist every evaluation audit envelope and alert on audit persistence failures.
- Verify audit chains during export, archival, incident review, and store migration:

```go
envelopes, _ := store.ListAuditEnvelopes(ctx)
err := condition.VerifyAuditChain(envelopes)
```

## Authorization Gate

- Install middleware that authenticates callers and adds `Principal` to the request context.
- Use `NewAuthzAuthorizerFromFile` with a reviewed `.authz` policy file for `github.com/oarkflow/authz` integration.
- Keep the `.authz` file in source control, validate it in release checks, and mount it as config in container deployments.
- Scope authorizations by permission and resource:
  - package publish/list/read
  - decision evaluate
  - package simulate/compare/test
  - audit read
- For multi-tenant deployments, include tenant identity in `Principal.Tenant` and store keys/queries.

## Live Runtime Gate

- Use atomic writes for package files.
- Enable validation and embedded package tests before publish.
- Monitor rejected reload events.
- Keep last-known-good runtime enabled.

## Example Commands

```sh
go test ./...
go test -run TestSQLiteDecisionStoreConformance
go run ./examples/production_readiness_catalog
go test ./bcl -fuzz FuzzParsePackage -fuzztime 30s
go test ./bcl -bench 'BenchmarkBCL|BenchmarkAppend'
docker build -f examples/production_decision_server/Dockerfile -t production-decision-server:local .
kubectl apply -f examples/production_decision_server/k8s.yaml
```
