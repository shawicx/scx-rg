package preview

import (
	"os"
	"strings"
)

// Protocol 终端图形协议类型。
type Protocol string

const (
	ProtocolAuto      Protocol = "auto"
	ProtocolKitty     Protocol = "kitty"
	ProtocolSixel     Protocol = "sixel"
	ProtocolHalfblock Protocol = "halfblock"
	ProtocolNone      Protocol = "none"
)

// ParseProtocol 解析 CLI 参数；非法值回退 auto。
func ParseProtocol(s string) Protocol {
	p := Protocol(strings.ToLower(strings.TrimSpace(s)))
	switch p {
	case ProtocolKitty, ProtocolSixel, ProtocolHalfblock, ProtocolNone, ProtocolAuto:
		return p
	}
	return ProtocolAuto
}

// queryDeviceAttrs 向控制终端发 DA1 查询并等待响应；包级变量便于测试注入，
// 避免单测向真实终端写转义序列。
var queryDeviceAttrs = queryDA1

// Detect 探测终端图形能力，按优先级返回渲染档位：
//  1. kitty 系（kitty / ghostty / wezterm）——环境变量标志明确；kitty 图形协议
//     没有 DA1 标志位，环境变量是唯一可靠依据；
//  2. DA1 查询（ESC[c）回报 sixel ——覆盖 xterm -ti vt340、alacritty-sixel、
//     st 等没有环境标志的终端；
//  3. TERM 启发式（foot / yaft / mlterm）——DA1 无响应（管道 / 慢终端）时兜底；
//  4. halfblock——纯文本半块渲染，任何彩色终端可用。
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
	if da1HasSixel(queryDeviceAttrs()) {
		return ProtocolSixel
	}
	switch {
	case strings.Contains(term, "foot"),
		strings.Contains(term, "yaft"),
		strings.Contains(term, "mlterm"):
		return ProtocolSixel
	}
	return ProtocolHalfblock
}

// da1HasSixel 解析 DA1 响应（ESC[?62;4;6c 形态）：顶层属性 4 表示 sixel。
func da1HasSixel(resp string) bool {
	i := strings.Index(resp, "\x1b[?")
	if i < 0 {
		return false
	}
	rest := resp[i+3:]
	if j := strings.IndexByte(rest, 'c'); j >= 0 {
		rest = rest[:j]
	}
	for _, f := range strings.Split(rest, ";") {
		if f == "4" {
			return true
		}
	}
	return false
}
