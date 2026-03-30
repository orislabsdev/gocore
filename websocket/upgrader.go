package websocket

import (
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/orislabsdev/gocore/handler"
)

const magicString = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// HandlerFunc is the function signature for a WebSocket handler.
//
// Like standard gocore handlers, it returns an error that can be processed
// by midway error handling.
type HandlerFunc func(c *handler.Context, conn *Conn) error

// Upgrader handles the HTTP-to-WebSocket protocol upgrade process.
type Upgrader struct {
	// CheckOrigin allows an application to custom-validate the Origin header.
	// If nil, defaults to checking that Origin matches the Host.
	// Returning true permits the upgrade, false rejects it with a 403.
	CheckOrigin func(r *http.Request) bool

	// ReadBufferSize and WriteBufferSize are the lengths of the buffers
	// used for reading and writing to the network.
	ReadBufferSize  int
	WriteBufferSize int
}

func defaultCheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := r.URL.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func (u *Upgrader) checkOrigin(r *http.Request) bool {
	if u.CheckOrigin != nil {
		return u.CheckOrigin(r)
	}
	return defaultCheckOrigin(r)
}

// Upgrade takes an incoming HTTP request via handler.Context and upgrades it.
// If successful, it calls the provided WebSocket HandlerFunc.
// Returns a standard gocore handler.HandlerFunc that you can register.
func (u *Upgrader) Upgrade(h HandlerFunc) handler.HandlerFunc {
	return func(c *handler.Context) {
		req := c.Request
		res := c.ResponseWriter()

		// 1. Verify Method
		if req.Method != http.MethodGet {
			http.Error(res, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 2. Verify Upgrade headers
		if !strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
			http.Error(res, "invalid upgrade header", http.StatusBadRequest)
			return
		}
		if !strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade") {
			http.Error(res, "invalid connection header", http.StatusBadRequest)
			return
		}

		// 3. Verify Version
		if req.Header.Get("Sec-WebSocket-Version") != "13" {
			res.Header().Set("Sec-WebSocket-Version", "13")
			http.Error(res, "unsupported websocket version", http.StatusBadRequest)
			return
		}

		// 4. Validate Origin
		if !u.checkOrigin(req) {
			http.Error(res, "origin not allowed", http.StatusForbidden)
			return
		}

		// 5. Get the Key and compute Accept
		key := req.Header.Get("Sec-WebSocket-Key")
		if key == "" {
			http.Error(res, "missing websocket key", http.StatusBadRequest)
			return
		}
		acceptKey := computeAcceptKey(key)

		// 6. Hijack the connection
		hijacker, ok := res.(http.Hijacker)
		if !ok {
			http.Error(res, "server does not support hijacking", http.StatusInternalServerError)
			return
		}
		netConn, brw, err := hijacker.Hijack()
		if err != nil {
			// Cannot use http.Error after Hijack fails, usually res isn't completely usable but we can try
			http.Error(res, "hijacking failed", http.StatusInternalServerError)
			return
		}

		// 7. Send the Upgrade response directly
		// Note: since we hijacked, we must write the HTTP response manually.
		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

		if _, err := netConn.Write([]byte(resp)); err != nil {
			netConn.Close()
			return
		}

		// 8. Create the Conn wrapper
		wsConn := newConn(netConn, brw.Reader, brw.Writer)

		// 9. Call the handler and close on exit
		_ = h(c, wsConn)
		wsConn.Close() // Ensure we close out the connection safely when done
	}
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(magicString))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
