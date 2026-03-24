package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// HeartbeatResult is the outcome of a single patrol check.
type HeartbeatResult struct {
	Timestamp time.Time
	Prompt    string
	Response  string
	Duration  time.Duration
	Error     string
}

// AlertFunc is called when a heartbeat check has a result to push to IM.
type AlertFunc func(text string) error

// RunHeartbeat starts a periodic patrol loop. It blocks until ctx is cancelled.
// Each tick creates a fresh agent, runs the prompt, and logs the result.
// If alertFn is non-nil AND the response indicates anomalies, alertFn is called.
func RunHeartbeat(
	ctx context.Context,
	interval time.Duration,
	prompt string,
	newAgentFn func() *Agent,
	logger *SessionLogger,
	alertFn AlertFunc,
) {
	log.Printf("[heartbeat] starting patrol every %s", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once immediately on start.
	runOnce(ctx, prompt, newAgentFn, logger, alertFn)

	for {
		select {
		case <-ctx.Done():
			log.Println("[heartbeat] stopped")
			return
		case <-ticker.C:
			runOnce(ctx, prompt, newAgentFn, logger, alertFn)
		}
	}
}

func runOnce(
	ctx context.Context,
	prompt string,
	newAgentFn func() *Agent,
	logger *SessionLogger,
	alertFn AlertFunc,
) {
	start := time.Now()
	ag := newAgentFn()

	response, err := ag.Chat(ctx, prompt, func(name string, args map[string]interface{}) {
		log.Printf("[heartbeat] tool: %s", name)
	})

	duration := time.Since(start)

	result := HeartbeatResult{
		Timestamp: start,
		Prompt:    prompt,
		Response:  response,
		Duration:  duration,
	}
	if err != nil {
		result.Error = err.Error()
		result.Response = fmt.Sprintf("Error: %v", err)
	}

	// Always log the result.
	if logger != nil {
		logger.Log("heartbeat", SessionEvent{
			Type:       "heartbeat",
			Content:    result.Response,
			DurationMs: duration.Milliseconds(),
		})
	}
	log.Printf("[heartbeat] completed in %s (%d chars response)", duration, len(result.Response))

	// Push to IM if alertFn is provided and the response indicates anomalies.
	if alertFn != nil && !isNormalResponse(result.Response) {
		if pushErr := alertFn(result.Response); pushErr != nil {
			log.Printf("[heartbeat] alert push error: %v", pushErr)
		}
	}
}

// isNormalResponse returns true when the heartbeat response signals that
// everything is fine, suppressing the alert notification.
func isNormalResponse(response string) bool {
	normalIndicators := []string{
		"巡检正常",
		"无异常",
		"一切正常",
		"无变化",
		"状态良好",
	}
	for _, ind := range normalIndicators {
		if strings.Contains(response, ind) {
			return true
		}
	}
	return false
}
