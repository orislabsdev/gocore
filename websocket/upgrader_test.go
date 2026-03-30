package websocket

import (
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
	h := u.Upgrade(func(c *handler.Context, conn *Conn) error {
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
