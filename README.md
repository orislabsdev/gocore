# gocore

> **Engineering-first HTTP backend library for Go.** 
> Built for performance, security, and developer productivity at Oris Labs.

[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://go.dev)
[![Go Report Card](https://goreportcard.com/badge/github.com/orislabsdev/gocore)](https://goreportcard.com/report/github.com/orislabsdev/gocore)
[![Coverage Status](https://img.shields.io/codecov/c/github/orislabsdev/gocore)](https://codecov.io/gh/orislabsdev/gocore)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Security Policy](https://img.shields.io/badge/security-policy-blue)](SECURITY.md)

---

## Why gocore?

In a world of "magic" frameworks, **gocore** takes a different approach. It provides a structured, production-ready foundation for Go services without hiding the standard library.

### Comparison: net/http vs. Popular Frameworks vs. gocore

| Feature | `net/http` | Gin / Echo | **gocore** |
| :--- | :---: | :---: | :---: |
| **Complexity** | Low | High (Magic) | **Medium (Transparent)** |
| **Router** | Basic (pre-1.22) | High-speed Radix | **High-speed Trie** |
| **Security** | Manual | Middleware-based | **Hardened Defaults** |
| **Boilerplate** | High | Low | **Low (Modular)** |
| **Standard Lib** | 100% | Replaces Context | **100% Compatible** |

**gocore** is designed for engineering teams who need to move fast but refuse to compromise on visibility, reliability, or security.

---

## Engineering Evidence

### Security Design & Threat Model

`gocore` is engineered with a multi-layered security approach:

1.  **Attack Surface Reduction**: Only 4 external dependencies (`jwt`, `x/time`, `prometheus`, `redis`). No bloated dependency trees.
2.  **Hardened Defaults**:
    - **HSTS**: Enforces HTTPS for 1 year by default.
    - **CSP**: Restrictive `default-src 'self'` policy.
    - **Slowloris Protection**: `ReadHeaderTimeout` set to 10s by default.
    - **mTLS**: Native support for client certificate verification.
3.  **Threat Model Mitigation**:
    - **Injection**: Path parameters are strictly parsed via Trie nodes.
    - **Brute Force**: Pluggable memory or Redis-backed token-bucket rate limiter per IP/Client.
    - **Token Hijacking**: JWT multi-source extraction (Header/Cookie) with TTL enforcement.
    - **Metrics Cardinality**: Prometheus metrics are protected against memory-exhaustion by tracking the underlying router pattern (e.g., `/users/:id`) instead of raw request URLs.

### Architecture & Tradeoffs

The `gocore` architecture is strictly acyclic (`config -> auth -> middleware -> router -> server -> core`). 

**Tradeoff: Trie vs. Radix Router**
We chose a **Trie-based router** over a Radix tree. While Radix trees can be slightly faster for massive routing tables, the Trie implementation provides **O(depth)** matching and significantly clearer code for debugging complex REST patterns with wildcards and path parameters.

### Benchmarks (O(depth) Performance)

Preliminary routing benchmarks indicate sub-microsecond matching latency for deep trees:

```text
BenchmarkRouter/Static-4         232.3 ns/op          64 B/op          2 allocs/op
BenchmarkRouter/Param-4          440.7 ns/op         400 B/op          3 allocs/op
BenchmarkRouter/Wildcard-4       621.9 ns/op         544 B/op          5 allocs/op
```

---

## Production Ready

### Simplified Setup

```go
app := gocore.New() // Starts with safe, hardened defaults
app.UseDefaults()    // RequestID, Recovery, Logger, Security, CORS, RateLimit

api := app.Group("/api/v1")
api.GET("/health", builtin.HealthCheck()).Public()

if err := app.Run(); err != nil {
    app.Logger().Fatal("server failed", "error", err)
}
```

### Strategic Roadmap (v0.x)

- [x] **v0.2.0**: Prometheus metrics exporter integration.
- [x] **v0.3.0**: Distributed rate limiting (Redis provider).
- [x] **v0.4.0**: Automatic OpenAPI (Swagger) documentation generation.
- [ ] **v0.5.0**: Websocket support.
- [ ] **v1.0.0**: Stable API release.

---

## Resources

- [Architecture Guide](RESOURCES/ARCHITECTURE.md)
- [Example Application](example/main.go)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

---

&copy; 2026 Oris Labs. Built by engineers, for engineers.
