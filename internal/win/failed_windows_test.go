package win

import "testing"

func TestFailed(t *testing.T) {
	cases := []struct {
		name string
		hr   uintptr
		want bool
	}{
		{"S_OK", 0, false},
		{"S_FALSE", 1, false},
		{"E_FAIL", 0x80004005, true},
		{"upper bits set, low 0", 0xFFFFFFFF00000000, false},
	}
	for _, c := range cases {
		if got := Failed(c.hr); got != c.want {
			t.Errorf("%s: Failed(0x%X) = %v, want %v", c.name, c.hr, got, c.want)
		}
	}
}
