package node

// 目标写成本机自己的名称、主机名或内网 IP 时按 self 处理的测试。

import (
	"context"
	"errors"
	"testing"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/mesh/meshtest"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

func TestSelfTargetByOwnName(t *testing.T) {
	fake := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	fake.SetStatus(mesh.Status{
		Kind: mesh.KindTailscale, State: mesh.StateConnected,
		Self: &mesh.Peer{ID: "self-1", Name: "desk-pc", HostName: "DESK-PC.local", Addrs: []string{"100.64.0.1"}, Mine: true, Online: true},
	})
	// 同名的对端不应被选中：本机名称优先按 self 处理，不经过网络。
	fake.SetPeers([]mesh.Peer{{ID: "other", Name: "other-pc", Addrs: []string{"100.64.0.2"}, Mine: true, Online: true}})
	c := client.New(startNodeChannels(t, "win-pc", fake))
	ctx := context.Background()

	for _, target := range []string{"desk-pc", "DESK-PC", "desk-pc.local", "100.64.0.1", "tailscale:desk-pc", "self"} {
		capability, err := c.Capability(ctx, target)
		if err != nil {
			t.Errorf("目标 %q：%v", target, err)
			continue
		}
		if capability.Name != "win-pc" {
			t.Errorf("目标 %q 返回的设备名 = %q，期望本机的 win-pc", target, capability.Name)
		}
	}

	// 指定了别的连接方式时，本机名称不匹配。
	_, err := c.Capability(ctx, "lan:desk-pc")
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.CodeDeviceNotFound {
		t.Errorf("lan:desk-pc 应返回 device_not_found，实际 %v", err)
	}

	res, err := c.Probe(ctx, "desk-pc")
	if err != nil || !res.Self || !res.DeskBridge || res.Peer.Name != "desk-pc" {
		t.Errorf("Probe(本机名称) = %+v，%v", res, err)
	}
}
