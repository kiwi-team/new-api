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

import React, { useEffect, useState, useRef } from 'react';
import {
  Table,
  Button,
  Input,
  Modal,
  Form,
  Space,
  InputNumber,
  Typography,
  Tag,
  Popconfirm,
} from '@douyinfe/semi-ui';
import {
  IconDelete,
  IconPlus,
  IconSave,
  IconEdit,
  IconSearch,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function TieredPriceVisualEditor({ value, onChange, onSave, loading: externalLoading }) {
  const { t } = useTranslation();
  const [models, setModels] = useState([]);
  const [visible, setVisible] = useState(false);
  const [tierModalVisible, setTierModalVisible] = useState(false);
  const [editingModel, setEditingModel] = useState(null);
  const [editingTierIndex, setEditingTierIndex] = useState(-1);
  const [searchText, setSearchText] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const formRef = useRef(null);
  const tierFormRef = useRef(null);
  const pageSize = 10;

  // Parse JSON string to model array
  useEffect(() => {
    try {
      const parsed = JSON.parse(value || '{}');
      const modelData = Object.entries(parsed).map(([name, tiers]) => ({
        name,
        tiers: Array.isArray(tiers) ? tiers : [],
      }));
      setModels(modelData);
    } catch {
      setModels([]);
    }
  }, [value]);

  // Sync models back to JSON string
  const syncToJson = (updatedModels) => {
    const obj = {};
    updatedModels.forEach(({ name, tiers }) => {
      if (tiers.length > 0) {
        obj[name] = tiers.sort((a, b) => a.max_tokens - b.max_tokens);
      }
    });
    const json = JSON.stringify(obj, null, 2);
    onChange?.(json);
  };

  // Add new model
  const handleAddModel = (values) => {
    const { name } = values;
    if (models.some((m) => m.name === name)) {
      return showError(t('模型已存在'));
    }
    const updated = [...models, { name, tiers: [] }];
    setModels(updated);
    syncToJson(updated);
    setVisible(false);
  };

  // Delete model
  const handleDeleteModel = (name) => {
    const updated = models.filter((m) => m.name !== name);
    setModels(updated);
    syncToJson(updated);
  };

  // Open tier editor for a model
  const openTierEditor = (modelName, tierIndex = -1) => {
    setEditingModel(modelName);
    setEditingTierIndex(tierIndex);
    setTierModalVisible(true);

    const model = models.find((m) => m.name === modelName);
    if (tierIndex >= 0 && model?.tiers[tierIndex]) {
      const tier = model.tiers[tierIndex];
      setTimeout(() => {
        tierFormRef.current?.setValues({
          max_tokens: tier.max_tokens,
          input_price: tier.input_price,
          output_price: tier.output_price,
        });
      }, 0);
    }
  };

  // Save tier
  const handleSaveTier = (values) => {
    const { max_tokens, input_price, output_price } = values;
    const updated = models.map((m) => {
      if (m.name !== editingModel) return m;
      const newTiers = [...m.tiers];
      const tier = { max_tokens, input_price, output_price };

      if (editingTierIndex >= 0) {
        newTiers[editingTierIndex] = tier;
      } else {
        // Check duplicate max_tokens
        if (newTiers.some((t) => t.max_tokens === max_tokens)) {
          showError(t('该阈值已存在'));
          return m;
        }
        newTiers.push(tier);
      }
      newTiers.sort((a, b) => a.max_tokens - b.max_tokens);
      return { ...m, tiers: newTiers };
    });
    setModels(updated);
    syncToJson(updated);
    setTierModalVisible(false);
  };

  // Delete tier
  const handleDeleteTier = (modelName, tierIndex) => {
    const updated = models.map((m) => {
      if (m.name !== modelName) return m;
      const newTiers = m.tiers.filter((_, i) => i !== tierIndex);
      return { ...m, tiers: newTiers };
    });
    setModels(updated);
    syncToJson(updated);
  };

  const formatTokens = (n) => {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(n % 1_000 === 0 ? 0 : 1)}K`;
    return String(n);
  };

  const filteredModels = models.filter((m) =>
    searchText ? m.name.toLowerCase().includes(searchText.toLowerCase()) : true,
  );

  const pagedData = filteredModels.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  const columns = [
    {
      title: t('模型名称'),
      dataIndex: 'name',
      key: 'name',
      width: 220,
      render: (text) => (
        <Typography.Text copyable>{text}</Typography.Text>
      ),
    },
    {
      title: t('价格档位'),
      dataIndex: 'tiers',
      key: 'tiers',
      render: (tiers, record) => (
        <Space wrap>
          {tiers.map((tier, idx) => (
            <Tag
              key={idx}
              color='blue'
              closable
              onClose={() => handleDeleteTier(record.name, idx)}
              onClick={() => openTierEditor(record.name, idx)}
              style={{ cursor: 'pointer' }}
            >
              ≤{formatTokens(tier.max_tokens)}: ${tier.input_price}/${tier.output_price}
            </Tag>
          ))}
          <Button
            icon={<IconPlus />}
            size='small'
            theme='borderless'
            onClick={() => openTierEditor(record.name)}
          />
        </Space>
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 80,
      render: (_, record) => (
        <Popconfirm
          title={t('确定删除该模型的阶梯价格？')}
          onConfirm={() => handleDeleteModel(record.name)}
        >
          <Button icon={<IconDelete />} type='danger' size='small' />
        </Popconfirm>
      ),
    },
  ];

  return (
    <>
      <Space vertical align='start' style={{ width: '100%' }}>
        <Typography.Text type='secondary' style={{ marginBottom: 8 }}>
          {t('可视化编辑模型阶梯价格，每个档位的价格单位为 $/1M tokens')}
        </Typography.Text>
        <Space className='mt-2'>
          <Button icon={<IconPlus />} onClick={() => setVisible(true)}>
            {t('添加模型')}
          </Button>
          {onSave && (
            <Button
              type='primary'
              icon={<IconSave />}
              onClick={onSave}
              loading={externalLoading}
            >
              {t('保存')}
            </Button>
          )}
          <Input
            prefix={<IconSearch />}
            placeholder={t('搜索模型名称')}
            value={searchText}
            onChange={(v) => { setSearchText(v); setCurrentPage(1); }}
            style={{ width: 200 }}
            showClear
          />
        </Space>
        <Table
          columns={columns}
          dataSource={pagedData}
          rowKey='name'
          pagination={{
            currentPage,
            pageSize,
            total: filteredModels.length,
            onPageChange: (page) => setCurrentPage(page),
            showTotal: true,
            showSizeChanger: false,
          }}
          empty={
            <Typography.Text type='tertiary'>
              {t('暂无阶梯价格配置，点击"添加模型"开始')}
            </Typography.Text>
          }
        />
      </Space>

      {/* Add model modal */}
      <Modal
        title={t('添加模型')}
        visible={visible}
        onCancel={() => setVisible(false)}
        onOk={() => {
          formRef.current?.validate().then((values) => {
            handleAddModel(values);
          }).catch(() => showError(t('请检查输入')));
        }}
        width={400}
      >
        <Form getFormApi={(api) => (formRef.current = api)}>
          <Form.Input
            field='name'
            label={t('模型名称')}
            placeholder={t('例如 qwen-long 或 qwen*（支持通配符）')}
            rules={[{ required: true, message: t('请输入模型名称') }]}
          />
        </Form>
      </Modal>

      {/* Add/edit tier modal */}
      <Modal
        title={editingTierIndex >= 0 ? t('编辑价格档位') : t('添加价格档位')}
        visible={tierModalVisible}
        onCancel={() => setTierModalVisible(false)}
        onOk={() => {
          tierFormRef.current?.validate().then((values) => {
            handleSaveTier(values);
          }).catch(() => showError(t('请检查输入')));
        }}
        width={450}
      >
        <Form getFormApi={(api) => (tierFormRef.current = api)}>
          <Form.InputNumber
            field='max_tokens'
            label={t('输入 Token 上限')}
            placeholder='128000'
            min={1}
            style={{ width: '100%' }}
            rules={[{ required: true, message: t('请输入 Token 上限') }]}
            suffix='tokens'
          />
          <Form.InputNumber
            field='input_price'
            label={t('输入价格（$/1M tokens）')}
            placeholder='0.5'
            min={0}
            step={0.01}
            style={{ width: '100%' }}
            rules={[{ required: true, message: t('请输入输入价格') }]}
          />
          <Form.InputNumber
            field='output_price'
            label={t('输出价格（$/1M tokens）')}
            placeholder='2.0'
            min={0}
            step={0.01}
            style={{ width: '100%' }}
            rules={[{ required: true, message: t('请输入输出价格') }]}
          />
        </Form>
      </Modal>
    </>
  );
}
