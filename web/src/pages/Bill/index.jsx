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

import React, { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
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
  Tabs,
  TabPane,
} from '@douyinfe/semi-ui';
import { IconSearch, IconDownload, IconRefresh } from '@douyinfe/semi-icons';
import { API } from '../../helpers/api';

const { Title, Text } = Typography;

function daysAgo(n) {
  const d = new Date();
  d.setDate(d.getDate() - n);
  d.setHours(0, 0, 0, 0);
  return d;
}

const Bill = () => {
  const { t } = useTranslation();

  // ----- Bill tab state -----
  const [loading, setLoading] = useState(false);
  const [billData, setBillData] = useState(null);
  const [dateRange, setDateRange] = useState([daysAgo(7), new Date()]);
  const [modelKeyword, setModelKeyword] = useState('');

  // ----- Pricing tab state -----
  const [pricingLoading, setPricingLoading] = useState(false);
  const [pricingData, setPricingData] = useState([]);
  const [pricingLoaded, setPricingLoaded] = useState(false);
  const [pricingKeyword, setPricingKeyword] = useState('');

  const [activeTab, setActiveTab] = useState('bill');

  const handleQuery = async (range = dateRange) => {
    if (!range || range.length !== 2) {
      Toast.warning(t('请选择查询时间范围'));
      return;
    }
    const [start, end] = range;
    const startTs = Math.floor(new Date(start).getTime() / 1000);
    const endTs = Math.floor(new Date(end).getTime() / 1000);

    if (startTs >= endTs) {
      Toast.warning(t('开始时间不能晚于结束时间'));
      return;
    }

    setLoading(true);
    try {
      const res = await API.get('/api/settlement/bill/self', {
        params: { start_timestamp: startTs, end_timestamp: endTs },
      });
      const { success, data, message } = res.data;
      if (success) {
        setBillData(data);
      } else {
        Toast.error(message || t('查询账单失败'));
      }
    } catch (err) {
      Toast.error(err.response?.data?.message || t('查询账单失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    handleQuery([daysAgo(7), new Date()]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const loadPricing = async () => {
    setPricingLoading(true);
    try {
      const res = await API.get('/api/settlement/config/self');
      const { success, data, message } = res.data;
      if (success) {
        setPricingData(Array.isArray(data) ? data : []);
        setPricingLoaded(true);
      } else {
        Toast.error(message || t('查询结算价格失败'));
      }
    } catch (err) {
      Toast.error(err.response?.data?.message || t('查询结算价格失败'));
    } finally {
      setPricingLoading(false);
    }
  };

  // Lazy-load pricing on first switch to that tab
  useEffect(() => {
    if (activeTab === 'pricing' && !pricingLoaded && !pricingLoading) {
      loadPricing();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab]);

  const filteredItems = useMemo(() => {
    if (!billData?.items) return [];
    if (!modelKeyword.trim()) return billData.items;
    const kw = modelKeyword.trim().toLowerCase();
    return billData.items.filter((item) =>
      item.model_name.toLowerCase().includes(kw),
    );
  }, [billData, modelKeyword]);

  const filteredTotal = useMemo(() => {
    return filteredItems.reduce(
      (sum, item) => sum + (item.total_amount || 0),
      0,
    );
  }, [filteredItems]);

  const filteredPricing = useMemo(() => {
    if (!pricingKeyword.trim()) return pricingData;
    const kw = pricingKeyword.trim().toLowerCase();
    return pricingData.filter((item) =>
      (item.model_name || '').toLowerCase().includes(kw),
    );
  }, [pricingData, pricingKeyword]);

  const handleExport = async () => {
    if (!dateRange || dateRange.length !== 2) {
      Toast.warning(t('请先查询账单后再导出'));
      return;
    }
    const [start, end] = dateRange;
    const startTs = Math.floor(new Date(start).getTime() / 1000);
    const endTs = Math.floor(new Date(end).getTime() / 1000);

    try {
      const res = await API.get('/api/settlement/bill/self/export', {
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
      Toast.error(t('导出失败'));
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

  const formatPrice = (v) =>
    v != null && !Number.isNaN(Number(v)) ? Number(v).toFixed(4) : '0';

  const billColumns = [
    {
      title: t('模型名称'),
      dataIndex: 'model_name',
      key: 'model_name',
      render: (text, record) => (
        <span>
          {text}
          {!record.configured && (
            <Tag color='grey' size='small' style={{ marginLeft: 8 }}>
              {t('未配置价格')}
            </Tag>
          )}
        </span>
      ),
    },
    {
      title: t('输入 Token'),
      dataIndex: 'input_tokens',
      key: 'input_tokens',
      render: formatNumber,
      align: 'right',
    },
    {
      title: t('输出 Token'),
      dataIndex: 'output_tokens',
      key: 'output_tokens',
      render: formatNumber,
      align: 'right',
    },
    {
      title: t('请求次数'),
      dataIndex: 'request_count',
      key: 'request_count',
      render: formatNumber,
      align: 'right',
    },
    {
      title: t('输入金额'),
      dataIndex: 'input_amount',
      key: 'input_amount',
      render: formatAmount,
      align: 'right',
    },
    {
      title: t('输出金额'),
      dataIndex: 'output_amount',
      key: 'output_amount',
      render: formatAmount,
      align: 'right',
    },
    {
      title: t('次数金额'),
      dataIndex: 'request_amount',
      key: 'request_amount',
      render: (val) =>
        val != null && Number(val) > 0 ? formatAmount(val) : '-',
      align: 'right',
    },
    {
      title: t('合计金额'),
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

  const pricingColumns = [
    {
      title: t('模型名称'),
      dataIndex: 'model_name',
      key: 'model_name',
    },
    {
      title: t('输入价格') + ' ($/1M tokens)',
      dataIndex: 'input_price',
      key: 'input_price',
      render: formatPrice,
      align: 'right',
    },
    {
      title: t('输出价格') + ' ($/1M tokens)',
      dataIndex: 'output_price',
      key: 'output_price',
      render: formatPrice,
      align: 'right',
    },
    {
      title: t('次数价格') + ' ($/次)',
      dataIndex: 'request_price',
      key: 'request_price',
      render: formatPrice,
      align: 'right',
    },
  ];

  return (
    <div className='mt-[60px] px-2'>
      <div style={{ padding: '24px', maxWidth: 1100, margin: '0 auto' }}>
        <Title heading={4} style={{ marginBottom: 16 }}>
          {t('账单查询')}
        </Title>

        <Tabs
          type='line'
          activeKey={activeTab}
          onChange={(key) => setActiveTab(key)}
        >
          <TabPane tab={t('账单查询')} itemKey='bill'>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                margin: '16px 0 20px',
                flexWrap: 'wrap',
              }}
            >
              <DatePicker
                type='dateTimeRange'
                density='compact'
                placeholder={[t('开始时间'), t('结束时间')]}
                value={dateRange}
                format='yyyy-MM-dd HH:mm:ss'
                timePickerOpts={{ showSeconds: true }}
                style={{ width: 460 }}
                onChange={(dates) => setDateRange(dates || [])}
              />
              <Button
                type='primary'
                icon={<IconSearch />}
                loading={loading}
                onClick={() => handleQuery()}
              >
                {t('查询')}
              </Button>
              <Input
                placeholder={t('模型关键词筛选')}
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
                {t('导出 CSV')}
              </Button>
            </div>

            {loading ? (
              <div style={{ textAlign: 'center', padding: '60px 0' }}>
                <Spin size='large' />
              </div>
            ) : billData ? (
              <>
                <Table
                  columns={billColumns}
                  dataSource={filteredItems}
                  rowKey='model_name'
                  pagination={false}
                  size='middle'
                  rowClassName={(record) =>
                    !record.configured ? 'unconfigured-row' : ''
                  }
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
                  <Text size='normal' style={{ marginRight: 8 }}>
                    {modelKeyword.trim() ? t('筛选合计：') : t('合计金额：')}
                  </Text>
                  <Text size='normal' strong style={{ fontSize: 18 }}>
                    {formatAmount(filteredTotal)}
                  </Text>
                </div>
              </>
            ) : (
              <Empty
                title={t('请选择时间范围查询账单')}
                description={t('选择开始和结束日期后点击查询按钮')}
                style={{ padding: '60px 0' }}
              />
            )}
          </TabPane>

          <TabPane tab={t('结算价格')} itemKey='pricing'>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                margin: '16px 0 20px',
                flexWrap: 'wrap',
              }}
            >
              <Input
                placeholder={t('模型关键词筛选')}
                prefix={<IconSearch />}
                value={pricingKeyword}
                onChange={(val) => setPricingKeyword(val)}
                showClear
                style={{ width: 200 }}
              />
              <Button
                icon={<IconRefresh />}
                loading={pricingLoading}
                onClick={loadPricing}
              >
                {t('刷新')}
              </Button>
            </div>

            {pricingLoading ? (
              <div style={{ textAlign: 'center', padding: '60px 0' }}>
                <Spin size='large' />
              </div>
            ) : pricingLoaded && pricingData.length === 0 ? (
              <Empty
                title={t('暂无结算价格配置')}
                description={t('当前账号尚未配置任何模型的结算价格')}
                style={{ padding: '60px 0' }}
              />
            ) : (
              <Table
                columns={pricingColumns}
                dataSource={filteredPricing}
                rowKey='id'
                pagination={false}
                size='middle'
              />
            )}
          </TabPane>
        </Tabs>

        <style>{`
          .unconfigured-row td {
            color: var(--semi-color-text-2) !important;
          }
        `}</style>
      </div>
    </div>
  );
};

export default Bill;
