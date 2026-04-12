# Release Notes - v0.5.6

## Overview

Version **v0.5.6** introduces a critical enhancement to the GoCore response handling system, ensuring full compatibility with protocol upgrades (such as WebSockets) across all environments.

## Highlights

### 🚀 HTTP Hijacker Support
We have implemented the `http.Hijacker` interface in our internal `responseWriter`. 

**Why this matters:**
Previously, when GoCore wrapped the standard `http.ResponseWriter` to provide status and size tracking, the underlying "hijack" capability was lost. This caused issues when trying to upgrade connections to WebSockets in environments that rely on this interface. With this update, the `Hijack()` method is now correctly exposed, allowing seamless protocol upgrades while maintaining our telemetry features.

## Full Changelog

### Added
- **Core**: Implemented `http.Hijack()` in `handler.responseWriter`.

### Changed
- **Project**: Updated `.gitignore` to exclude `issue.md`.

---

For a complete history of changes, see the [CHANGELOG.md](CHANGELOG.md).
