package chat

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	MinNameLen    = 3
	MaxNameLen    = 20
	MaxMessageLen = 500
)

// ValidateName checks user and room names.
//
// *ValidationError returns with Field "name" when:
//   - the name is shorter than MinNameLen or longer than MaxNameLen
//   - it contains anything other than letters, digits, '-' or '_'
//
// nil returns when the name is fine
func ValidateName(name string) error {
	if len([]rune(name)) < MinNameLen || len([]rune(name)) > MaxNameLen {
		return &ValidationError{
			Field:  "name",
			Reason: fmt.Sprintf("name must be between %d and %d characters long", MinNameLen, MaxNameLen),
		}
	}
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' {
			return &ValidationError{
				Field:  "name",
				Reason: "name can only contain letters, digits, hyphens, or underscores",
			}
		}
	}
	return nil
}

// ValidateMessage checks message text.
//
// *ValidationError returns with Field "text" if the text is empty
// (after strings.TrimSpace) or longer than MaxMessageLen characters.
func ValidateMessage(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return &ValidationError{
			Field:  "text",
			Reason: "message cannot be empty",
		}
	}
	if len([]rune(text)) > MaxMessageLen {
		return &ValidationError{
			Field:  "text",
			Reason: fmt.Sprintf("message cannot be longer than %d characters", MaxMessageLen),
		}
	}
	return nil
}
