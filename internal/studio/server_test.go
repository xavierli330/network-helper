// internal/studio/server_test.go
package studio_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
	"github.com/xavierli/nethelper/internal/studio"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestServerRoutes(t *testing.T) {
	db := openTestDB(t)

	srv := studio.NewServer(db, nil, nil, nil, nil) // generate=nil until Task 9 wires codegen

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

func TestAPIFields(t *testing.T) {
	db := openTestDB(t)

	reg := parser.NewRegistry()
	reg.Register(&stubFieldParser{})
	fr := parser.BuildFieldRegistry(reg)

	srv := studio.NewServer(db, nil, nil, nil, fr)

	tests := []struct {
		query   string
		wantVal string
	}{
		{"/api/fields", "testvendor"},
		{"/api/fields?vendor=testvendor", "interface"},
		{"/api/fields?vendor=testvendor&command=display+interface+brief", "interface"},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.query, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d, body: %s", tc.query, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), tc.wantVal) {
			t.Errorf("GET %s: expected %q in body, got: %s", tc.query, tc.wantVal, w.Body.String())
		}
	}
}

// stubFieldParser satisfies parser.VendorParser for studio tests.
type stubFieldParser struct{}

func (s *stubFieldParser) Vendor() string                     { return "testvendor" }
func (s *stubFieldParser) DetectPrompt(string) (string, bool) { return "", false }
func (s *stubFieldParser) ClassifyCommand(cmd string) model.CommandType {
	if strings.Contains(cmd, "interface") {
		return model.CmdInterface
	}
	return model.CmdUnknown
}
func (s *stubFieldParser) ParseOutput(model.CommandType, string) (model.ParseResult, error) {
	return model.ParseResult{}, nil
}
func (s *stubFieldParser) SupportedCmdTypes() []model.CommandType {
	return []model.CommandType{model.CmdInterface}
}
func (s *stubFieldParser) FieldSchema(ct model.CommandType) []parser.FieldDef {
	if ct == model.CmdInterface {
		return []parser.FieldDef{{Name: "name", Type: parser.FieldTypeString}}
	}
	return nil
}
