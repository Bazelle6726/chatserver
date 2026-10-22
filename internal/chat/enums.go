package chat

// Enums with iota.

type Role int

const (
	RoleGuest Role = iota // 0 - also the zero value, so an unset Role is a Guest
	RoleMember
	RoleAdmin
)

// String makes Role satisfy fmt.Stringer, so fmt.Println(RoleAdmin) prints "admin".
//
// TODO(1): use a switch to return
// "guest", "member", "admin", and "unknown" for anything else.
func (r Role) String() string {
	return ""
}

type MessageKind int

const (
	KindUnspecified MessageKind = iota // matches protobuf's _UNSPECIFIED zero-value rule
	KindText
	KindJoin
	KindLeave
	KindSystem
)

// TODO(2): return "text", "join", "leave", "system", or "unspecified".
func (k MessageKind) String() string {
	return ""
}

// CanModerate reports whether this role may delete messages or ban users.
//
// TODO(3): only admins can moderate.
func (r Role) CanModerate() bool {
	return false
}
