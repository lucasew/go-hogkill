package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lucasew/go-hogkill/internal/proc"
)

func TestLimitTop(t *testing.T) {
	groups := []proc.Group{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}
	cases := []struct {
		n    int
		want []string
	}{
		{n: 2, want: []string{"a", "b"}},
		{n: 0, want: []string{"a", "b", "c"}},
		{n: -1, want: []string{"a", "b", "c"}},
		{n: 9, want: []string{"a", "b", "c"}},
		{n: 3, want: []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		got := limitTop(groups, tc.n)
		if len(got) != len(tc.want) {
			t.Fatalf("n=%d len=%d want %d", tc.n, len(got), len(tc.want))
		}
		for i, name := range tc.want {
			if got[i].Name != name {
				t.Fatalf("n=%d [%d]=%q want %q", tc.n, i, got[i].Name, name)
			}
		}
	}
}

func TestWriteJSONAndTableShareTopBound(t *testing.T) {
	groups := []proc.Group{
		{Name: "alpha", Procs: []proc.Proc{{Name: "alpha", PID: 1}}},
		{Name: "beta", Procs: []proc.Proc{{Name: "beta", PID: 2}}},
	}

	var js bytes.Buffer
	if err := WriteJSON(&js, groups, 1); err != nil {
		t.Fatal(err)
	}
	var payload []jsonGroup
	if err := json.Unmarshal(js.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload[0].Name != "alpha" {
		t.Fatalf("json payload = %+v", payload)
	}

	var tbl bytes.Buffer
	WriteTable(&tbl, groups, TableOptions{Top: 1})
	out := tbl.String()
	if !strings.Contains(out, "alpha") {
		t.Fatalf("table missing alpha:\n%s", out)
	}
	if strings.Contains(out, "beta") {
		t.Fatalf("table should omit beta:\n%s", out)
	}
}
