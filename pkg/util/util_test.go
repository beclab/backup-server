package util

import (
	"fmt"
	"testing"
	"time"
)

func TestT(t *testing.T) {
	var a = ""
	var b = Base64encode([]byte(a))
	fmt.Println(b)
}

func TestS(t *testing.T) {
	// 1970-01-01 20:03:00
	// 43380000
	var a = fmt.Sprintf("1970-01-01 %s:00", "22:06")
	var b, _ = time.ParseInLocation("2006-01-02 15:04:05", a, time.Local)
	fmt.Println("---1---", b.UnixMilli())

}

func TestX(t *testing.T) {
	c, _ := ParseTimestampToLocal("1744898760")
	fmt.Println("---c---", c)
}
