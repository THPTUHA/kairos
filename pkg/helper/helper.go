package helper

import (
	"fmt"
	"regexp"
	"time"
)

func IsSlug(candidate string) (bool, string) {
	illegalCharPattern, _ := regexp.Compile(`[^\p{Ll}0-9_-]`)
	whyNot := illegalCharPattern.FindString(candidate)
	return whyNot == "", whyNot
}

func GetTimeNow() int64 {
	location, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		fmt.Println("Lỗi khi tải múi giờ: ", err)
		return 0
	}

	now := time.Now()

	return now.In(location).Unix()
}
