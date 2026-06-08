/*
Copyright (C) 2025 QuantumNous

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

import React, { useMemo, useState } from 'react';
import {
  Button,
  Card,
  Select,
  SideSheet,
  Space,
  TabPane,
  Table,
  Tabs,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  Gauge,
  KeyRound,
  LayoutGrid,
  ListFilter,
  RefreshCw,
  TriangleAlert,
} from 'lucide-react';
import './style.css';

const { Text, Title } = Typography;

const STATUS_META = {
  healthy: { text: '健康', color: 'green', order: 3 },
  degraded: { text: '波动', color: 'amber', order: 2 },
  down: { text: '异常', color: 'red', order: 1 },
  unknown: { text: '未知', color: 'grey', order: 4 },
};

const PROVIDER_META = {
  claude: { name: 'Claude', color: 'purple', order: 1 },
  openai: { name: 'OpenAI', color: 'blue', order: 2 },
  gemini: { name: 'Gemini', color: 'cyan', order: 3 },
  minimax: { name: 'Minimax', color: 'violet', order: 4 },
  qwen: { name: 'Qwen', color: 'teal', order: 5 },
  doubao: { name: 'Doubao', color: 'orange', order: 6 },
  glm: { name: 'GLM', color: 'indigo', order: 7 },
  step: { name: 'Step', color: 'green', order: 8 },
  other: { name: '其他', color: 'grey', order: 99 },
};

const MONITOR_DATA = [
  {
    keyId: 52,
    keyName: 'cc1',
    keyHint: '0mtTyBA***TGGg',
    channelId: 1543,
    channelName: 'nuwa-cc',
    channelType: 'type-14',
    group: 'default, cc, rpm50',
    model: 'claude-opus-4-8',
    status: 'degraded',
    successRate: 82.35,
    requests: 29067,
    errors: 6229,
    firstMs: null,
    p95FirstMs: null,
    useMs: 24230,
    p95UseMs: 63000,
    tps: 51.8,
    samples: 35296,
    lastSuccess: '11:58',
    lastError: '11:56',
    trend: [
      18000, 22400, 19800, 31600, 24230, 28700, 33100, 26000, 41000, 53500,
      37800, 22900, 20100, 48200, 63000, 34100, 24800, 27000, 31800, 45600,
      29500, 25500, 36700, 51100,
    ],
    errorMarks: [3, 8, 9, 14, 15, 18, 20, 22],
    errorsTop: [
      {
        count: 4673,
        last: '11:56',
        code: '400',
        text: 'HTTP 400: request body too small, may be interrupted or tampered',
      },
      {
        count: 1556,
        last: '11:52',
        code: '502',
        text: 'HTTP 502: 检测到客户端异常，请检查客户端网络或请求格式',
      },
    ],
  },
  {
    keyId: 52,
    keyName: 'cc1',
    keyHint: '0mtTyBA***TGGg',
    channelId: 1479,
    channelName: 'nuwa-anthropic-官转',
    channelType: 'type-14',
    group: 'default',
    model: 'claude-opus-4-7',
    status: 'healthy',
    successRate: 98.13,
    requests: 24897,
    errors: 475,
    firstMs: null,
    p95FirstMs: null,
    useMs: 10330,
    p95UseMs: 36000,
    tps: 68.9,
    samples: 25372,
    lastSuccess: '11:57',
    lastError: '11:31',
    trend: [
      6200, 8800, 10330, 7400, 11200, 15900, 9800, 18700, 22000, 36000,
      12400, 9100, 7800, 16800, 9700, 8400, 12200, 14100, 8900, 7600, 11200,
      9200, 13500, 18800,
    ],
    errorMarks: [8, 9, 17],
    errorsTop: [
      {
        count: 475,
        last: '11:31',
        code: '503',
        text: 'HTTP 503: upstream error: do request failed',
      },
    ],
  },
  {
    keyId: 52,
    keyName: 'cc1',
    keyHint: '0mtTyBA***TGGg',
    channelId: 1543,
    channelName: 'nuwa-cc',
    channelType: 'type-14',
    group: 'default, cc, rpm50',
    model: 'claude-opus-4-6',
    status: 'down',
    successRate: 1.61,
    requests: 194,
    errors: 11825,
    firstMs: null,
    p95FirstMs: null,
    useMs: 16460,
    p95UseMs: 36700,
    tps: 31.2,
    samples: 12019,
    lastSuccess: '11:55',
    lastError: '11:58',
    trend: [
      9800, 12400, 15000, 16460, 19700, 22000, 18100, 25400, 28600, 36700,
      20000, 17100, 14300, 18900, 23100, 33200, 21400, 15700, 17600, 26100,
      20300, 14900, 34000, 28800,
    ],
    errorMarks: Array.from({ length: 24 }, (_, i) => i),
    errorsTop: [
      {
        count: 11824,
        last: '11:58',
        code: '400',
        text: 'HTTP 400: 参数问题，请检查模型 claude-opus-4-6 的请求参数',
      },
    ],
  },
  {
    keyId: 52,
    keyName: 'cc1',
    keyHint: '0mtTyBA***TGGg',
    channelId: 1538,
    channelName: 'yunwu-cc',
    channelType: 'type-14',
    group: 'default',
    model: 'claude-opus-4-6',
    status: 'degraded',
    successRate: 99.47,
    requests: 9830,
    errors: 52,
    firstMs: null,
    p95FirstMs: null,
    useMs: 127400,
    p95UseMs: 878000,
    tps: 12.4,
    samples: 9882,
    lastSuccess: '11:58',
    lastError: '10:42',
    trend: [
      48000, 72400, 96000, 127400, 188000, 243000, 98000, 512000, 376000,
      878000, 164000, 122000, 87500, 221000, 315000, 142000, 97000, 410000,
      238000, 189000, 706000, 132000, 88000, 560000,
    ],
    errorMarks: [7, 17, 20],
    errorsTop: [
      {
        count: 52,
        last: '10:42',
        code: 'timeout',
        text: 'timeout: upstream request duration is too long',
      },
    ],
  },
  {
    keyId: 19,
    keyName: 'BcCeAf',
    keyHint: 'm83fTuc***CeAf',
    channelId: 1454,
    channelName: 'tongyang-minimax',
    channelType: 'type-14',
    group: 'default',
    model: 'minimax-m3',
    status: 'down',
    successRate: 9.57,
    requests: 2229,
    errors: 21065,
    firstMs: null,
    p95FirstMs: null,
    useMs: 26440,
    p95UseMs: 78000,
    tps: 18.6,
    samples: 23294,
    lastSuccess: '11:56',
    lastError: '11:58',
    trend: [
      12200, 18500, 26440, 35100, 78000, 42100, 23400, 19700, 55000, 64000,
      28700, 21800, 19300, 33000, 47600, 70200, 26200, 24100, 39500, 62100,
      33600, 21800, 49200, 58000,
    ],
    errorMarks: Array.from({ length: 18 }, (_, i) => i + 2),
    errorsTop: [
      {
        count: 10370,
        last: '11:58',
        code: '500',
        text: 'HTTP 500: system error (1033), sensitive image detected',
      },
      {
        count: 6420,
        last: '11:54',
        code: '500',
        text: 'HTTP 500: upstream system error while generating response',
      },
    ],
  },
  {
    keyId: 19,
    keyName: 'BcCeAf',
    keyHint: 'm83fTuc***CeAf',
    channelId: 1522,
    channelName: 'huoshan-vicky',
    channelType: 'type-45',
    group: 'default',
    model: 'doubao-seed-2-0-pro-260215',
    status: 'down',
    successRate: 11.27,
    requests: 2327,
    errors: 18322,
    firstMs: null,
    p95FirstMs: null,
    useMs: 38230,
    p95UseMs: 145700,
    tps: 16.2,
    samples: 20649,
    lastSuccess: '11:53',
    lastError: '11:58',
    trend: [
      22000, 38230, 41000, 58000, 92600, 145700, 118000, 66200, 43100,
      78100, 99000, 122000, 54800, 38200, 68500, 112000, 137000, 70500,
      49300, 82500, 126000, 61700, 40200, 109000,
    ],
    errorMarks: Array.from({ length: 20 }, (_, i) => i + 1),
    errorsTop: [
      {
        count: 1120,
        last: '11:58',
        code: '400',
        text: 'HTTP 400: InvalidParameter, video processing timeout',
      },
      {
        count: 1089,
        last: '11:55',
        code: '400',
        text: 'HTTP 400: InvalidParameter, media task timeout',
      },
    ],
  },
  {
    keyId: 17,
    keyName: '123CbBa',
    keyHint: 'LfUBAup***CbBa',
    channelId: 1435,
    channelName: 'ppio',
    channelType: 'type-1',
    group: 'default',
    model: 'glm-4.7-flash',
    status: 'degraded',
    successRate: 87.82,
    requests: 6931,
    errors: 961,
    firstMs: null,
    p95FirstMs: null,
    useMs: 39200,
    p95UseMs: 81500,
    tps: 25.7,
    samples: 7892,
    lastSuccess: '11:58',
    lastError: '11:49',
    trend: [
      18300, 24600, 39200, 44100, 81500, 52300, 33200, 28600, 61000, 74400,
      42500, 29100, 23300, 36400, 55300, 78200, 44200, 39800, 61500, 76200,
      41800, 30400, 49700, 66000,
    ],
    errorMarks: [4, 5, 8, 9, 15, 16, 19, 20],
    errorsTop: [
      {
        count: 961,
        last: '11:49',
        code: '429',
        text: 'HTTP 429: account rate limit exceeded',
      },
    ],
  },
  {
    keyId: 63,
    keyName: 'step',
    keyHint: 'step***prod',
    channelId: 63,
    channelName: 'step',
    channelType: 'type-1',
    group: 'default',
    model: 'step-3.7-flash',
    status: 'healthy',
    successRate: 99.34,
    requests: 8627,
    errors: 57,
    firstMs: null,
    p95FirstMs: null,
    useMs: 12450,
    p95UseMs: 60000,
    tps: 74.1,
    samples: 8684,
    lastSuccess: '11:58',
    lastError: '10:27',
    trend: [
      4200, 6100, 12450, 8500, 14200, 20300, 11600, 9600, 27600, 60000,
      14200, 9800, 7100, 16800, 12100, 8400, 10200, 35500, 9100, 7200,
      11800, 15400, 9400, 22300,
    ],
    errorMarks: [9, 17],
    errorsTop: [
      {
        count: 57,
        last: '10:27',
        code: '503',
        text: 'HTTP 503: upstream service temporarily unavailable',
      },
    ],
  },
  {
    keyId: 17,
    keyName: '123CbBa',
    keyHint: 'LfUBAup***CbBa',
    channelId: 1499,
    channelName: 'yunwu-claude123',
    channelType: 'type-14',
    group: 'default',
    model: 'claude-opus-4-7',
    status: 'healthy',
    successRate: 99.98,
    requests: 4629,
    errors: 1,
    firstMs: null,
    p95FirstMs: null,
    useMs: 4650,
    p95UseMs: 9000,
    tps: 112.3,
    samples: 4630,
    lastSuccess: '11:58',
    lastError: '08:12',
    trend: [
      2100, 2800, 4650, 3900, 5200, 6100, 4800, 3600, 7400, 9000, 5500,
      4700, 3300, 5900, 4200, 3800, 5100, 6400, 3700, 2900, 4300, 5600,
      4100, 6200,
    ],
    errorMarks: [14],
    errorsTop: [
      {
        count: 1,
        last: '08:12',
        code: '502',
        text: 'HTTP 502: upstream connection reset',
      },
    ],
  },
  {
    keyId: 41,
    keyName: 'openai-官方key',
    keyHint: 'sk-proj***9zQp',
    channelId: 1411,
    channelName: 'openai-official',
    channelType: 'type-1',
    group: 'default',
    model: 'gpt-4.1-mini',
    status: 'healthy',
    successRate: 99.72,
    requests: 15880,
    errors: 45,
    firstMs: null,
    p95FirstMs: null,
    useMs: 6820,
    p95UseMs: 18200,
    tps: 96.4,
    samples: 15925,
    lastSuccess: '11:58',
    lastError: '10:03',
    trend: [
      4200, 5100, 6820, 6100, 7400, 8200, 5300, 4900, 11200, 18200, 7600,
      5900, 4800, 8800, 6200, 5400, 7100, 9300, 5600, 4700, 6500, 8100,
      5200, 9400,
    ],
    errorMarks: [9, 17],
    errorsTop: [
      {
        count: 45,
        last: '10:03',
        code: '429',
        text: 'HTTP 429: rate limit exceeded for project',
      },
    ],
  },
  {
    keyId: 21,
    keyName: 'aice-key',
    keyHint: 'aice***prod',
    channelId: 1488,
    channelName: 'gemini-proxy',
    channelType: 'type-24',
    group: 'default',
    model: 'gemini-2.5-pro',
    status: 'degraded',
    successRate: 94.68,
    requests: 11922,
    errors: 670,
    firstMs: null,
    p95FirstMs: null,
    useMs: 18600,
    p95UseMs: 74200,
    tps: 43.7,
    samples: 12592,
    lastSuccess: '11:58',
    lastError: '11:44',
    trend: [
      9200, 11200, 18600, 15100, 21800, 33100, 14900, 12600, 44700, 74200,
      25400, 18800, 12100, 29800, 19500, 14600, 22200, 36800, 17100, 13300,
      28700, 39400, 16200, 31200,
    ],
    errorMarks: [5, 8, 9, 15, 17, 21],
    errorsTop: [
      {
        count: 418,
        last: '11:44',
        code: '503',
        text: 'HTTP 503: model overloaded, please retry later',
      },
      {
        count: 252,
        last: '11:36',
        code: '429',
        text: 'HTTP 429: quota exceeded for gemini project',
      },
    ],
  },
  {
    keyId: 27,
    keyName: 'tools-调用',
    keyHint: 'qwen***tool',
    channelId: 95,
    channelName: 'wuxitongyang-qwen',
    channelType: 'type-17',
    group: 'default',
    model: 'qwen3-coder-plus',
    status: 'healthy',
    successRate: 98.92,
    requests: 10440,
    errors: 114,
    firstMs: null,
    p95FirstMs: null,
    useMs: 14320,
    p95UseMs: 43000,
    tps: 59.5,
    samples: 10554,
    lastSuccess: '11:58',
    lastError: '11:12',
    trend: [
      7200, 9300, 14320, 11600, 16200, 20400, 9800, 8700, 31100, 43000,
      17400, 12200, 9100, 22600, 13100, 9800, 15600, 25400, 10700, 8900,
      14800, 19600, 10100, 21800,
    ],
    errorMarks: [9, 17, 21],
    errorsTop: [
      {
        count: 114,
        last: '11:12',
        code: '429',
        text: 'HTTP 429: insufficient_quota',
      },
    ],
  },
  {
    keyId: 27,
    keyName: 'tools-调用',
    keyHint: 'qwen***tool',
    channelId: 95,
    channelName: 'wuxitongyang-qwen',
    channelType: 'type-17',
    group: 'default',
    model: 'qwen3-vl-max',
    status: 'down',
    successRate: 28.76,
    requests: 710,
    errors: 1759,
    firstMs: null,
    p95FirstMs: null,
    useMs: 24800,
    p95UseMs: 93800,
    tps: 21.6,
    samples: 2469,
    lastSuccess: '11:42',
    lastError: '11:58',
    trend: [
      14800, 19600, 24800, 33000, 93800, 57400, 29600, 22000, 65400, 82600,
      42100, 26300, 20100, 38900, 61200, 77000, 30100, 25400, 53000, 68800,
      35600, 23700, 59900, 74200,
    ],
    errorMarks: Array.from({ length: 16 }, (_, i) => i + 4),
    errorsTop: [
      {
        count: 1116,
        last: '11:58',
        code: '429',
        text: 'HTTP 429: insufficient_quota',
      },
      {
        count: 643,
        last: '11:51',
        code: '400',
        text: 'HTTP 400: invalid image payload',
      },
    ],
  },
];

function formatMs(value) {
  if (value === null || value === undefined) return '-';
  return `${Number(value).toLocaleString()} ms`;
}

function formatFirstMs(value) {
  if (value === null || value === undefined) return '待埋点';
  return formatMs(value);
}

function formatCount(value) {
  return Number(value).toLocaleString();
}

function formatTps(value) {
  if (value === null || value === undefined) return '-';
  return `${Number(value).toFixed(1)} tok/s`;
}

function getKeyLabel(record) {
  return `Key ID ${record.keyId} · ${record.keyName} · ${record.keyHint}`;
}

function getMatrixLabel(record) {
  return `${getKeyLabel(record)} · ${record.channelName}`;
}

function getRecordVolume(record) {
  return record.requests + record.errors;
}

function getProviderKey(record) {
  const source = `${record.model} ${record.channelName}`.toLowerCase();
  if (source.includes('claude') || source.includes('anthropic')) return 'claude';
  if (source.includes('openai') || source.includes('gpt-')) return 'openai';
  if (source.includes('gemini')) return 'gemini';
  if (source.includes('minimax')) return 'minimax';
  if (source.includes('qwen')) return 'qwen';
  if (source.includes('doubao') || source.includes('huoshan')) return 'doubao';
  if (source.includes('glm')) return 'glm';
  if (source.includes('step')) return 'step';
  return 'other';
}

function getProviderMeta(recordOrKey) {
  const key = typeof recordOrKey === 'string' ? recordOrKey : getProviderKey(recordOrKey);
  return PROVIDER_META[key] || PROVIDER_META.other;
}

function getAggregateStatus(successRate, p95UseMs) {
  if (successRate < 90) return 'down';
  if (successRate < 98 || p95UseMs > 120000) return 'degraded';
  return 'healthy';
}

function buildModelStats(records) {
  const map = new Map();
  records.forEach((record) => {
    const current = map.get(record.model) || {
      model: record.model,
      provider: getProviderKey(record),
      requests: 0,
      errors: 0,
      p95UseMs: 0,
      records: 0,
    };
    current.requests += record.requests;
    current.errors += record.errors;
    current.p95UseMs = Math.max(current.p95UseMs, record.p95UseMs || 0);
    current.records += 1;
    map.set(record.model, current);
  });
  return Array.from(map.values())
    .map((item) => {
      const total = item.requests + item.errors;
      return {
        ...item,
        total,
        successRate: total ? (item.requests / total) * 100 : 0,
        status: getAggregateStatus(total ? (item.requests / total) * 100 : 0, item.p95UseMs),
      };
    })
    .sort((a, b) => b.total - a.total);
}

function buildProviderStats(records) {
  const map = new Map();
  records.forEach((record) => {
    const providerKey = getProviderKey(record);
    const current = map.get(providerKey) || {
      providerKey,
      provider: getProviderMeta(providerKey).name,
      requests: 0,
      errors: 0,
      p95UseMs: 0,
      models: new Set(),
      channels: new Set(),
      topError: null,
    };
    current.requests += record.requests;
    current.errors += record.errors;
    current.p95UseMs = Math.max(current.p95UseMs, record.p95UseMs || 0);
    current.models.add(record.model);
    current.channels.add(record.channelId);
    const firstError = record.errorsTop[0];
    const errorWithContext = firstError
      ? {
          ...firstError,
          channelId: record.channelId,
          channelName: record.channelName,
          model: record.model,
          record,
        }
      : null;
    if (
      errorWithContext &&
      (!current.topError || errorWithContext.count > current.topError.count)
    ) {
      current.topError = errorWithContext;
    }
    map.set(providerKey, current);
  });
  return Array.from(map.values())
    .map((item) => {
      const total = item.requests + item.errors;
      const successRate = total ? (item.requests / total) * 100 : 0;
      return {
        ...item,
        total,
        successRate,
        status: getAggregateStatus(successRate, item.p95UseMs),
        modelCount: item.models.size,
        channelCount: item.channels.size,
      };
    })
    .sort(
      (a, b) =>
        STATUS_META[a.status].order - STATUS_META[b.status].order ||
        getProviderMeta(a.providerKey).order - getProviderMeta(b.providerKey).order,
    );
}

function buildErrorBreakdown(records) {
  return records
    .flatMap((record) =>
      record.errorsTop.map((error, index) => ({
        ...error,
        index,
        channelId: record.channelId,
        channelName: record.channelName,
        channelType: record.channelType,
        keyId: record.keyId,
        keyName: record.keyName,
        keyHint: record.keyHint,
        model: record.model,
        provider: getProviderMeta(record).name,
        status: record.status,
        record,
      })),
    )
    .sort((a, b) => b.count - a.count);
}

function MetricCard({ label, value, hint, tone = 'default', icon }) {
  return (
    <div className={`mcm-summary-card tone-${tone}`}>
      <div className='mcm-summary-head'>
        <span>{label}</span>
        {icon}
      </div>
      <strong>{value}</strong>
      <p>{hint}</p>
    </div>
  );
}

function StatusBadge({ status }) {
  const meta = STATUS_META[status] || STATUS_META.unknown;
  return (
    <span className={`mcm-status mcm-status-${status}`}>
      <span />
      {meta.text}
    </span>
  );
}

function TrendChart({ record }) {
  const width = 500;
  const height = 96;
  const pad = 8;
  const values = record.trend.length ? record.trend : Array(24).fill(0);
  const max = Math.max(...values, 1000);
  const min = Math.min(...values, 0);
  const span = Math.max(max - min, 1);
  const step = (width - pad * 2) / (values.length - 1);
  const points = values
    .map((value, index) => {
      const x = pad + index * step;
      const y = height - pad - ((value - min) / span) * (height - pad * 2);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');

  return (
    <div className='mcm-chart'>
      <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio='none'>
        <rect x='0' y='0' width={width} height={height} fill='#fff' />
        <line
          x1='8'
          y1='78'
          x2={width - 8}
          y2='78'
          stroke='#edf0f5'
          strokeWidth='1'
        />
        {record.errorMarks.map((mark) => {
          const x = pad + mark * step;
          return (
            <line
              key={mark}
              x1={x}
              y1='12'
              x2={x}
              y2='50'
              stroke='#ff8b8b'
              strokeWidth='2'
              opacity='0.8'
            />
          );
        })}
        {record.status === 'down' ? (
          <line
            x1='8'
            y1='52'
            x2={width - 8}
            y2='52'
            stroke='#ef4444'
            strokeWidth='2'
            strokeDasharray='4 4'
          />
        ) : null}
        {record.trend.length ? (
          <polyline
            fill='none'
            stroke='#2f6df6'
            strokeWidth='2.3'
            points={points}
          />
        ) : null}
      </svg>
      <div className='mcm-axis'>
        <span>11:58</span>
        <span>17:56</span>
        <span>00:09</span>
        <span>06:03</span>
        <span>11:57</span>
      </div>
    </div>
  );
}

function ErrorList({ record, limit = 2 }) {
  if (!record.errorsTop.length) {
    return <div className='mcm-empty-error'>过去 24 小时没有聚合错误。</div>;
  }
  return (
    <div className='mcm-error-list'>
      {record.errorsTop.slice(0, limit).map((error, index) => (
        <div className='mcm-error-item' key={`${error.code}-${error.last}-${index}`}>
          <div className='mcm-error-meta'>
            <span>{formatCount(error.count)} 次</span>
            <small>最近 {error.last}</small>
          </div>
          <div className='mcm-error-text'>{error.text}</div>
        </div>
      ))}
    </div>
  );
}

function ErrorChannelStrip({ errors, onOpen }) {
  if (!errors.length) {
    return null;
  }
  return (
    <section className='mcm-error-channels'>
      <div className='mcm-section-line'>
        <span>主要错误渠道</span>
        <span>按错误次数排序</span>
      </div>
      <div className='mcm-error-channel-grid'>
        {errors.slice(0, 6).map((error) => (
          <button
            key={`${error.channelId}-${error.model}-${error.code}-${error.index}`}
            type='button'
            onClick={() => onOpen(error.record)}
          >
            <div className='mcm-error-channel-head'>
              <strong>{error.channelName}</strong>
              <Tag color={error.status === 'down' ? 'red' : 'amber'} size='small'>
                {formatCount(error.count)} 次
              </Tag>
            </div>
            <div className='mcm-error-channel-meta'>
              <span>ID {error.channelId}</span>
              <span>{error.provider}</span>
              <span>{error.model}</span>
            </div>
            <p>
              {error.code ? `${error.code}: ` : ''}
              {error.text}
            </p>
          </button>
        ))}
      </div>
    </section>
  );
}

function ChannelErrorTable({ errors, onOpen }) {
  if (!errors.length) {
    return <div className='mcm-empty-error'>过去 24 小时没有聚合错误。</div>;
  }
  return (
    <div className='mcm-channel-error-table'>
      {errors.map((error) => (
        <button
          key={`${error.channelId}-${error.model}-${error.code}-${error.index}`}
          type='button'
          onClick={() => onOpen(error.record)}
        >
          <div>
            <strong>{error.channelName}</strong>
            <span>ID {error.channelId} · {error.channelType}</span>
          </div>
          <div>
            <strong>{error.model}</strong>
            <span>{error.provider}</span>
          </div>
          <div>
            <strong>{formatCount(error.count)} 次</strong>
            <span>最近 {error.last}</span>
          </div>
          <p>{error.text}</p>
        </button>
      ))}
    </div>
  );
}

function ModelRankStrip({ models, activeModel, onSelect }) {
  return (
    <div className='mcm-model-rank'>
      {models.map((model, index) => (
        <button
          className={activeModel === model.model ? 'active' : ''}
          key={model.model}
          type='button'
          onClick={() => onSelect(activeModel === model.model ? 'all' : model.model)}
        >
          <span>#{index + 1}</span>
          <strong>{model.model}</strong>
          <small>
            {getProviderMeta(model.provider).name} · {formatCount(model.total)} 次 ·{' '}
            {model.successRate.toFixed(1)}%
          </small>
        </button>
      ))}
    </div>
  );
}

function ProviderGrid({ providers }) {
  return (
    <div className='mcm-provider-grid'>
      {providers.map((provider) => {
        const meta = getProviderMeta(provider.providerKey);
        return (
          <article className='mcm-provider-card' key={provider.providerKey}>
            <div className='mcm-provider-head'>
              <div>
                <Tag color={meta.color} size='small'>
                  {provider.provider}
                </Tag>
                <strong>{provider.successRate.toFixed(1)}%</strong>
              </div>
              <StatusBadge status={provider.status} />
            </div>
            <div className='mcm-provider-stats'>
              <span>模型 {provider.modelCount}</span>
              <span>渠道 {provider.channelCount}</span>
              <span>请求 {formatCount(provider.total)}</span>
              <span>错误 {formatCount(provider.errors)}</span>
            </div>
            <div className='mcm-provider-foot'>
              <span>P95 {formatMs(provider.p95UseMs)}</span>
              <small>
                {provider.topError
                  ? `${provider.topError.channelName} · ${provider.topError.model} · ${provider.topError.text}`
                  : '过去 24 小时无聚合错误'}
              </small>
            </div>
          </article>
        );
      })}
    </div>
  );
}

function MonitorCard({ record, onOpen }) {
  const useTone =
    record.p95UseMs > 120000
      ? 'bad'
      : record.p95UseMs > 60000
        ? 'warn'
        : 'good';
  const successTone =
    record.successRate < 90
      ? 'bad'
      : record.successRate < 98
        ? 'warn'
        : 'good';

  return (
    <article className='mcm-card' onClick={() => onOpen(record)}>
      <div className='mcm-card-top'>
        <div className='mcm-card-title'>
          <h2>{record.model}</h2>
          <div className='mcm-key-line'>
            <KeyRound size={14} />
            <span>{getKeyLabel(record)}</span>
          </div>
          <Space spacing={6} wrap>
            <Tag color='blue' size='small'>
              {record.channelName}
            </Tag>
            <Tag color='purple' size='small'>
              {record.group}
            </Tag>
            <Tag color='cyan' size='small'>
              {record.channelType}
            </Tag>
          </Space>
        </div>
        <StatusBadge status={record.status} />
      </div>
      <div className='mcm-kpis'>
        <div>
          <span>成功率</span>
          <strong className={`tone-${successTone}`}>{record.successRate}%</strong>
          <p>
            成功 {formatCount(record.requests)} / 错误 {formatCount(record.errors)}
          </p>
        </div>
        <div className='focus'>
          <span>P95 耗时</span>
          <strong className={`tone-${useTone}`}>{formatMs(record.p95UseMs)}</strong>
          <p>当前日志 use_time 口径</p>
        </div>
        <div>
          <span>输出速度</span>
          <strong>{formatTps(record.tps)}</strong>
          <p>completion tokens / use time</p>
        </div>
      </div>
      <div className='mcm-card-chart'>
        <div className='mcm-section-line'>
          <span>总耗时 · 过去 24 小时</span>
          <span>{record.samples} 个样本点</span>
        </div>
        <TrendChart record={record} />
      </div>
      <div className='mcm-card-errors'>
        <h3>最近 24h 错误原因</h3>
        <ErrorList record={record} />
      </div>
    </article>
  );
}

function groupRecords(records, mode) {
  const map = new Map();
  records.forEach((record) => {
    let key = getKeyLabel(record);
    if (mode === 'channel') key = `${record.channelName} · #${record.channelId}`;
    if (mode === 'model') key = record.model;
    if (mode === 'provider') key = getProviderMeta(record).name;
    if (!map.has(key)) map.set(key, []);
    map.get(key).push(record);
  });
  return Array.from(map.entries());
}

function buildSamples(record) {
  const okText = `首字 ${formatFirstMs(record.firstMs)} · 总耗时 ${formatMs(record.useMs)} · request_id req_${record.keyId}${record.channelId}`;
  return [
    { time: '11:57', ok: record.status !== 'down', title: '消费日志', text: okText },
    {
      time: record.lastError,
      ok: false,
      title: '错误日志',
      text: record.errorsTop[0]?.text || '无错误',
    },
    { time: '10:42', ok: true, title: '消费日志', text: okText },
    {
      time: '09:18',
      ok: !record.errorsTop[1],
      title: record.errorsTop[1] ? '错误日志' : '消费日志',
      text: record.errorsTop[1]?.text || okText,
    },
  ];
}

export default function ModelChannelMonitor() {
  const [scopeTab, setScopeTab] = useState('topModels');
  const [groupMode, setGroupMode] = useState('key');
  const [modelFilter, setModelFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all');
  const [topLimit, setTopLimit] = useState(8);
  const [activeTab, setActiveTab] = useState('cards');
  const [selected, setSelected] = useState(null);

  const modelStats = useMemo(() => buildModelStats(MONITOR_DATA), []);

  const modelOptions = useMemo(
    () =>
      modelStats.map((item) => ({
        label: `${item.model} · ${formatCount(item.total)} 次`,
        value: item.model,
      })),
    [modelStats],
  );

  const topModelStats = useMemo(() => {
    if (topLimit === 'all') return modelStats;
    return modelStats.slice(0, Number(topLimit));
  }, [modelStats, topLimit]);

  const topModelNames = useMemo(
    () => new Set(topModelStats.map((item) => item.model)),
    [topModelStats],
  );

  const baseRecords = useMemo(() => {
    return MONITOR_DATA.filter(
      (item) => modelFilter === 'all' || item.model === modelFilter,
    ).filter((item) => statusFilter === 'all' || item.status === statusFilter);
  }, [modelFilter, statusFilter]);

  const filteredRecords = useMemo(() => {
    return baseRecords
      .filter(
        (item) =>
          scopeTab !== 'topModels' ||
          modelFilter !== 'all' ||
          topModelNames.has(item.model),
      )
      .sort(
        (a, b) =>
          STATUS_META[a.status].order - STATUS_META[b.status].order ||
          getRecordVolume(b) - getRecordVolume(a),
      );
  }, [baseRecords, modelFilter, scopeTab, topModelNames]);

  const providerStats = useMemo(
    () => buildProviderStats(baseRecords),
    [baseRecords],
  );

  const errorBreakdown = useMemo(
    () => buildErrorBreakdown(filteredRecords),
    [filteredRecords],
  );

  const summary = useMemo(() => {
    const totalRequests = filteredRecords.reduce(
      (sum, item) => sum + item.requests + item.errors,
      0,
    );
    const totalSuccess = filteredRecords.reduce(
      (sum, item) => sum + item.requests,
      0,
    );
    const totalErrors = filteredRecords.reduce(
      (sum, item) => sum + item.errors,
      0,
    );
    const successRate = totalRequests ? (totalSuccess / totalRequests) * 100 : 0;
    const useSamples = filteredRecords.filter((item) => item.useMs !== null);
    const avgUse = useSamples.length
      ? useSamples.reduce((sum, item) => sum + item.useMs, 0) / useSamples.length
      : 0;
    return {
      totalRequests,
      totalErrors,
      successRate,
      avgUse,
      down: filteredRecords.filter((item) => item.status === 'down').length,
      degraded: filteredRecords.filter((item) => item.status === 'degraded')
        .length,
    };
  }, [filteredRecords]);

  const tableColumns = [
    {
      title: 'Key',
      dataIndex: 'keyName',
      render: (_, record) => (
        <div className='mcm-table-key'>
          <strong>{record.keyName}</strong>
          <span>
            ID {record.keyId} · {record.keyHint}
          </span>
        </div>
      ),
    },
    {
      title: '渠道',
      dataIndex: 'channelName',
      render: (_, record) => (
        <div className='mcm-table-key'>
          <strong>{record.channelName}</strong>
          <span>ID {record.channelId} · {record.channelType}</span>
        </div>
      ),
    },
    { title: '模型', dataIndex: 'model' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (status) => <StatusBadge status={status} />,
    },
    {
      title: '请求数',
      render: (_, record) => formatCount(record.requests + record.errors),
      sorter: (a, b) => a.requests + a.errors - (b.requests + b.errors),
    },
    {
      title: '成功率',
      dataIndex: 'successRate',
      render: (value) => `${value}%`,
      sorter: (a, b) => a.successRate - b.successRate,
    },
    {
      title: '平均耗时',
      dataIndex: 'useMs',
      render: formatMs,
      sorter: (a, b) => (a.useMs || 0) - (b.useMs || 0),
    },
    {
      title: 'P95 耗时',
      dataIndex: 'p95UseMs',
      render: formatMs,
      sorter: (a, b) => (a.p95UseMs || 0) - (b.p95UseMs || 0),
    },
    {
      title: 'Top 错误',
      render: (_, record) => record.errorsTop[0]?.text || '-',
    },
  ];

  const matrixModels = Array.from(
    new Set(filteredRecords.map((item) => item.model)),
  );
  const matrixKeys = Array.from(
    new Set(filteredRecords.map((item) => getMatrixLabel(item))),
  );

  return (
    <div className='mcm-page'>
      <div className='mcm-header'>
        <div>
          <Title heading={3} className='!mb-1'>
            模型渠道监控
          </Title>
          <Text type='secondary'>
            线上样本映射：按 Key、渠道、模型聚合健康状态、耗时、成功率和错误原因。
          </Text>
        </div>
        <div className='mcm-refresh-note'>
          <span />
          线上样本 mock · 2026-06-08 11:58
        </div>
      </div>

      <Card className='mcm-toolbar' bodyStyle={{ padding: 14 }}>
        <Tabs
          activeKey={scopeTab}
          className='mcm-head-tabs'
          onChange={setScopeTab}
          type='button'
        >
          <TabPane tab='Top 调用模型' itemKey='topModels' />
          <TabPane tab='厂商可用性' itemKey='providers' />
        </Tabs>
        <div className='mcm-filter-grid'>
          <label>
            <span>时间范围</span>
            <Select defaultValue='24h' style={{ width: '100%' }}>
              <Select.Option value='24h'>过去 24 小时</Select.Option>
              <Select.Option value='6h'>过去 6 小时</Select.Option>
              <Select.Option value='1h'>过去 1 小时</Select.Option>
              <Select.Option value='15m'>过去 15 分钟</Select.Option>
            </Select>
          </label>
          <label>
            <span>展示模型</span>
            <Select
              disabled={scopeTab !== 'topModels'}
              value={topLimit}
              onChange={setTopLimit}
              style={{ width: '100%' }}
            >
              <Select.Option value={5}>Top 5</Select.Option>
              <Select.Option value={8}>Top 8</Select.Option>
              <Select.Option value={10}>Top 10</Select.Option>
              <Select.Option value='all'>全部</Select.Option>
            </Select>
          </label>
          <label>
            <span>分组方式</span>
            <Select
              value={groupMode}
              onChange={setGroupMode}
              style={{ width: '100%' }}
            >
              <Select.Option value='key'>按 Key</Select.Option>
              <Select.Option value='channel'>按渠道</Select.Option>
              <Select.Option value='model'>按模型</Select.Option>
              <Select.Option value='provider'>按厂商</Select.Option>
            </Select>
          </label>
          <label>
            <span>模型</span>
            <Select
              value={modelFilter}
              onChange={setModelFilter}
              style={{ width: '100%' }}
              optionList={[{ label: '全部模型', value: 'all' }, ...modelOptions]}
            />
          </label>
          <label>
            <span>状态</span>
            <Select
              value={statusFilter}
              onChange={setStatusFilter}
              style={{ width: '100%' }}
            >
              <Select.Option value='all'>全部状态</Select.Option>
              <Select.Option value='healthy'>健康</Select.Option>
              <Select.Option value='degraded'>波动</Select.Option>
              <Select.Option value='down'>异常</Select.Option>
              <Select.Option value='unknown'>未知</Select.Option>
            </Select>
          </label>
          <Button icon={<RefreshCw size={15} />} theme='solid' type='primary'>
            刷新
          </Button>
        </div>
      </Card>

      <div className='mcm-summary-grid'>
        <MetricCard
          label='组合数'
          value={filteredRecords.length}
          hint={scopeTab === 'topModels' ? '当前 Top 模型组合' : '当前筛选下全部组合'}
          icon={<LayoutGrid size={16} />}
        />
        <MetricCard
          label='整体成功率'
          value={`${summary.successRate.toFixed(1)}%`}
          hint='成功日志 / 成功日志+错误日志'
          tone='good'
          icon={<Activity size={16} />}
        />
        <MetricCard
          label='平均耗时'
          value={formatMs(Math.round(summary.avgUse))}
          hint='当前生产 logs.use_time 口径'
          tone='warn'
          icon={<Gauge size={16} />}
        />
        <MetricCard
          label='错误请求数'
          value={summary.totalErrors.toLocaleString()}
          hint='真实错误日志聚合'
          tone='bad'
          icon={<TriangleAlert size={16} />}
        />
        <MetricCard
          label='异常 / 波动'
          value={`${summary.down} / ${summary.degraded}`}
          hint='异常优先展示'
          icon={<ListFilter size={16} />}
        />
      </div>

      <ErrorChannelStrip errors={errorBreakdown} onOpen={setSelected} />

      {scopeTab === 'topModels' ? (
        <ModelRankStrip
          activeModel={modelFilter}
          models={topModelStats}
          onSelect={setModelFilter}
        />
      ) : (
        <ProviderGrid providers={providerStats} />
      )}

      <Tabs activeKey={activeTab} onChange={setActiveTab} type='button'>
        <TabPane tab='卡片视图' itemKey='cards'>
          <div className='mcm-groups'>
            {groupRecords(filteredRecords, groupMode).map(([group, items]) => {
              const healthy = items.filter(
                (item) => item.status === 'healthy',
              ).length;
              return (
                <section key={group}>
                  <div className='mcm-group-title'>
                    <div>
                      <strong>{group}</strong>
                      <Tag size='small' color='grey'>
                        {healthy} / {items.length} 健康
                      </Tag>
                    </div>
                    <span>{items.length} 个组合</span>
                  </div>
                  <div className='mcm-card-grid'>
                    {items.map((item) => (
                      <MonitorCard
                        key={`${item.keyId}-${item.channelId}-${item.model}`}
                        record={item}
                        onOpen={setSelected}
                      />
                    ))}
                  </div>
                </section>
              );
            })}
          </div>
        </TabPane>
        <TabPane tab='表格视图' itemKey='table'>
          <Card bodyStyle={{ padding: 0 }} className='mcm-table-card'>
            <Table
              rowKey={(record) => `${record.keyId}-${record.channelId}-${record.model}`}
              columns={tableColumns}
              dataSource={filteredRecords}
              pagination={false}
              size='small'
            />
          </Card>
        </TabPane>
        <TabPane tab='矩阵视图' itemKey='matrix'>
          <Card bodyStyle={{ padding: 0 }} className='mcm-matrix-card'>
            <div className='mcm-matrix'>
              <table>
                <thead>
                  <tr>
                    <th>Key / 渠道 \\ 模型</th>
                    {matrixModels.map((model) => (
                      <th key={model}>{model}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {matrixKeys.map((keyLabel) => (
                    <tr key={keyLabel}>
                      <td>{keyLabel}</td>
                      {matrixModels.map((model) => {
                        const record = filteredRecords.find(
                          (item) =>
                            getMatrixLabel(item) === keyLabel &&
                            item.model === model,
                        );
                        return (
                          <td key={`${keyLabel}-${model}`}>
                            {record ? (
                              <button
                                className={`mcm-matrix-cell ${record.status}`}
                                type='button'
                                onClick={() => setSelected(record)}
                              >
                                <strong>
                                  {STATUS_META[record.status].text} ·{' '}
                                  {record.successRate}%
                                </strong>
                                <span>P95 {formatMs(record.p95UseMs)}</span>
                                <span>错误 {formatCount(record.errors)} 次</span>
                              </button>
                            ) : (
                              <div className='mcm-matrix-cell unknown'>
                                <strong>未知</strong>
                                <span>无样本</span>
                              </div>
                            )}
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>
        </TabPane>
      </Tabs>

      <SideSheet
        title={selected ? selected.model : '模型渠道详情'}
        visible={Boolean(selected)}
        onCancel={() => setSelected(null)}
        width={560}
      >
        {selected ? (
          <div className='mcm-detail'>
            <Text type='secondary'>
              {getKeyLabel(selected)} · {selected.channelName}
            </Text>
            <div className='mcm-detail-grid'>
              <div>
                <span>状态</span>
                <StatusBadge status={selected.status} />
              </div>
              <div>
                <span>成功率</span>
                <strong>{selected.successRate}%</strong>
              </div>
              <div>
                <span>平均耗时</span>
                <strong>{formatMs(selected.useMs)}</strong>
              </div>
              <div>
                <span>P95 耗时</span>
                <strong>{formatMs(selected.p95UseMs)}</strong>
              </div>
              <div>
                <span>首字延迟</span>
                <strong>{formatFirstMs(selected.firstMs)}</strong>
              </div>
              <div>
                <span>输出速度</span>
                <strong>{formatTps(selected.tps)}</strong>
              </div>
              <div>
                <span>样本数</span>
                <strong>{formatCount(selected.samples)}</strong>
              </div>
              <div>
                <span>错误数</span>
                <strong>{formatCount(selected.errors)}</strong>
              </div>
            </div>
            <section className='mcm-detail-section'>
              <h3>总耗时趋势</h3>
              <TrendChart record={selected} />
            </section>
            <section className='mcm-detail-section'>
              <h3>错误原因</h3>
              <ErrorList record={selected} limit={5} />
            </section>
            <section className='mcm-detail-section'>
              <h3>渠道错误明细</h3>
              <ChannelErrorTable
                errors={buildErrorBreakdown([selected])}
                onOpen={setSelected}
              />
            </section>
            <section className='mcm-detail-section'>
              <h3>最近样本</h3>
              <div className='mcm-samples'>
                {buildSamples(selected).map((sample) => (
                  <div className='mcm-sample' key={`${sample.time}-${sample.title}`}>
                    <strong>{sample.time}</strong>
                    <div>
                      <span>{sample.title}</span>
                      <p>{sample.text}</p>
                    </div>
                    <Tag color={sample.ok ? 'green' : 'red'}>
                      {sample.ok ? '成功' : '错误'}
                    </Tag>
                  </div>
                ))}
              </div>
            </section>
          </div>
        ) : null}
      </SideSheet>
    </div>
  );
}
