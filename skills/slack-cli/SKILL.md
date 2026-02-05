---
name: slack-cli
description: Read Slack messages, threads, and channels via CLI. Use when asked to view Slack URLs, search Slack, or look up Slack users.
allowed-tools: Bash(slack:*)
---

# Slack CLI

A CLI for reading Slack content - messages, threads, channels, and users.

## Prerequisites

The `slack` command must be available on PATH. To check:

```bash
slack --version
```

If not installed, follow the instructions at:
https://github.com/lox/slack-cli#installation

## Available Commands

```
slack view <url>          # View any Slack URL (message, thread, or channel)
slack search <query>      # Search messages
slack channel list        # List channels you're a member of
slack channel read        # Read recent messages from a channel
slack channel info        # Show channel information
slack thread read         # Read a thread by URL or channel+timestamp
slack user list           # List users in the workspace
slack user info           # Show user information
slack auth config         # Configure Slack app credentials
slack auth login          # Authenticate with Slack via OAuth
slack auth status         # Show authentication status
```

## Common Patterns

### View a Slack URL the user shared

```bash
slack view "https://workspace.slack.com/archives/C123/p1234567890" --markdown
```

### Search for messages

```bash
slack search "from:@username keyword"
slack search "in:#channel-name keyword"
```

### Read a channel

```bash
slack channel read #general --limit 50
```

## Discovering Options

To see available subcommands and flags, run `--help` on any command:

```bash
slack --help
slack view --help
slack search --help
```

## Notes

- Use `--markdown` flag when you need to process or quote the output
- Thread URLs with `thread_ts` parameter are automatically detected
- Channel names can include or omit the `#` prefix
- User lookup accepts both user IDs (U123ABC) and email addresses
