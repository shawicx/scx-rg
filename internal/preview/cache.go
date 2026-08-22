package preview

import (
	"container/list"
	"os"
	"sync"
)

// cacheKey 渲染结果的全量依赖：输入相同则输出可复用。
// size/mtime 让 --follow 等文件变化自然失效（key 变化即 miss）。
type cacheKey struct {
	path             string
	cols, rows, jump int
	proto            Protocol
	query            string
	size             int64
	mtime            int64
}

// Cache 渲染结果 LRU：切选回访时免去重读 + 重高亮。并发安全——
// Get 在 Update 循环调用，Put 在 tea.Cmd goroutine 里调用。
type Cache struct {
	mu      sync.Mutex
	cap     int
	entries map[cacheKey]Rendered
	order   *list.List // front = 最近使用
	index   map[cacheKey]*list.Element
}

// NewCache 创建容量为 capacity 的渲染缓存（容量至少 1）。
func NewCache(capacity int) *Cache {
	if capacity < 1 {
		capacity = 1
	}
	return &Cache{
		cap:     capacity,
		entries: make(map[cacheKey]Rendered),
		order:   list.New(),
		index:   make(map[cacheKey]*list.Element),
	}
}

// Get 查缓存；文件 stat 失败（被删/不可读）视为 miss。
// 参数与 Render 一一对应，保证 key 覆盖渲染的全部输入。
func (c *Cache) Get(path string, cols, rows int, proto Protocol, jump int, query string) (Rendered, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return Rendered{}, false
	}
	k := cacheKey{path: path, cols: cols, rows: rows, jump: jump, proto: proto, query: query, size: st.Size(), mtime: st.ModTime().UnixNano()}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.index[k]
	if !ok {
		return Rendered{}, false
	}
	c.order.MoveToFront(el)
	return c.entries[k], true
}

// Put 写入渲染结果；stat 失败时丢弃（文件已不可读，缓存无意义）。
func (c *Cache) Put(path string, cols, rows int, proto Protocol, jump int, query string, ren Rendered) {
	st, err := os.Stat(path)
	if err != nil {
		return
	}
	k := cacheKey{path: path, cols: cols, rows: rows, jump: jump, proto: proto, query: query, size: st.Size(), mtime: st.ModTime().UnixNano()}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.index[k]; ok {
		c.entries[k] = ren
		c.order.MoveToFront(el)
		return
	}
	c.entries[k] = ren
	c.index[k] = c.order.PushFront(k)
	for c.order.Len() > c.cap {
		last := c.order.Back()
		key := last.Value.(cacheKey)
		delete(c.entries, key)
		delete(c.index, key)
		c.order.Remove(last)
	}
}
