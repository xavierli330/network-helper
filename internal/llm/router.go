package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/xavierli/nethelper/internal/config"
)

// Router dispatches LLM requests to the appropriate provider based on capability.
type Router struct {
	defaultProvider Provider
	capProviders    map[Capability]Provider
}

func NewRouter() *Router {
	return &Router{capProviders: make(map[Capability]Provider)}
}

func (r *Router) SetDefault(p Provider) {
	r.defaultProvider = p
}

func (r *Router) SetForCapability(cap Capability, p Provider) {
	r.capProviders[cap] = p
}

// Available returns true if at least one provider is configured.
func (r *Router) Available() bool {
	return r.defaultProvider != nil || len(r.capProviders) > 0
}

// Chat routes a request to the appropriate provider for the given capability.
func (r *Router) Chat(ctx context.Context, cap Capability, req ChatRequest) (ChatResponse, error) {
	p := r.providerFor(cap)
	if p == nil {
		return ChatResponse{}, fmt.Errorf("no LLM provider configured (run 'nethelper config llm' to set up)")
	}
	return p.Chat(ctx, req)
}

func (r *Router) providerFor(cap Capability) Provider {
	if p, ok := r.capProviders[cap]; ok && p.Supports(cap) {
		return p
	}
	if r.defaultProvider != nil {
		return r.defaultProvider
	}
	return nil
}

// BuildFromConfig creates a Router from the application config.
func BuildFromConfig(cfg config.LLMConfig) *Router {
	router := NewRouter()

	providers := make(map[string]Provider)
	for name, pc := range cfg.Providers {
		p := createProvider(name, pc)
		providers[name] = p
	}

	if def, ok := providers[cfg.Default]; ok {
		router.SetDefault(def)
	}

	// Map routing config to capabilities
	capMap := map[string]Capability{
		"extract": CapExtract,
		"analyze": CapAnalyze,
		"explain": CapExplain,
		"parse":   CapParse,
	}
	for capName, providerName := range cfg.Routing {
		cap, ok := capMap[capName]
		if !ok {
			continue
		}
		if p, ok := providers[providerName]; ok {
			router.SetForCapability(cap, p)
		}
	}

	return router
}

// createProvider picks the right provider implementation based on name/URL heuristics.
// Names containing "anthropic" or "kimi" use the Anthropic Messages API protocol.
// Everything else uses the OpenAI Chat Completions protocol.
func createProvider(name string, pc config.LLMProviderConfig) Provider {
	lower := strings.ToLower(name)
	baseURL := strings.ToLower(pc.BaseURL)

	// Anthropic protocol: Anthropic itself, or Kimi Coding (which uses Anthropic protocol)
	if lower == "anthropic" || strings.Contains(lower, "kimi") ||
		strings.Contains(baseURL, "anthropic") || strings.Contains(baseURL, "kimi") {
		return NewAnthropicCompatProvider(name, pc.APIKey, pc.Model, pc.BaseURL)
	}

	// Default: OpenAI protocol
	return NewOpenAICompatProvider(name, pc.APIKey, pc.Model, pc.BaseURL)
}
