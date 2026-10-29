package api

import (
	"time"

	"chatserver/internal/chat"
)

// DTOs ("data transfer objects") are the shapes we send as JSON.
// They're separate from the domain types so the API can stay stable even if
// the internals change, and so we choose exactly what's exposed.
// The `json:"..."` struct tags set the JSON field names.

type userDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type roomDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Members int    `json:"members"`
}

type messageDTO struct {
	ID        string    `json:"id"`
	Room      string    `json:"room"`
	Author    string    `json:"author"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserDTO(u *chat.User) userDTO {
	return userDTO{ID: string(u.ID), Name: u.Name, Role: u.Role.String()}
}

func toRoomDTO(r *chat.Room) roomDTO {
	return roomDTO{ID: string(r.ID), Name: r.Name, Members: r.MemberCount()}
}

// toMessageDTO converts a domain message into its JSON shape.
//
// TODO(24): fill in every field, following toUserDTO above:
//   - IDs are defined types (chat.MessageID, chat.RoomID, chat.UserID):
//     convert them with string(...)
//   - m.ID and m.CreatedAt come from the embedded Meta (promoted fields)
//   - Kind should be the readable name: m.Kind.String()
func toMessageDTO(m chat.Message) messageDTO {
	return messageDTO{
		ID:        string(m.ID),
		Room:      string(m.Room),
		Author:    string(m.Author),
		Kind:      m.Kind.String(),
		Text:      m.Text,
		CreatedAt: m.CreatedAt,
	}
}

// toMessageDTOs converts a slice. Note make with a length, then fill by index.
func toMessageDTOs(ms []chat.Message) []messageDTO {
	out := make([]messageDTO, len(ms))
	for i, m := range ms {
		out[i] = toMessageDTO(m)
	}
	return out
}
