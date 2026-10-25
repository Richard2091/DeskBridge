package browser

// 页面快照：把可访问性树（Accessibility.getFullAXTree）整理成给 AI 看的
// 扁平节点列表，带稳定的 ref 编号，供 browser.click / browser.type 使用。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// 快照的默认值和上限。
const (
	defaultSnapshotNodes = 400
	maxSnapshotNodes     = 2000
	defaultSnapshotBytes = 256 << 10
	maxSnapshotBytes     = 1 << 20
	maxNameRunes         = 400
)

// axValue 是 CDP 里带类型标记的值。
type axValue struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// axProperty 是节点的属性。
type axProperty struct {
	Name  string  `json:"name"`
	Value axValue `json:"value"`
}

// axNode 是 CDP 返回的可访问性节点。
type axNode struct {
	NodeID           string       `json:"nodeId"`
	Ignored          bool         `json:"ignored"`
	Role             *axValue     `json:"role"`
	Name             *axValue     `json:"name"`
	Description      *axValue     `json:"description"`
	Value            *axValue     `json:"value"`
	Properties       []axProperty `json:"properties"`
	ChildIDs         []string     `json:"childIds"`
	BackendDOMNodeID int64        `json:"backendDOMNodeId"`
	ParentID         string       `json:"parentId"`
}

// axTree 是 Accessibility.getFullAXTree 的返回。
type axTree struct {
	Nodes []axNode `json:"nodes"`
}

// SnapshotNode 是快照里的一个节点。
type SnapshotNode struct {
	Ref      string `json:"ref,omitempty"`
	Role     string `json:"role,omitempty"`
	Name     string `json:"name,omitempty"`
	Value    string `json:"value,omitempty"`
	Checked  string `json:"checked,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Expanded *bool  `json:"expanded,omitempty"`
	Selected *bool  `json:"selected,omitempty"`
	Level    int    `json:"level,omitempty"`
	URL      string `json:"url,omitempty"`
}

// Snapshot 是 browser.snapshot 的返回值。
type Snapshot struct {
	SnapshotID string         `json:"snapshot_id"`
	URL        string         `json:"url"`
	Title      string         `json:"title"`
	Nodes      []SnapshotNode `json:"nodes"`
	Truncated  bool           `json:"truncated"`
}

// interestingRoles 是没有名字也值得展示的角色。
var interestingRoles = map[string]bool{
	"button": true, "link": true, "textbox": true, "searchbox": true,
	"checkbox": true, "radio": true, "combobox": true, "listbox": true,
	"option": true, "menuitem": true, "menuitemcheckbox": true, "menuitemradio": true,
	"tab": true, "switch": true, "slider": true, "spinbutton": true,
	"heading": true, "img": true, "alert": true, "dialog": true, "alertdialog": true,
	"listitem": true, "treeitem": true, "row": true, "cell": true,
}

// Snapshot 采集当前页面的可访问性树。
func (s *Session) Snapshot(ctx context.Context, maxNodes, maxBytes int) (Snapshot, error) {
	p, perr := s.currentPage()
	if perr != nil {
		return Snapshot{}, perr
	}
	if maxNodes <= 0 {
		maxNodes = defaultSnapshotNodes
	}
	if maxNodes > maxSnapshotNodes {
		maxNodes = maxSnapshotNodes
	}
	if maxBytes <= 0 {
		maxBytes = defaultSnapshotBytes
	}
	if maxBytes > maxSnapshotBytes {
		maxBytes = maxSnapshotBytes
	}
	var tree axTree
	if err := s.client.call(ctx, p.sessionID, "Accessibility.getFullAXTree", nil, &tree); err != nil {
		return Snapshot{}, cdpProtocolError(err)
	}

	refs := make(map[string]int64)
	nodes := buildSnapshotNodes(tree.Nodes, refs)
	truncated := false
	if len(nodes) > maxNodes {
		nodes = nodes[:maxNodes]
		truncated = true
	}
	url, title := p.info()
	snapshotID := randomID()
	snap := Snapshot{
		SnapshotID: snapshotID,
		URL:        url,
		Title:      title,
		Nodes:      nodes,
		Truncated:  truncated,
	}
	// 超出字节上限时从尾部丢弃节点，直到放得下。
	for len(nodes) > 0 {
		b, err := json.Marshal(snap)
		if err != nil {
			return Snapshot{}, protocol.Errorf(protocol.CodeInternal, "序列化快照失败：%v", err)
		}
		if len(b) <= maxBytes {
			break
		}
		cut := len(nodes) / 10
		if cut < 1 {
			cut = 1
		}
		nodes = nodes[:len(nodes)-cut]
		snap.Nodes = nodes
		snap.Truncated = true
	}
	// 只保留最终节点列表里的 ref。
	finalRefs := make(map[string]int64, len(nodes))
	for _, n := range nodes {
		if n.Ref != "" {
			if backend, ok := refs[n.Ref]; ok {
				finalRefs[n.Ref] = backend
			}
		}
	}
	p.mu.Lock()
	p.refs = finalRefs
	p.snapshotID = snapshotID
	p.mu.Unlock()
	return snap, nil
}

// buildSnapshotNodes 深度优先遍历可访问性树，生成扁平节点列表。
// 返回的 refs 是 ref → backendNodeId 的映射。
func buildSnapshotNodes(all []axNode, refs map[string]int64) []SnapshotNode {
	byID := make(map[string]*axNode, len(all))
	childrenOf := make(map[string][]string, len(all))
	var roots []string
	for i := range all {
		n := &all[i]
		byID[n.NodeID] = n
	}
	for i := range all {
		n := &all[i]
		if n.ParentID == "" {
			roots = append(roots, n.NodeID)
			continue
		}
		if _, ok := byID[n.ParentID]; !ok {
			roots = append(roots, n.NodeID)
			continue
		}
		childrenOf[n.ParentID] = append(childrenOf[n.ParentID], n.NodeID)
	}
	if len(roots) == 0 && len(all) > 0 {
		roots = append(roots, all[0].NodeID)
	}

	var out []SnapshotNode
	refSeq := 0
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		if depth > 200 {
			return
		}
		n := byID[id]
		if n == nil {
			return
		}
		if !n.Ignored {
			if sn, ok := nodeToSnapshot(n, &refSeq, refs); ok {
				out = append(out, sn)
			}
		}
		for _, child := range n.ChildIDs {
			walk(child, depth+1)
		}
		// 有些浏览器的子节点关系只出现在 parentId 上。
		for _, child := range childrenOf[id] {
			if !containsString(n.ChildIDs, child) {
				walk(child, depth+1)
			}
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	return out
}

// containsString 判断切片里是否有某个字符串。
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// nodeToSnapshot 把一个可访问性节点转换成快照节点；不值得展示时返回 false。
func nodeToSnapshot(n *axNode, refSeq *int, refs map[string]int64) (SnapshotNode, bool) {
	role := axString(n.Role)
	name := axString(n.Name)
	value := axString(n.Value)
	if len(name) > maxNameRunes {
		name = name[:maxNameRunes] + "…"
	}
	interesting := name != "" || value != "" || interestingRoles[strings.ToLower(role)]
	if !interesting {
		return SnapshotNode{}, false
	}
	sn := SnapshotNode{Role: role, Name: name, Value: value}
	if n.BackendDOMNodeID > 0 {
		*refSeq = *refSeq + 1
		ref := fmt.Sprintf("e%d", *refSeq)
		sn.Ref = ref
		refs[ref] = n.BackendDOMNodeID
	}
	for _, prop := range n.Properties {
		switch prop.Name {
		case "checked":
			sn.Checked = axString(&prop.Value)
		case "disabled":
			sn.Disabled = axBool(&prop.Value)
		case "expanded":
			if b, ok := axBoolPtr(&prop.Value); ok {
				sn.Expanded = b
			}
		case "selected":
			if b, ok := axBoolPtr(&prop.Value); ok {
				sn.Selected = b
			}
		case "level":
			sn.Level = axInt(&prop.Value)
		case "url":
			if u := axString(&prop.Value); u != "" {
				sn.URL = u
			}
		}
	}
	return sn, true
}

// axString 取出 axValue 的字符串值。
func axString(v *axValue) string {
	if v == nil || v.Value == nil {
		return ""
	}
	switch t := v.Value.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%g", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// axBool 取出布尔值。
func axBool(v *axValue) bool {
	b, ok := axBoolPtr(v)
	return ok && *b
}

// axBoolPtr 取出布尔值指针，取不到时返回 false。
func axBoolPtr(v *axValue) (*bool, bool) {
	if v == nil || v.Value == nil {
		return nil, false
	}
	switch t := v.Value.(type) {
	case bool:
		return &t, true
	case string:
		if t == "true" {
			b := true
			return &b, true
		}
		if t == "false" {
			b := false
			return &b, true
		}
	}
	return nil, false
}

// axInt 取出整数值。
func axInt(v *axValue) int {
	if v == nil || v.Value == nil {
		return 0
	}
	switch t := v.Value.(type) {
	case float64:
		return int(t)
	case string:
		var n int
		if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
			return n
		}
	}
	return 0
}
