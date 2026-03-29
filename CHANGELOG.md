# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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

## [0.2.0] - 2026-03-28

### Added
- Integrated Prometheus metrics exporter (`builtin.Prometheus()`).
- Added Prometheus middleware (`middleware.Prometheus()`) with cardinality protection for dynamic routes.
- Exposed matched route pattern in router for downstream monitoring.

## [0.1.0] - 2026-03-18

### Added
- Beta release of the core library.
