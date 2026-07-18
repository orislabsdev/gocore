package websocket

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/orislabsdev/gocore/handler"
)

func TestComputeAcceptKey(t *testing.T) {
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	expected := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="

	res := computeAcceptKey(key)
	if res != expected {
		t.Errorf("Expected %s, got %s", expected, res)
	}
}

func TestUpgrader_Failures(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	res := httptest.NewRecorder()
	ctx := handler.NewContext(res, req)

	u := Upgrader{}
	h := u.Upgrade(func(_ *handler.Context, _ *Conn) error {
		return nil
	})

	h(ctx)

	if res.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status %d, got %d", http.StatusBadRequest, res.Result().StatusCode)
	}
	if !strings.Contains(res.Body.String(), "invalid upgrade header") {
		t.Fatalf("Expected invalid upgrade header error in body, got body: %s", res.Body.String())
	}
}

type fakeHijacker struct {
	netConn net.Conn
	br      *bufio.Reader
	bw      *bufio.Writer
}

func (f *fakeHijacker) Header() http.Header         { return http.Header{} }
func (f *fakeHijacker) Write(b []byte) (int, error) { return f.netConn.Write(b) }
func (f *fakeHijacker) WriteHeader(statusCode int)  {}
func (f *fakeHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return f.netConn, bufio.NewReadWriter(f.br, f.bw), nil
}

func testUpgraderUpgrade(t *testing.T, subProtocols []string, requestedProto string) (string, error) {
	t.Helper()

	server, client := net.Pipe()
	defer client.Close()

	br := bufio.NewReader(server)
	bw := bufio.NewWriter(server)

	fh := &fakeHijacker{netConn: server, br: br, bw: bw}
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	if requestedProto != "" {
		req.Header.Set("Sec-WebSocket-Protocol", requestedProto)
	}

	ctx := handler.NewContext(fh, req)

	u := &Upgrader{
		SubProtocols: subProtocols,
	}
	h := u.Upgrade(func(_ *handler.Context, _ *Conn) error {
		return nil
	})

	done := make(chan struct{})
	go func() {
		h(ctx)
		close(done)
	}()

	buf := make([]byte, 1024)
	n, err := client.Read(buf)
	<-done

	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

func TestUpgrader_SubProtocol_Negotiated(t *testing.T) {
	resp, err := testUpgraderUpgrade(t, []string{"graphql-ws", "chat"}, "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp, "Sec-WebSocket-Protocol: chat\r\n") {
		t.Fatalf("expected Sec-WebSocket-Protocol: chat in response, got:\n%s", resp)
	}
}

func TestUpgrader_SubProtocol_FirstMatch(t *testing.T) {
	resp, err := testUpgraderUpgrade(t, []string{"graphql-ws", "chat"}, "chat, graphql-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp, "Sec-WebSocket-Protocol: chat\r\n") {
		t.Fatalf("expected first match 'chat' in response, got:\n%s", resp)
	}
}

func TestUpgrader_SubProtocol_NoMatch(t *testing.T) {
	resp, err := testUpgraderUpgrade(t, []string{"graphql-ws"}, "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(resp, "Sec-WebSocket-Protocol:") {
		t.Fatalf("expected no Sec-WebSocket-Protocol header when no match, got:\n%s", resp)
	}
}

func TestUpgrader_SubProtocol_NoneRequested(t *testing.T) {
	resp, err := testUpgraderUpgrade(t, []string{"graphql-ws"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(resp, "Sec-WebSocket-Protocol:") {
		t.Fatalf("expected no Sec-WebSocket-Protocol header when none requested, got:\n%s", resp)
	}
}

func TestUpgrader_SubProtocol_ServerNotConfigured(t *testing.T) {
	resp, err := testUpgraderUpgrade(t, nil, "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(resp, "Sec-WebSocket-Protocol:") {
		t.Fatalf("expected no Sec-WebSocket-Protocol header when server has no subprotocols, got:\n%s", resp)
	}
}

func TestNegotiateSubProtocol(t *testing.T) {
	tests := []struct {
		name     string
		server   []string
		client   string
		expected string
	}{
		{"no server protocols", nil, "chat", ""},
		{"no client header", []string{"chat"}, "", ""},
		{"exact match", []string{"chat"}, "chat", "chat"},
		{"case insensitive", []string{"Chat"}, "chat", "Chat"},
		{"multiple client picks first match", []string{"a", "b"}, "b, a", "b"},
		{"no match", []string{"chat"}, "binary", ""},
		{"comma separated client", []string{"graphql-ws", "chat"}, "chat, graphql-ws", "chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &Upgrader{SubProtocols: tt.server}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.client != "" {
				req.Header.Set("Sec-WebSocket-Protocol", tt.client)
			}
			got := u.negotiateSubProtocol(req)
			if got != tt.expected {
				t.Errorf("negotiateSubProtocol() = %q, want %q", got, tt.expected)
			}
		})
	}
}
