package main

import "github.com/xavierli/nethelper/internal/cli"

// version is set during build via -ldflags
var version = "dev"

func main() {
	cli.SetVersion(version)
	cli.Execute()
}
