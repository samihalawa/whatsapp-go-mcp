package chat

// Request and Response structures for chat operations

type ListChatsRequest struct {
	Limit    int    `json:"limit" query:"limit"`
	Offset   int    `json:"offset" query:"offset"`
	Search   string `json:"search" query:"search"`
	HasMedia bool   `json:"has_media" query:"has_media"`
}

type ListChatsResponse struct {
	Data       []ChatInfo         `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
}

type GetChatMessagesRequest struct {
	ChatJID   string  `json:"chat_jid" uri:"chat_jid"`
	Limit     int     `json:"limit" query:"limit"`
	Offset    int     `json:"offset" query:"offset"`
	StartTime *string `json:"start_time" query:"start_time"`
	EndTime   *string `json:"end_time" query:"end_time"`
	MediaOnly bool    `json:"media_only" query:"media_only"`
	IsFromMe  *bool   `json:"is_from_me" query:"is_from_me"`
	Search    string  `json:"search" query:"search"`
}

type GetChatMessagesResponse struct {
	Data       []MessageInfo      `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
	ChatInfo   ChatInfo           `json:"chat_info"`
}

// Pin Chat operations
type PinChatRequest struct {
	ChatJID string `json:"chat_jid" uri:"chat_jid"`
	Pinned  bool   `json:"pinned"`
}

type PinChatResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	ChatJID string `json:"chat_jid"`
	Pinned  bool   `json:"pinned"`
}

type ChatInfo struct {
	JID                 string  `json:"jid"`
	Name                string  `json:"name"`
	LastMessageTime     string  `json:"last_message_time"`
	LastMessage         string  `json:"last_message,omitempty"`
	LastMessageFrom     string  `json:"last_message_from,omitempty"`
	LastMessageType     string  `json:"last_message_type,omitempty"`
	UnreadCount         int     `json:"unread_count"`
	IsPinned            bool    `json:"is_pinned"`
	IsArchived          bool    `json:"is_archived"`
	IsMuted             bool    `json:"is_muted"`
	IsGroup             bool    `json:"is_group"`
	MessagesSynced      bool    `json:"messages_synced"` // Indicates if message history has been synced from WhatsApp
	EphemeralExpiration uint32  `json:"ephemeral_expiration"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

type MessageInfo struct {
	ID         string `json:"id"`
	ChatJID    string `json:"chat_jid"`
	SenderJID  string `json:"sender_jid"`
	Content    string `json:"content"`
	Timestamp  string `json:"timestamp"`
	IsFromMe   bool   `json:"is_from_me"`
	MediaType  string `json:"media_type"`
	Filename   string `json:"filename"`
	URL        string `json:"url"`
	FileLength uint64 `json:"file_length"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type PaginationResponse struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// Archive Chat operations
type ArchiveChatRequest struct {
	ChatJID string `json:"chat_jid" uri:"chat_jid"`
	Archive bool   `json:"archive"`
}

type ArchiveChatResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	ChatJID string `json:"chat_jid"`
	Archived bool   `json:"archived"`
}

// Delete Chat operations
type DeleteChatRequest struct {
	ChatJID     string `json:"chat_jid" uri:"chat_jid"`
	KeepStarred bool   `json:"keep_starred"`
}

type DeleteChatResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	ChatJID string `json:"chat_jid"`
}

// Mark Chat As Read operations
type MarkChatAsReadRequest struct {
	ChatJID string `json:"chat_jid" uri:"chat_jid"`
}

type MarkChatAsReadResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	ChatJID string `json:"chat_jid"`
}
