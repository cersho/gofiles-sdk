package typeutil

const GenericContentType = "application/octet-stream"

func EffectiveContentType(hint string, candidates ...string) string {
	if hint != "" {
		return hint
	}
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return GenericContentType
}
