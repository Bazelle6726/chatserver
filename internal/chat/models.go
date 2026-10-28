package chat

import (
	"strings"
	"time"
)

// Meta is embedded by value into Message (struct embedding).
// Its fields are "promoted": you can write msg.ID instead of msg.Meta.ID.
type Meta struct {
	ID        MessageID
	CreatedAt time.Time
}

type User struct {
	ID   UserID
	Name string
	Role Role
}

// Admin embeds *User - a POINTER (embedded pointer).
// Changing admin.Name changes the same User the store holds, not a copy.
type Admin struct {
	*User
	BannedWords []string
}

type Message struct {
	Meta   // embedded by value: small, always present
	Room   RoomID
	Author UserID
	Kind   MessageKind
	Text   string
}

// Promote turns a user into an admin.
//
// User's Role is set to RoleAdmin and return an Admin that
// embeds the SAME *User (don't copy it). BannedWords can stay nil.
func Promote(u *User) Admin {
	u.Role = RoleAdmin
	return Admin{User: u}
}

// Rename uses a pointer receiver because it modifies the user.
//
// The new name is validated with ValidateName first; on error it is returned
func (u *User) Rename(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	u.Name = name
	return nil
}

// IsBanned uses a value receiver because it only reads.
// Because Admin embeds *User, admin.Rename(...)
//
// Returns true if text contains any of a.BannedWords,
// ignoring case.
func (a Admin) IsBanned(text string) bool {
	text = strings.ToLower(text)
	for _, word := range a.BannedWords {
		if strings.Contains(text, strings.ToLower(word)) {
			return true
		}
	}
	return false
}
