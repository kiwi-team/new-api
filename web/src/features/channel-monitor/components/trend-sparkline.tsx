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
type TrendSparklineProps = {
  /** Per-bucket values across the loaded window */
  values: number[]
  /** Bucket indexes that saw errors, drawn as vertical marks */
  errorMarks?: number[]
  /** Draws the dashed "down" baseline */
  isDown?: boolean
  axisLabels?: string[]
}

const WIDTH = 500
const HEIGHT = 96
const PADDING = 8

/**
 * Inline request-volume sparkline. Deliberately hand-rolled SVG rather than a
 * chart library: one polyline per row, rendered many times in a list.
 */
export function TrendSparkline({
  values,
  errorMarks = [],
  isDown = false,
  axisLabels = [],
}: TrendSparklineProps) {
  const data = values.length > 0 ? values : Array.from({ length: 24 }, () => 0)
  // A floor on the max keeps a nearly-flat series from looking like a spike.
  const max = Math.max(...data, 1000)
  const min = Math.min(...data, 0)
  const span = Math.max(max - min, 1)
  const step = (WIDTH - PADDING * 2) / Math.max(data.length - 1, 1)
  const points = data
    .map((value, index) => {
      const x = PADDING + index * step
      const y = HEIGHT - PADDING - ((value - min) / span) * (HEIGHT - PADDING * 2)
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')

  return (
    <div className='space-y-1'>
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        preserveAspectRatio='none'
        className='h-24 w-full'
        role='img'
      >
        <line
          x1={PADDING}
          y1={HEIGHT - 18}
          x2={WIDTH - PADDING}
          y2={HEIGHT - 18}
          className='stroke-border'
          strokeWidth='1'
        />
        {errorMarks.map((mark) => (
          <line
            key={mark}
            x1={PADDING + mark * step}
            y1='12'
            x2={PADDING + mark * step}
            y2='50'
            className='stroke-destructive'
            strokeWidth='2'
            opacity='0.7'
          />
        ))}
        {isDown && (
          <line
            x1={PADDING}
            y1='52'
            x2={WIDTH - PADDING}
            y2='52'
            className='stroke-destructive'
            strokeWidth='2'
            strokeDasharray='4 4'
          />
        )}
        <polyline
          fill='none'
          className='stroke-primary'
          strokeWidth='2.3'
          points={points}
        />
      </svg>
      {axisLabels.length > 0 && (
        <div className='text-muted-foreground flex justify-between text-[10px]'>
          {axisLabels.map((label, index) => (
            // Ticks are fixed positions across the window; position is the identity.
            // eslint-disable-next-line react/no-array-index-key
            <span key={index}>{label}</span>
          ))}
        </div>
      )}
    </div>
  )
}
