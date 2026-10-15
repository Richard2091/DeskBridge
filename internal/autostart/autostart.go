// Package autostart 让节点在用户登录后自动启动。Windows 用计划任务：
// 登录时触发，以当前用户在桌面会话中运行；进程失败（崩溃或被结束）后
// 由计划任务重启。其他平台返回 platform.ErrUnsupportedPlatform。
package autostart

import (
	"bytes"
	"encoding/xml"
	"strings"
	"unicode/utf16"
)

// DefaultTaskName 是计划任务的默认名称。
const DefaultTaskName = "DeskBridgeNode"

// Config 描述要安装的自启动项。
type Config struct {
	TaskName string   // 计划任务名，空则用 DefaultTaskName
	Exe      string   // 可执行文件的绝对路径
	Args     []string // 传给程序的参数（含子命令）
	User     string   // 触发和运行的用户，形如 DOMAIN\name
}

func (c Config) name() string {
	if c.TaskName == "" {
		return DefaultTaskName
	}
	return c.TaskName
}

// quoteArg 按 Windows 命令行规则给参数加引号（CommandLineToArgvW 的逆过程）。
func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			backslashes++
			b.WriteRune(r)
			continue
		case '"':
			b.WriteString(strings.Repeat(`\`, backslashes+1))
		}
		backslashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, backslashes)) // 结尾引号前的反斜杠要加倍
	b.WriteByte('"')
	return b.String()
}

func joinArgs(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = quoteArg(a)
	}
	return strings.Join(q, " ")
}

func esc(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// taskXML 生成计划任务定义。
//   - 登录触发 + InteractiveToken：只在用户登录时运行，在桌面会话中，
//     LeastPrivilege 不提权；
//   - 不限运行时间，使用电池也不停止，已在运行时忽略新实例；
//   - RestartOnFailure：失败后每分钟重试（计划任务的最小间隔）。
func taskXML(c Config) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>DeskBridge 节点：用户登录后自动启动</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + esc(c.User) + `</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + esc(c.User) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>999</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + esc(c.Exe) + `</Command>
      <Arguments>` + esc(joinArgs(c.Args)) + `</Arguments>
    </Exec>
  </Actions>
</Task>
`
}

// utf16LE 编码为带 BOM 的 UTF-16LE，schtasks /xml 可靠接受的格式。
func utf16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 0, 2+2*len(u))
	out = append(out, 0xFF, 0xFE)
	for _, v := range u {
		out = append(out, byte(v), byte(v>>8))
	}
	return out
}
