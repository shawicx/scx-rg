package preview

import "testing"

func TestParseProtocol(t *testing.T) {
	cases := map[string]Protocol{
		"auto":      ProtocolAuto,
		"kitty":     ProtocolKitty,
		"sixel":     ProtocolSixel,
		"halfblock": ProtocolHalfblock,
		"none":      ProtocolNone,
		"  KITTY ":  ProtocolKitty,
		"bogus":     ProtocolAuto,
		"":          ProtocolAuto,
	}
	for in, want := range cases {
		if got := ParseProtocol(in); got != want {
			t.Errorf("ParseProtocol(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestDA1HasSixel(t *testing.T) {
	cases := []struct {
		resp string
		want bool
	}{
		{"\x1b[?62;4;6c", true}, // xterm -ti vt340：sixel 属性 4
		{"\x1b[?4c", true},      // 仅 sixel
		{"\x1b[?64;1;2;4c", true},
		{"garbage\x1b[?62;4;6c", true}, // 前置按键噪声
		{"\x1b[?6c", false},            // VT102 无 sixel
		{"\x1b[?62;22c", false},        // 有属性但无 4
		{"\x1b[?1;2cx", false},         // c 之后的字节不影响解析
		{"", false},                    // 无响应（超时 / 无 tty）
		{"garbage", false},             // 终端未回 CSI 响应
	}
	for _, c := range cases {
		if got := da1HasSixel(c.resp); got != c.want {
			t.Errorf("da1HasSixel(%q) = %v, 期望 %v", c.resp, got, c.want)
		}
	}
}

// stubDA1 替换 DA1 查询，避免单测向真实终端写转义序列。
func stubDA1(resp string) func() {
	old := queryDeviceAttrs
	queryDeviceAttrs = func() string { return resp }
	return func() { queryDeviceAttrs = old }
}

// clearTerminalEnv 清空协议相关环境标志，让 Detect 走 DA1 / 兜底链路。
func clearTerminalEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "WEZTERM_PANE", "WEZTERM_EXECUTABLE"} {
		t.Setenv(k, "")
	}
	t.Setenv("TERM", "xterm-256color")
}

func TestDetectEnvKittyFamily(t *testing.T) {
	defer stubDA1("\x1b[?62;4;6c")() // 即便 DA1 报 sixel，kitty 环境标志也优先

	clearTerminalEnv(t)
	t.Setenv("KITTY_WINDOW_ID", "1")
	if got := Detect(); got != ProtocolKitty {
		t.Errorf("KITTY_WINDOW_ID 应探测 kitty，实际 %q", got)
	}

	clearTerminalEnv(t)
	t.Setenv("GHOSTTY_RESOURCES_DIR", "/Applications/Ghostty.app")
	if got := Detect(); got != ProtocolKitty {
		t.Errorf("GHOSTTY_RESOURCES_DIR 应探测 kitty，实际 %q", got)
	}

	clearTerminalEnv(t)
	t.Setenv("WEZTERM_PANE", "0")
	if got := Detect(); got != ProtocolKitty {
		t.Errorf("WEZTERM_PANE 应探测 kitty，实际 %q", got)
	}
}

func TestDetectDA1Sixel(t *testing.T) {
	defer stubDA1("\x1b[?62;4;6c")()
	clearTerminalEnv(t)
	if got := Detect(); got != ProtocolSixel {
		t.Errorf("DA1 回报 sixel 应探测 sixel，实际 %q", got)
	}
}

func TestDetectTermFallbackWhenDA1Silent(t *testing.T) {
	defer stubDA1("")()
	clearTerminalEnv(t)
	t.Setenv("TERM", "foot-extra")
	if got := Detect(); got != ProtocolSixel {
		t.Errorf("DA1 无响应时 TERM 启发式应兜底 sixel，实际 %q", got)
	}
}

func TestDetectDefaultHalfblock(t *testing.T) {
	defer stubDA1("")()
	clearTerminalEnv(t) // TERM=xterm-256color：无图形协议标志
	if got := Detect(); got != ProtocolHalfblock {
		t.Errorf("kitty/sixel 均不可用时应兜底 halfblock，实际 %q", got)
	}
}
