package telegram

type Update struct {
	UpdateID   int64              `json:"update_id"`
	Message    *Message           `json:"message,omitempty"`
	ChatMember *ChatMemberUpdated `json:"chat_member,omitempty"`
}

type Message struct {
	MessageID      int64  `json:"message_id"`
	Date           int64  `json:"date"`
	From           *User  `json:"from,omitempty"`
	SenderChat     *Chat  `json:"sender_chat,omitempty"`
	Chat           Chat   `json:"chat"`
	Text           string `json:"text,omitempty"`
	NewChatMembers []User `json:"new_chat_members,omitempty"`
}

type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name,omitempty"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

type ChatMemberUpdated struct {
	Chat           Chat        `json:"chat"`
	From           User        `json:"from"`
	Date           int64       `json:"date"`
	OldChatMember  ChatMember  `json:"old_chat_member"`
	NewChatMember  ChatMember  `json:"new_chat_member"`
	InviteLink     *InviteLink `json:"invite_link,omitempty"`
	ViaJoinRequest bool        `json:"via_join_request,omitempty"`
}

type InviteLink struct {
	InviteLink string `json:"invite_link"`
	Name       string `json:"name,omitempty"`
}

type ChatMember struct {
	Status   string `json:"status"`
	User     User   `json:"user"`
	IsMember bool   `json:"is_member,omitempty"`
}

func (m ChatMember) IsAdministrator() bool {
	return m.Status == "creator" || m.Status == "administrator"
}

func (m ChatMember) IsActiveMember() bool {
	switch m.Status {
	case "creator", "administrator", "member":
		return true
	case "restricted":
		return m.IsMember
	default:
		return false
	}
}
