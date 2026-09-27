package wails

import (
	"strconv"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func procedureTextPage(value, cursor string, limit int) (string, *string, bool, error) {
	if limit == 0 {
		limit = 4096
	}

	if limit < 1 || limit > 16384 {
		return "", nil, false, &shared.Error{Code: "validation_failed", Message: "textLimitが範囲外です"}
	}

	offset := 0

	if cursor != "" {
		var err error

		offset, err = strconv.Atoi(cursor)
		if err != nil || offset < 0 {
			return "", nil, false, &shared.Error{Code: "validation_failed", Message: "textCursorが不正です"}
		}
	}

	runes := []rune(value)
	if offset > len(runes) {
		return "", nil, false, &shared.Error{Code: "validation_failed", Message: "textCursorが範囲外です"}
	}

	end := min(offset+limit, len(runes))
	if end == len(runes) {
		return string(runes[offset:end]), nil, false, nil
	}

	next := strconv.Itoa(end)

	return string(runes[offset:end]), &next, true, nil
}
