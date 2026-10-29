package daemon

import "testing"

func TestExpiryReason(t *testing.T) {
	const wrapper, harness = 10, 11
	for _, tc := range []struct {
		wrapper, harness int
		running          []int
		want             string
	}{
		{0, 0, nil, "no renewal"},
		{wrapper, harness, nil, "wrapper and harness gone"},
		{wrapper, harness, []int{wrapper}, "harness gone"},
		{0, harness, nil, "harness gone"},
		{0, harness, []int{harness}, "harness still running"},
		{wrapper, harness, []int{harness}, "wrapper gone, harness still running"},
		{wrapper, harness, []int{wrapper, harness}, "renewals stopped, wrapper still running"},
	} {
		alive := func(pid int) bool {
			for _, p := range tc.running {
				if p == pid {
					return true
				}
			}
			return false
		}
		if got := expiryReason(tc.wrapper, tc.harness, alive); got != tc.want {
			t.Errorf("expiryReason(%d, %d) with %v running = %q, want %q",
				tc.wrapper, tc.harness, tc.running, got, tc.want)
		}
	}
}
