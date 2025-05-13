package util

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestT(t *testing.T) {
	// var u = "/Files/Home/Documents/space-cos"
	var u = "https://cos.ap-beijing.myqcloud.com/olares-test-zy-1254615039/olares备份/olares-backups/xxx-uuid"
	pp, _ := url.Parse(u)
	fmt.Println("---1 / scheme---", pp.Scheme)
	fmt.Println("---2 / path---", pp.Path)
	fmt.Println("---3 / host---", pp.Host)
	fmt.Println("---4 / first c---", pp.Path[:1])

	var i = strings.Index(pp.Path, "olares-backups/")
	var jj = pp.Path[i:]
	fmt.Println("---5---", jj)

	// var a = ""
	// var b = Base64encode([]byte(a))
	// fmt.Println(b)
}

func TestS(t *testing.T) {
	// 1970-01-01 20:03:00
	// 43380000
	var a = fmt.Sprintf("1970-01-01 %s:00", "15:56")
	var b, _ = time.ParseInLocation("2006-01-02 15:04:05", a, time.Local)
	fmt.Println("---1---", b.UnixMilli())

}

func TestX(t *testing.T) {
	c, _ := ParseTimestampToLocal("1744898760")
	fmt.Println("---c---", c)
}

func TestMD5(t *testing.T) {
	var a = MD5("/Files/Home/Documents")
	fmt.Println(a)
}
