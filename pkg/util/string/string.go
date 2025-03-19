package string

import (
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
