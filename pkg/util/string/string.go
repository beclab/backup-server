package string

import "unicode/utf8"

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
