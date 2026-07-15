<!-- BEGIN:backend-skills -->
# Backend skills — use them

This repo ships project skills in `.agents/skills/`. Activate them proactively when the work matches:

## Golang

- **golang-code-style** — naming, formatting, idiomatic Go conventions
- **golang-concurrency** — goroutines, channels, sync primitives, patterns
- **golang-context** — context usage, cancellation, deadlines
- **golang-error-handling** — errors, wrapping, sentinel types, panic/recover
- **golang-structs-interfaces** — composition, embedding, interface design
- **golang-testing** — unit/integration tests, table-driven tests, fuzzing
- **golang-performance** — profiling, benchmarking, escape analysis, allocations
- **golang-security** — input validation, SQL injection, XSS, crypto, secrets
- **golang-database** — sql.DB, queries, transactions, migrations
- **golang-dependency-management** — go modules, vendoring, versioning
- **golang-project-layout** — package structure, standard layout conventions
- **golang-lint** — staticcheck, govet, golangci-lint configuration
- **golang-grpc** — protobuf, server/client implementation, interceptors
- **golang-graphql** — gqlgen, resolvers, schema design
- **golang-swagger** — OpenAPI specs, swagger-go, endpoint documentation
- **golang-observability** — logging, metrics, tracing, OpenTelemetry
- **golang-cli** — cobra, flags, argument parsing
- **golang-dependency-injection** — wire, dig, fx patterns
- **golang-modernize** — modern Go idioms, go1.23 features
- **golang-design-patterns** — common Go design patterns
- **golang-refactoring** — code transformation, restructuring
- **golang-safety** — memory safety, nil checks, race detection
- **golang-naming** — package/variable/function naming conventions
- **golang-benchmark** — benchmark writing, interpretation
- **golang-data-structures** — slices, maps, custom types
- **golang-documentation** — godoc, comments, examples
- **golang-continuous-integration** — CI/CD workflows
- **golang-troubleshooting** — debugging, profiling, common issues
- **golang-how-to** — idiomatic solutions to common problems

### Libraries & Frameworks

- **golang-spf13-cobra** — CLI framework
- **golang-spf13-viper** — configuration management
- **golang-stretchr-testify** — assertions and mocking
- **golang-google-wire** — compile-time dependency injection
- **golang-uber-dig** — runtime dependency injection
- **golang-uber-fx** — application framework
- **golang-gopls** — LSP server usage and configuration
- **golang-pkg-go-dev** — standard library reference
- **golang-popular-libraries** — ecosystem overview
- **golang-stay-updated** — keeping up with Go releases
- **golang-samber-lo** — generic utility library
- **golang-samber-mo** — monad library (option, result)
- **golang-samber-ro** — channel-based reactive streams
- **golang-samber-do** — functional programming helpers
- **golang-samber-oops** — structured error library
- **golang-samber-hot** — hot-reload utility
- **golang-samber-slog** — structured logging handler

## Supabase

- **supabase** — Database, Auth, Edge Functions, Realtime, Storage, Vectors, Cron, Queues, client libraries, SSR, CLI, MCP
- **supabase-postgres-best-practices** — query performance, schema design, RLS, connection pooling, indexing, monitoring

## Stack notes

- Go 1.23 with go-zero `rest` framework (no zRPC)
- All persistence goes through Supabase REST/PostgREST — never direct SQL
- Admin auth uses bcrypt passwords, SHA-256 session tokens, and optional bearer tokens
- Rate limiting uses Redis when available, falls back to in-memory counters
- The config file (`etc/portfolio-api.yaml`) resolves `${ENV}` placeholders at startup
- Migrations are additive SQL files in `migrations/` — apply in numeric order
<!-- END:backend-skills -->
