package chat

// Enums with iota.

type Role int

const (
	RoleGuest Role = iota // 0 - also the zero value, so an unset Role is a Guest
	RoleMember
	RoleAdmin
)

// String makes Role satisfy fmt.Stringer, so fmt.Println(RoleAdmin) prints "admin".

func (r Role) String() string {
	switch r {
	case RoleGuest:
		return "guest"
	case RoleMember:
		return "member"
	case RoleAdmin:
		return "admin"
	default:
		return "unknown"
	}

}

type MessageKind int

const (
	KindUnspecified MessageKind = iota // matches protobuf's _UNSPECIFIED zero-value rule
	KindText
	KindJoin
	KindLeave
	KindSystem
)

func (k MessageKind) String() string {
	switch k {
	case KindUnspecified:
		return "unspecified"
	case KindText:
		return "text"
	case KindJoin:
		return "join"
	case KindLeave:
		return "leave"
	case KindSystem:
		return "system"
	default:
		return "unknown"
	}
}

// CanModerate reports whether this role may delete messages or ban users.

func (r Role) CanModerate() bool {
	return r == RoleAdmin
}
