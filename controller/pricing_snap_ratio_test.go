package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// snapRatio 只做「消除除法残留的浮点噪声」，不是四舍五入到固定位数。
// 这里锁住两类边界：噪声必须被吸附掉，真实精度必须原样保留。
func TestSnapRatio(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		// 缓存价/输入价 = 0.1 的除法残留
		{"cache ratio noise", 0.100000136, 0.1},
		// 输入价/2 的除法残留
		{"model ratio noise", 2.0000000000000004, 2},
		// 2.000002 与 2 相差 2e-6，超出绝对容差，属真实精度而非噪声
		{"beyond abs tolerance preserved", 2.000002, 2.000002},
		// 只用相对容差会被吸附成 73.529，反显变成 ¥999.99——绝对容差必须兜住
		{"large ratio keeps 6 decimals", 73.529412, 73.529412},
		// 真实的两位/三位精度不能被压掉
		{"small real ratio preserved", 0.075, 0.075},
		{"tiny real ratio preserved", 0.001, 0.001},
		// 整数与零原样返回
		{"integer", 3, 3},
		{"zero", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, snapRatio(tc.in))
		})
	}
}
