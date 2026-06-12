package envutil

func First(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
