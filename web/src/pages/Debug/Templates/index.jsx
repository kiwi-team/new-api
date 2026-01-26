/*
Copyright (C) 2025 QuantumNous
*/

import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Table,
  Button,
  Space,
  Input,
  Tag,
  Typography,
  Card,
  Avatar,
  Banner,
  Popconfirm,
  Modal,
  Form,
  Select,
  TextArea,
  Upload,
  Toast,
} from '@douyinfe/semi-ui';
import {
  IconSearch,
  IconPlus,
  IconEdit,
  IconDelete,
  IconRefresh,
  IconCopy,
  IconUpload,
  IconDownload,
} from '@douyinfe/semi-icons';
import { FileText, Play, Zap } from 'lucide-react';
import { API, showError, showSuccess } from '../../../helpers';

const { Text, Title } = Typography;

// 厂商选项
const VENDOR_OPTIONS = [
  { value: 'openai', label: 'OpenAI', color: 'green' },
  { value: 'claude', label: 'Claude', color: 'orange' },
  { value: 'gemini', label: 'Gemini', color: 'blue' },
  { value: 'other', label: '其他', color: 'grey' },
];

// 分类选项
const CATEGORY_OPTIONS = [
  { value: 'basic', label: '基础对话', color: 'cyan' },
  { value: 'multimodal', label: '多模态', color: 'purple' },
  { value: 'tools', label: 'Tools调用', color: 'yellow' },
  { value: 'thinking', label: 'Thinking', color: 'pink' },
  { value: 'multi_turn', label: '多轮对话', color: 'indigo' },
];

const DebugTemplates = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [templates, setTemplates] = useState([]);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [filterVendor, setFilterVendor] = useState('');
  const [filterCategory, setFilterCategory] = useState('');
  const [pagination, setPagination] = useState({
    currentPage: 1,
    pageSize: 20,
    total: 0,
  });
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingTemplate, setEditingTemplate] = useState(null);
  const [formLoading, setFormLoading] = useState(false);

  // 加载模板列表
  const loadTemplates = async (page = 1, keyword = '') => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: page,
        page_size: pagination.pageSize,
      });
      
      if (keyword) params.append('keyword', keyword);
      if (filterVendor) params.append('vendor', filterVendor);
      if (filterCategory) params.append('category', filterCategory);
      
      const res = await API.get(`/api/debug/test-data?${params.toString()}`);
      const { success, message, data } = res.data;
      
      if (success) {
        setTemplates(data?.list || []);
        setPagination({
          ...pagination,
          currentPage: page,
          total: data?.total || 0,
        });
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('加载测试数据列表失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTemplates();
  }, [filterVendor, filterCategory]);

  const handleSearch = () => loadTemplates(1, searchKeyword);
  const handleRefresh = () => {
    setSearchKeyword('');
    setFilterVendor('');
    setFilterCategory('');
    loadTemplates(1, '');
  };

  const handleAdd = () => {
    setEditingTemplate(null);
    setEditModalVisible(true);
  };

  const handleEdit = (template) => {
    setEditingTemplate(template);
    setEditModalVisible(true);
  };

  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/debug/test-data/${id}`);
      if (res.data.success) {
        showSuccess(t('删除成功'));
        loadTemplates(pagination.currentPage, searchKeyword);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('删除失败'));
    }
  };

  // 快速调试 - 跳转到调试器并填充数据
  const handleQuickDebug = (template) => {
    // 将模板数据存储到 sessionStorage，调试器页面读取
    sessionStorage.setItem('debug_template_data', JSON.stringify({
      vendor: template.vendor,
      request_body: template.request_body,
      name: template.name,
    }));
    navigate('/console/debug/executor');
  };

  // 导出测试数据
  const handleExport = async () => {
    try {
      const res = await API.get('/api/debug/test-data/export');
      if (res.data.success) {
        const blob = new Blob([JSON.stringify(res.data.data, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `debug-test-data-${new Date().toISOString().slice(0, 10)}.json`;
        a.click();
        URL.revokeObjectURL(url);
        showSuccess('导出成功');
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError('导出失败');
    }
  };

  // 导入测试数据
  const handleImport = async (file) => {
    try {
      const text = await file.text();
      const data = JSON.parse(text);
      
      const res = await API.post('/api/debug/test-data/import', { data });
      if (res.data.success) {
        showSuccess(`导入成功，共导入 ${res.data.data?.count || 0} 条数据`);
        loadTemplates(1, '');
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError('导入失败，请检查文件格式');
    }
    return false; // 阻止默认上传
  };

  // 保存模板
  const handleSave = async (values) => {
    setFormLoading(true);
    try {
      const payload = {
        ...values,
        tags: values.tags ? values.tags.join(',') : '',
      };

      let res;
      if (editingTemplate) {
        res = await API.put(`/api/debug/test-data/${editingTemplate.id}`, payload);
      } else {
        res = await API.post('/api/debug/test-data', payload);
      }

      if (res.data.success) {
        showSuccess(editingTemplate ? t('更新成功') : t('创建成功'));
        setEditModalVisible(false);
        loadTemplates(pagination.currentPage, searchKeyword);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(editingTemplate ? t('更新失败') : t('创建失败'));
    } finally {
      setFormLoading(false);
    }
  };

  // 表格列定义
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 70,
    },
    {
      title: t('名称'),
      dataIndex: 'name',
      render: (text, record) => (
        <div>
          <Text strong>{text}</Text>
          {record.description && (
            <div>
              <Text type="tertiary" size="small" ellipsis={{ showTooltip: true }} style={{ maxWidth: 200 }}>
                {record.description}
              </Text>
            </div>
          )}
        </div>
      ),
    },
    {
      title: t('厂商'),
      dataIndex: 'vendor',
      width: 100,
      render: (vendor) => {
        const option = VENDOR_OPTIONS.find(v => v.value === vendor);
        return option ? (
          <Tag color={option.color}>{option.label}</Tag>
        ) : (
          <Tag>{vendor}</Tag>
        );
      },
    },
    {
      title: t('分类'),
      dataIndex: 'category',
      width: 100,
      render: (category) => {
        const option = CATEGORY_OPTIONS.find(c => c.value === category);
        return option ? (
          <Tag color={option.color}>{option.label}</Tag>
        ) : (
          <Tag>{category}</Tag>
        );
      },
    },
    {
      title: t('标签'),
      dataIndex: 'tags',
      width: 150,
      render: (tags) => {
        if (!tags) return '-';
        const tagList = tags.split(',').filter(t => t.trim());
        return (
          <Space wrap>
            {tagList.slice(0, 3).map((tag, index) => (
              <Tag key={index} size="small">{tag}</Tag>
            ))}
            {tagList.length > 3 && <Tag size="small">+{tagList.length - 3}</Tag>}
          </Space>
        );
      },
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      width: 160,
      render: (time) => time ? new Date(time).toLocaleString() : '-',
    },
    {
      title: t('操作'),
      dataIndex: 'actions',
      width: 250,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button
            theme="solid"
            type="primary"
            size="small"
            icon={<Play size={12} />}
            onClick={() => handleQuickDebug(record)}
          >
            {t('调试')}
          </Button>
          <Button
            theme="borderless"
            type="tertiary"
            size="small"
            icon={<IconEdit />}
            onClick={() => handleEdit(record)}
          />
          <Popconfirm
            title={t('确定删除此测试数据吗？')}
            onConfirm={() => handleDelete(record.id)}
          >
            <Button
              theme="borderless"
              type="danger"
              size="small"
              icon={<IconDelete />}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const getInitialValues = () => {
    if (editingTemplate) {
      return {
        name: editingTemplate.name || '',
        description: editingTemplate.description || '',
        vendor: editingTemplate.vendor || 'openai',
        category: editingTemplate.category || 'basic',
        request_body: editingTemplate.request_body || '',
        expected_response: editingTemplate.expected_response || '',
        tags: editingTemplate.tags ? editingTemplate.tags.split(',').filter(t => t.trim()) : [],
      };
    }
    return {
      vendor: 'openai',
      category: 'basic',
    };
  };

  return (
    <div className="mt-[60px] px-4">
      <Card className="!rounded-2xl shadow-sm border-0 mb-6">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center">
            <Avatar size="large" color="blue" className="mr-3 shadow-md">
              <FileText size={24} />
            </Avatar>
            <div>
              <Title heading={3} className="m-0">{t('测试数据管理')}</Title>
              <Text type="tertiary" size="small">
                {t('管理调试测试数据，支持按厂商和功能分类')}
              </Text>
            </div>
          </div>
          <Space>
            <Upload
              accept=".json"
              showUploadList={false}
              beforeUpload={handleImport}
            >
              <Button icon={<IconUpload />}>{t('导入')}</Button>
            </Upload>
            <Button icon={<IconDownload />} onClick={handleExport}>{t('导出')}</Button>
          </Space>
        </div>

        {/* Banner */}
        <Banner
          type="info"
          description={t('测试数据用于保存常用的请求体，支持按厂商（OpenAI/Claude/Gemini）和功能（多模态/Tools/Thinking）分类。')}
          className="!rounded-lg mb-4"
        />

        {/* Toolbar */}
        <div className="mb-4">
          <div className="flex items-center justify-between flex-wrap gap-2">
            <Space wrap>
              <Input
                prefix={<IconSearch />}
                placeholder={t('搜索名称或描述')}
                value={searchKeyword}
                onChange={setSearchKeyword}
                onEnterPress={handleSearch}
                style={{ width: 200 }}
                showClear
              />
              <Select
                placeholder={t('厂商')}
                value={filterVendor}
                onChange={setFilterVendor}
                style={{ width: 120 }}
                showClear
              >
                {VENDOR_OPTIONS.map(v => (
                  <Select.Option key={v.value} value={v.value}>{v.label}</Select.Option>
                ))}
              </Select>
              <Select
                placeholder={t('分类')}
                value={filterCategory}
                onChange={setFilterCategory}
                style={{ width: 120 }}
                showClear
              >
                {CATEGORY_OPTIONS.map(c => (
                  <Select.Option key={c.value} value={c.value}>{c.label}</Select.Option>
                ))}
              </Select>
              <Button type="primary" onClick={handleSearch} icon={<IconSearch />}>
                {t('搜索')}
              </Button>
              <Button onClick={handleRefresh} icon={<IconRefresh />}>
                {t('重置')}
              </Button>
            </Space>
            <Button theme="solid" type="primary" icon={<IconPlus />} onClick={handleAdd}>
              {t('新建测试数据')}
            </Button>
          </div>
        </div>

        {/* Table */}
        <Table
          columns={columns}
          dataSource={templates}
          loading={loading}
          pagination={{
            currentPage: pagination.currentPage,
            pageSize: pagination.pageSize,
            total: pagination.total,
            onPageChange: (page) => loadTemplates(page, searchKeyword),
          }}
          rowKey="id"
          scroll={{ x: 1000 }}
        />
      </Card>

      {/* Edit Modal */}
      <Modal
        title={editingTemplate ? t('编辑测试数据') : t('新建测试数据')}
        visible={editModalVisible}
        onCancel={() => setEditModalVisible(false)}
        footer={null}
        width={700}
      >
        <Form
          key={editingTemplate?.id || 'new'}
          initValues={getInitialValues()}
          onSubmit={handleSave}
          labelPosition="left"
          labelWidth={100}
        >
          <Form.Input
            field="name"
            label={t('名称')}
            placeholder={t('请输入名称')}
            rules={[{ required: true, message: t('请输入名称') }]}
          />
          
          <Form.TextArea
            field="description"
            label={t('描述')}
            placeholder={t('请输入描述')}
            rows={2}
          />

          <Form.Select
            field="vendor"
            label={t('厂商')}
            style={{ width: '100%' }}
            rules={[{ required: true, message: t('请选择厂商') }]}
          >
            {VENDOR_OPTIONS.map(v => (
              <Select.Option key={v.value} value={v.value}>{v.label}</Select.Option>
            ))}
          </Form.Select>

          <Form.Select
            field="category"
            label={t('分类')}
            style={{ width: '100%' }}
            rules={[{ required: true, message: t('请选择分类') }]}
          >
            {CATEGORY_OPTIONS.map(c => (
              <Select.Option key={c.value} value={c.value}>{c.label}</Select.Option>
            ))}
          </Form.Select>

          <Form.TextArea
            field="request_body"
            label={t('请求体')}
            placeholder={t('JSON 格式的请求体')}
            rows={10}
            style={{ fontFamily: 'monospace' }}
            rules={[{ required: true, message: t('请输入请求体') }]}
          />

          <Form.TextArea
            field="expected_response"
            label={t('预期响应')}
            placeholder={t('可选，用于验证响应')}
            rows={4}
            style={{ fontFamily: 'monospace' }}
          />

          <Form.TagInput
            field="tags"
            label={t('标签')}
            placeholder={t('输入标签后按回车')}
          />

          <div className="flex justify-end mt-4">
            <Space>
              <Button onClick={() => setEditModalVisible(false)}>{t('取消')}</Button>
              <Button theme="solid" type="primary" htmlType="submit" loading={formLoading}>
                {t('保存')}
              </Button>
            </Space>
          </div>
        </Form>
      </Modal>
    </div>
  );
};

export default DebugTemplates;
