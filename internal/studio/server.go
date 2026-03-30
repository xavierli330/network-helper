package studio

import (
	"embed"
	"io/fs"
	"log/slog"
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
	mux          *http.ServeMux
	db           *store.DB
	eng          *discovery.Engine
	llmR         *llm.Router
	generate     GenerateFn // nil means codegen not available (dry-run mode)
	fieldReg     *parser.FieldRegistry
	repoRoot     string
	hintCache    *store.VendorHintCache
	patternCache *store.PatternCache
	runtimeReg   *store.RuntimeRegistry
}

// NewServer creates a Rule Studio server. eng, llmR, generate and fieldReg may be nil.
func NewServer(db *store.DB, eng *discovery.Engine, llmR *llm.Router, generate GenerateFn, fieldReg *parser.FieldRegistry, repoRoot string) *Server {
	// Initialize caches
	hintCache := store.NewVendorHintCache()
	patternCache := store.NewPatternCache()
	runtimeReg := store.NewRuntimeRegistry()

	// Seed classification patterns (no-op if already seeded)
	if err := db.SeedClassificationPatterns(); err != nil {
		slog.Warn("failed to seed classification patterns", "error", err)
	}

	// Load caches from DB
	if err := hintCache.Reload(db); err != nil {
		slog.Warn("failed to load vendor hints cache", "error", err)
	}
	if err := patternCache.Reload(db); err != nil {
		slog.Warn("failed to load pattern cache", "error", err)
	}
	if err := runtimeReg.Reload(db); err != nil {
		slog.Warn("failed to load runtime rules", "error", err)
	}

	s := &Server{
		mux:          http.NewServeMux(),
		db:           db,
		eng:          eng,
		llmR:         llmR,
		generate:     generate,
		fieldReg:     fieldReg,
		repoRoot:     repoRoot,
		hintCache:    hintCache,
		patternCache: patternCache,
		runtimeReg:   runtimeReg,
	}
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

	h := &handlers{
		db:           s.db,
		eng:          s.eng,
		generate:     s.generate,
		fieldReg:     s.fieldReg,
		repoRoot:     s.repoRoot,
		hintCache:    s.hintCache,
		patternCache: s.patternCache,
		runtimeReg:   s.runtimeReg,
	}

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

	// Batch Import
	s.mux.HandleFunc("/import", h.batchImportPage)
	s.mux.HandleFunc("/api/import/analyze", h.apiAnalyzeLog)
	s.mux.HandleFunc("/api/import/generate", h.apiGenerateBatch)
	s.mux.HandleFunc("/api/import/manual", h.apiManualAdd)

	// Unknown batch operations
	s.mux.HandleFunc("/api/unknown/batch-generate", h.apiUnknownBatchGenerate)

	// APIs
	s.mux.HandleFunc("/api/rule/", h.apiDispatch)
	s.mux.HandleFunc("/api/rules/delete-drafts", h.apiDeleteDraftRules)
	s.mux.HandleFunc("/api/rules/search", h.apiSearchRules)
	s.mux.HandleFunc("/api/discover", h.apiDiscover)
	s.mux.HandleFunc("/api/dashboard", h.apiDashboard)
	s.mux.HandleFunc("/api/fields", h.apiFields)
	s.mux.HandleFunc("/api/fields/vendors-html", h.apiFieldsVendorsHTML)
	s.mux.HandleFunc("/api/fields/schema-html", h.apiFieldsSchemaHTML)
	s.mux.HandleFunc("/api/test", h.apiParserTest)
	s.mux.HandleFunc("/api/unknown/", h.unknownDispatch)
	s.mux.HandleFunc("/api/status", h.apiStatus)

	// ── Phase 3A: Vendor Hostname Hints CRUD ──────────────────────────
	s.mux.HandleFunc("/vendor-hints", h.vendorHintsPage)
	s.mux.HandleFunc("/api/vendor-hints", h.apiVendorHints)
	s.mux.HandleFunc("/api/vendor-hints/", h.apiVendorHintDispatch)

	// ── Phase 3C: Classification Patterns CRUD ────────────────────────
	s.mux.HandleFunc("/api/patterns", h.apiPatterns)
	s.mux.HandleFunc("/api/patterns/", h.apiPatternDispatch)

	// ── Phase 3D: Field Schemas CRUD ──────────────────────────────────
	s.mux.HandleFunc("/api/field-schemas", h.apiFieldSchemas)
	s.mux.HandleFunc("/api/field-schemas/sync", h.apiFieldSchemasSync)
	s.mux.HandleFunc("/api/field-schemas/", h.apiFieldSchemaDispatch)

	// ── Phase 3B: Runtime Rules CRUD ──────────────────────────────────
	s.mux.HandleFunc("/api/runtime-rules", h.apiRuntimeRules)
	s.mux.HandleFunc("/api/runtime-rules/", h.apiRuntimeRuleDispatch)

	// ── Phase 3E: Vendor Reassignment ─────────────────────────────────
	s.mux.HandleFunc("/api/vendor-reassign", h.apiVendorReassign)

	// ── Self-Check Engine: Coverage ──────────────────────────────────
	s.mux.HandleFunc("/coverage", h.coveragePage)
	s.mux.HandleFunc("/coverage/", h.coverageDetail)
	s.mux.HandleFunc("/api/coverage/recheck", h.apiCoverageRecheck)
	s.mux.HandleFunc("/api/coverage/ssh", h.apiCoverageSSH)
	s.mux.HandleFunc("/api/coverage/summary", h.apiCoverageSummary)
	s.mux.HandleFunc("/api/coverage/boost", h.apiCoverageBoost)
}
