package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/xavierli/nethelper/internal/codegen"
	"github.com/xavierli/nethelper/internal/config"
	"github.com/xavierli/nethelper/internal/discovery"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/parser/cisco"
	"github.com/xavierli/nethelper/internal/parser/h3c"
	"github.com/xavierli/nethelper/internal/parser/huawei"
	"github.com/xavierli/nethelper/internal/parser/juniper"
	"github.com/xavierli/nethelper/internal/store"
	"github.com/xavierli/nethelper/internal/studio"
)

//go:embed frontend/index.html
var assets embed.FS

// App struct holds the Wails application context and nethelper services.
type App struct {
	ctx       context.Context
	db        *store.DB
	studioSrv *studio.Server
	studioURL string
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize nethelper services
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: config load failed: %v, using defaults", err)
		cfg = config.Default()
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Printf("Warning: DB open failed: %v", err)
		return
	}
	a.db = db

	registry := parser.NewRegistry()
	registry.Register(huawei.New())
	registry.Register(cisco.New())
	registry.Register(h3c.New())
	registry.Register(juniper.New())
	fieldRegistry := parser.BuildFieldRegistry(registry)

	llmRouter := llm.BuildFromConfig(cfg.LLM)
	eng := discovery.New(db, llmRouter)
	repoRoot, _ := os.Getwd()

	generateFn := studio.GenerateFn(func(rule store.PendingRule, testCases []store.RuleTestCase, root, approvedBy string) (string, error) {
		return codegen.Generate(rule, testCases, codegen.GeneratorOptions{
			RepoRoot:   root,
			ApprovedBy: approvedBy,
		})
	})

	a.studioSrv = studio.NewServer(db, eng, llmRouter, generateFn, fieldRegistry, repoRoot)

	// Start the embedded HTTP server on a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("Warning: failed to start studio server: %v", err)
		return
	}
	a.studioURL = fmt.Sprintf("http://%s", listener.Addr().String())
	log.Printf("Rule Studio backend at %s", a.studioURL)

	go func() {
		if err := http.Serve(listener, a.studioSrv); err != nil {
			log.Printf("Studio server error: %v", err)
		}
	}()
}

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	if a.db != nil {
		a.db.Close()
	}
}

// GetStudioURL returns the URL of the embedded studio server.
func (a *App) GetStudioURL() string {
	return a.studioURL
}

// GetStatus returns system status for the desktop UI.
func (a *App) GetStatus() map[string]any {
	if a.db == nil {
		return map[string]any{"error": "database not available"}
	}
	unknowns, _ := a.db.ListUnknownOutputs("", "new", 1000)
	drafts, _ := a.db.ListPendingRules("draft", 100)
	testing_, _ := a.db.ListPendingRules("testing", 100)
	approved, _ := a.db.ListPendingRules("approved", 100)
	return map[string]any{
		"unknownCount":  len(unknowns),
		"draftCount":    len(drafts),
		"testingCount":  len(testing_),
		"approvedCount": len(approved),
		"studioURL":     a.studioURL,
		"timestamp":     time.Now().Format(time.RFC3339),
	}
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Rule Studio",
		Width:     1400,
		Height:    900,
		MinWidth:  1000,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
