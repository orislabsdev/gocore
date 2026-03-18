package router

import (
	"strings"

	"github.com/orislabsdev/gocore/handler"
)

// Group is a collection of routes that share a URL prefix and a set of
// middleware. Groups can be nested arbitrarily deep.
//
// Example:
//
//	api := router.Group("/api/v1")
//	api.Use(middleware.Auth(...))
//
//	users := api.Group("/users")
//	users.GET("",        listUsers)   // GET  /api/v1/users
//	users.POST("",       createUser)  // POST /api/v1/users
//	users.GET("/:id",    getUser)     // GET  /api/v1/users/:id
//	users.PUT("/:id",    updateUser)  // PUT  /api/v1/users/:id
//	users.DELETE("/:id", deleteUser)  // DELETE /api/v1/users/:id
type Group struct {
	router     *Router
	prefix     string
	middleware []handler.MiddlewareFunc
}

// newGroup constructs a Group. It is package-private; callers use
// Router.Group() or Group.Group().
func newGroup(r *Router, prefix string, mw []handler.MiddlewareFunc) *Group {
	return &Group{
		router:     r,
		prefix:     cleanPrefix(prefix),
		middleware: mw,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Middleware
// ─────────────────────────────────────────────────────────────────────────────

// Use appends middleware to the group. This middleware runs after global router
// middleware and before any route-specific middleware.
//
// Middleware added after route registration still applies to previously
// registered routes because the chain is assembled at request time.
func (g *Group) Use(mw ...handler.MiddlewareFunc) *Group {
	g.middleware = append(g.middleware, mw...)
	return g
}

// ─────────────────────────────────────────────────────────────────────────────
// Route registration
// ─────────────────────────────────────────────────────────────────────────────

// Handle registers a handler for the given method and path within this group.
// The route-specific middleware from this group is merged into the route entry.
func (g *Group) Handle(method, path string, h handler.HandlerFunc) *Route {
	fullPath := joinPath(g.prefix, path)
	route := g.router.Handle(method, fullPath, h)

	// Attach the group's accumulated middleware to the route so it is applied
	// in the correct order at request time.
	if len(g.middleware) > 0 && route.node.handlers[strings.ToUpper(method)] != nil {
		entry := route.node.handlers[strings.ToUpper(method)]
		// Prepend group middleware so it runs before any route-specific middleware
		// added via route.Use().
		combined := make([]handler.MiddlewareFunc, 0, len(g.middleware)+len(entry.middleware))
		combined = append(combined, g.middleware...)
		combined = append(combined, entry.middleware...)
		entry.middleware = combined
	}

	return route
}

// GET registers a GET handler within this group.
func (g *Group) GET(path string, h handler.HandlerFunc) *Route {
	return g.Handle("GET", path, h)
}

// POST registers a POST handler within this group.
func (g *Group) POST(path string, h handler.HandlerFunc) *Route {
	return g.Handle("POST", path, h)
}

// PUT registers a PUT handler within this group.
func (g *Group) PUT(path string, h handler.HandlerFunc) *Route {
	return g.Handle("PUT", path, h)
}

// PATCH registers a PATCH handler within this group.
func (g *Group) PATCH(path string, h handler.HandlerFunc) *Route {
	return g.Handle("PATCH", path, h)
}

// DELETE registers a DELETE handler within this group.
func (g *Group) DELETE(path string, h handler.HandlerFunc) *Route {
	return g.Handle("DELETE", path, h)
}

// OPTIONS registers an OPTIONS handler within this group.
func (g *Group) OPTIONS(path string, h handler.HandlerFunc) *Route {
	return g.Handle("OPTIONS", path, h)
}

// HEAD registers a HEAD handler within this group.
func (g *Group) HEAD(path string, h handler.HandlerFunc) *Route {
	return g.Handle("HEAD", path, h)
}

// ─────────────────────────────────────────────────────────────────────────────
// Sub-groups
// ─────────────────────────────────────────────────────────────────────────────

// Group creates a nested sub-group with an additional path prefix.
// The sub-group inherits a copy of the parent group's middleware, so adding
// middleware to the parent after creating a sub-group does not affect the
// sub-group.
func (g *Group) Group(prefix string) *Group {
	// Copy parent middleware so child changes don't affect the parent.
	mwCopy := make([]handler.MiddlewareFunc, len(g.middleware))
	copy(mwCopy, g.middleware)

	return newGroup(g.router, joinPath(g.prefix, prefix), mwCopy)
}

// ─────────────────────────────────────────────────────────────────────────────
// Path helpers
// ─────────────────────────────────────────────────────────────────────────────

// cleanPrefix ensures the prefix starts with "/" and does not end with "/"
// (unless it is the root "/").
func cleanPrefix(prefix string) string {
	if prefix == "" {
		return "/"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if prefix != "/" {
		prefix = strings.TrimRight(prefix, "/")
	}
	return prefix
}

// joinPath concatenates a group prefix and a route path, ensuring exactly
// one slash between them.
func joinPath(prefix, path string) string {
	if path == "" || path == "/" {
		return prefix
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if prefix == "/" {
		return path
	}
	return strings.TrimRight(prefix, "/") + path
}
