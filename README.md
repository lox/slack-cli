# slack-cli

A CLI for Slack - search, read channels/threads, and browse users from the command line.

## Installation

```bash
go install github.com/lox/slack-cli@latest
```

Or build from source:

```bash
task
```

## Authentication

```bash
slack auth login
```

This opens your browser for Slack OAuth. Approve the permissions and you're logged in.

### Using your own Slack App

To use your own Slack app instead of the default:

1. Create an app at https://api.slack.com/apps using `slack-app-manifest.yaml`
2. Set environment variables:
   ```bash
   export SLACK_CLIENT_ID="your-client-id"
   export SLACK_CLIENT_SECRET="your-client-secret"
   ```

## Usage

```bash
# Authentication
slack auth login      # Set your token
slack auth status     # Check auth status
slack auth logout     # Clear stored token

# Channels
slack channel list              # List channels you're in
slack channel read #general     # Read recent messages
slack channel info #general     # Show channel details

# Search
slack search "from:@alice project update"    # Search messages
slack search "in:#engineering bug"           # Search in channel

# Threads
slack thread read <url>                      # Read thread by URL
slack thread read -c C123 -t 1234567890.123  # Read by channel+ts

# Users
slack user list                 # List workspace users
slack user info U123            # Show user details
slack user info alice@acme.com  # Lookup by email
```

## Required Scopes

If creating your own Slack app, you'll need these user token scopes:

- `channels:history` - Read public channel messages
- `channels:read` - List public channels
- `groups:history` - Read private channel messages
- `groups:read` - List private channels
- `search:read` - Search messages
- `users:read` - List users
- `users:read.email` - Lookup users by email

## License

MIT
