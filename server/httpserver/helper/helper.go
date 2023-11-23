package helper

import (
	"fmt"
	"time"
)

func AutoRedirctUrl(url string) string {
	return fmt.Sprintf(`
		<body>
			<script type="text/javascript">
			const link = document.createElement("a");
			link.href = "%s";
			document.body.appendChild(link);
			link.click();
		</script>
		</body>
	`, url)
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
