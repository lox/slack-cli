---
name: slack-cli
description: Read Slack messages, threads, and channels via CLI. Use when asked to view Slack URLs, search Slack, or look up Slack users.
---

# Slack CLI

CLI tool for reading Slack content. Located at `~/Develop/slack-cli`.

## Commands

```bash
# View any Slack URL (message, thread, or channel)
slack view <url>
slack view <url> --markdown    # Output as markdown (useful for processing)

# Search messages
slack search "query"
slack search "from:@user query"
slack search "in:#channel query"

# Read channel messages
slack channel list
slack channel read #channel-name
slack channel read #channel-name --limit 50

# Read threads
slack thread read <url>
slack thread read -c C123ABC -t 1234567890.123456

# User lookup
slack user list
slack user info U123ABC
slack user info user@example.com
```

## Common Patterns

### View a Slack URL the user shared
```bash
slack view "https://buildkite-corp.slack.com/archives/C123/p1234567890" --markdown
```

### Search for recent messages from a user
```bash
slack search "from:@username"
```

### Search in a specific channel
```bash
slack search "in:#channel-name keyword"
```

## Notes

- Use `--markdown` flag when you need to process or quote the output
- Thread URLs with `thread_ts` parameter are automatically detected
- Channel names can include or omit the `#` prefix
- User lookup accepts both user IDs (U123ABC) and email addresses
