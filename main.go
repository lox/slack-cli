package main

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/lox/slack-cli/cmd"
	"github.com/lox/slack-cli/internal/config"
)

var version = "dev"

func main() {
	c := &cmd.CLI{}
	ctx := kong.Parse(c,
		kong.Name("slack"),
		kong.Description("A CLI for Slack"),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)

	cfg, err := config.Load()
	ctx.FatalIfErrorf(err)

	err = ctx.Run(&cmd.Context{Config: cfg})
	ctx.FatalIfErrorf(err)
	os.Exit(0)
}
