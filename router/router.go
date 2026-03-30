// Package router provides a high-performance, trie-based HTTP router for the
// gocore library.
//
// # Design
//
// Routes are stored in a per-method trie (one per HTTP verb). Each node of the
// trie represents one URL path segment. During lookup the router walks the trie
// from the root, trying static children first (O(1) hash lookup), then the
// single parameter child, then the wildcard child. This gives O(depth)
// matching — proportional to the number of segments in the path, not the
// total number of registered routes.
//
// # Path syntax
//
//   - Static segment    /users/profile
//   - Named parameter   /users/:id          → Params{"id": "42"}
//   - Wildcard          /files/*path        → Params{"path": "a/b/c.txt"}
//
// # Public vs private routes
//
// Each route carries an IsPublic flag. The router exposes this flag to the
// server so that the JWT authentication middleware can be bypassed for public
// endpoints without requiring separate route registration or path-prefix tricks.
package router

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/orislabsdev/gocore/handler"
	"github.com/orislabsdev/gocore/middleware"
)

// ─────────────────────────────────────────────────────────────────────────────
// Route entry
// ─────────────────────────────────────────────────────────────────────────────

// ResponsePair represents an expected response for OpenAPI documentation.
type ResponsePair struct {
	Code        int
	Description string
	Example     any
}

// routeEntry holds the handler and metadata for a single method on a trie node.
type routeEntry struct {
	handler     handler.HandlerFunc
	middleware  []handler.MiddlewareFunc
	isPublic    bool
	name        string
	pattern     string // e.g. /users/:id
	summary     string
	description string
	tags        []string
	requestBody any            // Optional struct for input JSON
	responses   []ResponsePair // Documented responses
}

// ─────────────────────────────────────────────────────────────────────────────
// Trie node
// ─────────────────────────────────────────────────────────────────────────────

// trieNode is one segment in the routing trie.
type trieNode struct {
	segment  string // literal segment value for static nodes
	children map[string]*trieNode

	// paramChild is the single child node that matches any segment via :name.
	paramChild *trieNode
	paramName  string // the :name part (without the colon)

	// wildChild matches the remainder of the path via *name.
	wildChild *trieNode
	wildName  string // the *name part (without the asterisk)

	// handlers is the per-method map. nil if no route is registered at this node.
	handlers map[string]*routeEntry // HTTP method → entry
}

func newTrieNode(segment string) *trieNode {
	return &trieNode{segment: segment}
}

// ─────────────────────────────────────────────────────────────────────────────
// Match result
// ─────────────────────────────────────────────────────────────────────────────

// MatchResult is returned by Router.Match and carries everything needed to
// dispatch the request.
type MatchResult struct {
	Handler    handler.HandlerFunc
	Middleware []handler.MiddlewareFunc
	Params     handler.Params
	IsPublic   bool
	Pattern    string
}

// ─────────────────────────────────────────────────────────────────────────────
// Router
// ─────────────────────────────────────────────────────────────────────────────

// Router dispatches incoming HTTP requests to registered handlers.
// It is safe for concurrent reads after all routes have been registered.
// Registering routes concurrently with serving requests is not supported.
type Router struct {
	// roots holds one trie root per HTTP method.
	roots map[string]*trieNode

	// globalMiddleware is applied to every request before route-specific
	// middleware and the actual handler.
	globalMiddleware []handler.MiddlewareFunc

	// notFound is called when no route matches the request path.
	notFound handler.HandlerFunc

	// methodNotAllowed is called when a path matches but not for the requested
	// method.
	methodNotAllowed handler.HandlerFunc
}

// New creates a Router with sensible default not-found and
// method-not-allowed handlers.
func New() *Router {
	return &Router{
		roots:            make(map[string]*trieNode),
		notFound:         defaultNotFound,
		methodNotAllowed: defaultMethodNotAllowed,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Route registration
// ─────────────────────────────────────────────────────────────────────────────

// Route represents a registered route. The fluent methods (Public, Private,
// Name) configure the route after registration.
type Route struct {
	node   *trieNode
	method string
}

// Public marks the route as publicly accessible.
// Public routes bypass the JWT authentication middleware.
func (r *Route) Public() *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.isPublic = true
	}
	return r
}

// Private marks the route as requiring authentication (this is the default).
func (r *Route) Private() *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.isPublic = false
	}
	return r
}

// Name assigns a human-readable name to the route (useful for logging and
// generating URLs from named routes in templates).
func (r *Route) Name(name string) *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.name = name
	}
	return r
}

// Summary assigns a brief summary to the route (used for OpenAPI docs).
func (r *Route) Summary(summary string) *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.summary = summary
	}
	return r
}

// Description assigns a verbose description to the route (used for OpenAPI docs).
func (r *Route) Description(desc string) *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.description = desc
	}
	return r
}

// Tags assigns OpenAPI logical tags to the route.
func (r *Route) Tags(tags ...string) *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.tags = append(entry.tags, tags...)
	}
	return r
}

// Body assigns a JSON request body type to the route for OpenAPI documentation.
// Provide a zero-value struct or pointer to a struct.
func (r *Route) Body(example any) *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.requestBody = example
	}
	return r
}

// Returns assigns an expected JSON response payload to the route for OpenAPI docs.
// Provide the HTTP status code, a description, and a zero-value struct for the payload.
func (r *Route) Returns(code int, desc string, example any) *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.responses = append(entry.responses, ResponsePair{
			Code:        code,
			Description: desc,
			Example:     example,
		})
	}
	return r
}

// Use appends middleware to this specific route. Route-level middleware runs
// after global middleware but before the handler.
func (r *Route) Use(mw ...handler.MiddlewareFunc) *Route {
	if entry, ok := r.node.handlers[r.method]; ok {
		entry.middleware = append(entry.middleware, mw...)
	}
	return r
}

// Handle registers a handler for the given HTTP method and path pattern.
// Panics if the same method+pattern pair is registered twice.
func (r *Router) Handle(method, pattern string, h handler.HandlerFunc) *Route {
	method = strings.ToUpper(method)

	root, ok := r.roots[method]
	if !ok {
		root = newTrieNode("/")
		r.roots[method] = root
	}

	segments := splitPath(pattern)
	leafNode := insert(root, segments)

	if leafNode.handlers == nil {
		leafNode.handlers = make(map[string]*routeEntry)
	}
	if _, exists := leafNode.handlers[method]; exists {
		panic(fmt.Sprintf("router: duplicate route %s %s", method, pattern))
	}
	entry := &routeEntry{handler: h, pattern: pattern}
	leafNode.handlers[method] = entry

	return &Route{node: leafNode, method: method}
}

// GET registers a handler for GET requests.
func (r *Router) GET(pattern string, h handler.HandlerFunc) *Route {
	return r.Handle(http.MethodGet, pattern, h)
}

// POST registers a handler for POST requests.
func (r *Router) POST(pattern string, h handler.HandlerFunc) *Route {
	return r.Handle(http.MethodPost, pattern, h)
}

// PUT registers a handler for PUT requests.
func (r *Router) PUT(pattern string, h handler.HandlerFunc) *Route {
	return r.Handle(http.MethodPut, pattern, h)
}

// PATCH registers a handler for PATCH requests.
func (r *Router) PATCH(pattern string, h handler.HandlerFunc) *Route {
	return r.Handle(http.MethodPatch, pattern, h)
}

// DELETE registers a handler for DELETE requests.
func (r *Router) DELETE(pattern string, h handler.HandlerFunc) *Route {
	return r.Handle(http.MethodDelete, pattern, h)
}

// OPTIONS registers a handler for OPTIONS requests.
func (r *Router) OPTIONS(pattern string, h handler.HandlerFunc) *Route {
	return r.Handle(http.MethodOptions, pattern, h)
}

// HEAD registers a handler for HEAD requests.
func (r *Router) HEAD(pattern string, h handler.HandlerFunc) *Route {
	return r.Handle(http.MethodHead, pattern, h)
}

// Any registers a handler for all standard HTTP methods.
func (r *Router) Any(pattern string, h handler.HandlerFunc) {
	for _, m := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodOptions, http.MethodHead,
	} {
		r.Handle(m, pattern, h)
	}
}

// Group creates a route group with a shared path prefix. See Group for details.
func (r *Router) Group(prefix string) *Group {
	return newGroup(r, prefix, nil)
}

// Use appends global middleware applied to every request.
// Global middleware must be registered before any requests are served.
func (r *Router) Use(mw ...handler.MiddlewareFunc) {
	r.globalMiddleware = append(r.globalMiddleware, mw...)
}

// NotFound sets a custom handler for requests that match no registered route.
func (r *Router) NotFound(h handler.HandlerFunc) { r.notFound = h }

// MethodNotAllowed sets a custom handler for requests where the path matches
// but the HTTP method does not.
func (r *Router) MethodNotAllowed(h handler.HandlerFunc) { r.methodNotAllowed = h }

// ─────────────────────────────────────────────────────────────────────────────
// Introspection
// ─────────────────────────────────────────────────────────────────────────────

// RouteInfo represents a fully configured route for external introspection (e.g., OpenAPI).
type RouteInfo struct {
	Method      string
	Pattern     string
	Name        string
	IsPublic    bool
	Summary     string
	Description string
	Tags        []string
	RequestBody any
	Responses   []ResponsePair
}

// Routes returns a slice of all registered routes and their associated metadata.
// Useful for generating automatic documentation.
func (r *Router) Routes() []RouteInfo {
	var list []RouteInfo

	var walk func(node *trieNode)
	walk = func(node *trieNode) {
		if node == nil {
			return
		}
		for method, entry := range node.handlers {
			list = append(list, RouteInfo{
				Method:      method,
				Pattern:     entry.pattern,
				Name:        entry.name,
				IsPublic:    entry.isPublic,
				Summary:     entry.summary,
				Description: entry.description,
				Tags:        entry.tags,
				RequestBody: entry.requestBody,
				Responses:   entry.responses,
			})
		}
		for _, child := range node.children {
			walk(child)
		}
		if node.paramChild != nil {
			walk(node.paramChild)
		}
		if node.wildChild != nil {
			walk(node.wildChild)
		}
	}

	for _, root := range r.roots {
		walk(root)
	}

	return list
}

// ─────────────────────────────────────────────────────────────────────────────
// Request dispatch (ServeHTTP)
// ─────────────────────────────────────────────────────────────────────────────

// ServeHTTP implements http.Handler. It matches the request, builds the
// middleware chain, and invokes the handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := handler.NewContext(w, req)
	defer handler.Release(ctx)

	result, found, methodExists := r.match(req.Method, req.URL.Path)

	var h handler.HandlerFunc
	switch {
	case !found && !methodExists:
		h = r.notFound
	case found && !methodExists:
		h = r.methodNotAllowed
	default:
		// Inject URL parameters into the request context.
		if len(result.Params) > 0 {
			ctx.Request = handler.SetParams(ctx.Request, result.Params)
		}

		// Inject matched pattern for metrics cardinality prevention.
		if result.Pattern != "" {
			ctx.Set("route_pattern", result.Pattern)
		}

		// Build the complete middleware chain:
		//   global middleware → route-specific middleware → handler
		h = middleware.Chain(result.Handler, append(r.globalMiddleware, result.Middleware...)...)
	}

	h(ctx)
}

// ─────────────────────────────────────────────────────────────────────────────
// Matching
// ─────────────────────────────────────────────────────────────────────────────

// match walks the trie for the given method and path.
// Returns:
//   - result   — populated when a full match is found.
//   - found    — true when the path matches any method's trie.
//   - methodOK — true when the path+method pair has a registered handler.
func (r *Router) match(method, path string) (result MatchResult, found, methodOK bool) {
	segments := splitPath(path)

	// Check whether this path exists for any method (needed for 405 detection).
	for m, root := range r.roots {
		params := make(handler.Params)
		entry, ok := search(root, segments, params)
		if !ok {
			continue
		}
		found = true
		if m == method {
			methodOK = true
			result = MatchResult{
				Handler:    entry.handler,
				Middleware: entry.middleware,
				Params:     params,
				IsPublic:   entry.isPublic,
				Pattern:    entry.pattern,
			}
		}
	}
	return
}

// ─────────────────────────────────────────────────────────────────────────────
// Trie operations
// ─────────────────────────────────────────────────────────────────────────────

// insert walks the trie for the given segments and returns the leaf node,
// creating nodes as needed.
func insert(node *trieNode, segments []string) *trieNode {
	if len(segments) == 0 {
		return node
	}

	seg := segments[0]
	rest := segments[1:]

	switch {
	case strings.HasPrefix(seg, "*"):
		// Wildcard — must be the last segment.
		if node.wildChild == nil {
			node.wildChild = newTrieNode(seg)
			node.wildName = seg[1:] // strip leading "*"
		}
		return node.wildChild

	case strings.HasPrefix(seg, ":"):
		// Named parameter.
		if node.paramChild == nil {
			node.paramChild = newTrieNode(seg)
			node.paramName = seg[1:] // strip leading ":"
		}
		return insert(node.paramChild, rest)

	default:
		// Static segment.
		if node.children == nil {
			node.children = make(map[string]*trieNode)
		}
		child, ok := node.children[seg]
		if !ok {
			child = newTrieNode(seg)
			node.children[seg] = child
		}
		return insert(child, rest)
	}
}

// search walks the trie for the given path segments, populating params as it
// goes. Returns the matching routeEntry and true on success.
func search(node *trieNode, segments []string, params handler.Params) (*routeEntry, bool) {
	// Base case: no more segments — check for a handler at this node.
	if len(segments) == 0 {
		// Check for any method's handler at this node.
		for _, entry := range node.handlers {
			return entry, true
		}
		return nil, false
	}

	seg := segments[0]
	rest := segments[1:]

	// Priority 1: exact static match.
	if node.children != nil {
		if child, ok := node.children[seg]; ok {
			if entry, found := search(child, rest, params); found {
				return entry, true
			}
		}
	}

	// Priority 2: named parameter child.
	if node.paramChild != nil {
		params[node.paramName] = seg
		if entry, found := search(node.paramChild, rest, params); found {
			return entry, true
		}
		delete(params, node.paramName) // undo if the sub-path didn't match
	}

	// Priority 3: wildcard child — greedily consumes all remaining segments.
	if node.wildChild != nil {
		params[node.wildName] = strings.Join(append([]string{seg}, rest...), "/")
		for _, entry := range node.wildChild.handlers {
			return entry, true
		}
		delete(params, node.wildName)
	}

	return nil, false
}

// ─────────────────────────────────────────────────────────────────────────────
// Utilities
// ─────────────────────────────────────────────────────────────────────────────

// splitPath splits a URL path into non-empty segments.
// "/users/:id/posts" → ["users", ":id", "posts"]
// "/"               → []
func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// defaultNotFound responds with 404.
var defaultNotFound handler.HandlerFunc = func(ctx *handler.Context) {
	ctx.NotFound("")
}

// defaultMethodNotAllowed responds with 405.
var defaultMethodNotAllowed handler.HandlerFunc = func(ctx *handler.Context) {
	ctx.Fail(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
}
