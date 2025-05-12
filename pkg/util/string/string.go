package string

import (
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var caser cases.Caser

func init() {
	caser = cases.Title(language.English)
}

func Title(str string) string {
	return caser.String(str)
}

func ReverseString(str string) string {
	if str == "" {
		return str
	}

	r := make([]rune, 0)

	for len(str) > 0 {
		c, size := utf8.DecodeLastRuneInString(str)
		r = append(r, c)
		str = str[:len(str)-size]
	}
	return string(r)
}

func Default(v, def string) string {
	if "" == v && "" == def {
		return ""
	}

	if v != "" {
		return v
	}
	return def
}

func IsOnlyWhitespace(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func IsReservedName(name string) bool {
	reserved := []string{".", ".."}
	for _, r := range reserved {
		if name == r {
			return true
		}
	}
	return false
}

func ContainsIllegalChars(name string) (bool, rune) {
	illegalChars := []rune{'\\', '/', '@', '!', '#'}
	for _, r := range name {
		for _, illegal := range illegalChars {
			if r == illegal {
				return true, r
			}
		}
	}
	return false, 0
}

func ContainsS3IllegalChars(name string) (bool, rune) {
	illegalChars := []rune{'\\', '{', '}', '^', '%', '`', ']', '[', '"', '<', '>', '#', '|', '?', '*'}
	for _, r := range name {
		for _, illegal := range illegalChars {
			if r == illegal {
				return true, r
			}
		}
	}
	return false, 0
}
