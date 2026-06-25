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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  DatePicker,
  Select,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Database,
  DollarSign,
  RefreshCw,
  TrendingUp,
  WalletCards,
} from 'lucide-react';
import { API, showError } from '../../helpers';
import './style.css';

const { Text, Title } = Typography;

function formatCount(value) {
  return Number(value || 0).toLocaleString();
}

function formatUsd(value) {
  return `$${Number(value || 0).toLocaleString(undefined, {
    maximumFractionDigits: 2,
    minimumFractionDigits: 2,
  })}`;
}

function formatPercent(value) {
  return `${Number(value || 0).toFixed(2)}%`;
}

function formatMs(value) {
  const v = Number(value || 0);
  return v > 0 ? `${v.toLocaleString()} ms` : '-';
}

function buildUsageSummary(rows) {
  return rows.reduce(
    (summary, row) => ({
      costUsd: summary.costUsd + (row.cost_usd || 0),
      totalRequests: summary.totalRequests + (row.total_requests || 0),
      cacheWriteRequests:
        summary.cacheWriteRequests + (row.cache_write_requests || 0),
      cacheReadRequests:
        summary.cacheReadRequests + (row.cache_read_requests || 0),
      cacheWriteTokens:
        summary.cacheWriteTokens + (row.cache_write_tokens || 0),
      cacheReadTokens: summary.cacheReadTokens + (row.cache_read_tokens || 0),
      inputTokens: summary.inputTokens + (row.input_tokens || 0),
      outputTokens: summary.outputTokens + (row.output_tokens || 0),
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
  const now = new Date();
  const weekAgo = new Date(now.getTime() - 7 * 24 * 3600 * 1000);

  const [dateRange, setDateRange] = useState([weekAgo, now]);
  const [rawData, setRawData] = useState([]);
  const [loading, setLoading] = useState(false);

  const [tokenFilter, setTokenFilter] = useState('all');

  const fetchData = async () => {
    if (
      !dateRange ||
      dateRange.length !== 2 ||
      !dateRange[0] ||
      !dateRange[1]
    ) {
      showError('请先选择日期范围');
      return;
    }
    setLoading(true);
    const startTimestamp = Math.floor(dateRange[0].getTime() / 1000);
    const endTimestamp = Math.floor(dateRange[1].getTime() / 1000);
    try {
      const res = await API.get('/api/data/model-usage-analysis', {
        params: {
          start_timestamp: startTimestamp,
          end_timestamp: endTimestamp,
        },
      });
      const { success, message, data } = res.data;
      if (success) {
        setRawData(Array.isArray(data) ? data : []);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const tokenOptions = useMemo(
    () =>
      Array.from(
        new Set(rawData.map((item) => item.token_name).filter(Boolean)),
      ).map((tokenName) => ({ label: tokenName, value: tokenName })),
    [rawData],
  );

  const rows = useMemo(() => {
    return rawData.filter(
      (item) => tokenFilter === 'all' || item.token_name === tokenFilter,
    );
  }, [rawData, tokenFilter]);

  const summary = useMemo(() => buildUsageSummary(rows), [rows]);
  const tokenCount = useMemo(
    () => new Set(rows.map((item) => item.token_name)).size,
    [rows],
  );
  const writeRatio = summary.totalRequests
    ? (summary.cacheWriteRequests / summary.totalRequests) * 100
    : 0;
  const readRatio = summary.totalRequests
    ? (summary.cacheReadRequests / summary.totalRequests) * 100
    : 0;

  const columns = [
    { title: '日期', dataIndex: 'date', fixed: 'left', width: 132 },
    {
      title: 'TokenName',
      dataIndex: 'token_name',
      fixed: 'left',
      width: 168,
      render: (tokenName, row) => (
        <div className='mua-token-cell'>
          <strong>{tokenName || '-'}</strong>
          <span>Key ID {row.token_id || '-'}</span>
        </div>
      ),
    },
    {
      title: '模型',
      dataIndex: 'model_name',
      width: 180,
      render: (modelName) => modelName || '-',
    },
    {
      title: '总请求',
      dataIndex: 'total_requests',
      align: 'right',
      render: formatCount,
      sorter: (a, b) => (a.total_requests || 0) - (b.total_requests || 0),
      width: 118,
    },
    {
      title: '缓存写请求',
      dataIndex: 'cache_write_requests',
      align: 'right',
      sorter: (a, b) =>
        (a.cache_write_requests || 0) - (b.cache_write_requests || 0),
      render: (_, row) => (
        <div className='mua-token-cell' style={{ alignItems: 'flex-end' }}>
          <strong>{formatCount(row.cache_write_requests)}</strong>
          <span>
            5m {formatCount(row.cache_write_5m_requests)} · 1h{' '}
            {formatCount(row.cache_write_1h_requests)}
          </span>
        </div>
      ),
      width: 150,
    },
    {
      title: '写占比',
      align: 'right',
      render: (_, row) =>
        formatPercent(
          row.total_requests
            ? (row.cache_write_requests / row.total_requests) * 100
            : 0,
        ),
      width: 100,
    },
    {
      title: '缓存读请求',
      dataIndex: 'cache_read_requests',
      align: 'right',
      render: formatCount,
      width: 132,
    },
    {
      title: '读占比',
      align: 'right',
      render: (_, row) =>
        formatPercent(
          row.total_requests
            ? (row.cache_read_requests / row.total_requests) * 100
            : 0,
        ),
      width: 100,
    },
    {
      title: '缓存写 tokens',
      dataIndex: 'cache_write_tokens',
      align: 'right',
      render: formatCount,
      width: 150,
    },
    {
      title: '缓存读 tokens',
      dataIndex: 'cache_read_tokens',
      align: 'right',
      render: formatCount,
      width: 150,
    },
    {
      title: '输入 tokens',
      dataIndex: 'input_tokens',
      align: 'right',
      render: formatCount,
      width: 132,
    },
    {
      title: '输出 tokens',
      dataIndex: 'output_tokens',
      align: 'right',
      render: formatCount,
      width: 132,
    },
    {
      title: '平均耗时',
      align: 'right',
      width: 160,
      render: (_, row) => (
        <div className='mua-token-cell' style={{ alignItems: 'flex-end' }}>
          <strong>首字 {formatMs(row.avg_first_token_ms)}</strong>
          <span>请求 {formatMs(row.avg_use_time_ms)}</span>
        </div>
      ),
    },
    {
      title: '消耗 (USD)',
      dataIndex: 'cost_usd',
      align: 'right',
      render: formatUsd,
      sorter: (a, b) => (a.cost_usd || 0) - (b.cost_usd || 0),
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
            按日期、TokenName 与模型查看成本、请求量、缓存读写与输入输出
            tokens。
          </Text>
        </div>
      </div>

      <Card className='mua-toolbar' bodyStyle={{ padding: 14 }}>
        <div className='mua-filter-grid'>
          <label>
            <span>时间范围</span>
            <DatePicker
              type='dateTimeRange'
              value={dateRange}
              onChange={setDateRange}
              style={{ width: '100%' }}
            />
          </label>
          <label>
            <span>TokenName</span>
            <Select
              value={tokenFilter}
              onChange={setTokenFilter}
              style={{ width: '100%' }}
              optionList={[
                { label: '全部 TokenName', value: 'all' },
                ...tokenOptions,
              ]}
            />
          </label>
          <Button
            icon={<RefreshCw size={15} />}
            theme='solid'
            type='primary'
            loading={loading}
            onClick={fetchData}
          >
            刷新
          </Button>
        </div>
      </Card>

      <div className='mua-summary-grid'>
        <SummaryCard
          label='总消耗 (USD)'
          value={formatUsd(summary.costUsd)}
          hint={`${tokenCount} 个 TokenName，${rows.length} 条记录`}
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
          rowKey={(record) =>
            `${record.date}-${record.token_id}-${record.model_name}`
          }
          columns={columns}
          dataSource={rows}
          loading={loading}
          pagination={false}
          scroll={{ x: 1762 }}
          size='small'
        />
      </Card>
    </div>
  );
}
