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
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Database,
  DollarSign,
  RefreshCw,
  TrendingUp,
  WalletCards,
} from 'lucide-react';
import './style.css';

const { Text, Title } = Typography;

const TOKEN_META = {
  cc1: { provider: 'Claude', keyId: 52, keyHint: '0mtTyBA***TGGg' },
  BcCeAf: { provider: 'Minimax', keyId: 19, keyHint: 'm83fTuc***CeAf' },
  '123CbBa': { provider: 'Claude', keyId: 17, keyHint: 'LfUBAup***CbBa' },
  'tools-调用': { provider: 'Qwen', keyId: 27, keyHint: 'qwen***tool' },
};

const USAGE_DATA = [
  {
    dayIndex: 3,
    date: '6月3日',
    tokenName: 'cc1',
    costUsd: 9519.55,
    totalRequests: 24203,
    cacheWriteRequests: 21866,
    cacheReadRequests: 16898,
    cacheWriteTokens: 1181928092,
    cacheReadTokens: 1850328539,
    inputTokens: 27447530,
    outputTokens: 20246318,
  },
  {
    dayIndex: 4,
    date: '6月4日',
    tokenName: 'cc1',
    costUsd: 26367.24,
    totalRequests: 73176,
    cacheWriteRequests: 62239,
    cacheReadRequests: 46968,
    cacheWriteTokens: 3339784080,
    cacheReadTokens: 5984220208,
    inputTokens: 68355631,
    outputTokens: 56689744,
  },
  {
    dayIndex: 5,
    date: '6月5日',
    tokenName: 'cc1',
    costUsd: 29232.68,
    totalRequests: 109535,
    cacheWriteRequests: 92390,
    cacheReadRequests: 79599,
    cacheWriteTokens: 3279075306,
    cacheReadTokens: 9370647384,
    inputTokens: 282007361,
    outputTokens: 105507757,
  },
  {
    dayIndex: 6,
    date: '6月6日',
    tokenName: 'cc1',
    costUsd: 15012.7,
    totalRequests: 55606,
    cacheWriteRequests: 40730,
    cacheReadRequests: 37044,
    cacheWriteTokens: 1378406524,
    cacheReadTokens: 6328731212,
    inputTokens: 310882713,
    outputTokens: 66654905,
  },
  {
    dayIndex: 7,
    date: '6月7日',
    tokenName: 'cc1',
    costUsd: 17406.47,
    totalRequests: 66116,
    cacheWriteRequests: 43239,
    cacheReadRequests: 38483,
    cacheWriteTokens: 1577787486,
    cacheReadTokens: 6125791296,
    inputTokens: 566239644,
    outputTokens: 64445088,
  },
  {
    dayIndex: 8,
    date: '6月8日',
    tokenName: 'cc1',
    costUsd: 28363.76,
    totalRequests: 141773,
    cacheWriteRequests: 96947,
    cacheReadRequests: 87079,
    cacheWriteTokens: 2361971679,
    cacheReadTokens: 13667933785,
    inputTokens: 621225999,
    outputTokens: 136386810,
  },
  {
    dayIndex: 9,
    date: '6月9日',
    tokenName: 'cc1',
    costUsd: 32950.69,
    totalRequests: 126381,
    cacheWriteRequests: 119617,
    cacheReadRequests: 108177,
    cacheWriteTokens: 3059710950,
    cacheReadTokens: 17130071942,
    inputTokens: 513544326,
    outputTokens: 105816399,
  },
  {
    dayIndex: 10,
    date: '6月10日',
    tokenName: 'cc1',
    costUsd: 40735.71,
    totalRequests: 149483,
    cacheWriteRequests: 136928,
    cacheReadRequests: 131979,
    cacheWriteTokens: 4102322584,
    cacheReadTokens: 18813590748,
    inputTokens: 372312052,
    outputTokens: 151121020,
  },
  {
    dayIndex: 11,
    date: '6月11日',
    tokenName: 'cc1',
    costUsd: 31043.15,
    totalRequests: 154737,
    cacheWriteRequests: 141228,
    cacheReadRequests: 144877,
    cacheWriteTokens: 2546606087,
    cacheReadTokens: 19777639725,
    inputTokens: 355905354,
    outputTokens: 130631460,
  },
  {
    dayIndex: 11,
    date: '6月11日',
    tokenName: 'BcCeAf',
    costUsd: 4838.22,
    totalRequests: 43943,
    cacheWriteRequests: 12890,
    cacheReadRequests: 11642,
    cacheWriteTokens: 331204800,
    cacheReadTokens: 894220112,
    inputTokens: 72180444,
    outputTokens: 24310572,
  },
  {
    dayIndex: 11,
    date: '6月11日',
    tokenName: '123CbBa',
    costUsd: 2119.84,
    totalRequests: 12522,
    cacheWriteRequests: 8234,
    cacheReadRequests: 7712,
    cacheWriteTokens: 188340221,
    cacheReadTokens: 560113920,
    inputTokens: 31890341,
    outputTokens: 11840220,
  },
  {
    dayIndex: 11,
    date: '6月11日',
    tokenName: 'tools-调用',
    costUsd: 1763.38,
    totalRequests: 13023,
    cacheWriteRequests: 9410,
    cacheReadRequests: 8054,
    cacheWriteTokens: 233903117,
    cacheReadTokens: 620145900,
    inputTokens: 27900118,
    outputTokens: 9434201,
  },
];

function formatCount(value) {
  return Number(value).toLocaleString();
}

function formatUsd(value) {
  return `$${Number(value).toLocaleString(undefined, {
    maximumFractionDigits: 2,
    minimumFractionDigits: 2,
  })}`;
}

function formatPercent(value) {
  return `${Number(value || 0).toFixed(2)}%`;
}

function getTokenMeta(tokenName) {
  return TOKEN_META[tokenName] || {
    provider: '其他',
    keyId: '-',
    keyHint: '-',
  };
}

function buildUsageSummary(rows) {
  return rows.reduce(
    (summary, row) => ({
      costUsd: summary.costUsd + row.costUsd,
      totalRequests: summary.totalRequests + row.totalRequests,
      cacheWriteRequests: summary.cacheWriteRequests + row.cacheWriteRequests,
      cacheReadRequests: summary.cacheReadRequests + row.cacheReadRequests,
      cacheWriteTokens: summary.cacheWriteTokens + row.cacheWriteTokens,
      cacheReadTokens: summary.cacheReadTokens + row.cacheReadTokens,
      inputTokens: summary.inputTokens + row.inputTokens,
      outputTokens: summary.outputTokens + row.outputTokens,
    }),
    {
      costUsd: 0,
      totalRequests: 0,
      cacheWriteRequests: 0,
      cacheReadRequests: 0,
      cacheWriteTokens: 0,
      cacheReadTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
    },
  );
}

function SummaryCard({ label, value, hint, icon, tone = 'default' }) {
  return (
    <div className={`mua-summary-card tone-${tone}`}>
      <div className='mua-summary-head'>
        <span>{label}</span>
        {icon}
      </div>
      <strong>{value}</strong>
      <p>{hint}</p>
    </div>
  );
}

export default function ModelUsageAnalysis() {
  const [tokenFilter, setTokenFilter] = useState('all');
  const [providerFilter, setProviderFilter] = useState('all');
  const [rangeFilter, setRangeFilter] = useState('all');

  const tokenOptions = useMemo(
    () =>
      Array.from(new Set(USAGE_DATA.map((item) => item.tokenName))).map(
        (tokenName) => ({
          label: `${tokenName} · Key ID ${getTokenMeta(tokenName).keyId}`,
          value: tokenName,
        }),
      ),
    [],
  );

  const providerOptions = useMemo(
    () =>
      Array.from(
        new Set(USAGE_DATA.map((item) => getTokenMeta(item.tokenName).provider)),
      ).map((provider) => ({ label: provider, value: provider })),
    [],
  );

  const rows = useMemo(() => {
    const minDay = rangeFilter === '3d' ? 9 : rangeFilter === '7d' ? 5 : 0;
    return USAGE_DATA.filter(
      (item) => tokenFilter === 'all' || item.tokenName === tokenFilter,
    )
      .filter(
        (item) =>
          providerFilter === 'all' ||
          getTokenMeta(item.tokenName).provider === providerFilter,
      )
      .filter((item) => item.dayIndex >= minDay)
      .sort((a, b) => b.dayIndex - a.dayIndex || a.tokenName.localeCompare(b.tokenName));
  }, [providerFilter, rangeFilter, tokenFilter]);

  const summary = useMemo(() => buildUsageSummary(rows), [rows]);
  const tokenCount = useMemo(
    () => new Set(rows.map((item) => item.tokenName)).size,
    [rows],
  );
  const writeRatio = summary.totalRequests
    ? (summary.cacheWriteRequests / summary.totalRequests) * 100
    : 0;
  const readRatio = summary.totalRequests
    ? (summary.cacheReadRequests / summary.totalRequests) * 100
    : 0;

  const columns = [
    { title: '日期', dataIndex: 'date', fixed: 'left', width: 96 },
    {
      title: 'TokenName',
      dataIndex: 'tokenName',
      fixed: 'left',
      width: 158,
      render: (tokenName) => {
        const meta = getTokenMeta(tokenName);
        return (
          <div className='mua-token-cell'>
            <strong>{tokenName}</strong>
            <span>Key ID {meta.keyId} · {meta.keyHint}</span>
          </div>
        );
      },
    },
    {
      title: '厂商',
      width: 98,
      render: (_, row) => (
        <Tag color='blue' size='small'>
          {getTokenMeta(row.tokenName).provider}
        </Tag>
      ),
    },
    {
      title: '消耗 (USD)',
      dataIndex: 'costUsd',
      align: 'right',
      render: formatUsd,
      sorter: (a, b) => a.costUsd - b.costUsd,
      width: 132,
    },
    {
      title: '总请求',
      dataIndex: 'totalRequests',
      align: 'right',
      render: formatCount,
      sorter: (a, b) => a.totalRequests - b.totalRequests,
      width: 118,
    },
    {
      title: '缓存写请求',
      dataIndex: 'cacheWriteRequests',
      align: 'right',
      render: formatCount,
      width: 132,
    },
    {
      title: '写占比',
      align: 'right',
      render: (_, row) =>
        formatPercent((row.cacheWriteRequests / row.totalRequests) * 100),
      width: 100,
    },
    {
      title: '缓存读请求',
      dataIndex: 'cacheReadRequests',
      align: 'right',
      render: formatCount,
      width: 132,
    },
    {
      title: '读占比',
      align: 'right',
      render: (_, row) =>
        formatPercent((row.cacheReadRequests / row.totalRequests) * 100),
      width: 100,
    },
    {
      title: '缓存写 tokens',
      dataIndex: 'cacheWriteTokens',
      align: 'right',
      render: formatCount,
      width: 150,
    },
    {
      title: '缓存读 tokens',
      dataIndex: 'cacheReadTokens',
      align: 'right',
      render: formatCount,
      width: 150,
    },
    {
      title: '输入 tokens',
      dataIndex: 'inputTokens',
      align: 'right',
      render: formatCount,
      width: 132,
    },
    {
      title: '输出 tokens',
      dataIndex: 'outputTokens',
      align: 'right',
      render: formatCount,
      width: 132,
    },
  ];

  return (
    <div className='mua-page'>
      <div className='mua-header'>
        <div>
          <Title heading={3} className='!mb-1'>
            用量分析
          </Title>
          <Text type='secondary'>
            按 TokenName 和日期查看成本、请求量、缓存读写与输入输出 tokens。
          </Text>
        </div>
        <div className='mua-refresh-note'>
          <span />
          线上样本 mock · 2026-06-11
        </div>
      </div>

      <Card className='mua-toolbar' bodyStyle={{ padding: 14 }}>
        <div className='mua-filter-grid'>
          <label>
            <span>时间范围</span>
            <Select value={rangeFilter} onChange={setRangeFilter} style={{ width: '100%' }}>
              <Select.Option value='all'>全部样本</Select.Option>
              <Select.Option value='7d'>最近 7 天</Select.Option>
              <Select.Option value='3d'>最近 3 天</Select.Option>
            </Select>
          </label>
          <label>
            <span>TokenName</span>
            <Select
              value={tokenFilter}
              onChange={setTokenFilter}
              style={{ width: '100%' }}
              optionList={[{ label: '全部 TokenName', value: 'all' }, ...tokenOptions]}
            />
          </label>
          <label>
            <span>厂商</span>
            <Select
              value={providerFilter}
              onChange={setProviderFilter}
              style={{ width: '100%' }}
              optionList={[{ label: '全部厂商', value: 'all' }, ...providerOptions]}
            />
          </label>
          <Button icon={<RefreshCw size={15} />} theme='solid' type='primary'>
            刷新
          </Button>
        </div>
      </Card>

      <div className='mua-summary-grid'>
        <SummaryCard
          label='总消耗 (USD)'
          value={formatUsd(summary.costUsd)}
          hint={`${tokenCount} 个 TokenName，${rows.length} 条按日记录`}
          tone='warn'
          icon={<DollarSign size={16} />}
        />
        <SummaryCard
          label='总请求'
          value={formatCount(summary.totalRequests)}
          hint='当前筛选范围请求总量'
          icon={<TrendingUp size={16} />}
        />
        <SummaryCard
          label='缓存写'
          value={formatPercent(writeRatio)}
          hint={`${formatCount(summary.cacheWriteRequests)} 次 · ${formatCount(summary.cacheWriteTokens)} tokens`}
          icon={<Database size={16} />}
        />
        <SummaryCard
          label='缓存读'
          value={formatPercent(readRatio)}
          hint={`${formatCount(summary.cacheReadRequests)} 次 · ${formatCount(summary.cacheReadTokens)} tokens`}
          icon={<Database size={16} />}
        />
        <SummaryCard
          label='输入 / 输出 tokens'
          value={`${formatCount(summary.inputTokens)} / ${formatCount(summary.outputTokens)}`}
          hint='非缓存输入与模型输出合计'
          icon={<WalletCards size={16} />}
        />
      </div>

      <Card bodyStyle={{ padding: 0 }} className='mua-table-card'>
        <Table
          rowKey={(record) => `${record.date}-${record.tokenName}`}
          columns={columns}
          dataSource={rows}
          pagination={false}
          scroll={{ x: 1680 }}
          size='small'
        />
      </Card>
    </div>
  );
}
