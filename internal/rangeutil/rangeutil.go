package rangeutil

import "strconv"

func Header(start int64, end *int64) string {
	if end == nil {
		return "bytes=" + strconv.FormatInt(start, 10) + "-"
	}
	return "bytes=" + strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(*end, 10)
}

func Size(fullSize int64, start int64, end *int64) int64 {
	last := fullSize - 1
	if end != nil && *end < last {
		last = *end
	}
	if last < start {
		return 0
	}
	return last - start + 1
}

func Slice(data []byte, start int64, end *int64) []byte {
	if start >= int64(len(data)) {
		return nil
	}
	last := int64(len(data)) - 1
	if end != nil && *end < last {
		last = *end
	}
	return data[start : last+1]
}
