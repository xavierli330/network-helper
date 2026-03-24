package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/agent"
	"github.com/xavierli/nethelper/internal/channel"
	"github.com/xavierli/nethelper/internal/channel/feishu"
	"github.com/xavierli/nethelper/internal/llm"
)

const defaultHeartbeatPrompt = "检查所有设备的网络拓扑状态，查找单点故障(SPOF)和异常。如有变化或异常，给出简要报告。如果一切正常，只需说'巡检正常，无异常'。"

func newHeartbeatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "heartbeat",
		Short: "Periodic network patrol",
	}
	cmd.AddCommand(newHeartbeatStartCmd())
	return cmd
}

func newHeartbeatStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start periodic heartbeat patrol",
		Long:  "Runs an AI agent on a schedule to check network health, detect SPOFs, and push alerts to IM when anomalies are found. Blocks until SIGINT/SIGTERM.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if llmRouter == nil {
				return fmt.Errorf("LLM not configured — add llm section to ~/.nethelper/config.yaml")
			}

			hbCfg := cfg.Heartbeat
			if !hbCfg.Enabled {
				return fmt.Errorf("heartbeat not enabled in config — set heartbeat.enabled: true in ~/.nethelper/config.yaml")
			}

			interval, err := time.ParseDuration(hbCfg.Interval)
			if err != nil || interval < time.Minute {
				log.Printf("[heartbeat] invalid or missing interval %q, defaulting to 30m", hbCfg.Interval)
				interval = 30 * time.Minute
			}

			prompt := hbCfg.Prompt
			if prompt == "" {
				prompt = defaultHeartbeatPrompt
			}

			embedder := llm.BuildEmbedder(cfg.Embedding)
			sessionLogger := agent.NewSessionLogger(cfg.DataDir)

			// Factory: creates a fresh agent for each patrol run.
			newAgentFn := func() *agent.Agent {
				reg := agent.NewRegistry()
				agent.RegisterNethelperTools(reg, db, pipeline)
				return agent.New(llmRouter, reg, embedder, db, agent.AgentOptions{
					Logger:     sessionLogger,
					UserKey:    "heartbeat",
					ContextCfg: cfg.Context,
					DataDir:    cfg.DataDir,
				})
			}

			// Optional IM alert function.
			var alertFn agent.AlertFunc
			if hbCfg.Channel != "" && hbCfg.ChatID != "" {
				ch := createChannelByName(hbCfg.Channel)
				if ch != nil {
					bgCtx := context.Background()
					go func() {
						if startErr := ch.Start(bgCtx, nil); startErr != nil {
							log.Printf("[heartbeat] channel %s start error: %v", hbCfg.Channel, startErr)
						}
					}()
					// Give the channel a moment to connect before using it.
					time.Sleep(2 * time.Second)
					alertFn = func(text string) error {
						return ch.SendText(hbCfg.ChatID, text)
					}
				} else {
					log.Printf("[heartbeat] channel %q not available (check config credentials)", hbCfg.Channel)
				}
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go agent.RunHeartbeat(ctx, interval, prompt, newAgentFn, sessionLogger, alertFn)

			fmt.Printf("Heartbeat started (every %s)\nPress Ctrl+C to stop.\n", interval)
			<-sigCh
			cancel()
			return nil
		},
	}
}

// createChannelByName creates a Channel adapter by name using the loaded config.
// Returns nil when the named channel is not configured or credentials are missing.
func createChannelByName(name string) channel.Channel {
	switch name {
	case "feishu":
		fc := cfg.Channels.Feishu
		if fc.AppID != "" && fc.AppSecret != "" {
			return feishu.New(fc.AppID, fc.AppSecret)
		}
		log.Printf("[heartbeat] feishu: app_id and app_secret are required in config")
	default:
		log.Printf("[heartbeat] unsupported channel: %q", name)
	}
	return nil
}
