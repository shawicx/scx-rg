//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package preview

func cellSize() (int, int) { return 10, 20 }
