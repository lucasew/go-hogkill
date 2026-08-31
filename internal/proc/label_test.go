package proc

import "testing"

func TestExeBaseAndInterpreterShared(t *testing.T) {
	cases := []struct {
		name    string
		command string
		exe     string
		group   string
		display string
	}{
		{
			name:    "python script",
			command: "python3 /opt/app/main.py --debug",
			exe:     "/usr/bin/python3",
			group:   "python3 main.py",
			display: "python3 main.py",
		},
		{
			name:    "empty exe falls back to command",
			command: "mystery",
			exe:     "",
			group:   "mystery",
			display: "mystery",
		},
		{
			name:    "plain binary",
			command: "/usr/bin/rg --foo",
			exe:     "/usr/bin/rg",
			group:   "rg",
			display: "rg",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := GroupName(tc.command, tc.exe); got != tc.group {
				t.Fatalf("GroupName = %q, want %q", got, tc.group)
			}
			if got := DisplayName(tc.command, tc.exe); got != tc.display {
				t.Fatalf("DisplayName = %q, want %q", got, tc.display)
			}
		})
	}
}
