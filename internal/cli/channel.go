package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/channel"
	"github.com/xavierli/nethelper/internal/channel/feishu"
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
			router := channel.NewRouter(db, pipeline, llmRouter, embedder, perms)

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

			if len(channels) == 0 {
				return fmt.Errorf("no channels configured or enabled — set channels.feishu.enabled: true in config, or pass --feishu")
			}

			for _, ch := range channels {
				go func(c channel.Channel) {
					log.Printf("[channel] starting %s", c.Name())
					err := c.Start(ctx, func(msg channel.InMessage) {
						response := router.Handle(ctx, msg)
						if response != "" {
							if sendErr := c.SendText(msg.ChatID, response); sendErr != nil {
								log.Printf("[channel/%s] send error: %v", c.Name(), sendErr)
							}
						}
					})
					if err != nil {
						log.Printf("[channel/%s] error: %v", c.Name(), err)
					}
				}(ch)
			}

			fmt.Printf("Channels started: %d\nPress Ctrl+C to stop.\n", len(channels))

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
