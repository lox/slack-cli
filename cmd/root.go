package cmd

import "github.com/lox/slack-cli/internal/config"

type Context struct {
	Config *config.Config
}

type CLI struct {
	Auth    AuthCmd    `cmd:"" help:"Authentication commands"`
	View    ViewCmd    `cmd:"" help:"View any Slack URL (message, thread, or channel)"`
	Channel ChannelCmd `cmd:"" help:"Channel commands"`
	Search  SearchCmd  `cmd:"" help:"Search messages"`
	Thread  ThreadCmd  `cmd:"" help:"Thread commands"`
	User    UserCmd    `cmd:"" help:"User commands"`
	Version VersionCmd `cmd:"" help:"Show version"`
}

type VersionCmd struct {
	Version string `kong:"hidden,default='${version}'"`
}

func (c *VersionCmd) Run(ctx *Context) error {
	println("slack version " + c.Version)
	return nil
}
