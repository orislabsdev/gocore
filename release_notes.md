# Release Notes - v0.6.0

## Overview

---

Version **0.6.0** introduces a key feature for `gocore` as a package; most backends need to serve files such as images and documents securely, so fully functional features have been added for this purpose. 

## Highlights

---

### 🚀 File Server Support

We have implemented `Static` to serve files from disk and `StaticFS` for files embedded in the `gocore` module for your convenience. 

**Why this matters:**

In previous versions, it was necessary to use `http.FileServer`, which is deprecated and violated the standard set by `gocore`. As a result, integration with the router's wildcards was lost, forcing developers to write custom file-serving code that could cause security issues and other problems. 

Now the implementation can be carried out in a simple way. 

```go
app.GET("/api/images/*filepath", builtin.ServeDir("./uploads"))
```

Or, for a more convenient API, we recommend

```go
app.GET("/api/images", app.Static("./uploads"))
```



### Upgrade Guide

---

This is a non-breaking change; existing code works correctly without these implementations. 

## Full Changelog

---



### Fixed

- The `search()` function has been fixed; it previously did not allow empty segments in the path and broke support for `/*filepath`, since it returned a **404** error when no path was present.



### Added

- **Built-in**: The built-in `ServeDir()` and `ServeFS()` handlers have been implemented.
- **Core:** `Static()` and `StaticFS()` have been implemented for convenience.

---

For a complete history of changes, see the [CHANGELOG.md](CHANGELOG.md).