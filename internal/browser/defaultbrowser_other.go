//go:build !windows && !linux && !darwin

package browser

// 其他平台：不提供浏览器选择，浏览器工具会返回 unsupported_platform。

func defaultBrowser() (Candidate, bool) { return Candidate{}, false }

func fallbackCandidates() []Candidate { return nil }
