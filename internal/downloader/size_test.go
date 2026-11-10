package downloader

import "testing"

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"0", 0},
		{"500", 500},
		{"10KB", 10 * 1024},
		{"5MB", 5 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"2m", 2 * 1024 * 1024},
	}
	for _, tc := range cases {
		got, err := ParseSize(tc.in)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseSize(%q)=%d want %d", tc.in, got, tc.want)
		}
	}
}
