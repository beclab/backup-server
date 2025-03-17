package util

import (
	"fmt"
	"strings"
	"testing"
)

func TestT(t *testing.T) {
	var a = "my-test-1-1742191620003"
	var b = strings.Split(a, "-")
	var c = strings.Join(b[:len(b)-1], "-")
	fmt.Println(c)
}
