package generator

import (
	"fmt"
	"strings"
)

// ValidateName checks an entity name (the argument of
// "goforge generate handler <name>"). Entity names become Go
// identifiers and file names, so the rule is: snake_case, lowercase
// ASCII, starting with a letter, no leading/trailing/double
// underscores. "user-profile" is rejected on purpose — dashes are not
// legal in identifiers, and silently converting input the user typed
// is a code generator anti-pattern.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("entity name must not be empty")
	}
	if name[0] < 'a' || name[0] > 'z' {
		return fmt.Errorf("%q must start with a lowercase letter", name)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
		case c == '_':
			if i == len(name)-1 {
				return fmt.Errorf("%q must not end with an underscore", name)
			}
			if name[i+1] == '_' {
				return fmt.Errorf("%q must not contain consecutive underscores", name)
			}
		default:
			return fmt.Errorf("%q may only contain lowercase letters, digits and underscores", name)
		}
	}
	return nil
}

// Pascal converts snake_case to PascalCase: "user" → "User",
// "user_profile" → "UserProfile". Input must pass ValidateName first,
// so byte slicing is safe (ASCII only).
func Pascal(name string) string {
	var b strings.Builder
	for _, part := range strings.Split(name, "_") {
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

// Camel converts snake_case to camelCase: "user" → "user",
// "user_profile" → "userProfile".
func Camel(name string) string {
	p := Pascal(name)
	return strings.ToLower(p[:1]) + p[1:]
}
