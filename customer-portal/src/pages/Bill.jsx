import React, { useState, useEffect, useMemo } from 'react';
import {
  Table,
  DatePicker,
  Typography,
  Toast,
  Button,
  Tag,
  Spin,
  Empty,
  Input,
} from '@douyinfe/semi-ui';
import { IconSearch, IconDownload } from '@douyinfe/semi-icons';
import api from '../utils/api';

const { Title, Text } = Typography;

// Helper: get date N days ago at 00:00:00
function daysAgo(n) {
  const d = new Date();
  d.setDate(d.getDate() - n);
  d.setHours(0, 0, 0, 0);
  return d;
}

// Helper: get today at 23:59:59
function endOfToday() {
  const d = new Date();
  d.setHours(23, 59, 59, 999);
  return d;
}

export default function Bill() {
  const [loading, setLoading] = useState(false);
  const [billData, setBillData] = useState(null);
  const [dateRange, setDateRange] = useState([daysAgo(7), new Date()]);
  const [modelKeyword, setModelKeyword] = useState('');

  const handleQuery = async (range = dateRange) => {
    if (!range || range.length !== 2) {
      Toast.warning('请选择查询时间范围');
      return;
    }
    const [start, end] = range;
    const startTs = Math.floor(new Date(start).getTime() / 1000);
    const endTs = Math.floor(new Date(end).getTime() / 1000);

    if (startTs >= endTs) {
      Toast.warning('开始时间不能晚于结束时间');
      return;
    }

    setLoading(true);
    try {
      const res = await api.get('/api/settlement/bill/self', {
        params: { start_timestamp: startTs, end_timestamp: endTs },
      });
      const { success, data, message } = res.data;
      if (success) {
        setBillData(data);
      } else {
        Toast.error(message || '查询账单失败');
      }
    } catch (err) {
      Toast.error(err.response?.data?.message || '查询账单失败');
    } finally {
      setLoading(false);
    }
  };

  // Auto-query on mount with default 7-day range
  useEffect(() => {
    handleQuery([daysAgo(7), new Date()]);
  }, []);

  // Filter bill items by model keyword (client-side)
  const filteredItems = useMemo(() => {
    if (!billData?.items) return [];
    if (!modelKeyword.trim()) return billData.items;
    const kw = modelKeyword.trim().toLowerCase();
    return billData.items.filter((item) =>
      item.model_name.toLowerCase().includes(kw),
    );
  }, [billData, modelKeyword]);

  // Recalculate total for filtered items
  const filteredTotal = useMemo(() => {
    return filteredItems.reduce((sum, item) => sum + (item.total_amount || 0), 0);
  }, [filteredItems]);

  const handleExport = async () => {
    if (!dateRange || dateRange.length !== 2) {
      Toast.warning('请先查询账单后再导出');
      return;
    }
    const [start, end] = dateRange;
    const startTs = Math.floor(new Date(start).getTime() / 1000);
    const endTs = Math.floor(new Date(end).getTime() / 1000);

    try {
      const res = await api.get('/api/settlement/bill/self/export', {
        params: { start_timestamp: startTs, end_timestamp: endTs },
        responseType: 'blob',
      });
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', 'settlement_bill.csv');
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (err) {
      Toast.error('导出失败');
    }
  };

  const formatNumber = (num) => {
    if (num == null) return '-';
    return Number(num).toLocaleString();
  };

  const formatAmount = (num) => {
    if (num == null) return '-';
    return '' + Number(num).toFixed(6);
  };

  const columns = [
    {
      title: '模型名称',
      dataIndex: 'model_name',
      key: 'model_name',
      render: (text, record) => (
        <span>
          {text}
          {!record.configured && (
            <Tag color="grey" size="small" style={{ marginLeft: 8 }}>
              未配置价格
            </Tag>
          )}
        </span>
      ),
    },
    {
      title: '输入 Token',
      dataIndex: 'input_tokens',
      key: 'input_tokens',
      render: formatNumber,
      align: 'right',
    },
    {
      title: '输出 Token',
      dataIndex: 'output_tokens',
      key: 'output_tokens',
      render: formatNumber,
      align: 'right',
    },
    {
      title: '请求次数',
      dataIndex: 'request_count',
      key: 'request_count',
      render: formatNumber,
      align: 'right',
    },
    {
      title: '输入金额',
      dataIndex: 'input_amount',
      key: 'input_amount',
      render: formatAmount,
      align: 'right',
    },
    {
      title: '输出金额',
      dataIndex: 'output_amount',
      key: 'output_amount',
      render: formatAmount,
      align: 'right',
    },
    {
      title: '次数金额',
      dataIndex: 'request_amount',
      key: 'request_amount',
      render: (val) => (val != null && Number(val) > 0 ? formatAmount(val) : '-'),
      align: 'right',
    },
    {
      title: '合计金额',
      dataIndex: 'total_amount',
      key: 'total_amount',
      render: (val, record) => (
        <Text
          style={{
            color: record.configured
              ? 'var(--semi-color-text-0)'
              : 'var(--semi-color-text-2)',
            fontWeight: record.configured ? 600 : 400,
          }}
        >
          {formatAmount(val)}
        </Text>
      ),
      align: 'right',
    },
  ];

  return (
    <div style={{ padding: '24px', maxWidth: 1100, margin: '0 auto' }}>
      <Title heading={4} style={{ marginBottom: 16 }}>
        账单查询
      </Title>

      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          marginBottom: 20,
          flexWrap: 'wrap',
        }}
      >
        <DatePicker
          type="dateTimeRange"
          density="compact"
          placeholder={['开始时间', '结束时间']}
          value={dateRange}
          format="yyyy-MM-dd HH:mm:ss"
          timePickerOpts={{ showSeconds: true }}
          style={{ width: 460 }}
          onChange={(dates) => setDateRange(dates || [])}
        />
        <Button
          type="primary"
          icon={<IconSearch />}
          loading={loading}
          onClick={() => handleQuery()}
        >
          查询
        </Button>
        <Input
          placeholder="模型关键词筛选"
          prefix={<IconSearch />}
          value={modelKeyword}
          onChange={(val) => setModelKeyword(val)}
          showClear
          style={{ width: 200 }}
        />
        <Button
          icon={<IconDownload />}
          disabled={!billData}
          onClick={handleExport}
        >
          导出 CSV
        </Button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: '60px 0' }}>
          <Spin size="large" />
        </div>
      ) : billData ? (
        <>
          <Table
            columns={columns}
            dataSource={filteredItems}
            rowKey="model_name"
            pagination={false}
            size="middle"
            rowClassName={(record) => (!record.configured ? 'unconfigured-row' : '')}
          />
          <div
            style={{
              textAlign: 'right',
              marginTop: 16,
              padding: '12px 16px',
              background: 'var(--semi-color-fill-0)',
              borderRadius: 6,
            }}
          >
            <Text size="normal" style={{ marginRight: 8 }}>
              {modelKeyword.trim() ? '筛选合计：' : '合计金额：'}
            </Text>
            <Text size="normal" strong style={{ fontSize: 18 }}>
              {formatAmount(filteredTotal)}
            </Text>
          </div>
        </>
      ) : (
        <Empty
          title="请选择时间范围查询账单"
          description="选择开始和结束日期后点击查询按钮"
          style={{ padding: '60px 0' }}
        />
      )}

      <style>{`
        .unconfigured-row td {
          color: var(--semi-color-text-2) !important;
        }
      `}</style>
    </div>
  );
}
