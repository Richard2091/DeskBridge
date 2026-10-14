package builtin

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// codePages 是 sys.info 的 codepages 字段：Windows 的代码页。
type codePages struct {
	ANSI int `json:"ansi"`
	OEM  int `json:"oem"`
}

// sessionInfo 是 sys.info 的 session 字段：桌面会话。
type sessionInfo struct {
	ID          int  `json:"id,omitempty"`
	Interactive bool `json:"interactive"`
}

// sysPlatformInfo 是 sys.info 里按平台取的信息，空值在结果里省略。
type sysPlatformInfo struct {
	OSVersion string
	User      string
	Home      string
	MemTotal  int64
	MemAvail  int64
	BootTime  time.Time // 零值表示取不到
	Codepages *codePages
	Session   *sessionInfo
}

// sysInfoResult 是 sys.info 的返回值，字段名即 JSON 键。
type sysInfoResult struct {
	Name      string       `json:"name"`
	Hostname  string       `json:"hostname"`
	OS        string       `json:"os"`
	Arch      string       `json:"arch"`
	OSVersion string       `json:"os_version,omitempty"`
	Version   string       `json:"version"`
	User      string       `json:"user,omitempty"`
	Home      string       `json:"home,omitempty"`
	NumCPU    int          `json:"num_cpu"`
	MemTotal  int64        `json:"memory_total_bytes,omitempty"`
	MemAvail  int64        `json:"memory_available_bytes,omitempty"`
	BootTime  string       `json:"boot_time,omitempty"`
	Timezone  string       `json:"timezone"`
	Codepages *codePages   `json:"codepages,omitempty"`
	Session   *sessionInfo `json:"session,omitempty"`
}

// newSysInfoTool 构造 sys.info 工具。
func newSysInfoTool(info Info) *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "sys.info",
			Description: "获取本设备的基本信息：名称、系统、用户、内存、时区等。只读，不修改任何状态。没有参数。",
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		},
		Run: func(ctx context.Context, call *tools.Call) (any, error) {
			if err := checkCtx(ctx); err != nil {
				return nil, err
			}
			// 参数必须为空：解码到空结构，任何字段都会被拒绝。
			if err := tools.DecodeArgs(call.Args, &struct{}{}); err != nil {
				return nil, err
			}
			return buildSysInfo(info), nil
		},
	}
}

// buildSysInfo 汇总 sys.info 的返回值。
func buildSysInfo(info Info) *sysInfoResult {
	host, _ := os.Hostname()
	p := sysPlatform()
	res := &sysInfoResult{
		Name:      info.Name,
		Hostname:  host,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		OSVersion: p.OSVersion,
		Version:   info.Version,
		User:      p.User,
		Home:      p.Home,
		NumCPU:    runtime.NumCPU(),
		MemTotal:  p.MemTotal,
		MemAvail:  p.MemAvail,
		Timezone:  timezoneName(),
		Codepages: p.Codepages,
		Session:   p.Session,
	}
	if !p.BootTime.IsZero() {
		res.BootTime = p.BootTime.Format(time.RFC3339)
	}
	return res
}
