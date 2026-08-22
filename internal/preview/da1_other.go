//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package preview

// queryDA1 非 unix 平台（Windows 控制台无 VT DA1 查询）不做探测，
// Detect 回退环境变量启发式。
func queryDA1() string { return "" }
