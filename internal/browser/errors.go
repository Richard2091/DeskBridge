package browser

import (
	"errors"
	"strings"
)

// errorsAsCDP 是 errors.As 的小包装，便于在 manager 里判断 *cdpError。
func errorsAsCDP(err error, target **cdpError) bool {
	return errors.As(err, target)
}

// containsFold 判断 s 是否包含 sub（大小写不敏感）。
func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
