package webui

import "testing"

func TestHumanBytes(t *testing.T) {
	testCases := map[int64]string{
		0:               "0 B",
		1023:            "1023 B",
		1024:            "1.0 KiB",
		1536:            "1.5 KiB",
		5 * 1024 * 1024: "5.0 MiB",
		1 << 40:         "1.0 TiB",
	}
	for in, want := range testCases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
