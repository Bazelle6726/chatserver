package chat

const (
	MinNameLen    = 3
	MaxNameLen    = 20
	MaxMessageLen = 500
)

// ValidateName checks user and room names.
//
// TODO(7): return a *ValidationError with Field "name" when:
//   - the name is shorter than MinNameLen or longer than MaxNameLen
//     (count characters, not bytes: use len([]rune(name)))
//   - it contains anything other than letters, digits, '-' or '_'
//     (hint: loop with for-range and use unicode.IsLetter / unicode.IsDigit)
//
// Return nil when the name is fine. Pick your own Reason text.
func ValidateName(name string) error {
	return nil
}

// ValidateMessage checks message text.
//
// TODO(8): return a *ValidationError with Field "text" if the text is empty
// (after strings.TrimSpace) or longer than MaxMessageLen characters.
func ValidateMessage(text string) error {
	return nil
}
