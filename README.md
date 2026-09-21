# slack-cli

A CLI for Slack - search, read channels/threads, and browse users from the command line.

## Installation

### Homebrew

```bash
brew install --cask lox/tap/slack-cli
```

### mise

```bash
mise use -g github:lox/slack-cli
```

This installs the latest release globally and puts `slack-cli` on your PATH via mise shims.

### Go

```bash
go install github.com/lox/slack-cli@latest
```

### Binaries

Download from [releases](https://github.com/lox/slack-cli/releases/latest).

### From source

```bash
git clone https://github.com/lox/slack-cli.git
cd slack-cli
mise run build
```

## Setup

### 1. Create a Slack App

Each workspace requires a Slack app for OAuth:

1. Go to https://api.slack.com/apps → **Create New App** → **From an app manifest**
2. Select your workspace
3. Paste the contents of [`slack-app-manifest.yaml`](slack-app-manifest.yaml)
4. Click **Create**
5. From **Basic Information**, copy the **Client ID** and **Client Secret**

### 2. Authenticate

```bash
slack-cli auth login
```

`auth login` prompts for Client ID and Client Secret when needed, then opens your browser for OAuth.

If a workspace is already logged in, `auth login` prompts whether to replace it or add another workspace.

Use `--workspace` when you want to target a specific workspace:

```bash
slack-cli --workspace buildkite.slack.com auth login
slack-cli --workspace buildkite auth login
```

`--workspace` accepts a full host (`buildkite.slack.com`), short host (`buildkite`), or team ID (`T123...`).

OAuth app credentials and tokens are both stored per workspace in `~/.config/slack-cli/config.json`.

Repeat `slack-cli auth login` for each workspace you want to access. Tokens are stored per workspace in the same XDG config file (`~/.config/slack-cli/config.json`).

### Environment Variables (optional)

For CI or automation, you can set credentials via flags or environment variables:

```bash
export SLACK_CLIENT_ID="your-client-id"
export SLACK_CLIENT_SECRET="your-client-secret"
slack-cli --workspace buildkite.slack.com auth login
```

`auth login` also supports `--client-id` and `--client-secret`.

### Remote login through an OAuth broker

For an orb or remote machine without a local browser callback, use an HTTPS
OAuth broker operated by your organisation:

```bash
slack-cli --workspace T0123456789 auth login \
  --auth-server https://auth.example.com --no-browser
```

Open the printed URL on your own machine and enter the confirmation code shown
by the CLI. Only approve sessions you started. The CLI waits up to ten minutes,
validates the returned token with Slack, and saves it in the normal private
config. For a new workspace, pass its exact Slack team ID; a configured workspace
host also works when its team ID is already saved. Use `--replace` to replace an
existing login without prompting.

`SLACK_AUTH_SERVER` can supply the broker origin instead of `--auth-server`.
Trust the operator of this URL: the broker handles your Slack user token.
The broker retains the Slack app client secret; the CLI and orb still receive
your user token and can access the Slack data its scopes permit. Broker login
does not use or save app credentials, and cannot be combined with `--client-id`
or `--client-secret`. Without an auth server, the existing localhost OAuth flow
is unchanged. `--no-browser` only suppresses opening a browser; it does not make
the localhost callback usable from another machine.

Token delivery is one-time. If the connection fails during delivery, or token
validation/storage fails afterwards, start a new login rather than retrying the
old session. Broker sessions also expire or disappear when the broker restarts.

## Usage

### View any Slack URL

```bash
slack-cli view <url>                    # View message, thread, or channel
slack-cli view <url> --markdown         # Output as markdown
slack-cli view <url> --inline-images auto|always|never
```

### Channels

```bash
slack-cli channel list                  # List channels you're in
slack-cli channel read "#general"       # Read recent messages
slack-cli channel read <url> --markdown # Read by URL as markdown
slack-cli channel info "#general"       # Show channel details
```

### Files

```bash
slack-cli file list                             # List recent files
slack-cli file list --json                      # List recent files as JSON
slack-cli file info F123                        # Show file metadata
slack-cli file info F123 --json                 # Show file metadata as JSON
slack-cli file download F123                    # Download to the current directory
slack-cli file upload "#general" ./report.txt   # Upload and share a file
slack-cli file upload @alice ./report.txt       # Upload into a DM
slack-cli file delete F123                      # Delete a file
```

### Search

```bash
slack-cli search "from:@alice project"  # Search messages
slack-cli search "in:#engineering bug"  # Search in channel
```

### Threads

```bash
slack-cli thread read <url>                      # Read thread by URL
slack-cli thread read <url> --markdown           # Read thread as markdown
slack-cli thread read -c C123 -t 1234567890.123  # Read by channel+ts
```

### Date filters

`search` accepts calendar filters: `--after`, `--before`, and `--on`.
`channel read` and `thread read` also accept rolling windows with `--last`.
Dates are interpreted in UTC and accept unambiguous calendar-day formats such
as `YYYY-MM-DD`, `YYYY/MM/DD`, `18 Apr 2026`, and `Apr 18 2026`. Timestamps,
partial dates, and other inputs with times are rejected.

```bash
slack-cli search "deploy" --after 2026-04-01 --before 2026-04-30
slack-cli search "incident" --on "Apr 18 2026"
slack-cli channel read "#general" --last 2w
slack-cli thread read <url> --on "18 Apr 2026" --json
```

### Users

```bash
slack-cli user list                     # List workspace users
slack-cli user info U123                # Show user details
slack-cli user info alice@acme.com      # Lookup by email
```

### Machine-readable output

Read, search, list, and info commands accept `--json` (pretty JSON array or
object) and `--jsonl` (one JSON object per line) for scripting and agent
consumption: `search`, `channel read`, `channel list`, `channel info`,
`file list`, `file info`, `thread read`, `user list`, `user info`.

```bash
slack-cli search "deploy" --limit 100 --jsonl | jq -c 'select(.channel.type == "channel")'
slack-cli channel read "#general" --limit 50 --json
slack-cli file list --limit 20 --jsonl
slack-cli thread read <url> --json
slack-cli channel list --json
slack-cli user list --jsonl
slack-cli channel info C123 --json
```

Message records emit a full normalized shape for machines: `ts`,
`thread_ts` (when Slack provides it), `type`, `subtype` (when set, e.g.
`bot_message`, `channel_join`, `channel_archive`, `huddle_thread`),
`user`, `user_id`, `text` (resolver-formatted), `text_raw`, `channel`,
`workspace`, `permalink`, `reply_count`, and `files` when those fields are
available. When Slack channel metadata is available, `channel.type` is one
of `channel`, `private_channel`, `im`, or `mpim`.

### Authentication

```bash
slack-cli --workspace <workspace> auth login    # Authenticate with Slack for that workspace
slack-cli auth login --replace                  # Replace current workspace login
slack-cli auth login --add-new                  # Add another workspace login
slack-cli auth login --client-id <id> --client-secret <secret>  # Non-interactive creds override
slack-cli auth status   # Check auth status
slack-cli auth logout --all                     # Clear all stored auth state
slack-cli --workspace <workspace> auth logout   # Clear one workspace token
```

### Multiple Workspaces

```bash
slack-cli auth login
slack-cli auth login                      # Run again for another workspace
slack-cli --workspace buildkite.slack.com search "deploy"
slack-cli --workspace T12345678 channel list
```

For URL-based commands (`view`, `thread read <url>`, `channel read <url>`, `channel info <url>`), the CLI automatically selects the token from the URL workspace when possible.

When a channel lookup returns `channel_not_found` and you have multiple workspaces configured, the CLI now suggests `--workspace` values to try.

## Agent Skill

An [Amp](https://ampcode.com) skill is included for AI agent integration:

```bash
amp skill add lox/slack-cli
```

This enables agents to use the CLI when asked to view Slack URLs, search messages, or look up users. See the [Amp manual](https://ampcode.com/manual#agent-skills) for more on skills.

## Required Scopes

The included manifest requests these user token scopes:

- `channels:history` - Read public channel messages
- `channels:read` - List public channels
- `files:read` - Read file metadata and download private file/image URLs
- `files:write` - Upload and delete files
- `groups:history` - Read private channel messages
- `groups:read` - List private channels
- `im:history` - Read direct message history
- `im:read` - Access direct message metadata
- `im:write` - Open direct messages for DM-targeted file uploads
- `mpim:history` - Read multi-party direct message history
- `mpim:read` - Access multi-party direct message metadata
- `search:read` - Search messages
- `users:read` - List users
- `users:read.email` - Lookup users by email

## License

MIT
