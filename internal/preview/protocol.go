package preview

import (
	"os"
	"strings"
)

// Protocol 终端图形协议类型。
type Protocol string

const (
	ProtocolAuto  Protocol = "auto"
	ProtocolKitty Protocol = "kitty"
	ProtocolSixel Protocol = "sixel"
	ProtocolNone  Protocol = "none"
)

// ParseProtocol 解析 CLI 参数；非法值回退 auto。
func ParseProtocol(s string) Protocol {
	p := Protocol(strings.ToLower(strings.TrimSpace(s)))
	switch p {
	case ProtocolKitty, ProtocolSixel, ProtocolNone, ProtocolAuto:
		return p
	}
	return ProtocolAuto
}

// Detect 通过环境变量启发式探测终端图形能力。
// TODO: 发送 DA1 (ESC[c) 查询做 sixel 精确探测。
func Detect() Protocol {
	term := os.Getenv("TERM")
	if os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty") {
		return ProtocolKitty
	}
	if strings.Contains(term, "ghostty") || os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return ProtocolKitty
	}
	if os.Getenv("WEZTERM_PANE") != "" || os.Getenv("WEZTERM_EXECUTABLE") != "" {
		return ProtocolKitty // wezterm 支持 kitty 图形协议
	}
	switch {
	case strings.Contains(term, "foot"),
		strings.Contains(term, "yaft"),
		strings.Contains(term, "mlterm"):
		return ProtocolSixel
	}
	return ProtocolNone
}
