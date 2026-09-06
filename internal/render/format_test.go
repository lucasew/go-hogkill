package render

import "testing"

func TestPluralSuffix(t *testing.T) {
	cases := []struct {
		n      int
		suffix string
		want   string
	}{
		{n: 1, suffix: "es", want: ""},
		{n: 2, suffix: "es", want: "es"},
		{n: 0, suffix: "es", want: "es"},
		{n: 1, suffix: "s", want: ""},
		{n: 3, suffix: "s", want: "s"},
	}
	for _, tc := range cases {
		if got := PluralSuffix(tc.n, tc.suffix); got != tc.want {
			t.Fatalf("PluralSuffix(%d, %q)=%q want %q", tc.n, tc.suffix, got, tc.want)
		}
	}
}
