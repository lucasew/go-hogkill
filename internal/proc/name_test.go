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
