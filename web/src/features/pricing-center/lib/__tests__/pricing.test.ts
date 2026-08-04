/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { formatPrice, nearlyEqual, round6 } from '../pricing'

// 汇率 6.8 下 ¥1 存成 $0.147059，后端再把倍率吸附成 0.07353，反显回来就是
// ¥1.000008。展示与提交判定都必须挡住这个往返噪声，否则页面显示 1.000008，
// 用户重新输入 ¥1 又落回同一个库值，看起来就是「改了价但没进数据库」。
describe('formatPrice', () => {
  test('absorbs currency round-trip noise', () => {
    assert.equal(formatPrice(1.000008), '1.00')
    assert.equal(formatPrice(1.999986), '2.00')
    assert.equal(formatPrice(19.9999968), '20.00')
  })

  test('keeps real precision', () => {
    assert.equal(formatPrice(0.075), '0.075')
    assert.equal(formatPrice(0.002), '0.002')
    // 末位落在 5e-6 绝对容差内时按展示位收缩，真值由聚焦时展开。
    assert.equal(formatPrice(0.147059), '0.14706')
    assert.equal(formatPrice(0.1234), '0.1234')
  })

  test('handles zero and missing values', () => {
    assert.equal(formatPrice(0), '0.00')
    assert.equal(formatPrice(null), '-')
    assert.equal(formatPrice(undefined), '-')
    assert.equal(formatPrice(Number.NaN), '-')
  })
})

describe('nearlyEqual', () => {
  test('treats a stored price and its CNY round trip as unchanged', () => {
    const rate = 6.8
    const stored = 0.14706 // 后端吸附后的库值
    const retyped = round6(1 / rate) // 用户再次输入 ¥1
    assert.equal(nearlyEqual(retyped, stored), true)
  })

  test('detects a real repricing', () => {
    assert.equal(nearlyEqual(0.147059, 0.294118), false)
    assert.equal(nearlyEqual(0, 0.000002), false)
  })

  test('treats zero and empty as the same price', () => {
    assert.equal(nearlyEqual(0, 0), true)
  })
})
