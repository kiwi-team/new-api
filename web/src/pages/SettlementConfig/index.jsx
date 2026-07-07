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

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { API, showError, showSuccess } from '../../helpers';
import {
  Button,
  Table,
  Modal,
  Form,
  Select,
  Space,
  Typography,
  Popconfirm,
  TextArea,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const { Title } = Typography;

const SettlementConfigPage = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState([]);
  const [searchUserId, setSearchUserId] = useState('');
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [formApi, setFormApi] = useState(null);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [batchModalVisible, setBatchModalVisible] = useState(false);
  const [batchJson, setBatchJson] = useState('');
  const [batchLoading, setBatchLoading] = useState(false);

  // User search state
  const [userOptions, setUserOptions] = useState([]);
  const [userSearchLoading, setUserSearchLoading] = useState(false);
  const searchTimerRef = useRef(null);

  const initFormValues = {
    user_id: '',
    model_name: '',
    discount: 1,
    input_price: 0,
    output_price: 0,
    request_price: 0,
  };
  const [formValues, setFormValues] = useState({ ...initFormValues });

  // Fetch user options via /api/user/search with debounce
  const fetchUserOptions = useCallback(async (keyword = '') => {
    setUserSearchLoading(true);
    try {
      const url = keyword.trim()
        ? `/api/user/search?keyword=${encodeURIComponent(keyword.trim())}&p=1&page_size=20`
        : `/api/user/?p=1&page_size=20`;
      const res = await API.get(url);
      const { success, data } = res.data;
      if (success && data?.items) {
        setUserOptions(
          data.items.map((u) => ({
            value: u.id,
            label: `${u.username} (ID: ${u.id})`,
          })),
        );
      } else if (success && Array.isArray(data)) {
        setUserOptions(
          data.map((u) => ({
            value: u.id,
            label: `${u.username} (ID: ${u.id})`,
          })),
        );
      }
    } catch (e) {
      // silently fail
    } finally {
      setUserSearchLoading(false);
    }
  }, []);

  const handleUserSearch = useCallback(
    (val) => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
      searchTimerRef.current = setTimeout(() => {
        fetchUserOptions(val);
      }, 300);
    },
    [fetchUserOptions],
  );

  // Load initial user options
  useEffect(() => {
    fetchUserOptions();
  }, [fetchUserOptions]);

  const fetchData = async (userId = searchUserId) => {
    if (!userId) {
      setData([]);
      return;
    }
    setLoading(true);
    try {
      const res = await API.get('/api/settlement/config/', {
        params: { user_id: userId },
      });
      const { success, message, data } = res.data;
      if (success) {
        setData(data || []);
      } else {
        showError(message || t('加载失败'));
      }
    } catch (e) {
      showError(t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  const openCreate = () => {
    setEditing(null);
    setFormValues({
      ...initFormValues,
      user_id: searchUserId || '',
    });
    setModalVisible(true);
  };

  const openEdit = (record) => {
    setEditing(record);
    setFormValues({
      user_id: record.user_id,
      model_name: record.model_name,
      discount: record.discount != null ? record.discount : 1,
      input_price: record.input_price,
      output_price: record.output_price,
      request_price: record.request_price,
    });
    setModalVisible(true);
  };

  const openCopy = (record) => {
    setEditing(null);
    setFormValues({
      user_id: record.user_id,
      model_name: record.model_name + '_copy',
      discount: record.discount != null ? record.discount : 1,
      input_price: record.input_price,
      output_price: record.output_price,
      request_price: record.request_price,
    });
    setModalVisible(true);
  };

  const handleCloseModal = () => {
    setModalVisible(false);
    setEditing(null);
    formApi && formApi.reset();
    setFormValues({ ...initFormValues });
  };

  const handleSubmit = async () => {
    setSubmitLoading(true);
    try {
      const payload = formApi ? formApi.getValues() : { ...formValues };
      // Ensure numeric types
      payload.user_id = parseInt(payload.user_id, 10);
      payload.discount = parseFloat(payload.discount);
      if (!payload.discount || Number.isNaN(payload.discount)) {
        payload.discount = 1;
      }
      if (payload.discount < 0.01 || payload.discount > 10) {
        showError(t('模型折扣必须在 0.01 到 10.00 之间'));
        setSubmitLoading(false);
        return;
      }
      payload.input_price = parseFloat(payload.input_price) || 0;
      payload.output_price = parseFloat(payload.output_price) || 0;
      payload.request_price = parseFloat(payload.request_price) || 0;

      let res;
      if (editing) {
        res = await API.put('/api/settlement/config/', {
          ...payload,
          id: editing.id,
        });
      } else {
        res = await API.post('/api/settlement/config/', payload);
      }
      const { success, message } = res.data;
      if (success) {
        showSuccess(editing ? t('更新成功') : t('创建成功'));
        handleCloseModal();
        fetchData(searchUserId);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    } finally {
      setSubmitLoading(false);
    }
  };

  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/settlement/config/${id}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('删除成功'));
        fetchData(searchUserId);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  const handleBatchImport = async () => {
    if (!batchJson.trim()) {
      showError(t('请输入JSON格式的配置数据'));
      return;
    }
    setBatchLoading(true);
    try {
      const parsed = JSON.parse(batchJson);
      const res = await API.post('/api/settlement/config/batch', parsed);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('导入成功'));
        setBatchModalVisible(false);
        setBatchJson('');
        if (parsed.user_id) {
          setSearchUserId(parsed.user_id);
          fetchData(parsed.user_id);
        }
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      if (e instanceof SyntaxError) {
        showError('JSON格式错误');
      } else {
        showError(t('操作失败'));
      }
    } finally {
      setBatchLoading(false);
    }
  };

  useEffect(() => {
    if (modalVisible && formApi) {
      formApi.setValues(formValues);
    }
  }, [modalVisible, formValues, formApi]);

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: t('用户ID'), dataIndex: 'user_id', width: 100 },
    { title: t('模型名称'), dataIndex: 'model_name', width: 250 },
    {
      title: t('模型折扣'),
      dataIndex: 'discount',
      width: 120,
      render: (v) => (v != null ? Number(v).toFixed(2) : '1.00'),
    },
    {
      title: t('输入价格') + ' ($/1M tokens)',
      dataIndex: 'input_price',
      width: 180,
      render: (v) => (v != null ? v.toFixed(4) : '0'),
    },
    {
      title: t('输出价格') + ' ($/1M tokens)',
      dataIndex: 'output_price',
      width: 180,
      render: (v) => (v != null ? v.toFixed(4) : '0'),
    },
    {
      title: t('次数价格') + ' ($/次)',
      dataIndex: 'request_price',
      width: 150,
      render: (v) => (v != null ? v.toFixed(4) : '0'),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      width: 180,
      render: (v) => (v ? new Date(v * 1000).toLocaleString() : '-'),
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      width: 220,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button size='small' onClick={() => openEdit(record)}>
            {t('编辑')}
          </Button>
          <Button size='small' onClick={() => openCopy(record)}>
            {t('复制')}
          </Button>
          <Popconfirm
            title={t('确认删除')}
            onConfirm={() => handleDelete(record.id)}
          >
            <Button size='small' type='danger'>
              {t('删除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className='mt-[60px] px-2'>
      <div className='flex items-center justify-between mb-3'>
        <Title heading={4}>{t('结算价格管理')}</Title>
        <Space>
          <Select
            placeholder={t('搜索用户')}
            style={{ width: 240 }}
            filter
            remote
            onSearch={handleUserSearch}
            loading={userSearchLoading}
            optionList={userOptions}
            value={searchUserId}
            onChange={(val) => {
              setSearchUserId(val);
              if (val) fetchData(val);
              else setData([]);
            }}
            showClear
            onClear={() => {
              setSearchUserId('');
              setData([]);
            }}
          />
          <Button type='primary' onClick={openCreate}>
            {t('添加')}
          </Button>
          <Button onClick={() => setBatchModalVisible(true)}>
            {t('批量导入')}
          </Button>
        </Space>
      </div>
      <Table
        loading={loading}
        columns={columns}
        dataSource={data}
        rowKey='id'
        pagination={false}
        empty={t('未配置结算价格')}
      />

      {/* Create/Edit Modal */}
      <Modal
        title={editing ? t('编辑') + ' ' + t('结算价格') : t('添加') + ' ' + t('结算价格')}
        visible={modalVisible}
        onCancel={handleCloseModal}
        onOk={handleSubmit}
        okButtonProps={{ loading: submitLoading }}
        centered
      >
        <Form getFormApi={setFormApi} initValues={formValues}>
          {editing ? (
            <Form.Input
              field='user_id'
              label={t('用户')}
              disabled
            />
          ) : (
            <Form.Select
              field='user_id'
              label={t('用户')}
              placeholder={t('搜索用户')}
              filter
              remote
              onSearch={handleUserSearch}
              loading={userSearchLoading}
              optionList={userOptions}
              rules={[{ required: true, message: t('请选择用户') }]}
              showClear
              style={{ width: '100%' }}
            />
          )}
          <Form.Input
            field='model_name'
            label={t('模型名称')}
            placeholder={t('模型名称')}
            rules={[{ required: true, message: t('模型名称') }]}
          />
          <Form.InputNumber
            field='discount'
            label={t('模型折扣')}
            min={0.01}
            max={10}
            step={0.01}
            placeholder='1'
            extraText={t('如 0.8 表示按 8 折计费；范围 0.01 ~ 10.00，1 为不打折')}
          />
          <Form.InputNumber
            field='input_price'
            label={t('输入价格') + ' ($/1M tokens)'}
            min={0}
            step={0.0001}
            placeholder='0'
          />
          <Form.InputNumber
            field='output_price'
            label={t('输出价格') + ' ($/1M tokens)'}
            min={0}
            step={0.0001}
            placeholder='0'
          />
          <Form.InputNumber
            field='request_price'
            label={t('次数价格') + ' ($/次)'}
            min={0}
            step={0.0001}
            placeholder='0'
          />
        </Form>
      </Modal>

      {/* Batch Import Modal */}
      <Modal
        title={t('批量导入')}
        visible={batchModalVisible}
        onCancel={() => {
          setBatchModalVisible(false);
          setBatchJson('');
        }}
        onOk={handleBatchImport}
        okButtonProps={{ loading: batchLoading }}
        centered
        width={600}
      >
        <TextArea
          placeholder={t('请输入JSON格式的配置数据')}
          value={batchJson}
          onChange={(v) => setBatchJson(v)}
          autosize={{ minRows: 10, maxRows: 20 }}
        />
        <div style={{ marginTop: 8, color: 'var(--semi-color-text-2)', fontSize: 12 }}>
          {`{
  "user_id": 1,
  "configs": [
    {"model_name": "gpt-4o*", "discount": 0.8, "input_price": 2.5, "output_price": 10.0, "request_price": 0},
    {"model_name": "gpt-image-1", "discount": 1, "input_price": 0, "output_price": 0, "request_price": 0.02}
  ]
}`}
        </div>
      </Modal>
    </div>
  );
};

export default SettlementConfigPage;
