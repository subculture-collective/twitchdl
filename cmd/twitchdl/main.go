package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/subculture-collective/twitchdl/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := cli.New(os.Stdout, os.Stderr)
	command.SetContext(ctx)
	if err := command.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "twitchdl: error:", err)
		os.Exit(cli.ExitCode(err))
	}
}
