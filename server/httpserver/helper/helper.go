package helper

import "fmt"

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
