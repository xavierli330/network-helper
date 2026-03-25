package studio

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/xavierli/nethelper/internal/discovery"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

//go:embed static/htmx.min.js static/style.css static/app.js
var staticFS embed.FS

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
	fieldReg *parser.FieldRegistry
	repoRoot string
}

// NewServer creates a Rule Studio server. eng, llmR, generate and fieldReg may be nil.
func NewServer(db *store.DB, eng *discovery.Engine, llmR *llm.Router, generate GenerateFn, fieldReg *parser.FieldRegistry, repoRoot string) *Server {
	s := &Server{mux: http.NewServeMux(), db: db, eng: eng, llmR: llmR, generate: generate, fieldReg: fieldReg, repoRoot: repoRoot}
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
	// Serve embedded static files
	staticSub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	h := &handlers{db: s.db, eng: s.eng, generate: s.generate, fieldReg: s.fieldReg, repoRoot: s.repoRoot}

	// Pages
	s.mux.HandleFunc("/", h.dashboard)
	s.mux.HandleFunc("/rules", h.list)
	s.mux.HandleFunc("/rule/", h.ruleDispatch)
	s.mux.HandleFunc("/test", h.tester)
	s.mux.HandleFunc("/fields", h.fields)
	s.mux.HandleFunc("/compare", h.compare)
	s.mux.HandleFunc("/patterns", h.patterns)
	s.mux.HandleFunc("/unknown", h.unknownList)
	s.mux.HandleFunc("/history", h.history)

	// APIs
	s.mux.HandleFunc("/api/rule/", h.apiDispatch)
	s.mux.HandleFunc("/api/discover", h.apiDiscover)
	s.mux.HandleFunc("/api/dashboard", h.apiDashboard)
	s.mux.HandleFunc("/api/fields", h.apiFields)
	s.mux.HandleFunc("/api/fields/vendors-html", h.apiFieldsVendorsHTML)
	s.mux.HandleFunc("/api/fields/schema-html", h.apiFieldsSchemaHTML)
	s.mux.HandleFunc("/api/test", h.apiParserTest)
	s.mux.HandleFunc("/api/unknown/", h.unknownDispatch)
	s.mux.HandleFunc("/api/status", h.apiStatus)
}
