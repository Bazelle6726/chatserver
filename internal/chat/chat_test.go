package chat

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRoleString(t *testing.T) {
	tests := []struct {
		role Role
		want string
	}{
		{RoleGuest, "guest"},
		{RoleMember, "member"},
		{RoleAdmin, "admin"},
		{Role(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.role.String(); got != tt.want {
			t.Errorf("Role(%d).String() = %q, want %q", int(tt.role), got, tt.want)
		}
	}
	// fmt uses String() automatically because Role implements fmt.Stringer.
	if got := fmt.Sprint(RoleAdmin); got != "admin" {
		t.Errorf("fmt.Sprint(RoleAdmin) = %q, want %q", got, "admin")
	}
}

func TestMessageKindString(t *testing.T) {
	want := map[MessageKind]string{
		KindUnspecified: "unspecified",
		KindText:        "text",
		KindJoin:        "join",
		KindLeave:       "leave",
		KindSystem:      "system",
	}
	for k, w := range want {
		if got := k.String(); got != w {
			t.Errorf("MessageKind(%d).String() = %q, want %q", int(k), got, w)
		}
	}
}

func TestCanModerate(t *testing.T) {
	if RoleGuest.CanModerate() || RoleMember.CanModerate() {
		t.Error("only admins should be able to moderate")
	}
	if !RoleAdmin.CanModerate() {
		t.Error("admins should be able to moderate")
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"bob", "alice_99", "go-gophers", "Zoë", strings.Repeat("a", MaxNameLen)}
	invalid := []string{"", "ab", "has space", "semi;colon", strings.Repeat("a", MaxNameLen+1)}

	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range invalid {
		err := ValidateName(name)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("ValidateName(%q) = %v, want a *ValidationError", name, err)
			continue
		}
		if ve.Field != "name" {
			t.Errorf("ValidateName(%q): Field = %q, want %q", name, ve.Field, "name")
		}
	}
}

func TestValidateMessage(t *testing.T) {
	if err := ValidateMessage("hello"); err != nil {
		t.Errorf("ValidateMessage(hello) = %v, want nil", err)
	}
	for _, text := range []string{"", "   ", strings.Repeat("x", MaxMessageLen+1)} {
		var ve *ValidationError
		if err := ValidateMessage(text); !errors.As(err, &ve) || ve.Field != "text" {
			t.Errorf("ValidateMessage(len %d) = %v, want *ValidationError on field text", len(text), err)
		}
	}
	// 500 multi-byte characters is still 500 characters.
	if err := ValidateMessage(strings.Repeat("é", MaxMessageLen)); err != nil {
		t.Errorf("ValidateMessage(500 x 'é') = %v, want nil (count runes, not bytes)", err)
	}
}

func TestPromoteSharesUser(t *testing.T) {
	u := &User{ID: "u-1", Name: "bob", Role: RoleMember}
	admin := Promote(u)

	if admin.User != u {
		t.Fatal("Promote should embed the same *User, not a copy")
	}
	if u.Role != RoleAdmin {
		t.Errorf("after Promote, user Role = %v, want admin", u.Role)
	}
	// Field promotion: admin.Name reads u.Name through the embedded pointer.
	u.Name = "robert"
	if admin.Name != "robert" {
		t.Errorf("admin.Name = %q, want %q (embedded pointer should see changes)", admin.Name, "robert")
	}
}

func TestRename(t *testing.T) {
	u := &User{Name: "bob"}
	if err := u.Rename("bobby"); err != nil {
		t.Fatalf("Rename(bobby) = %v", err)
	}
	if u.Name != "bobby" {
		t.Errorf("Name = %q, want bobby", u.Name)
	}

	err := u.Rename("x")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("Rename(x) = %v, want *ValidationError", err)
	}
	if u.Name != "bobby" {
		t.Errorf("Name changed to %q after a failed rename", u.Name)
	}
}

func TestIsBanned(t *testing.T) {
	a := Admin{User: &User{}, BannedWords: []string{"spam", "scam"}}
	if !a.IsBanned("Buy my SPAM now") {
		t.Error("should match ignoring case")
	}
	if a.IsBanned("hello there") {
		t.Error("clean text should not be banned")
	}
}

// newTestRoom fails the test instead of panicking if NewRoom isn't done yet.
func newTestRoom(t *testing.T, max int) *Room {
	t.Helper()
	r := NewRoom("r-1", "general", max)
	if r.members == nil || r.Settings == nil {
		t.Fatal("NewRoom must initialise members and Settings with make (TODO 9)")
	}
	return r
}

func TestRoomJoinLeave(t *testing.T) {
	r := newTestRoom(t, 10)

	if err := r.Join("u-1"); err != nil {
		t.Fatalf("Join = %v", err)
	}
	if !r.IsMember("u-1") || r.MemberCount() != 1 {
		t.Fatal("u-1 should be the only member")
	}
	if err := r.Join("u-1"); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("second Join = %v, want ErrAlreadyExists", err)
	}
	if err := r.Leave("u-1"); err != nil {
		t.Errorf("Leave = %v", err)
	}
	if err := r.Leave("u-1"); !errors.Is(err, ErrNotMember) {
		t.Errorf("second Leave = %v, want ErrNotMember", err)
	}
}

func msg(i int, author UserID) Message {
	return Message{
		Meta:   Meta{ID: MessageID(fmt.Sprintf("m-%d", i)), CreatedAt: time.Unix(int64(i), 0)},
		Author: author,
		Kind:   KindText,
		Text:   fmt.Sprintf("message %d", i),
	}
}

func TestAddMessageRequiresMember(t *testing.T) {
	r := newTestRoom(t, 10)
	if err := r.AddMessage(msg(1, "stranger")); !errors.Is(err, ErrNotMember) {
		t.Errorf("AddMessage from non-member = %v, want ErrNotMember", err)
	}
}

func TestHistoryIsCapped(t *testing.T) {
	r := newTestRoom(t, 3)
	_ = r.Join("u-1")
	for i := 1; i <= 5; i++ {
		if err := r.AddMessage(msg(i, "u-1")); err != nil {
			t.Fatalf("AddMessage(%d) = %v", i, err)
		}
	}
	got := r.LastN(10)
	if len(got) != 3 {
		t.Fatalf("LastN(10) returned %d messages, want 3 (history capped at 3)", len(got))
	}
	for i, want := range []MessageID{"m-3", "m-4", "m-5"} {
		if got[i].ID != want {
			t.Errorf("LastN[%d].ID = %s, want %s (oldest first)", i, got[i].ID, want)
		}
	}
	if cap(r.history) > 3 {
		t.Errorf("history capacity grew to %d; reuse the backing array instead of reslicing forever", cap(r.history))
	}
}

func TestLastNReturnsCopy(t *testing.T) {
	r := newTestRoom(t, 10)
	_ = r.Join("u-1")
	for i := 1; i <= 4; i++ {
		_ = r.AddMessage(msg(i, "u-1"))
	}

	got := r.LastN(2)
	if len(got) != 2 || got[0].ID != "m-3" || got[1].ID != "m-4" {
		t.Fatalf("LastN(2) = %v, want m-3, m-4", ids(got))
	}
	got[0].Text = "changed by caller"
	if again := r.LastN(2); again[0].Text == "changed by caller" {
		t.Error("modifying the returned slice changed the room's history; return a copy")
	}

	if empty := r.LastN(0); empty == nil || len(empty) != 0 {
		t.Errorf("LastN(0) = %#v, want an empty non-nil slice", empty)
	}
}

func TestSettingsCopy(t *testing.T) {
	r := newTestRoom(t, 10)
	r.Settings["topic"] = "Go"
	c := r.SettingsCopy()
	c["topic"] = "Rust"
	if r.Settings["topic"] != "Go" {
		t.Error("changing the copy changed the room's settings")
	}
}

func ids(ms []Message) []MessageID {
	out := make([]MessageID, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}
