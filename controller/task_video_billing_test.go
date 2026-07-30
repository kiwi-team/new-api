package controller

import "testing"

// computeDurationRebill 是按时长重算的核心算术，必须与提交时的预扣公式同源：
// relay/relay_task.go 中 quota = price × groupRatio × Π(otherRatios)，其中 seconds
// 是 otherRatios 的一项，因此 quota 对 seconds 线性 —— 等比缩放即可。
func TestComputeDurationRebill(t *testing.T) {
	cases := []struct {
		name      string
		preQuota  int
		assumed   float64
		actual    float64
		wantQuota int
		wantDelta int
		wantOK    bool
	}{
		{
			name:     "actual shorter than assumed refunds the difference",
			preQuota: 1500, assumed: 15, actual: 5,
			wantQuota: 500, wantDelta: -1000, wantOK: true,
		},
		{
			name:     "actual equals assumed is a no-op",
			preQuota: 1500, assumed: 15, actual: 15,
			wantQuota: 1500, wantDelta: 0, wantOK: true,
		},
		{
			name:     "actual longer than assumed tops up",
			preQuota: 1000, assumed: 10, actual: 12,
			wantQuota: 1200, wantDelta: 200, wantOK: true,
		},
		{
			name:     "fractional seconds truncate toward zero",
			preQuota: 1000, assumed: 15, actual: 4,
			wantQuota: 266, wantDelta: -734, wantOK: true,
		},
		{
			name:     "missing assumed seconds is skipped",
			preQuota: 1000, assumed: 0, actual: 5,
			wantOK: false,
		},
		{
			name:     "missing actual seconds is skipped",
			preQuota: 1000, assumed: 15, actual: 0,
			wantOK: false,
		},
		{
			name:     "zero pre-quota is skipped",
			preQuota: 0, assumed: 15, actual: 5,
			wantOK: false,
		},
		{
			name:     "negative actual is skipped",
			preQuota: 1000, assumed: 15, actual: -1,
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotQuota, gotDelta, gotOK := computeDurationRebill(tc.preQuota, tc.assumed, tc.actual)
			if gotOK != tc.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if gotQuota != tc.wantQuota {
				t.Errorf("actualQuota = %d, want %d", gotQuota, tc.wantQuota)
			}
			if gotDelta != tc.wantDelta {
				t.Errorf("delta = %d, want %d", gotDelta, tc.wantDelta)
			}
			// 不变式：重算额度 = 预扣额度 + 差额
			if gotQuota != tc.preQuota+gotDelta {
				t.Errorf("invariant broken: %d != %d + %d", gotQuota, tc.preQuota, gotDelta)
			}
		})
	}
}

// 预扣按上限、实际更短 —— 这是 Seedance 2.0 duration=-1 的典型场景，
// 用户拿回差额，绝不应该出现补扣。
func TestComputeDurationRebill_ModelChosenDurationNeverOvercharges(t *testing.T) {
	const preQuota = 1500 // 按上限 15s 预扣
	for actual := 1; actual <= 15; actual++ {
		_, delta, ok := computeDurationRebill(preQuota, 15, float64(actual))
		if !ok {
			t.Fatalf("actual=%d: expected rebill to apply", actual)
		}
		if actual < 15 && delta >= 0 {
			t.Errorf("actual=%ds: delta = %d, want negative (a refund)", actual, delta)
		}
		if actual == 15 && delta != 0 {
			t.Errorf("actual=15s: delta = %d, want 0", delta)
		}
	}
}
