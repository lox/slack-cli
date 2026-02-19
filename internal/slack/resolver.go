package slack

import (
	"strings"

	"github.com/enescakir/emoji"
)

// Resolver resolves Slack user IDs and channel IDs to human-readable names,
// and formats message text by replacing mentions and emoji shortcodes.
// Results are cached for the lifetime of the Resolver.
type Resolver struct {
	client       *Client
	userCache    map[string]string
	channelCache map[string]string
}

// NewResolver creates a Resolver that uses the given client for API lookups.
func NewResolver(client *Client) *Resolver {
	return &Resolver{
		client:       client,
		userCache:    make(map[string]string),
		channelCache: make(map[string]string),
	}
}

// ResolveUser returns a display name for the given user ID.
// It prefers DisplayName, then RealName, then Username.
// Returns "bot" for empty IDs and falls back to the raw ID on error.
func (r *Resolver) ResolveUser(userID string) string {
	if userID == "" {
		return "bot"
	}

	if name, ok := r.userCache[userID]; ok {
		return name
	}

	user, err := r.client.GetUserInfo(userID)
	if err != nil {
		r.userCache[userID] = userID
		return userID
	}

	name := user.Profile.DisplayName
	if name == "" {
		name = user.RealName
	}
	if name == "" {
		name = user.Name
	}

	r.userCache[userID] = name
	return name
}

// ResolveChannel returns a channel name for the given channel ID.
// Falls back to the raw ID on error.
func (r *Resolver) ResolveChannel(channelID string) string {
	if name, ok := r.channelCache[channelID]; ok {
		return name
	}

	channel, err := r.client.GetConversationInfo(channelID)
	if err != nil {
		r.channelCache[channelID] = channelID
		return channelID
	}

	r.channelCache[channelID] = channel.Name
	return channel.Name
}

// FormatText replaces user mentions (<@U123>), channel mentions (<#C123|name>),
// URL links (<http://...|label>), and emoji shortcodes in message text.
func (r *Resolver) FormatText(text string) string {
	result := text

	// Resolve user mentions: <@U123ABC> -> @username
	for {
		start := strings.Index(result, "<@")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		mention := result[start : end+1]
		userID := result[start+2 : end]

		// Handle <@U123|name> format
		if pipeIdx := strings.Index(userID, "|"); pipeIdx != -1 {
			userID = userID[:pipeIdx]
		}

		displayName := r.ResolveUser(userID)
		result = strings.Replace(result, mention, "@"+displayName, 1)
	}

	// Resolve channel mentions: <#C123ABC|channel-name> -> #channel-name
	for {
		start := strings.Index(result, "<#")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		mention := result[start : end+1]
		channelPart := result[start+2 : end]

		var channelName string
		if pipeIdx := strings.Index(channelPart, "|"); pipeIdx != -1 {
			channelName = channelPart[pipeIdx+1:]
		} else {
			channelName = r.ResolveChannel(channelPart)
		}

		result = strings.Replace(result, mention, "#"+channelName, 1)
	}

	// Resolve URL links: <http://example.com|label> -> label (http://example.com)
	for {
		start := strings.Index(result, "<http")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		link := result[start : end+1]
		linkContent := result[start+1 : end]

		if pipeIdx := strings.Index(linkContent, "|"); pipeIdx != -1 {
			linkURL := linkContent[:pipeIdx]
			label := linkContent[pipeIdx+1:]
			result = strings.Replace(result, link, label+" ("+linkURL+")", 1)
		} else {
			result = strings.Replace(result, link, linkContent, 1)
		}
	}

	// Convert emoji shortcodes: :smile: -> 😊
	result = emoji.Parse(result)

	return result
}
