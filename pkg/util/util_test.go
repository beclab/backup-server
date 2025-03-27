package util

import (
	"fmt"
	"testing"
)

func TestT(t *testing.T) {
	var a = ""
	var b = Base64encode([]byte(a))
	fmt.Println(b)
}
