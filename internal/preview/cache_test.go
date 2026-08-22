package preview

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func writeCacheFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCacheHitAndMiss(t *testing.T) {
	dir := t.TempDir()
	p := writeCacheFile(t, dir, "f.txt", "hello\n")
	c := NewCache(4)
	c.Put(p, 80, 10, ProtocolNone, 0, "", Rendered{Kind: KindCode, Content: "x"})

	if r, ok := c.Get(p, 80, 10, ProtocolNone, 0, ""); !ok || r.Content != "x" {
		t.Fatalf("相同参数应命中: ok=%v content=%q", ok, r.Content)
	}
	if _, ok := c.Get(p, 100, 10, ProtocolNone, 0, ""); ok {
		t.Error("宽度变化应 miss")
	}
	if _, ok := c.Get(p, 80, 10, ProtocolNone, 5, ""); ok {
		t.Error("jump 变化应 miss")
	}
	if _, ok := c.Get(p, 80, 10, ProtocolNone, 0, "q"); ok {
		t.Error("query 变化应 miss")
	}
	if _, ok := c.Get(filepath.Join(dir, "gone.txt"), 80, 10, ProtocolNone, 0, ""); ok {
		t.Error("不存在的文件应 miss")
	}
}

func TestCacheInvalidatesOnFileChange(t *testing.T) {
	dir := t.TempDir()
	p := writeCacheFile(t, dir, "f.txt", "hello\n")
	c := NewCache(4)
	c.Put(p, 80, 10, ProtocolNone, 0, "", Rendered{Content: "v1"})
	// 内容变化（size 不同）后 key 变化，即使 mtime 粒度不足也应 miss
	writeCacheFile(t, dir, "f.txt", "hello\nworld 更长的内容\n")
	if _, ok := c.Get(p, 80, 10, ProtocolNone, 0, ""); ok {
		t.Error("文件内容变化后应 miss")
	}
}

func TestCacheEvictsOldest(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(2)
	paths := []string{
		writeCacheFile(t, dir, "a.txt", "a"),
		writeCacheFile(t, dir, "b.txt", "b"),
		writeCacheFile(t, dir, "c.txt", "c"),
	}
	c.Put(paths[0], 80, 10, ProtocolNone, 0, "", Rendered{Content: "a"})
	c.Put(paths[1], 80, 10, ProtocolNone, 0, "", Rendered{Content: "b"})
	c.Put(paths[2], 80, 10, ProtocolNone, 0, "", Rendered{Content: "c"}) // 逐出 paths[0]
	if _, ok := c.Get(paths[0], 80, 10, ProtocolNone, 0, ""); ok {
		t.Error("容量 2 时最旧项应被逐出")
	}
	if r, ok := c.Get(paths[1], 80, 10, ProtocolNone, 0, ""); !ok || r.Content != "b" {
		t.Error("未逐出项应命中")
	}
	// 命中会刷新新旧顺序：再写一个新 key，被逐出的应是 paths[2] 而非 paths[1]
	c.Put(writeCacheFile(t, dir, "d.txt", "d"), 80, 10, ProtocolNone, 0, "", Rendered{Content: "d"})
	if _, ok := c.Get(paths[1], 80, 10, ProtocolNone, 0, ""); !ok {
		t.Error("最近命中的项不应被逐出")
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	p := writeCacheFile(t, dir, "f.txt", "hello\n")
	c := NewCache(8)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				c.Put(p, 80+g%3, 10, ProtocolNone, i%5, fmt.Sprintf("q%d", i%4), Rendered{Content: "x"})
				c.Get(p, 80+g%3, 10, ProtocolNone, i%5, fmt.Sprintf("q%d", i%4))
			}
		}(g)
	}
	wg.Wait()
}
