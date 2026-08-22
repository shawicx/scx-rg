//go:build darwin || linux || freebsd || netbsd || openbsd

package preview

import (
	"bytes"
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// da1Timeout DA1 响应等待上限：探测是启动期的一次性动作，宁可放弃
// 也不能让不响应查询的终端卡住启动。
const da1Timeout = 150 * time.Millisecond

// queryDA1 向控制终端发送 DA1 查询（ESC[c）并在超时内收集响应。
// raw mode + 非阻塞轮询读——/dev/tty 不支持 SetReadDeadline，裸 Read 在
// 终端不响应时会永久阻塞。失败（无 /dev/tty、管道、无响应）返回空串，
// 不产生副作用。必须在 bubbletea 接管终端之前调用（Detect 于 main 早期执行）。
func queryDA1() string {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return ""
	}
	defer tty.Close()

	fd := int(tty.Fd())
	state, err := term.MakeRaw(uintptr(fd))
	if err != nil {
		return ""
	}
	defer term.Restore(uintptr(fd), state) //nolint:errcheck // 探测路径的恢复失败无需处理

	if err := unix.SetNonblock(fd, true); err != nil {
		return ""
	}
	defer unix.SetNonblock(fd, false) //nolint:errcheck // fd 随 Close 释放，恢复失败无害

	if _, err := tty.WriteString("\x1b[c"); err != nil {
		return ""
	}
	// DA1 响应以 'c' 结尾且不超过几十字节；上限 64 防御终端异常输出。
	// 读到的可能是用户按键噪声，交给 da1HasSixel 按 CSI 形态过滤。
	var resp []byte
	buf := make([]byte, 64)
	for deadline := time.Now().Add(da1Timeout); time.Now().Before(deadline) && len(resp) < 64; {
		n, err := tty.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
			if bytes.IndexByte(resp, 'c') >= 0 {
				break
			}
			continue
		}
		if err != nil && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			break // fd 异常，放弃
		}
		time.Sleep(10 * time.Millisecond)
	}
	return string(resp)
}
