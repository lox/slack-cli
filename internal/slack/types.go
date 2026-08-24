package slack

import "encoding/json"

type Message struct {
	Type        string       `json:"type"`
	Subtype     string       `json:"subtype,omitempty"`
	User        string       `json:"user"`
	Text        string       `json:"text"`
	TS          string       `json:"ts"`
	ThreadTS    string       `json:"thread_ts,omitempty"`
	ReplyCount  int          `json:"reply_count,omitempty"`
	Channel     *Channel     `json:"channel,omitempty"`
	Permalink   string       `json:"permalink,omitempty"`
	Files       []File       `json:"files,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Blocks      []Block      `json:"blocks,omitempty"`
}

type File struct {
	ID                 string   `json:"id"`
	Created            int64    `json:"created,omitempty"`
	Timestamp          int64    `json:"timestamp,omitempty"`
	Name               string   `json:"name"`
	Title              string   `json:"title"`
	Mimetype           string   `json:"mimetype"`
	Filetype           string   `json:"filetype"`
	PrettyType         string   `json:"pretty_type,omitempty"`
	User               string   `json:"user,omitempty"`
	Editable           bool     `json:"editable,omitempty"`
	Size               int      `json:"size"`
	Mode               string   `json:"mode,omitempty"`
	IsExternal         bool     `json:"is_external,omitempty"`
	IsPublic           bool     `json:"is_public,omitempty"`
	PublicURLShared    bool     `json:"public_url_shared,omitempty"`
	URLPrivate         string   `json:"url_private"`
	URLPrivateDownload string   `json:"url_private_download"`
	Permalink          string   `json:"permalink"`
	PermalinkPublic    string   `json:"permalink_public,omitempty"`
	Channels           []string `json:"channels,omitempty"`
	Groups             []string `json:"groups,omitempty"`
	IMs                []string `json:"ims,omitempty"`
	FileAccess         string   `json:"file_access,omitempty"`
}

type Attachment struct {
	Title     string `json:"title"`
	Text      string `json:"text"`
	Fallback  string `json:"fallback"`
	ImageURL  string `json:"image_url"`
	TitleLink string `json:"title_link"`
}

type Block struct {
	Type     string     `json:"type"`
	Text     *BlockText `json:"text,omitempty"`
	ImageURL string     `json:"image_url,omitempty"`
	AltText  string     `json:"alt_text,omitempty"`
	Title    *BlockText `json:"title,omitempty"`
}

type BlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (t *BlockText) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		t.Type = "plain_text"
		return json.Unmarshal(data, &t.Text)
	}

	type blockText BlockText
	return json.Unmarshal(data, (*blockText)(t))
}

type RepliesResponse struct {
	OK       bool      `json:"ok"`
	Messages []Message `json:"messages"`
	HasMore  bool      `json:"has_more"`
}

type HistoryResponse struct {
	OK       bool      `json:"ok"`
	Messages []Message `json:"messages"`
	HasMore  bool      `json:"has_more"`
}

type User struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	RealName string  `json:"real_name"`
	Deleted  bool    `json:"deleted"`
	IsBot    bool    `json:"is_bot"`
	TZ       string  `json:"tz"`
	Profile  Profile `json:"profile"`
}

type Profile struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Title       string `json:"title"`
}

type UsersResponse struct {
	OK               bool             `json:"ok"`
	Members          []User           `json:"members"`
	ResponseMetadata ResponseMetadata `json:"response_metadata,omitempty"`
}

type Channel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsChannel  bool   `json:"is_channel"`
	IsGroup    bool   `json:"is_group"`
	IsIM       bool   `json:"is_im"`
	IsMPIM     bool   `json:"is_mpim"`
	User       string `json:"user,omitempty"`
	IsOpen     bool   `json:"is_open,omitempty"`
	IsPrivate  bool   `json:"is_private"`
	IsArchived bool   `json:"is_archived"`
	NumMembers int    `json:"num_members"`
	Topic      Topic  `json:"topic,omitempty"`
	Purpose    Topic  `json:"purpose,omitempty"`
}

type Topic struct {
	Value string `json:"value"`
}

type ConversationsResponse struct {
	OK               bool             `json:"ok"`
	Channels         []Channel        `json:"channels"`
	ResponseMetadata ResponseMetadata `json:"response_metadata,omitempty"`
}

type ResponseMetadata struct {
	NextCursor string `json:"next_cursor,omitempty"`
}

type OpenConversationResponse struct {
	OK          bool    `json:"ok"`
	AlreadyOpen bool    `json:"already_open,omitempty"`
	NoOp        bool    `json:"no_op,omitempty"`
	Channel     Channel `json:"channel"`
}

type SearchResponse struct {
	OK       bool `json:"ok"`
	Messages struct {
		Total   int           `json:"total"`
		Matches []SearchMatch `json:"matches"`
	} `json:"messages"`
}

type SearchMatch struct {
	Type      string        `json:"type"`
	Subtype   string        `json:"subtype,omitempty"`
	User      string        `json:"user"`
	Username  string        `json:"username"`
	Text      string        `json:"text"`
	TS        string        `json:"ts"`
	Channel   SearchChannel `json:"channel"`
	Permalink string        `json:"permalink"`
}

type SearchChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FilesListResponse struct {
	OK    bool   `json:"ok"`
	Files []File `json:"files"`
}

type FileInfoResponse struct {
	OK   bool `json:"ok"`
	File File `json:"file"`
}

type DeleteFileResponse struct {
	OK bool `json:"ok"`
}

type GetUploadURLExternalResponse struct {
	OK        bool   `json:"ok"`
	UploadURL string `json:"upload_url"`
	FileID    string `json:"file_id"`
}

type CompleteUploadExternalResponse struct {
	OK    bool   `json:"ok"`
	Files []File `json:"files"`
}

type AuthTestResponse struct {
	OK     bool   `json:"ok"`
	URL    string `json:"url"`
	Team   string `json:"team"`
	User   string `json:"user"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
}
