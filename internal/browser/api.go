package browser

// 给工具层使用的导出方法。

import (
	"context"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// Home 返回数据目录。
func (m *Manager) Home() string { return m.home }

// Console 返回当前页面的控制台消息。
func (s *Session) Console(ctx context.Context, level string, maxEntries int, clear bool) ([]ConsoleEntry, error) {
	p, perr := s.currentPage()
	if perr != nil {
		return nil, perr
	}
	return p.console.list(level, maxEntries, clear), nil
}

// Network 返回当前页面的网络请求。
func (s *Session) Network(ctx context.Context, filter string, maxEntries int) ([]NetEntry, error) {
	p, perr := s.currentPage()
	if perr != nil {
		return nil, perr
	}
	switch filter {
	case "", "all", "xhr", "document", "failed":
	default:
		return nil, protocol.Errorf(protocol.CodeInvalidArgs,
			"filter 只能是 all、xhr、document 或 failed")
	}
	return p.network.list(filter, maxEntries), nil
}

// PageInfo 返回当前页面的地址和标题。
func (s *Session) PageInfo() (string, string) {
	p, perr := s.currentPage()
	if perr != nil {
		return "", ""
	}
	return p.info()
}
