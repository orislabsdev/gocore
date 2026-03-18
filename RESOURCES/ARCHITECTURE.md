# gocore Architecture

## Core Philosophy

`gocore` is built on the principle of **Transparent Engineering**. We believe that a backend library should provide structure without obscuring the underlying mechanics of the Go standard library.

## Component Design

### 1. Unified Configuration (`config/`)
All configuration is centralized and versioned. We avoid global state and environment variable magic. The `config.Default()` function provides a "secure by default" starting point.

### 2. The Trie Router (`router/`)
Our router uses a custom Trie (prefix tree) implementation. 
- **Efficiency**: O(L) lookup where L is the length of the path.
- **Support**: Handles static routes, named parameters (`:id`), and wildcards (`*path`).
- **Isolation**: Each route group can have its own middleware stack, preventing unintended side effects.

### 3. Middleware Pipeline (`middleware/`)
Middleware in `gocore` follows the standard decorator pattern: `func(next HandlerFunc) HandlerFunc`.
The order of execution is deterministic:
`Request -> Middleware 1 -> Middleware 2 -> ... -> Handler -> ... -> Middleware 2 -> Middleware 1 -> Response`

### 4. Application Lifecycle (`core.go`)
The `Core` type bridges all components. It manages the server lifecycle, including:
- **Initialization**: Linking router, logger, and server.
- **Signal Handling**: Catching `SIGINT`/`SIGTERM` for graceful shutdown.
- **Dependency Draining**: Closing the `done` channel to notify all background tasks (like rate limiter cleanup) to exit cleanly.

## Security Architecture

We follow the **Defense in Depth** strategy:
- **Transport Layer**: Enforces TLS configurations.
- **Application Layer**: Structured JWT management with multi-source extraction.
- **Perimeter Layer**: Context-aware rate limiting and security headers middleware.

## Trade-offs

- **Memory vs. Performance**: The Trie uses more memory than a simple map for routes but offers far superior performance for complex URL patterns.
- **Explicit vs. Implicit**: `gocore` requires explicit route registration and middleware assignment. This slightly increases the initial line count but eliminates "why is this happening?" debugging sessions.
