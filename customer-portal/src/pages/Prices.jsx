import React, { useEffect, useState } from 'react';
import { Table, Typography, Toast, Spin, Empty } from '@douyinfe/semi-ui';
import { IconPriceTag } from '@douyinfe/semi-icons';
import api from '../utils/api';

const { Title, Text } = Typography;

export default function Prices() {
  const [loading, setLoading] = useState(true);
  const [configs, setConfigs] = useState([]);

  const fetchConfigs = async () => {
    setLoading(true);
    try {
      const res = await api.get('/api/settlement/config/self');
      const { success, data, message } = res.data;
      if (success) {
        setConfigs(Array.isArray(data) ? data : []);
      } else {
        Toast.error(message || '获取结算价格失败');
      }
    } catch (err) {
      Toast.error(err.response?.data?.message || '获取结算价格失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchConfigs();
  }, []);

  const columns = [
    {
      title: '模型名称',
      dataIndex: 'model_name',
      key: 'model_name',
    },
    {
      title: '输入价格（$/百万Token）',
      dataIndex: 'input_price',
      key: 'input_price',
      render: (val) => (val != null ? Number(val).toFixed(4) : '-'),
    },
    {
      title: '输出价格（$/百万Token）',
      dataIndex: 'output_price',
      key: 'output_price',
      render: (val) => (val != null ? Number(val).toFixed(4) : '-'),
    },
    {
      title: '次数价格（$/次）',
      dataIndex: 'request_price',
      key: 'request_price',
      render: (val) => (val != null && Number(val) > 0 ? Number(val).toFixed(4) : '-'),
    },
  ];

  return (
    <div style={{ padding: '24px', maxWidth: 960, margin: '0 auto' }}>
      <Title heading={4} style={{ marginBottom: 16 }}>
        我的结算价格
      </Title>
      {loading ? (
        <div style={{ textAlign: 'center', padding: '60px 0' }}>
          <Spin size="large" />
        </div>
      ) : configs.length === 0 ? (
        <Empty
          image={<IconPriceTag size="extra-large" style={{ color: 'var(--semi-color-text-2)' }} />}
          title="暂无结算价格配置"
          description="管理员尚未为您配置结算价格，请联系管理员。"
          style={{ padding: '60px 0' }}
        />
      ) : (
        <Table
          columns={columns}
          dataSource={configs}
          rowKey="id"
          pagination={false}
          size="middle"
        />
      )}
    </div>
  );
}
