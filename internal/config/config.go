package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type LLMProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
	BaseURL string `yaml:"base_url"`
}

type LLMConfig struct {
	Default   string                       `yaml:"default"`
	Providers map[string]LLMProviderConfig `yaml:"providers"`
	Routing   map[string]string            `yaml:"routing"`
}

type EmbeddingConfig struct {
	Provider  string                       `yaml:"provider"`
	Providers map[string]LLMProviderConfig `yaml:"providers"`
}

type FeishuChannelConfig struct {
	AppID     string `yaml:"app_id"`
	AppSecret string `yaml:"app_secret"`
	Enabled   bool   `yaml:"enabled"`
}

type DiscordChannelConfig struct {
	Token   string `yaml:"token"`
	Enabled bool   `yaml:"enabled"`
}

type TelegramChannelConfig struct {
	Token   string `yaml:"token"`
	Enabled bool   `yaml:"enabled"`
}

type WeChatChannelConfig struct {
	BridgeURL string `yaml:"bridge_url"`
	Token     string `yaml:"token"`
	Enabled   bool   `yaml:"enabled"`
}

type QQChannelConfig struct {
	WSURL   string `yaml:"ws_url"`
	Enabled bool   `yaml:"enabled"`
}

type ChannelsConfig struct {
	Feishu   FeishuChannelConfig   `yaml:"feishu"`
	Discord  DiscordChannelConfig  `yaml:"discord"`
	Telegram TelegramChannelConfig `yaml:"telegram"`
	WeChat   WeChatChannelConfig   `yaml:"wechat"`
	QQ       QQChannelConfig       `yaml:"qq"`
}

// HeartbeatConfig controls the periodic network patrol scheduler.
type HeartbeatConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"` // e.g. "30m", "1h"
	Prompt   string `yaml:"prompt"`   // agent prompt for each check
	Channel  string `yaml:"channel"`  // optional: "feishu", "discord", etc.
	ChatID   string `yaml:"chat_id"`  // optional: where to push alerts
}

// ContextConfig controls how the agent compresses its message history.
type ContextConfig struct {
	// MaxTokenBudget is the total character budget for all messages (rough proxy
	// for tokens at ~4 chars/token). Defaults to 50000 (~12K tokens).
	MaxTokenBudget int `yaml:"max_token_budget"`
	// ToolResultMaxLen is the maximum length kept for a single tool result.
	// Content beyond this is replaced with a truncation notice. Defaults to 2000.
	ToolResultMaxLen int `yaml:"tool_result_max_len"`
	// EnableSummary, when true, asks the LLM to produce a rolling summary instead
	// of simply dropping the oldest messages (not yet implemented, reserved).
	EnableSummary bool `yaml:"enable_summary"`
	// SummaryProvider names the LLM provider to use for rolling summaries.
	SummaryProvider string `yaml:"summary_provider"`
}

type PermGroupConfig struct {
	Users []string `yaml:"users"`
	Tools []string `yaml:"tools"`
}

type PermissionsConfig struct {
	Groups map[string]PermGroupConfig `yaml:"groups"`
}

// KnowledgeSourceConfig describes a single pluggable knowledge source.
type KnowledgeSourceConfig struct {
	Type     string `yaml:"type"`      // "local" | "http" | "ima"
	Name     string `yaml:"name"`      // display name used in SearchResult.Source prefix
	Path     string `yaml:"path"`      // for "local": directory path (default: <data_dir>/knowledge)
	URL      string `yaml:"url"`       // for "http": API base URL
	Token    string `yaml:"token"`     // for "http": optional Bearer auth token
	ClientID string `yaml:"client_id"` // for "ima": IMA client ID
	APIKey   string `yaml:"api_key"`   // for "ima": IMA API key
	KBID     string `yaml:"kb_id"`     // for "ima": knowledge base ID
	Enabled  bool   `yaml:"enabled"`   // when false the source is skipped at startup
}

// KnowledgeConfig groups all knowledge source definitions.
type KnowledgeConfig struct {
	Sources []KnowledgeSourceConfig `yaml:"sources"`
}

// RuleConfig configures the Rule Studio and Discovery Engine.
type RuleConfig struct {
	// DiscoveryInterval is how often the studio server auto-runs discovery.
	// Empty or "0" = disabled (default). Example: "30m", "1h".
	DiscoveryInterval string `yaml:"discovery_interval"`
	// StudioPort is the default port for `nethelper rule studio`. Default: 7070.
	StudioPort int `yaml:"studio_port"`
}

type Config struct {
	DataDir     string            `yaml:"data_dir"`
	DBPath      string            `yaml:"db_path"`
	WatchDirs   []string          `yaml:"watch_dirs"`
	LLM         LLMConfig         `yaml:"llm"`
	Embedding   EmbeddingConfig   `yaml:"embedding"`
	Channels    ChannelsConfig    `yaml:"channels"`
	Permissions PermissionsConfig `yaml:"permissions"`
	Context     ContextConfig     `yaml:"context"`
	Heartbeat   HeartbeatConfig   `yaml:"heartbeat"`
	Knowledge   KnowledgeConfig   `yaml:"knowledge"`
	Rule        RuleConfig        `yaml:"rule"`
}

func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".nethelper")
}

func Default() *Config {
	dataDir := DefaultDataDir()
	return &Config{
		DataDir: dataDir,
		DBPath:  filepath.Join(dataDir, "nethelper.db"),
		Rule:    RuleConfig{StudioPort: 7070},
	}
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultDataDir(), "config.yaml")
}

func LoadFrom(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Load() (*Config, error) {
	return LoadFrom(DefaultConfigPath())
}
