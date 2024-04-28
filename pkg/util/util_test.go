package util

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestT(t *testing.T) {
	var a = "-18000000"
	x, y := ParseTimestampToLocal(a)
	if y != nil {
		fmt.Println("---err---", y)
	}

	fmt.Println("---x---", x)
}

func TestP(t *testing.T) {
	var a = "21:30"
	var b = fmt.Sprintf("1970-01-01 %s:00", a)
	// var utcLocation, _ = time.LoadLocation("")

	localzone, localoffset := time.Now().Zone()
	fmt.Println("---1---", localzone)
	fmt.Println("---2---", localoffset)

	a1, _ := time.Parse("2006-01-02 15:04:05", b)
	a2 := a1.Add(-time.Duration(localoffset) * time.Second)
	fmt.Println("---a1---", a1)
	fmt.Println("---a2---", a2)
	fmt.Println("---a3---", a2.UnixMilli())

}

func TestTimeofDay(t *testing.T) {
	// var aa = "48600000"
	var aa = "13:30"
	if !strings.Contains(aa, ":") {
		timeInUTC, err := ParseTimestampToLocal(aa)
		if err != nil {
			fmt.Println("invalid times of day format, eg: '48600000'")
			return
		}
		aa = timeInUTC
	} else {
		timeSplit := strings.Split(aa, ":")
		if !strings.Contains(aa, ":") || len(timeSplit) != 2 {
			fmt.Println("invalid times of day format, eg: '07:30'")
			return
		}
	}
	fmt.Println("---r---", aa)
}

func TestX1(t *testing.T) {
	var a = "21:30"
	var b, err = ParseLocalToTimestamp(a)
	if err != nil {
		fmt.Println("---err---", err)
	}
	fmt.Println("---b---", b)
}
