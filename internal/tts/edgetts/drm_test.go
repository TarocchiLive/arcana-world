// SPDX-License-Identifier: GPL-3.0-only

package edgetts

import (
	"testing"
	"time"
)

func TestSecMSGECV2(t *testing.T) {
	// 上游 PR #492 给出的 Edge 抓包向量，不使用本实现生成期望值。
	const want = "492D7694A183A45E-56E821F95D4579C5B7C7E33DC7303FF379EC36D510213555BBBB9EB17EF15876"
	previous := clockSkew.Swap(0)
	t.Cleanup(func() { clockSkew.Store(previous) })
	for _, offset := range []int64{0, 1, 299} {
		if got := secMSGEC(time.Unix(1791888000+offset, 0)); got != want {
			t.Errorf("window offset %d: got %s, want %s", offset, got, want)
		}
	}
	if secMSGEC(time.Unix(1791888300, 0)) == want {
		t.Error("next window reused the previous token")
	}
	clockSkew.Store(int64(5 * time.Minute))
	if got := secMSGEC(time.Unix(1791887700, 0)); got != want {
		t.Errorf("clock correction: got %s, want %s", got, want)
	}
}
