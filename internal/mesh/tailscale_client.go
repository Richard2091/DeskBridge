package mesh

// 对端客户端类型（Peer.Client）的读取与缓存。
//
// ipnstate.PeerStatus 不含 Hostinfo，要另取。这里不用 WatchIPNBus 维护映射，
// 而用 LocalClient.WhoIs(对端 IP) 读 Node.Hostinfo 并按 StableID 缓存：
// WatchIPNBus 文档标明不是稳定 API，增量的设备信息变化不一定带完整网络图，
// 还要自己做断线重连；WhoIs 是稳定接口，本包已在用，在进程内调用很快，
// 加上缓存后列设备不会反复做慢操作。

import (
	"context"
	"sync"
	"time"
)

// 客户端类型缓存和查询参数。
const (
	clientCacheTTL   = 5 * time.Minute  // 读到的类型缓存时间，设备信息很少变
	clientMissTTL    = 30 * time.Second // 读不到时的缓存时间，避免每次都等超时
	clientLookupTime = 2 * time.Second  // 单个对端的查询超时
	clientLookupPar  = 8                // 并发查询数
)

// clientEntry 是一个对端的缓存项；client 为空表示读不到。
type clientEntry struct {
	client  string
	expires time.Time
}

// clientCache 是并发安全的客户端类型缓存，零值可用。
type clientCache struct {
	mu sync.Mutex
	m  map[string]clientEntry
}

// get 返回未过期的缓存项。
func (c *clientCache) get(id string, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[id]
	if !ok || !now.Before(e.expires) {
		return "", false
	}
	return e.client, true
}

// put 写入缓存项；读不到（client 为空）时用较短的有效期。
func (c *clientCache) put(id, client string, now time.Time) {
	ttl := clientCacheTTL
	if client == "" {
		ttl = clientMissTTL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string]clientEntry)
	}
	c.m[id] = clientEntry{client: client, expires: now.Add(ttl)}
}

// retain 删除不在 ids 里的缓存项，防止已删除的设备一直占着缓存。
func (c *clientCache) retain(ids map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id := range c.m {
		if !ids[id] {
			delete(c.m, id)
		}
	}
}

// fillClients 给对端填写 Client：命中缓存的直接用，没命中的并发查询
// （最多 clientLookupPar 个，各自有超时），整体受 ctx 限制。
func (t *Tailscale) fillClients(ctx context.Context, peers []Peer) {
	lc, ok := t.client()
	if !ok || len(peers) == 0 {
		return
	}
	now := time.Now()
	ids := make(map[string]bool, len(peers))
	var missing []int
	for i := range peers {
		ids[peers[i].ID] = true
		if c, ok := t.clients.get(peers[i].ID, now); ok {
			peers[i].Client = c
		} else if len(peers[i].Addrs) > 0 {
			missing = append(missing, i)
		}
	}
	t.clients.retain(ids)
	if len(missing) == 0 {
		return
	}
	sem := make(chan struct{}, clientLookupPar)
	var wg sync.WaitGroup
	for _, i := range missing {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			lctx, cancel := context.WithTimeout(ctx, clientLookupTime)
			defer cancel()
			client := ""
			if resp, err := lc.WhoIs(lctx, peers[i].Addrs[0]); err == nil {
				client = clientFromNode(resp.Node)
			}
			// ctx 被取消造成的失败不缓存，下次请求再试。
			if client == "" && ctx.Err() != nil {
				return
			}
			peers[i].Client = client // 每个协程只写自己的下标
			t.clients.put(peers[i].ID, client, time.Now())
		}()
	}
	wg.Wait()
}
