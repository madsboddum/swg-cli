package archive

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrBadPattern reports a malformed glob pattern.
var ErrBadPattern = errors.New("syntax error in pattern")

// MatchPath reports whether path matches a glob pattern.
//
// The syntax is shell-like, over slash-separated paths:
//
//	**         any sequence of characters, including slashes
//	*          any sequence of characters except a slash
//	?          any single character except a slash
//	[abc]      any character in the class; [!abc] any character outside it
//	\x         a literal x
func MatchPath(pattern, path string) (bool, error) {
	re, err := compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(path), nil
}

// HasMeta reports whether pattern holds any glob metacharacter, and so needs
// matching rather than a plain comparison.
func HasMeta(pattern string) bool {
	return strings.ContainsAny(pattern, `*?[\`)
}

// compile translates a glob pattern into an anchored regular expression.
func compile(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString(`\A`)

	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				b.WriteString(`.*`)
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		case '[':
			class, n, err := compileClass(pattern[i:])
			if err != nil {
				return nil, err
			}
			b.WriteString(class)
			i += n - 1
		case '\\':
			if i+1 >= len(pattern) {
				return nil, fmt.Errorf("%w %q: trailing backslash", ErrBadPattern, pattern)
			}
			i++
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}

	b.WriteString(`\z`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrBadPattern, pattern, err)
	}
	return re, nil
}

// compileClass translates the bracket expression at the start of s, returning
// the regexp equivalent and how many bytes of s it consumed.
func compileClass(s string) (string, int, error) {
	var b strings.Builder
	b.WriteByte('[')

	i := 1
	if i < len(s) && (s[i] == '!' || s[i] == '^') {
		b.WriteByte('^')
		i++
	}
	// A ] in the first position is a literal, matching shell globs.
	if i < len(s) && s[i] == ']' {
		b.WriteString(`\]`)
		i++
	}

	for i < len(s) && s[i] != ']' {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) {
				return "", 0, fmt.Errorf("%w %q: trailing backslash", ErrBadPattern, s)
			}
			i++
			b.WriteString(regexp.QuoteMeta(string(s[i])))
		case '-':
			b.WriteByte('-')
		case '^':
			b.WriteString(`\^`)
		default:
			b.WriteString(regexp.QuoteMeta(string(s[i])))
		}
		i++
	}
	if i >= len(s) {
		return "", 0, fmt.Errorf("%w %q: unclosed [", ErrBadPattern, s)
	}

	b.WriteByte(']')
	return b.String(), i + 1, nil
}
