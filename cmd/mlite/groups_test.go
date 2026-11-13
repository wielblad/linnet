//go:build windows

package main

import "testing"

func groupsFixture() func() {
	old := cfg
	cfg.HeadSize, cfg.Groups = 64, nil
	cfg.Heads = []headCfg{{ID: 1, X: 500, Y: 300}, {ID: 2, X: 700, Y: 300}, {ID: 3, X: 900, Y: 500}, {ID: 4, X: 200, Y: 600}}
	return func() { cfg = old }
}

func at(t *testing.T, id int64, x, y int32) {
	t.Helper()
	if hc := headCfgOf(id); hc.X != x || hc.Y != y {
		t.Errorf("head %d is at (%d,%d), want (%d,%d)", id, hc.X, hc.Y, x, y)
	}
}

// A head dropped under another makes a stack with it; the one it was dropped at stays put.
func TestGroupJoinBelowMakesAStack(t *testing.T) {
	defer groupsFixture()()
	step := headStep()
	h := headCfgOf(2)
	h.X, h.Y = 505, 300+step-4 // dropped just under head 1
	groupJoin(2, 1)
	g := groupOf(1)
	if g == nil || g.Dir != "v" || groupOf(2) != g {
		t.Fatalf("heads 1 and 2 are not one stack: %+v", cfg.Groups)
	}
	if m := groupMembers(g.ID); len(m) != 2 || m[0] != 1 || m[1] != 2 {
		t.Fatalf("order in the group: %v", m)
	}
	at(t, 1, 500, 300)
	at(t, 2, 500, 300+step)
}

// Dropped to the left of another, the head leads a strip and the other does not move.
func TestGroupJoinLeftLeadsAStrip(t *testing.T) {
	defer groupsFixture()()
	step := headStep()
	h := headCfgOf(2)
	h.X, h.Y = 500-step+6, 302
	groupJoin(2, 1)
	g := groupOf(1)
	if g == nil || g.Dir != "h" {
		t.Fatalf("not a strip: %+v", cfg.Groups)
	}
	if m := groupMembers(g.ID); m[0] != 2 || m[1] != 1 {
		t.Fatalf("order in the group: %v", m)
	}
	at(t, 1, 500, 300)
	at(t, 2, 500-step, 300)
}

// A third head joins the group of the one it was dropped at, in the group's own direction.
func TestGroupJoinExistingGroupKeepsDirection(t *testing.T) {
	defer groupsFixture()()
	step := headStep()
	headCfgOf(2).X, headCfgOf(2).Y = 500, 300+step
	groupJoin(2, 1) // a stack of 1, 2
	h := headCfgOf(3)
	h.X, h.Y = 500+step-2, 300+step+10 // beside head 2, a little lower: after it in the stack
	groupJoin(3, 2)
	g := groupOf(3)
	if g == nil || g.Dir != "v" || len(cfg.Groups) != 1 {
		t.Fatalf("groups: %+v", cfg.Groups)
	}
	if m := groupMembers(g.ID); len(m) != 3 || m[2] != 3 {
		t.Fatalf("order in the group: %v", m)
	}
	at(t, 3, 500, 300+2*step)
	r, _ := entityRect(2)
	if want := (rect{500, 300, 500 + headSize(), 300 + 2*step + headSize()}); r != want {
		t.Errorf("the place of the group is %+v, want %+v", r, want)
	}
	if entityKey(1) != entityKey(3) || entityKey(1) == entityKey(4) {
		t.Errorf("heads of one group must share a row of conversations, others must not")
	}
}

// The group moves as one, a folded group takes the room of one head, and a group that is
// left by all but one head is no group any more.
func TestGroupMoveFoldAndLeave(t *testing.T) {
	defer groupsFixture()()
	step := headStep()
	headCfgOf(2).X, headCfgOf(2).Y = 500, 300+step
	groupJoin(2, 1)
	entityMove(2, 600, 400+step) // dragged by its second head
	at(t, 1, 600, 400)
	at(t, 2, 600, 400+step)
	groupOf(1).Fold = true
	if !headHidden(2) || headHidden(1) {
		t.Errorf("a folded group shows its first head only")
	}
	if r, _ := entityRect(2); r != (rect{600, 400, 600 + headSize(), 400 + headSize()}) {
		t.Errorf("a folded group takes the room of one head, got %+v", r)
	}
	groupLeave(2)
	if len(cfg.Groups) != 0 || headCfgOf(1).Group != 0 || headCfgOf(2).Group != 0 {
		t.Errorf("one head left is no group: %+v %+v", cfg.Groups, cfg.Heads)
	}
}

// Heads that are far apart are not tied; the nearest of those close enough is chosen.
func TestNearestHead(t *testing.T) {
	defer groupsFixture()()
	if to := nearestHead(4); to != 0 {
		t.Errorf("head 4 is far from all, got %d", to)
	}
	h := headCfgOf(4)
	h.X, h.Y = 700+headSize()+10, 300
	if to := nearestHead(4); to != 2 {
		t.Errorf("head 4 was dropped beside head 2, got %d", to)
	}
}

// The size of the heads scales what is drawn on them.
func TestHeadSizes(t *testing.T) {
	defer groupsFixture()()
	for _, px := range []int{48, 64, 80} {
		cfg.HeadSize = px
		if headSize() != dp(px) {
			t.Errorf("head size %d gives %d", px, headSize())
		}
		if hs(52) >= headSize() || hs(52) < headSize()*3/4 {
			t.Errorf("the picture of a head of %d is %d", px, hs(52))
		}
	}
	cfg.HeadSize = 7
	if headPx() != 64 {
		t.Errorf("an unknown size falls back to the middle one")
	}
}
