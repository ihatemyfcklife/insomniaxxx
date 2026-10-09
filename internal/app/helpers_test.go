package app

import "testing"

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:        "0 B",
		1024:     "1.0 KiB",
		1 << 20:  "1.0 MiB",
		10 << 20: "10.0 MiB",
	}
	for input, want := range cases {
		if got := humanBytes(input); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", input, got, want)
		}
	}
}
