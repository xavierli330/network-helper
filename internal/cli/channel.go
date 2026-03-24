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
	"github.com/xavierli/nethelper/internal/channel/discord"
	"github.com/xavierli/nethelper/internal/channel/feishu"
	"github.com/xavierli/nethelper/internal/channel/qq"
	"github.com/xavierli/nethelper/internal/channel/telegram"
	"github.com/xavierli/nethelper/internal/channel/wechat"
	"github.com/xavierli/nethelper/internal/llm"
)

func newChannelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "IM channel management",
	}
	cmd.AddCommand(newChannelStartCmd())
	return cmd
}

func newChannelStartCmd() *cobra.Command {
	var feishuOnly bool
	var discordOnly bool
	var telegramOnly bool
	var wechatOnly bool
	var qqOnly bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start IM channel connections",
		Long:  "Connect to configured IM platforms and forward messages to the nethelper AI agent. Blocks until SIGINT/SIGTERM.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if llmRouter == nil {
				return fmt.Errorf("LLM not configured — add llm section to ~/.nethelper/config.yaml")
			}

			// Build permission config from loaded config.
			perms := buildPermissions()

			// Build optional embedder for vector memory.
			var embedder llm.Embedder
			if cfg != nil {
				embedder = llm.BuildEmbedder(cfg.Embedding)
			}

			// Create the channel router that dispatches messages to agent sessions.
			sessionLogger := agent.NewSessionLogger(cfg.DataDir)
			router := channel.NewRouter(db, pipeline, llmRouter, embedder, perms, channel.RouterOptions{
				SessionLogger: sessionLogger,
				ContextCfg:    cfg.Context,
				DataDir:       cfg.DataDir,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var channels []channel.Channel

			if cfg.Channels.Feishu.Enabled || feishuOnly {
				fc := cfg.Channels.Feishu
				if fc.AppID == "" || fc.AppSecret == "" {
					return fmt.Errorf("feishu: app_id and app_secret are required in config")
				}
				channels = append(channels, feishu.New(fc.AppID, fc.AppSecret))
			}

			if cfg.Channels.Discord.Enabled || discordOnly {
				dc := cfg.Channels.Discord
				if dc.Token == "" {
					return fmt.Errorf("discord: token required")
				}
				channels = append(channels, discord.New(dc.Token))
			}

			if cfg.Channels.Telegram.Enabled || telegramOnly {
				tc := cfg.Channels.Telegram
				if tc.Token == "" {
					return fmt.Errorf("telegram: token required")
				}
				channels = append(channels, telegram.New(tc.Token))
			}

			if cfg.Channels.WeChat.Enabled || wechatOnly {
				wc := cfg.Channels.WeChat
				if wc.BridgeURL == "" {
					return fmt.Errorf("wechat: bridge_url required")
				}
				channels = append(channels, wechat.New(wc.BridgeURL, wc.Token))
			}

			if cfg.Channels.QQ.Enabled || qqOnly {
				qc := cfg.Channels.QQ
				if qc.WSURL == "" {
					return fmt.Errorf("qq: ws_url required")
				}
				channels = append(channels, qq.New(qc.WSURL))
			}

			if len(channels) == 0 {
				return fmt.Errorf("no channels configured or enabled — set channels.<platform>.enabled: true in config, or pass --feishu / --discord / --telegram / --wechat / --qq")
			}

			for _, ch := range channels {
				go func(c channel.Channel) {
					log.Printf("[channel] starting %s", c.Name())
					err := c.Start(ctx, func(msg channel.InMessage) {
						// Check if this channel supports streaming card updates.
						sc, isStreaming := c.(channel.StreamingChannel)

						if isStreaming {
							// Send an immediate "thinking" card so the user gets
							// instant feedback while the agent works.
							msgID, sendErr := sc.SendInitCard(msg.ChatID, "⏳ 收到，正在思考...")
							if sendErr != nil {
								log.Printf("[channel/%s] init card error: %v", c.Name(), sendErr)
								// Fall back to non-streaming behaviour.
								response := router.Handle(ctx, msg)
								if response != "" {
									if sErr := c.SendText(msg.ChatID, response); sErr != nil {
										log.Printf("[channel/%s] send error: %v", c.Name(), sErr)
									}
								}
								return
							}

							// Run the agent; patch the card on each tool call.
							response := router.HandleWithProgress(ctx, msg, func(status string) {
								if uErr := sc.UpdateCard(msg.ChatID, msgID, status); uErr != nil {
									log.Printf("[channel/%s] update card error: %v", c.Name(), uErr)
								}
							})

							// Final update — turn off streaming, mark done (header → green).
							if response != "" {
								if uErr := sc.FinalizeCard(msgID, response); uErr != nil {
									log.Printf("[channel/%s] finalize card error: %v", c.Name(), uErr)
								}
							}
						} else {
							// Non-streaming channel — original behaviour.
							response := router.Handle(ctx, msg)
							if response != "" {
								if sendErr := c.SendText(msg.ChatID, response); sendErr != nil {
									log.Printf("[channel/%s] send error: %v", c.Name(), sendErr)
								}
							}
						}
					})
					if err != nil {
						log.Printf("[channel/%s] error: %v", c.Name(), err)
					}
				}(ch)
			}

			fmt.Printf("Channels started: %d\nPress Ctrl+C to stop.\n", len(channels))

			// Co-start heartbeat patrol alongside channels if configured.
			if cfg.Heartbeat.Enabled {
				hbCfg := cfg.Heartbeat
				hbInterval, err := time.ParseDuration(hbCfg.Interval)
				if err != nil || hbInterval < time.Minute {
					log.Printf("[channel] invalid heartbeat interval %q, defaulting to 30m", hbCfg.Interval)
					hbInterval = 30 * time.Minute
				}
				hbPrompt := hbCfg.Prompt
				if hbPrompt == "" {
					hbPrompt = defaultHeartbeatPrompt
				}
				hbNewAgentFn := func() *agent.Agent {
					reg := agent.NewRegistry()
					agent.RegisterNethelperTools(reg, db, pipeline)
					return agent.New(llmRouter, reg, embedder, db, agent.AgentOptions{
						Logger:     sessionLogger,
						UserKey:    "heartbeat",
						ContextCfg: cfg.Context,
						DataDir:    cfg.DataDir,
					})
				}
				var hbAlertFn agent.AlertFunc
				if hbCfg.Channel != "" && hbCfg.ChatID != "" {
					hbCh := createChannelByName(hbCfg.Channel)
					if hbCh != nil {
						go func() {
							if startErr := hbCh.Start(ctx, nil); startErr != nil {
								log.Printf("[heartbeat] channel %s start error: %v", hbCfg.Channel, startErr)
							}
						}()
						time.Sleep(2 * time.Second)
						chatID := hbCfg.ChatID
						hbAlertFn = func(text string) error {
							return hbCh.SendText(chatID, text)
						}
					}
				}
				go agent.RunHeartbeat(ctx, hbInterval, hbPrompt, hbNewAgentFn, sessionLogger, hbAlertFn)
				fmt.Printf("Heartbeat patrol started (every %s)\n", hbInterval)
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			fmt.Println("\nStopping channels...")
			for _, ch := range channels {
				if err := ch.Stop(); err != nil {
					log.Printf("[channel/%s] stop error: %v", ch.Name(), err)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&feishuOnly, "feishu", false, "Start only the Feishu channel (ignores enabled flag in config)")
	cmd.Flags().BoolVar(&discordOnly, "discord", false, "Start only the Discord channel (ignores enabled flag in config)")
	cmd.Flags().BoolVar(&telegramOnly, "telegram", false, "Start only the Telegram channel (ignores enabled flag in config)")
	cmd.Flags().BoolVar(&wechatOnly, "wechat", false, "Start only the WeChat bridge channel (ignores enabled flag in config)")
	cmd.Flags().BoolVar(&qqOnly, "qq", false, "Start only the QQ channel (ignores enabled flag in config)")
	return cmd
}

// buildPermissions constructs a PermissionConfig from the loaded app config.
// If no groups are defined, a permissive default is used that allows all users
// to call show_* and search_* tools.
func buildPermissions() *channel.PermissionConfig {
	pc := &channel.PermissionConfig{}
	for name, g := range cfg.Permissions.Groups {
		pc.Groups = append(pc.Groups, channel.PermissionGroup{
			Name:  name,
			Users: g.Users,
			Tools: g.Tools,
		})
	}
	if len(pc.Groups) == 0 {
		pc.Groups = append(pc.Groups, channel.PermissionGroup{
			Name:  "default",
			Users: []string{"*"},
			Tools: []string{"show_*", "search_*"},
		})
	}
	return pc
}
