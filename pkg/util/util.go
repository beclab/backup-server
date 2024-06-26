package util

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func ParseToInt64(v string) int64 {
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return i
}

// ToJSON returns a json string
func ToJSON(v any) string {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		panic(err)
	}
	return buf.String()
}

// PrettyJSON returns a pretty formated json string
func PrettyJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return buf.String()
}

// FilePathExists returns a boolean, whether the file or directory is exists
func FilePathExists(name string) bool {
	_, err := os.Stat(name)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

// ListContains returns a boolean that v is in items
func ListContains[T comparable](items []T, v T) bool {
	if items == nil {
		return false
	}

	for _, item := range items {
		if v == item {
			return true
		}
	}
	return false
}

func BytesToMD5Hash(in []byte) string {
	hash := md5.Sum(in)
	return hex.EncodeToString(hash[:])
}

func BytesToSha256Hash(in []byte) string {
	h := sha256.New()
	h.Write(in)

	return fmt.Sprintf("%x", h.Sum(nil))
}

func EncodeStringToBase64(in string) string {
	return base64.StdEncoding.EncodeToString([]byte(in))
}

func DecodeBase64ToString(in string) string {
	ou, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return "'"
	}
	return string(ou)
}

func ParseToCron(frequency, timesOfDay string, dayOfWeek int) (cron string, err error) {
	var (
		hourInt   int
		minuteInt int
	)

	splitTimes := strings.Split(timesOfDay, ":")
	if len(splitTimes) != 2 {
		return "", errors.Errorf("invalid times of day: %q", timesOfDay)
	}
	hour, minute := splitTimes[0], splitTimes[1]

	hourInt, err = strconv.Atoi(hour)
	if err != nil {
		return
	}
	minuteInt, err = strconv.Atoi(minute)
	if err != nil {
		return
	}
	if hourInt < 0 || hourInt > 24 {
		return "", errors.Errorf("invalid hour %q", hour)
	}
	if minuteInt < 0 || minuteInt > 59 {
		return "", errors.Errorf("invalid minute %q", minute)
	}

	minuteHour := minute + " " + hour

	switch frequency {
	case "@hourly":
		cron = fmt.Sprintf("*/%d * * * *", minuteInt)
	case "@daily":
		cron = minuteHour + " * * *"
	case "@weekly":
		cron = minuteHour + " * * " + strconv.Itoa(dayOfWeek)
	case "@monthly":
		cron = minuteHour + " 1 * *"
	case "@yearly":
		cron = minuteHour + " 1 1 *"
	}

	if cron == "" {
		err = fmt.Errorf("invalid frequency: %q or times of day: %q", frequency, timesOfDay)
	}
	return
}

func ParseTimestampToLocal(value string) (string, error) {
	var utcLocation, _ = time.LoadLocation("")
	var v, err = strconv.ParseInt(value, 10, 64)
	if err != nil {
		return "", err
	}

	var t = time.UnixMilli(v).In(utcLocation)

	var _, localoffset = time.Now().Zone()
	var utcTime = t.Add(time.Duration(localoffset) * time.Second)

	return fmt.Sprintf("%.2d:%.2d", utcTime.Hour(), utcTime.Minute()), nil
}

func ParseLocalToTimestamp(value string) (string, error) {
	var fmtValue = fmt.Sprintf("1970-01-01 %s:00", value)
	var fmtTime, err = time.Parse("2006-01-02 15:04:05", fmtValue)
	if err != nil {
		return "", err
	}
	_, localoffset := time.Now().Zone()
	utcTime := fmtTime.Add(-time.Duration(localoffset) * time.Second)
	var utcTimeStr = strconv.FormatInt(utcTime.UnixMilli(), 10)
	return utcTimeStr, nil
}

func ParseToNextUnixTime(frequency, timesOfDay string, dayOfWeek int) int64 {
	switch frequency {
	case "@daily":
		return 86400
	case "@weekly":
		return 604800
	default:
		return 0
	}
}

func GetFirstDayOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	offset := -time.Duration(weekday-1) * 24 * time.Hour

	return t.Add(offset).Truncate(24 * time.Hour)
}
