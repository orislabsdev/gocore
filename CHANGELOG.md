# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.6] - 2026-04-12

### Added
- Implemented `http.Hijacker` in `responseWriter` to support protocol upgrades (e.g. WebSockets).

### Changed
- Updated `.gitignore` to exclude `issue.md`.

## [0.5.5] - 2026-03-31

### Refactored
- Improved error handling and variable usage in core components.
- Aligned indentation of WebSocket error variables for consistency.
- Cleaned up whitespace formatting in OpenAPI structs and response handlers.

## [0.5.4] - 2026-03-31

### Added
- Added leeway support to JWT validation.

### Changed
- Upgraded Go to `1.25.1`.
- Configured `GOTOOLCHAIN` to resolve `covdata` bug in CI.
- Refactored `golangci-lint` configuration and upgraded `golangci-lint-action` to v7.

## [0.5.3] - 2026-03-31

### Refactored
- Standardized response handling and updated associated router tests.

## [0.5.2] - 2026-03-30

### Added
- `NewUpgrader` and `NewDefaultUpgrader` constructors for WebSocket configuration.

### Changed
- Updated documentation and example for WebSocket configuration and expansion.

## [0.5.1] - 2026-03-30

### Changed
- Changed the default host to "127.0.0.1" instead of "0.0.0.0".

### Upgraded
- Upgraded to Go `1.25.0`
- Upgraded `github.com/golang-jwt/jwt/v5` from `v5.2.1` to `v5.3.1`
- Upgraded `github.com/prometheus/common` from `v0.66.1` to `v0.67.5`
- Upgraded `github.com/prometheus/procfs` from `v0.16.1` to `v0.20.1`
- Upgraded `go.yaml.in/yaml/v2` from `v2.4.2` to `v2.4.4`
- Upgraded `golang.org/x/sys` from `v0.35.0` to `v0.42.0`
- Upgraded `golang.org/x/time` from `v0.5.0` to `v0.15.0`
- Upgraded `google.golang.org/protobuf` from `v1.36.8` to `v1.36.11`

## [0.5.0] - 2026-03-30

### Added
- Native, zero-dependency `websocket` package implementing RFC 6455.
- `websocket.Upgrader` for performing HTTP-to-WebSocket protocol upgrades on standard routes.
- `websocket.Conn` for framing and masking WebSocket data payloads.
- Added `/ws` echo endpoint example to the demonstration application.

## [0.4.0] - 2026-03-30

### Added
- Implemented native, zero-dependency OpenAPI v3.0 specification generation.
- Added route metadata builder methods (`Summary`, `Description`, `Tags`, `Body`, `Returns`) to `router.Route`.
- Added `Routes()` introspection method to `router.Router`.
- Added `openapi` package to automatically generate JSON schemas from Go structs via reflection.
- Added `builtin.SwaggerUI` handler to natively serve the Swagger UI single-page application.

## [0.3.0] - 2026-03-29

### Added
- Integrated `go-redis/v9` as a dependency for distributed token-bucket rate limiting.
- Abstracted the `RateLimit` middleware to allow pluggable backends (`Provider: "memory" | "redis"`).
- Added `RedisConfig` to the global configuration struct.

## [0.2.0] - 2026-03-28

### Added
- Integrated Prometheus metrics exporter (`builtin.Prometheus()`).
- Added Prometheus middleware (`middleware.Prometheus()`) with cardinality protection for dynamic routes.
- Exposed matched route pattern in router for downstream monitoring.

## [0.1.0] - 2026-03-18

### Added
- Beta release of the core library.
- Initial project structure.
- Trie-based router with groups and params.
- JWT authentication manager.
- Middleware: Logger, Recovery, Security, CORS, RateLimit.
- Request/Response context helpers.
- Built-in health and metrics handlers.
- Simplified validation package.
- Graceful shutdown support.
- Professional documentation: `SECURITY.md`, `CONTRIBUTING.md`.
- GitHub Actions CI for automated testing.
