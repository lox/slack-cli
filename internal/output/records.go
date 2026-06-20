package output

import (
	"strings"

	"github.com/lox/slack-cli/internal/slack"
)

// Message is the common shape emitted by search, channel read, and thread
// read. Callers populate whichever fields apply to their source.
type Message struct {
	TS         string      `json:"ts"`
	ThreadTS   string      `json:"thread_ts,omitempty"`
	Type       string      `json:"type,omitempty"`
	Subtype    string      `json:"subtype,omitempty"`
	User       string      `json:"user,omitempty"`
	UserID     string      `json:"user_id,omitempty"`
	Text       string      `json:"text"`
	TextRaw    string      `json:"text_raw,omitempty"`
	Channel    *ChannelRef `json:"channel,omitempty"`
	Workspace  string      `json:"workspace,omitempty"`
	Permalink  string      `json:"permalink,omitempty"`
	ReplyCount int         `json:"reply_count,omitempty"`
	Files      []FileRef   `json:"files,omitempty"`
}

// ChannelRef is a short reference embedded in Message results. Full channel
// records use the Channel type below.
type ChannelRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// FileRef is a compact file reference embedded on message attachments.
type FileRef struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Title     string `json:"title,omitempty"`
	Mimetype  string `json:"mimetype,omitempty"`
	Permalink string `json:"permalink,omitempty"`
}

// Channel is the shape emitted by channel list and channel info.
type Channel struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type"`
	IsPrivate  bool   `json:"is_private,omitempty"`
	IsArchived bool   `json:"is_archived,omitempty"`
	NumMembers int    `json:"num_members,omitempty"`
	Topic      string `json:"topic,omitempty"`
	Purpose    string `json:"purpose,omitempty"`
}

// User is the shape emitted by user list and user info.
type User struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	RealName    string `json:"real_name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Title       string `json:"title,omitempty"`
	TZ          string `json:"tz,omitempty"`
	IsBot       bool   `json:"is_bot,omitempty"`
	Deleted     bool   `json:"deleted,omitempty"`
}

// ChannelTypeFor maps Slack's channel flags to a single-word type tag that
// downstream consumers can branch on cheaply.
func ChannelTypeFor(ch slack.Channel) string {
	switch {
	case ch.IsIM:
		return "im"
	case ch.IsMPIM:
		return "mpim"
	case ch.IsPrivate, ch.IsGroup:
		return "private_channel"
	default:
		return "channel"
	}
}

// ChannelTypeFromID infers a type tag from a raw Slack channel/DM ID prefix.
// Only `D` is an unambiguous signal (direct message). Both `C` and `G` IDs
// are ambiguous: modern workspaces use `C` for public and private channels
// alike, and `G` covers multi-party IMs plus legacy private channels.
// Returning the empty tag for anything other than `D` forces callers that
// care about the public/private distinction to resolve the full
// slack.Channel via ChannelTypeFor rather than branching on a guess.
func ChannelTypeFromID(id string) string {
	if len(id) == 0 {
		return ""
	}
	if id[0] == 'D' {
		return "im"
	}
	return ""
}

// ToChannel converts a slack.Channel wire type into the public Channel record.
func ToChannel(ch slack.Channel) Channel {
	return Channel{
		ID:         ch.ID,
		Name:       ch.Name,
		Type:       ChannelTypeFor(ch),
		IsPrivate:  ch.IsPrivate,
		IsArchived: ch.IsArchived,
		NumMembers: ch.NumMembers,
		Topic:      ch.Topic.Value,
		Purpose:    ch.Purpose.Value,
	}
}

// ToChannelRef returns a compact reference suitable for embedding on Message.
func ToChannelRef(ch slack.Channel) *ChannelRef {
	return &ChannelRef{
		ID:   ch.ID,
		Name: ch.Name,
		Type: ChannelTypeFor(ch),
	}
}

// ChannelRefFromID builds a ChannelRef for a known channel ID. When resolver
// is non-nil it does a cached conversations.info lookup to produce an
// accurate type (public/private/mpim) from the full channel record, since
// the ID prefix alone cannot disambiguate modern C-prefixed public vs
// private channels. If the lookup fails or resolver is nil it falls back
// to prefix-based inference, which leaves the type empty for ambiguous
// prefixes. nameHint is used when the caller already knows the channel
// name and wants to avoid a redundant lookup.
func ChannelRefFromID(resolver *slack.Resolver, channelID, nameHint string) *ChannelRef {
	ref := &ChannelRef{ID: channelID, Name: nameHint}
	if resolver != nil {
		if info := resolver.ResolveChannelInfo(channelID); info != nil {
			ref.Type = ChannelTypeFor(*info)
			if ref.Name == "" {
				ref.Name = info.Name
			}
			return ref
		}
	}
	ref.Type = ChannelTypeFromID(channelID)
	return ref
}

// ToUser converts a slack.User wire type into the public User record.
func ToUser(u slack.User) User {
	return User{
		ID:          u.ID,
		Name:        u.Name,
		RealName:    u.RealName,
		DisplayName: u.Profile.DisplayName,
		Email:       u.Profile.Email,
		Title:       u.Profile.Title,
		TZ:          u.TZ,
		IsBot:       u.IsBot,
		Deleted:     u.Deleted,
	}
}

// ToFileRef returns a compact file reference suitable for embedding on Message.
func ToFileRef(f slack.File) FileRef {
	return FileRef{
		ID:        f.ID,
		Name:      f.Name,
		Title:     f.Title,
		Mimetype:  f.Mimetype,
		Permalink: f.Permalink,
	}
}

// MessageConverter converts a slack.Message to the public Message record. It
// needs a resolver for user display names and formatted text, plus optional
// channel context for embedded references and workspace host for citations.
type MessageConverter struct {
	Resolver  *slack.Resolver
	Channel   *ChannelRef
	Workspace string
}

// Convert turns one slack.Message into the public Message record.
func (mc MessageConverter) Convert(m slack.Message) Message {
	var display string
	var userID string
	if strings.TrimSpace(m.User) != "" {
		userID = m.User
		if mc.Resolver != nil {
			display = mc.Resolver.ResolveUser(m.User)
		}
	}

	text := m.BodyText()
	if mc.Resolver != nil {
		text = mc.Resolver.FormatText(text)
	}

	var files []FileRef
	if len(m.Files) > 0 {
		files = make([]FileRef, 0, len(m.Files))
		for _, f := range m.Files {
			files = append(files, ToFileRef(f))
		}
	}

	// Channel: prefer a per-record channel (search) when present. Otherwise
	// fall back to the command scope channel.
	var ch *ChannelRef
	if m.Channel != nil && m.Channel.ID != "" {
		ch = ToChannelRef(*m.Channel)
	} else {
		ch = mc.Channel
	}

	rec := Message{
		TS:         m.TS,
		ThreadTS:   m.ThreadTS,
		Type:       m.Type,
		Subtype:    m.Subtype,
		User:       display,
		UserID:     userID,
		Text:       text,
		TextRaw:    m.Text,
		Channel:    ch,
		Workspace:  mc.Workspace,
		Permalink:  m.Permalink,
		ReplyCount: m.ReplyCount,
		Files:      files,
	}
	return rec
}

// ConvertAll converts a slice of slack.Message records in order.
func (mc MessageConverter) ConvertAll(msgs []slack.Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, mc.Convert(m))
	}
	return out
}
