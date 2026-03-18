package router

import (
	"net/http"
	"testing"

	"github.com/orislabsdev/gocore/handler"
)

func BenchmarkRouter(b *testing.B) {
	r := New()
	h := func(ctx *handler.Context) {}

	r.GET("/static", h)
	r.GET("/api/v1/users/:id", h)
	r.GET("/files/*path", h)

	tests := []struct {
		name string
		path string
	}{
		{"Static", "/static"},
		{"Param", "/api/v1/users/123"},
		{"Wildcard", "/files/some/deep/path/to/file.png"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r.match(http.MethodGet, tt.path)
			}
		})
	}
}
