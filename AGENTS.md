# Slack CLI

A Go CLI for Slack using the kong CLI framework.

## Commands

```bash
mise run build    # Build binary
mise run test     # Run tests
mise run lint     # Run linter
mise run check    # Run all checks
```

## Structure

- `cmd/` - Command definitions (kong structs)
- `internal/config/` - Config file handling (~/.config/slack-cli/)
- `internal/slack/` - Slack API client

## Patterns

- Follow notion-cli patterns for kong command structure
- Use `ctx.Config.Token` for authentication in commands
- Check token before API calls: `if ctx.Config.Token == ""`
