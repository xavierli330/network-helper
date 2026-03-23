package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// RunREPL starts an interactive Read-Eval-Print Loop.
func RunREPL(ctx context.Context, ag *Agent) error {
	fmt.Println("nethelper agent — 输入问题开始对话，输入 exit 退出")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("nethelper> ")
		if !scanner.Scan() {
			ag.SaveMemory(ctx)
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			ag.SaveMemory(ctx)
			fmt.Println("再见！")
			return nil
		}
		if input == "/reset" {
			ag.Reset()
			fmt.Println("对话已重置。")
			continue
		}

		response, err := ag.Chat(ctx, input, func(name string, args map[string]interface{}) {
			fmt.Printf("  [tool] %s(%v)\n", name, summarizeArgs(args))
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			continue
		}

		fmt.Println()
		fmt.Println(response)
		fmt.Println()
	}
	return scanner.Err()
}

func summarizeArgs(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, ", ")
}
