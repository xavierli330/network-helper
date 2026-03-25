package studio

import (
	_ "embed"
	"net/http"

	"github.com/xavierli/nethelper/internal/discovery"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/store"
)

//go:embed static/htmx.min.js
var htmxJS []byte

// GenerateFn is a function that generates Go source files and creates a PR.
// Injected at startup to avoid a hard import cycle between studio and codegen.
type GenerateFn func(rule store.PendingRule, testCases []store.RuleTestCase, repoRoot, approvedBy string) (prURL string, err error)

// Server is the Rule Studio HTTP server.
type Server struct {
	mux      *http.ServeMux
	db       *store.DB
	eng      *discovery.Engine
	llmR     *llm.Router
	generate GenerateFn // nil means codegen not available (dry-run mode)
}

// NewServer creates a Rule Studio server. eng, llmR and generate may be nil.
func NewServer(db *store.DB, eng *discovery.Engine, llmR *llm.Router, generate GenerateFn) *Server {
	s := &Server{mux: http.NewServeMux(), db: db, eng: eng, llmR: llmR, generate: generate}
	s.registerRoutes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on addr (e.g. ":7070").
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/static/htmx.min.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(htmxJS)
	})
	h := &handlers{db: s.db, eng: s.eng, generate: s.generate}
	s.mux.HandleFunc("/", h.list)
	s.mux.HandleFunc("/rule/", h.ruleDispatch)    // /rule/:id and /rule/:id/sandbox
	s.mux.HandleFunc("/api/rule/", h.apiDispatch) // /api/rule/:id/test|testcase|approve|ignore
	s.mux.HandleFunc("/api/discover", h.apiDiscover)
}
