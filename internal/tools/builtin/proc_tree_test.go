package builtin

import (
	"reflect"
	"sort"
	"testing"
)

func TestPlausibleParents(t *testing.T) {
	// 1 是根；2 是 1 的子进程；3 的 PPID 指向 1，但 3 比 1 更早创建
	// （PPID 过期、PID 被复用）；4 取不到创建时间；5 的父进程 9 取不到创建时间。
	parents := map[int]int{2: 1, 3: 1, 4: 1, 5: 9, 6: 2}
	created := map[int]uint64{1: 100, 2: 100, 3: 50, 5: 200, 6: 300}
	got := plausibleParents(parents, created)
	want := map[int]int{2: 1, 6: 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plausibleParents = %v, want %v", got, want)
	}
}

func TestPlausibleParentsRootUnknown(t *testing.T) {
	parents := map[int]int{2: 1}
	if got := plausibleParents(parents, map[int]uint64{2: 10}); len(got) != 0 {
		t.Fatalf("根进程创建时间未知时不应有后代，得到 %v", got)
	}
}

func TestDescendantsWithStalePPID(t *testing.T) {
	parents := map[int]int{2: 1, 3: 2, 4: 1}
	created := map[int]uint64{1: 100, 2: 110, 3: 120, 4: 10} // 4 是复用 PID 的旧进程
	got := childrenOf(1, plausibleParents(parents, created))
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{2, 3}) {
		t.Fatalf("后代 = %v, want [2 3]", got)
	}
}

func TestChildrenOfTransitiveVerification(t *testing.T) {
	// 1 是根；2 是 1 的子进程，3 是 2 的子进程，4 是 3 的子进程；5 是 1 的正常子进程。
	parents := map[int]int{2: 1, 3: 2, 4: 3, 5: 1}
	run := func(name string, created map[int]uint64, want []int) {
		t.Run(name, func(t *testing.T) {
			got := childrenOf(1, plausibleParents(parents, created))
			sort.Ints(got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("后代 = %v, want %v", got, want)
			}
		})
	}
	// 中间节点 2 比根更早创建：2 及其子树（3、4）都被排除。
	run("中间节点早于根", map[int]uint64{1: 100, 2: 50, 3: 200, 4: 300, 5: 150}, []int{5})
	// 中间节点 2 缺少创建时间：同样排除整棵子树。
	run("中间节点缺失", map[int]uint64{1: 100, 3: 200, 4: 300, 5: 150}, []int{5})
	// 孙进程 4 自己早于 3：只排除 4。
	run("孙进程早于父进程", map[int]uint64{1: 100, 2: 110, 3: 200, 4: 150, 5: 150}, []int{2, 3, 5})
}
