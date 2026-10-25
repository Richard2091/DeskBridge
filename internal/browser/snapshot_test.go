package browser

import (
	"testing"
)

func node(id, parent, role, name string, backend int64, ignored bool) axNode {
	n := axNode{NodeID: id, ParentID: parent, BackendDOMNodeID: backend, Ignored: ignored}
	if role != "" {
		n.Role = &axValue{Type: "role", Value: role}
	}
	if name != "" {
		n.Name = &axValue{Type: "computedString", Value: name}
	}
	return n
}

func TestBuildSnapshotNodes(t *testing.T) {
	all := []axNode{
		node("root", "", "RootWebArea", "", 0, false),
		node("btn", "root", "button", "提交", 42, false),
		node("ignored-box", "root", "genericContainer", "", 0, true),
		node("link", "ignored-box", "link", "更多", 43, false),
		node("text", "root", "StaticText", "你好", 0, false),
		node("hidden", "root", "button", "", 44, true),
	}
	refs := make(map[string]int64)
	nodes := buildSnapshotNodes(all, refs)
	if len(nodes) != 3 {
		t.Fatalf("节点数 = %d，期望 3：%+v", len(nodes), nodes)
	}
	if nodes[0].Ref != "e1" || nodes[0].Role != "button" || nodes[0].Name != "提交" {
		t.Errorf("第一个节点不对：%+v", nodes[0])
	}
	if refs["e1"] != 42 {
		t.Errorf("ref e1 应指向 42，实际 %d", refs["e1"])
	}
	if nodes[1].Ref != "e2" || nodes[1].Name != "更多" {
		t.Errorf("被忽略节点的子节点没有保留：%+v", nodes[1])
	}
	if nodes[2].Ref != "" || nodes[2].Name != "你好" {
		t.Errorf("纯文本节点不应有 ref：%+v", nodes[2])
	}
}

func TestNodeProperties(t *testing.T) {
	n := axNode{
		NodeID: "cb",
		Role:   &axValue{Type: "role", Value: "checkbox"},
		Name:   &axValue{Type: "computedString", Value: "记住我"},
		Properties: []axProperty{
			{Name: "checked", Value: axValue{Type: "tristate", Value: "true"}},
			{Name: "disabled", Value: axValue{Type: "boolean", Value: true}},
			{Name: "level", Value: axValue{Type: "integer", Value: float64(2)}},
		},
	}
	refs := make(map[string]int64)
	seq := 0
	sn, ok := nodeToSnapshot(&n, &seq, refs)
	if !ok {
		t.Fatal("应该生成节点")
	}
	if sn.Checked != "true" || !sn.Disabled || sn.Level != 2 {
		t.Errorf("属性解析不对：%+v", sn)
	}
}

func TestQuadCenter(t *testing.T) {
	pt, err := quadCenter([]float64{0, 0, 10, 0, 10, 20, 0, 20})
	if err != nil {
		t.Fatal(err)
	}
	if pt.X != 5 || pt.Y != 10 {
		t.Errorf("中心点 = %+v，期望 (5,10)", pt)
	}
	if _, err := quadCenter([]float64{1, 2}); err == nil {
		t.Error("坐标不足时应报错")
	}
}

func TestModifiersMask(t *testing.T) {
	cases := []struct {
		mods []string
		want int
	}{
		{nil, 0},
		{[]string{"alt"}, 1},
		{[]string{"ctrl", "shift"}, 10},
		{[]string{"meta"}, 4},
		{[]string{"unknown"}, 0},
	}
	for _, c := range cases {
		if got := modifiersMask(c.mods); got != c.want {
			t.Errorf("modifiersMask(%v) = %d，期望 %d", c.mods, got, c.want)
		}
	}
}

func TestJSString(t *testing.T) {
	if got := jsString(`a"b`); got != `"a\"b"` {
		t.Errorf("jsString = %s", got)
	}
}
