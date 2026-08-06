package common

import "time"

// billingLocation 计费口径统一使用 UTC+8（北京时间）。
// 这里用 FixedZone 而非 time.LoadLocation("Asia/Shanghai")：LoadLocation 依赖容器内
// 存在 tzdata，在 scratch/distroless 镜像下会失败并退化成 UTC，导致月起点整整偏移 8 小时——
// 正好落在月末月初最容易判错的窗口。FixedZone 零依赖、永远正确。
var billingLocation = time.FixedZone("CST", 8*60*60)

// BillingMonthStartUnix 返回 ts 所在计费月起点的 Unix 时间戳。
// UID 预算按自然月结算，准入判定、预算看板与用量预警都必须用同一个月起点，
// 否则同一份消耗在三处会被算进不同的月份。
func BillingMonthStartUnix(ts int64) int64 {
	t := time.Unix(ts, 0).In(billingLocation)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, billingLocation).Unix()
}

// BillingMonthRangeUnix 返回 ts 所在计费月的起点与终点（终点为下月起点前一秒，闭区间）。
func BillingMonthRangeUnix(ts int64) (start int64, end int64) {
	t := time.Unix(ts, 0).In(billingLocation)
	start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, billingLocation).Unix()
	end = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, billingLocation).Unix() - 1
	return start, end
}
