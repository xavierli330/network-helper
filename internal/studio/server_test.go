// internal/studio/server_test.go
package studio_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xavierli/nethelper/internal/store"
	"github.com/xavierli/nethelper/internal/studio"
)

func TestServerRoutes(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	srv := studio.NewServer(db, nil, nil, nil) // generate=nil until Task 9 wires codegen

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET / want 200, got %d", w.Code)
	}

	req2 := httptest.NewRequest("GET", "/static/htmx.min.js", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("GET /static/htmx.min.js want 200, got %d", w2.Code)
	}
}
