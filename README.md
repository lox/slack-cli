# slack-cli

A CLI for Slack - search, read channels/threads, and browse users from the command line.

## Installation

```bash
go install github.com/lox/slack-cli@latest
```

Or build from source:

```bash
git clone https://github.com/lox/slack-cli.git
cd slack-cli
go build .
```

## Setup

### 1. Create a Slack App

Each workspace requires a Slack app for OAuth:

1. Go to https://api.slack.com/apps → **Create New App** → **From an app manifest**
2. Select your workspace
3. Paste the contents of [`slack-app-manifest.yaml`](slack-app-manifest.yaml)
4. Click **Create**
5. From **Basic Information**, copy the **Client ID** and **Client Secret**

### 2. Configure the CLI

```bash
slack auth config
```

This will prompt you to paste your Client ID and Secret, which are stored in `~/.config/slack-cli/config.json`.

### 3. Authenticate

```bash
slack auth login
```

This opens your browser for OAuth. Approve the permissions and you're logged in.

### Environment Variables (optional)

For CI or automation, you can use environment variables instead of `auth config`:

```bash
export SLACK_CLIENT_ID="your-client-id"
export SLACK_CLIENT_SECRET="your-client-secret"
slack auth login
```

Environment variables take precedence over the config file.

## Usage

### View any Slack URL

```bash
slack view <url>                    # View message, thread, or channel
slack view <url> --markdown         # Output as markdown
```

### Channels

```bash
slack channel list                  # List channels you're in
slack channel read #general         # Read recent messages
slack channel info #general         # Show channel details
```

### Search

```bash
slack search "from:@alice project"  # Search messages
slack search "in:#engineering bug"  # Search in channel
```

### Threads

```bash
slack thread read <url>                      # Read thread by URL
slack thread read -c C123 -t 1234567890.123  # Read by channel+ts
```

### Users

```bash
slack user list                     # List workspace users
slack user info U123                # Show user details
slack user info alice@acme.com      # Lookup by email
```

### Authentication

```bash
slack auth config   # Configure Slack app credentials
slack auth login    # Authenticate with Slack
slack auth status   # Check auth status
slack auth logout   # Clear stored token
```

## Agent Skill

An [Amp](https://ampcode.com) skill is included for AI agent integration:

```bash
ln -s "$(pwd)/skills/slack-cli" ~/.config/agents/skills/slack-cli
```

This enables agents to use the CLI when asked to view Slack URLs, search messages, or look up users.

## Required Scopes

The included manifest requests these user token scopes:

- `channels:history` - Read public channel messages
- `channels:read` - List public channels
- `groups:history` - Read private channel messages
- `groups:read` - List private channels
- `search:read` - Search messages
- `users:read` - List users
- `users:read.email` - Lookup users by email

## License

MIT
