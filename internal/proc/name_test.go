package proc

import "testing"

func TestGroupNameInterpreters(t *testing.T) {
	got := GroupName("node /home/u/app/server.js --port 1", "/usr/bin/node")
	if got != "node server.js" {
		t.Fatalf("got %q", got)
	}
}

func TestDisplayNameBundle(t *testing.T) {
	exe := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	got := DisplayName(exe+" --type=renderer", exe)
	if got == "" {
		t.Fatal("empty")
	}
	if AppBundle(exe) != "Google Chrome" {
		t.Fatalf("bundle %q", AppBundle(exe))
	}
}

func TestSortGroupsCPU(t *testing.T) {
	gs := []Group{
		{Name: "a", CPU: 1, RSS: 10},
		{Name: "b", CPU: 5, RSS: 1},
	}
	out := SortGroups(gs, SortCPU)
	if out[0].Name != "b" {
		t.Fatalf("want b first, got %s", out[0].Name)
	}
}

func TestSortProcsCPUThenRSS(t *testing.T) {
	ps := []Proc{
		{Name: "low", CPU: 1, RSS: 100},
		{Name: "tie-small", CPU: 5, RSS: 10},
		{Name: "tie-big", CPU: 5, RSS: 50},
	}
	SortProcs(ps)
	if ps[0].Name != "tie-big" || ps[1].Name != "tie-small" || ps[2].Name != "low" {
		t.Fatalf("got %s, %s, %s", ps[0].Name, ps[1].Name, ps[2].Name)
	}
}

func TestSortGroupsMemThenCPU(t *testing.T) {
	gs := []Group{
		{Name: "low", CPU: 9, RSS: 1},
		{Name: "tie-small", CPU: 1, RSS: 50},
		{Name: "tie-big", CPU: 4, RSS: 50},
	}
	out := SortGroups(gs, SortMem)
	if out[0].Name != "tie-big" || out[1].Name != "tie-small" || out[2].Name != "low" {
		t.Fatalf("got %s, %s, %s", out[0].Name, out[1].Name, out[2].Name)
	}
}

func TestSortGroupsCountThenRSS(t *testing.T) {
	one := []Proc{{}}
	two := []Proc{{}, {}}
	three := []Proc{{}, {}, {}}
	gs := []Group{
		{Name: "low", RSS: 100, Procs: two},
		{Name: "tie-small", RSS: 10, Procs: three},
		{Name: "tie-big", RSS: 40, Procs: three},
		{Name: "solo", RSS: 1000, Procs: one},
	}
	out := SortGroups(gs, SortCount)
	if out[0].Name != "tie-big" || out[1].Name != "tie-small" || out[2].Name != "low" || out[3].Name != "solo" {
		t.Fatalf("got %s, %s, %s, %s", out[0].Name, out[1].Name, out[2].Name, out[3].Name)
	}
}
