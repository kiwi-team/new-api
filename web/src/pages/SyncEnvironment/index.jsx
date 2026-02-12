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

import React, { useState, useEffect } from 'react';
import {
  Table,
  Button,
  Modal,
  Form,
  Input,
  Switch,
  Tag,
  Space,
  Popconfirm,
  Tabs,
  TabPane,
  Toast,
  Card,
  Typography,
  Spin,
  Select,
  Checkbox,
  Banner,
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconEdit,
  IconDelete,
  IconRefresh,
  IconLink,
  IconSync,
  IconHistory,
} from '@douyinfe/semi-icons';
import { API } from '../../helpers/api';

const { Text, Title } = Typography;

const SyncEnvironmentPage = () => {
  // 环境列表
  const [environments, setEnvironments] = useState([]);
  const [loading, setLoading] = useState(false);
  
  // 环境编辑弹窗
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingEnv, setEditingEnv] = useState(null);
  const [submitLoading, setSubmitLoading] = useState(false);
  
  // 同步历史
  const [syncLogs, setSyncLogs] = useState([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsTotal, setLogsTotal] = useState(0);
  const [logsPage, setLogsPage] = useState(1);
  const [logsPageSize, setLogsPageSize] = useState(20);
  
  // 当前Tab
  const [activeTab, setActiveTab] = useState('environments');

  // 加载环境列表
  const loadEnvironments = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/sync/environments');
      if (res.data.success) {
        setEnvironments(res.data.data || []);
      } else {
        Toast.error(res.data.message || '加载失败');
      }
    } catch (error) {
      Toast.error('加载环境列表失败');
    } finally {
      setLoading(false);
    }
  };

  // 加载同步历史
  const loadSyncLogs = async () => {
    setLogsLoading(true);
    try {
      const res = await API.get('/api/sync/logs', {
        params: {
          page: logsPage,
          page_size: logsPageSize,
        },
      });
      if (res.data.success) {
        setSyncLogs(res.data.data || []);
        setLogsTotal(res.data.total || 0);
      }
    } catch (error) {
      Toast.error('加载同步历史失败');
    } finally {
      setLogsLoading(false);
    }
  };

  useEffect(() => {
    loadEnvironments();
  }, []);

  useEffect(() => {
    if (activeTab === 'logs') {
      loadSyncLogs();
    }
  }, [activeTab, logsPage, logsPageSize]);

  // 打开编辑弹窗
  const openEditModal = (env = null) => {
    setEditingEnv(env);
    setEditModalVisible(true);
  };

  // 关闭编辑弹窗
  const closeEditModal = () => {
    setEditingEnv(null);
    setEditModalVisible(false);
  };

  // 提交环境配置
  const handleSubmit = async (values) => {
    setSubmitLoading(true);
    try {
      let res;
      if (editingEnv) {
        res = await API.put(`/api/sync/environments/${editingEnv.id}`, values);
      } else {
        res = await API.post('/api/sync/environments', values);
      }
      
      if (res.data.success) {
        Toast.success(editingEnv ? '更新成功' : '添加成功');
        closeEditModal();
        loadEnvironments();
      } else {
        Toast.error(res.data.message || '操作失败');
      }
    } catch (error) {
      Toast.error('操作失败');
    } finally {
      setSubmitLoading(false);
    }
  };

  // 删除环境
  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/sync/environments/${id}`);
      if (res.data.success) {
        Toast.success('删除成功');
        loadEnvironments();
      } else {
        Toast.error(res.data.message || '删除失败');
      }
    } catch (error) {
      Toast.error('删除失败');
    }
  };

  // 测试连接
  const handleTestConnection = async (id) => {
    try {
      const res = await API.post(`/api/sync/environments/${id}/test`);
      if (res.data.success) {
        Toast.success('连接成功');
      } else {
        Toast.error(res.data.message || '连接失败');
      }
    } catch (error) {
      Toast.error('连接测试失败');
    }
  };

  // 环境表格列定义
  const envColumns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: '环境名称',
      dataIndex: 'name',
      width: 150,
    },
    {
      title: 'API地址',
      dataIndex: 'api_url',
      width: 250,
      render: (text) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 230 }}>
          {text}
        </Text>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (status) => (
        <Tag color={status === 1 ? 'green' : 'grey'}>
          {status === 1 ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '备注',
      dataIndex: 'remark',
      width: 200,
      render: (text) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 180 }}>
          {text || '-'}
        </Text>
      ),
    },
    {
      title: '操作',
      width: 250,
      render: (_, record) => (
        <Space>
          <Button
            icon={<IconLink />}
            size="small"
            onClick={() => handleTestConnection(record.id)}
          >
            测试
          </Button>
          <Button
            icon={<IconEdit />}
            size="small"
            onClick={() => openEditModal(record)}
          >
            编辑
          </Button>
          <Popconfirm
            title="确定删除该环境吗？"
            onConfirm={() => handleDelete(record.id)}
          >
            <Button icon={<IconDelete />} size="small" type="danger">
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  // 同步历史表格列定义
  const logColumns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: '同步类型',
      dataIndex: 'sync_type',
      width: 120,
      render: (type) => (
        <Tag color={type === 'channel' ? 'blue' : 'purple'}>
          {type === 'channel' ? '渠道' : '模型价格'}
        </Tag>
      ),
    },
    {
      title: '目标环境',
      dataIndex: 'environment_name',
      width: 150,
    },
    {
      title: '数据摘要',
      dataIndex: 'data_summary',
      width: 300,
      render: (text) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 280 }}>
          {text}
        </Text>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (status) => (
        <Tag color={status === 1 ? 'green' : 'red'}>
          {status === 1 ? '成功' : '失败'}
        </Tag>
      ),
    },
    {
      title: '错误信息',
      dataIndex: 'error_message',
      width: 200,
      render: (text) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 180 }}>
          {text || '-'}
        </Text>
      ),
    },
    {
      title: '同步时间',
      dataIndex: 'created_time',
      width: 180,
      render: (time) => new Date(time * 1000).toLocaleString(),
    },
  ];

  return (
    <div className="mt-[60px] px-4">
      <Card>
        <Tabs activeKey={activeTab} onChange={setActiveTab}>
          <TabPane tab="环境管理" itemKey="environments">
            <div style={{ marginBottom: 16 }}>
              <Space>
                <Button
                  icon={<IconPlus />}
                  type="primary"
                  onClick={() => openEditModal()}
                >
                  添加环境
                </Button>
                <Button icon={<IconRefresh />} onClick={loadEnvironments}>
                  刷新
                </Button>
              </Space>
            </div>
            <Table
              columns={envColumns}
              dataSource={environments}
              loading={loading}
              rowKey="id"
              pagination={false}
            />
          </TabPane>
          
          <TabPane tab="同步历史" itemKey="logs">
            <div style={{ marginBottom: 16 }}>
              <Button icon={<IconRefresh />} onClick={loadSyncLogs}>
                刷新
              </Button>
            </div>
            <Table
              columns={logColumns}
              dataSource={syncLogs}
              loading={logsLoading}
              rowKey="id"
              pagination={{
                currentPage: logsPage,
                pageSize: logsPageSize,
                total: logsTotal,
                onPageChange: setLogsPage,
                onPageSizeChange: setLogsPageSize,
              }}
            />
          </TabPane>
        </Tabs>
      </Card>

      {/* 编辑环境弹窗 */}
      <Modal
        title={editingEnv ? '编辑环境' : '添加环境'}
        visible={editModalVisible}
        onCancel={closeEditModal}
        footer={null}
        width={500}
      >
        <Form
          onSubmit={handleSubmit}
          initValues={editingEnv || { status: 1 }}
          labelPosition="left"
          labelWidth={120}
        >
          <Form.Input
            field="name"
            label="环境名称"
            placeholder="请输入环境名称"
            rules={[{ required: true, message: '请输入环境名称' }]}
          />
          <Form.Input
            field="api_url"
            label="API地址"
            placeholder="https://api.example.com"
            rules={[{ required: true, message: '请输入API地址' }]}
          />
          <Form.Input
            field="root_token"
            label="Root Token"
            placeholder={editingEnv ? '留空则保持原有Token' : '请输入Root Token'}
            rules={editingEnv ? [] : [{ required: true, message: '请输入Root Token' }]}
            mode="password"
          />
          <Form.Input
            field="new_api_user"
            label="new-api-user"
            placeholder="请输入new-api-user Header值"
            rules={[{ required: true, message: '请输入new-api-user' }]}
          />
          <Form.Switch
            field="status"
            label="启用状态"
            checkedText="启用"
            uncheckedText="禁用"
          />
          <Form.TextArea
            field="remark"
            label="备注"
            placeholder="可选备注信息"
            rows={2}
          />
          <div style={{ textAlign: 'right', marginTop: 16 }}>
            <Space>
              <Button onClick={closeEditModal}>取消</Button>
              <Button type="primary" htmlType="submit" loading={submitLoading}>
                {editingEnv ? '更新' : '添加'}
              </Button>
            </Space>
          </div>
        </Form>
      </Modal>
    </div>
  );
};

export default SyncEnvironmentPage;
