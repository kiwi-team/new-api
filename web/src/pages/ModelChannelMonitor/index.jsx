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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Card,
  Select,
  SideSheet,
  Space,
  TabPane,
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
import { API, showError } from '../../helpers';
import { CHANNEL_OPTIONS } from '../../constants';
import './style.css';

const { Text, Title } = Typography;

const CHANNEL_TYPE_LABELS = CHANNEL_OPTIONS.reduce((map, option) => {
  map[option.value] = option.label;
  return map;
}, {});

function getChannelTypeLabel(typeId) {
  return CHANNEL_TYPE_LABELS[typeId] || `type-${typeId ?? '-'}`;
}

const STATUS_META = {
  healthy: { text: '健康', color: 'green', order: 3 },
  degraded: { text: '波动', color: 'amber', order: 2 },
  down: { text: '异常', color: 'red', order: 1 },
  unknown: { text: '未知', color: 'grey', order: 4 },
};

// 总耗时类指标以秒展示（入参为毫秒），最多 1 位小数
function formatSeconds(value) {
  if (value === null || value === undefined) return '-';
  return `${Number(value / 1000).toLocaleString(undefined, {
    maximumFractionDigits: 1,
  })} s`;
}

// 首字耗时也以秒展示（入参为毫秒），最多 2 位小数；无流式样本显示"待埋点"
function formatFirstSeconds(value) {
  if (value === null || value === undefined) return '待埋点';
  return `${Number(value / 1000).toLocaleString(undefined, {
    maximumFractionDigits: 2,
  })} s`;
}

// 各指标按自身阈值上色（入参为毫秒）；无数据返回空（中性色）
function useTimeTone(ms) {
  if (ms === null || ms === undefined) return '';
  if (ms > 120000) return 'bad';
  if (ms > 60000) return 'warn';
  return 'good';
}

function firstTokenTone(ms) {
  if (ms === null || ms === undefined) return '';
  if (ms > 5000) return 'bad';
  if (ms > 2000) return 'warn';
  return 'good';
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

function getRecordVolume(record) {
  return record.requests + record.errors;
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
        status: getAggregateStatus(
          total ? (item.requests / total) * 100 : 0,
          item.p95UseMs,
        ),
      };
    })
    .sort((a, b) => b.total - a.total);
}

function buildErrorBreakdown(records) {
  return records
    .flatMap((record) =>
      record.errorsTop.map((error, index) => ({
        ...error,
        index,
        channelId: record.channelId,
        channelName: record.channelName,
        channelTypeId: record.channelTypeId,
        keyId: record.keyId,
        keyName: record.keyName,
        keyHint: record.keyHint,
        model: record.model,
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

function TrendChart({ record, axisLabels = [] }) {
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
        {(axisLabels.length ? axisLabels : ['', '', '', '', '']).map(
          (label, index) => (
            <span key={index}>{label}</span>
          ),
        )}
      </div>
    </div>
  );
}

function getErrorPreview(error, showSensitive = false) {
  if (showSensitive) {
    return error.code ? `${error.code}: ${error.text}` : error.text;
  }
  return error.code ? `错误码 ${error.code}` : '错误类型未归类';
}

function ErrorList({ record, limit = 2, showSensitive = false }) {
  if (!record.errorsTop.length) {
    return <div className='mcm-empty-error'>该时间范围内没有聚合错误。</div>;
  }
  return (
    <div className='mcm-error-list'>
      {record.errorsTop.slice(0, limit).map((error, index) => (
        <div
          className='mcm-error-item'
          key={`${error.code}-${error.last}-${index}`}
        >
          <div className='mcm-error-meta'>
            <span>{formatCount(error.count)} 次</span>
            <small>最近 {error.last}</small>
          </div>
          <div className='mcm-error-text'>
            {getErrorPreview(error, showSensitive)}
          </div>
        </div>
      ))}
    </div>
  );
}

function ErrorChannelStrip({
  errors,
  onOpen,
  limit = 6,
  showSensitive = false,
}) {
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
        {errors.slice(0, limit).map((error) => (
          <button
            key={`${error.channelId}-${error.model}-${error.code}-${error.index}`}
            type='button'
            onClick={() => onOpen(error.record)}
          >
            <div className='mcm-error-channel-head'>
              <strong>
                {showSensitive
                  ? `${error.channelName} · ID ${error.channelId}`
                  : `渠道 ID ${error.channelId}`}
              </strong>
              <Tag
                color={error.status === 'down' ? 'red' : 'amber'}
                size='small'
              >
                {formatCount(error.count)} 次
              </Tag>
            </div>
            <div className='mcm-error-channel-meta'>
              <span>{error.model}</span>
              <span>{getChannelTypeLabel(error.channelTypeId)}</span>
            </div>
            <p>{getErrorPreview(error, showSensitive)}</p>
          </button>
        ))}
      </div>
    </section>
  );
}

function ChannelErrorTable({ errors, onOpen, showSensitive = false }) {
  if (!errors.length) {
    return <div className='mcm-empty-error'>该时间范围内没有聚合错误。</div>;
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
            <strong>
              {showSensitive
                ? `${error.channelName} · ID ${error.channelId}`
                : `渠道 ID ${error.channelId}`}
            </strong>
            <span>{getChannelTypeLabel(error.channelTypeId)}</span>
          </div>
          <div>
            <strong>{error.model}</strong>
          </div>
          <div>
            <strong>{formatCount(error.count)} 次</strong>
            <span>最近 {error.last}</span>
          </div>
          <p>{getErrorPreview(error, showSensitive)}</p>
        </button>
      ))}
    </div>
  );
}

function MonitorCard({
  record,
  onOpen,
  showSensitive = false,
  axisLabels = [],
  rangeLabel = '过去 24 小时',
}) {
  const successTone =
    record.successRate < 90 ? 'bad' : record.successRate < 98 ? 'warn' : 'good';

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
              {record.channelName
                ? `${record.channelName} · ID ${record.channelId}`
                : `渠道 ID ${record.channelId}`}
            </Tag>
            {record.channelGroup ? (
              <Tag color='purple' size='small'>
                分组 {record.channelGroup}
              </Tag>
            ) : null}
            <Tag color='cyan' size='small'>
              {getChannelTypeLabel(record.channelTypeId)}
            </Tag>
          </Space>
        </div>
        <StatusBadge status={record.status} />
      </div>
      <div className='mcm-kpis compact'>
        <div>
          <span>成功率</span>
          <strong className={`tone-${successTone}`}>
            {Number(record.successRate).toFixed(1)}%
          </strong>
          <p>
            成功 {formatCount(record.requests)} / 错误{' '}
            {formatCount(record.errors)}
          </p>
        </div>
        <div className='focus'>
          <span>P95 耗时</span>
          <div className='mcm-dual'>
            <div>
              <small>总耗时</small>
              <strong className={`tone-${useTimeTone(record.p95UseMs)}`}>
                {formatSeconds(record.p95UseMs)}
              </strong>
            </div>
            <div>
              <small>首字</small>
              <strong className={`tone-${firstTokenTone(record.p95FirstMs)}`}>
                {formatFirstSeconds(record.p95FirstMs)}
              </strong>
            </div>
          </div>
        </div>
        <div>
          <span>最近错误</span>
          <strong className={record.errors ? 'tone-bad' : ''}>
            {record.errors ? record.lastError : '-'}
          </strong>
          <p>{formatCount(record.errors)} 次错误</p>
        </div>
      </div>
      <div className='mcm-card-chart compact'>
        <div className='mcm-section-line'>
          <span>总耗时趋势 · {rangeLabel}</span>
          <span>{formatCount(record.samples)} 个样本点</span>
        </div>
        <TrendChart record={record} axisLabels={axisLabels} />
      </div>
      <div className='mcm-card-errors'>
        <h3>{showSensitive ? '错误原因' : '错误概览'}</h3>
        <ErrorList record={record} showSensitive={showSensitive} />
      </div>
    </article>
  );
}

export default function ModelChannelMonitor({ internalView = false }) {
  const [scene, setScene] = useState('overview');
  const [statusFilter, setStatusFilter] = useState('all');
  const [selected, setSelected] = useState(null);

  const [records, setRecords] = useState([]);
  const [loading, setLoading] = useState(false);
  const [timeRange, setTimeRange] = useState('24h');
  const [timeWindow, setTimeWindow] = useState(null);
  const [selectedModels, setSelectedModels] = useState([]);
  const modelsInitialized = useRef(false);

  const rangeLabel =
    timeRange === '1h'
      ? '过去 1 小时'
      : timeRange === '6h'
        ? '过去 6 小时'
        : '过去 24 小时';

  const fetchData = async (range = timeRange) => {
    setLoading(true);
    const hours = range === '1h' ? 1 : range === '6h' ? 6 : 24;
    const endTimestamp = Math.floor(Date.now() / 1000);
    const startTimestamp = endTimestamp - hours * 3600;
    setTimeWindow({ start: startTimestamp, end: endTimestamp });
    try {
      const res = await API.get('/api/data/channel-monitor', {
        params: {
          start_timestamp: startTimestamp,
          end_timestamp: endTimestamp,
        },
      });
      const { success, message, data } = res.data;
      if (success) {
        const list = Array.isArray(data) ? data : [];
        setRecords(list);
        // 首次加载默认关注样本量最高的 6 个模型
        if (!modelsInitialized.current) {
          modelsInitialized.current = true;
          setSelectedModels(
            buildModelStats(list)
              .slice(0, 6)
              .map((item) => item.model),
          );
        }
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
    fetchData(timeRange);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timeRange]);

  // 趋势图 X 轴标签：按实际时间窗均匀取 5 个刻度（本地时区 HH:MM）
  const axisLabels = useMemo(() => {
    if (!timeWindow) return [];
    const fmt = (ts) => {
      const d = new Date(ts * 1000);
      return `${String(d.getHours()).padStart(2, '0')}:${String(
        d.getMinutes(),
      ).padStart(2, '0')}`;
    };
    const span = timeWindow.end - timeWindow.start;
    return [0, 0.25, 0.5, 0.75, 1].map((f) => fmt(timeWindow.start + span * f));
  }, [timeWindow]);

  const modelStats = useMemo(() => buildModelStats(records), [records]);
  const selectedModelSet = useMemo(
    () => new Set(selectedModels),
    [selectedModels],
  );

  const modelOptions = useMemo(
    () =>
      modelStats.map((item) => ({
        label: `${item.model} · ${formatCount(item.total)} 次`,
        value: item.model,
      })),
    [modelStats],
  );

  const baseRecords = useMemo(() => {
    return records
      .filter((item) => selectedModelSet.has(item.model))
      .filter((item) => statusFilter === 'all' || item.status === statusFilter);
  }, [records, selectedModelSet, statusFilter]);

  const filteredRecords = useMemo(() => {
    return baseRecords.sort(
      (a, b) =>
        STATUS_META[a.status].order - STATUS_META[b.status].order ||
        getRecordVolume(b) - getRecordVolume(a),
    );
  }, [baseRecords]);

  const errorBreakdown = useMemo(
    () => buildErrorBreakdown(filteredRecords),
    [filteredRecords],
  );

  const focusRecords = useMemo(() => {
    const issues = filteredRecords.filter(
      (item) => item.status !== 'healthy' || item.errors > 0,
    );
    const source = issues.length ? issues : filteredRecords;
    return source
      .slice()
      .sort(
        (a, b) =>
          STATUS_META[a.status].order - STATUS_META[b.status].order ||
          b.errors - a.errors ||
          getRecordVolume(b) - getRecordVolume(a),
      )
      .slice(0, 6);
  }, [filteredRecords]);

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
    const successRate = totalRequests
      ? (totalSuccess / totalRequests) * 100
      : 0;
    const useSamples = filteredRecords.filter((item) => item.useMs !== null);
    const avgUse = useSamples.length
      ? useSamples.reduce((sum, item) => sum + item.useMs, 0) /
        useSamples.length
      : 0;
    // 首字耗时仅流式请求有值（firstMs 为 null 表示无流式样本，不参与平均）
    const firstSamples = filteredRecords.filter(
      (item) => item.firstMs !== null && item.firstMs !== undefined,
    );
    const avgFirst = firstSamples.length
      ? firstSamples.reduce((sum, item) => sum + item.firstMs, 0) /
        firstSamples.length
      : null;
    return {
      totalRequests,
      totalErrors,
      successRate,
      avgUse,
      avgFirst,
      down: filteredRecords.filter((item) => item.status === 'down').length,
      degraded: filteredRecords.filter((item) => item.status === 'degraded')
        .length,
    };
  }, [filteredRecords]);

  const overviewRecords = internalView ? filteredRecords : focusRecords;

  return (
    <div className='mcm-page'>
      <div className='mcm-header'>
        <div>
          <Title heading={3} className='!mb-1'>
            {internalView ? '内部渠道监控' : '模型渠道监控'}
          </Title>
          <Text type='secondary'>
            {internalView
              ? '内部排障视角：展示渠道名、详细错误和模型可用性趋势。'
              : '外部可用性视角：按 Key、渠道、模型聚合健康状态、耗时与成功率。'}
          </Text>
        </div>
        <div className='mcm-refresh-note'>
          <span />
          {loading ? '加载中…' : `共 ${records.length} 个渠道组合`}
        </div>
      </div>

      <Card className='mcm-toolbar' bodyStyle={{ padding: 14 }}>
        <Tabs
          activeKey={scene}
          className='mcm-head-tabs'
          onChange={setScene}
          type='button'
        >
          <TabPane tab='健康总览' itemKey='overview' />
          <TabPane tab='错误渠道' itemKey='errors' />
        </Tabs>
        <div className='mcm-filter-grid'>
          <label>
            <span>时间范围</span>
            <Select
              value={timeRange}
              onChange={setTimeRange}
              style={{ width: '100%' }}
            >
              <Select.Option value='24h'>过去 24 小时</Select.Option>
              <Select.Option value='6h'>过去 6 小时</Select.Option>
              <Select.Option value='1h'>过去 1 小时</Select.Option>
            </Select>
          </label>
          <label>
            <span>关注模型</span>
            <Select
              multiple
              filter
              maxTagCount={2}
              value={selectedModels}
              onChange={(value) =>
                setSelectedModels(Array.isArray(value) ? value : [])
              }
              placeholder='勾选要展示的模型'
              style={{ width: '100%' }}
              optionList={modelOptions}
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
          <Button
            icon={<RefreshCw size={15} />}
            theme='solid'
            type='primary'
            loading={loading}
            onClick={() => fetchData(timeRange)}
          >
            刷新
          </Button>
        </div>
      </Card>

      <div className='mcm-summary-grid'>
        <MetricCard
          label='关注组合'
          value={filteredRecords.length}
          hint={`${selectedModels.length} 个关注模型`}
          icon={<LayoutGrid size={16} />}
        />
        <MetricCard
          label='整体成功率'
          value={`${summary.successRate.toFixed(1)}%`}
          hint='成功日志 / 成功日志+错误日志'
          tone='good'
          icon={<Activity size={16} />}
        />
        <div className='mcm-summary-card'>
          <div className='mcm-summary-head'>
            <span>平均耗时</span>
            <Gauge size={16} />
          </div>
          <div className='mcm-dual'>
            <div>
              <small>总耗时</small>
              <strong className={`tone-${useTimeTone(summary.avgUse)}`}>
                {formatSeconds(summary.avgUse)}
              </strong>
            </div>
            <div>
              <small>首字</small>
              <strong className={`tone-${firstTokenTone(summary.avgFirst)}`}>
                {formatFirstSeconds(summary.avgFirst)}
              </strong>
            </div>
          </div>
        </div>
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
          hint='需要优先排查'
          icon={<ListFilter size={16} />}
        />
      </div>

      {scene === 'overview' ? (
        <section className='mcm-focus-section'>
          <div className='mcm-group-title'>
            <div>
              <strong>{internalView ? '内部全量视角' : '异常优先'}</strong>
              <Tag size='small' color='grey'>
                {overviewRecords.length} 个关注组合
              </Tag>
            </div>
            <span>
              {internalView
                ? '展示已筛选模型下的全部渠道组合，方便内部排障'
                : '按勾选模型过滤后异常优先'}
            </span>
          </div>
          {overviewRecords.length ? (
            <div className='mcm-card-grid'>
              {overviewRecords.map((item) => (
                <MonitorCard
                  key={`${item.keyId}-${item.channelId}-${item.model}`}
                  record={item}
                  onOpen={setSelected}
                  showSensitive={internalView}
                  axisLabels={axisLabels}
                  rangeLabel={rangeLabel}
                />
              ))}
            </div>
          ) : (
            <div className='mcm-empty-state'>当前没有选中的模型组合。</div>
          )}
        </section>
      ) : null}

      {scene === 'errors' ? (
        <Card bodyStyle={{ padding: 14 }} className='mcm-table-card'>
          <ErrorChannelStrip
            errors={errorBreakdown}
            limit={Math.min(errorBreakdown.length, 9)}
            onOpen={setSelected}
            showSensitive={internalView}
          />
          <ChannelErrorTable
            errors={errorBreakdown.slice(0, 12)}
            onOpen={setSelected}
            showSensitive={internalView}
          />
        </Card>
      ) : null}

      <SideSheet
        title={selected ? selected.model : '模型渠道详情'}
        visible={Boolean(selected)}
        onCancel={() => setSelected(null)}
        width={560}
      >
        {selected ? (
          <div className='mcm-detail'>
            <Text type='secondary'>
              {getKeyLabel(selected)} ·{' '}
              {selected.channelName
                ? `${selected.channelName} · ID ${selected.channelId}`
                : `渠道 ID ${selected.channelId}`}
              {selected.channelGroup ? ` · 分组 ${selected.channelGroup}` : ''}
            </Text>
            <div className='mcm-detail-grid'>
              <div>
                <span>状态</span>
                <StatusBadge status={selected.status} />
              </div>
              <div>
                <span>成功率</span>
                <strong>{Number(selected.successRate).toFixed(1)}%</strong>
              </div>
              <div>
                <span>平均耗时</span>
                <strong>{formatSeconds(selected.useMs)}</strong>
              </div>
              <div>
                <span>P95 耗时</span>
                <strong>{formatSeconds(selected.p95UseMs)}</strong>
              </div>
              <div>
                <span>平均首字</span>
                <strong>{formatFirstSeconds(selected.firstMs)}</strong>
              </div>
              <div>
                <span>P95 首字</span>
                <strong>{formatFirstSeconds(selected.p95FirstMs)}</strong>
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
              <TrendChart record={selected} axisLabels={axisLabels} />
            </section>
            <section className='mcm-detail-section'>
              <h3>{internalView ? '错误原因' : '错误概览'}</h3>
              <ErrorList
                record={selected}
                limit={5}
                showSensitive={internalView}
              />
            </section>
            <section className='mcm-detail-section'>
              <h3>渠道错误明细</h3>
              <ChannelErrorTable
                errors={buildErrorBreakdown([selected])}
                onOpen={setSelected}
                showSensitive={internalView}
              />
            </section>
          </div>
        ) : null}
      </SideSheet>
    </div>
  );
}
