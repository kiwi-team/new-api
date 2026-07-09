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

import React, {
  useEffect,
  useMemo,
  useRef,
  useState,
  useCallback,
} from 'react';
import {
  Card,
  Spin,
  Tabs,
  Table,
  Button,
  Modal,
  Form,
  Select,
  Space,
  Typography,
  Popconfirm,
  Input,
  Tag,
} from '@douyinfe/semi-ui';
import { IconSearch, IconRefresh, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import OfficialPriceTab from './OfficialPriceTab';

const { Title } = Typography;

// 系统基准：$2 / 1M tokens = 倍率 1，故 $/1M = ratio * 2
const ratioToUsdPer1M = (ratio) =>
  ratio === undefined || ratio === null || isNaN(ratio)
    ? null
    : parseFloat(ratio) * 2;

const fmtPrice = (v) =>
  v === null || v === undefined || isNaN(v) ? '-' : Number(v).toFixed(4);

/**
 * 客户折扣（按模型聚合）：纵览所有客户在各模型上的自定义折扣/结算价，并可维护。
 * props.inputs 为已加载的 option 集合（含 ModelRatio / CompletionRatio / ModelPrice），
 * 用于计算「官方价格」作对照。
 */
const CustomerDiscountTab = ({ inputs }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [allConfigs, setAllConfigs] = useState([]);
  const [keyword, setKeyword] = useState('');

  // 新增/编辑弹窗
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [formApi, setFormApi] = useState(null);
  const [submitLoading, setSubmitLoading] = useState(false);
  const initFormValues = {
    user_id: '',
    model_name: '',
    discount: 1,
    input_price: 0,
    output_price: 0,
    request_price: 0,
  };
  const [formValues, setFormValues] = useState({ ...initFormValues });

  // 用户远程搜索
  const [userOptions, setUserOptions] = useState([]);
  const [userSearchLoading, setUserSearchLoading] = useState(false);
  const searchTimerRef = useRef(null);

  const fetchUserOptions = useCallback(async (kw = '') => {
    setUserSearchLoading(true);
    try {
      const url = kw.trim()
        ? `/api/user/search?keyword=${encodeURIComponent(kw.trim())}&p=1&page_size=20`
        : `/api/user/?p=1&page_size=20`;
      const res = await API.get(url);
      const { success, data } = res.data;
      const items = success ? data?.items || (Array.isArray(data) ? data : []) : [];
      setUserOptions(
        items.map((u) => ({ value: u.id, label: `${u.username} (ID: ${u.id})` })),
      );
    } catch (e) {
      // 静默失败
    } finally {
      setUserSearchLoading(false);
    }
  }, []);

  const handleUserSearch = useCallback(
    (val) => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
      searchTimerRef.current = setTimeout(() => fetchUserOptions(val), 300);
    },
    [fetchUserOptions],
  );

  useEffect(() => {
    fetchUserOptions();
  }, [fetchUserOptions]);

  const fetchAll = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/settlement/config/all');
      const { success, message, data } = res.data;
      if (success) {
        setAllConfigs(Array.isArray(data) ? data : []);
      } else {
        showError(message || t('加载失败'));
      }
    } catch (e) {
      showError(t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 官方价格映射：模型名 -> {input, output, perCall}
  const officialPriceMap = useMemo(() => {
    const map = {};
    try {
      const modelRatio = JSON.parse(inputs?.ModelRatio || '{}');
      const completionRatio = JSON.parse(inputs?.CompletionRatio || '{}');
      const modelPrice = JSON.parse(inputs?.ModelPrice || '{}');
      const names = new Set([
        ...Object.keys(modelRatio),
        ...Object.keys(completionRatio),
        ...Object.keys(modelPrice),
      ]);
      names.forEach((name) => {
        const ratio = modelRatio[name];
        const comp = completionRatio[name];
        const perCall = modelPrice[name];
        const input = ratioToUsdPer1M(ratio);
        let output = null;
        if (ratio !== undefined && comp !== undefined) {
          output = parseFloat(ratio) * parseFloat(comp) * 2;
        } else if (ratio !== undefined) {
          output = input; // 未单独设置补全倍率时按与输入一致展示
        }
        map[name] = {
          input,
          output,
          perCall: perCall === undefined ? null : perCall,
        };
      });
    } catch (e) {
      // 忽略解析错误
    }
    return map;
  }, [inputs]);

  // 按模型聚合
  const groups = useMemo(() => {
    const m = new Map();
    allConfigs.forEach((c) => {
      if (!m.has(c.model_name)) m.set(c.model_name, []);
      m.get(c.model_name).push(c);
    });
    let arr = Array.from(m.entries()).map(([model_name, list]) => ({
      key: model_name,
      model_name,
      customerCount: list.length,
      official: officialPriceMap[model_name] || null,
      children: list,
    }));
    if (keyword.trim()) {
      const kw = keyword.trim().toLowerCase();
      arr = arr.filter((g) => g.model_name.toLowerCase().includes(kw));
    }
    return arr;
  }, [allConfigs, officialPriceMap, keyword]);

  const openCreate = (modelName = '') => {
    setEditing(null);
    setFormValues({ ...initFormValues, model_name: modelName });
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

  const handleCloseModal = () => {
    setModalVisible(false);
    setEditing(null);
    formApi && formApi.reset();
    setFormValues({ ...initFormValues });
  };

  useEffect(() => {
    if (modalVisible && formApi) {
      formApi.setValues(formValues);
    }
  }, [modalVisible, formValues, formApi]);

  const handleSubmit = async () => {
    setSubmitLoading(true);
    try {
      const payload = formApi ? formApi.getValues() : { ...formValues };
      payload.user_id = parseInt(payload.user_id, 10);
      payload.discount = parseFloat(payload.discount);
      if (!payload.discount || Number.isNaN(payload.discount)) payload.discount = 1;
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
        res = await API.put('/api/settlement/config/', { ...payload, id: editing.id });
      } else {
        res = await API.post('/api/settlement/config/', payload);
      }
      const { success, message } = res.data;
      if (success) {
        showSuccess(editing ? t('更新成功') : t('创建成功'));
        handleCloseModal();
        fetchAll();
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
        fetchAll();
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  // 外层：模型聚合表
  const outerColumns = [
    { title: t('模型名称'), dataIndex: 'model_name' },
    {
      title: t('官方输入价') + ' ($/1M)',
      dataIndex: 'official_input',
      align: 'right',
      render: (_, r) => fmtPrice(r.official?.input),
    },
    {
      title: t('官方输出价') + ' ($/1M)',
      dataIndex: 'official_output',
      align: 'right',
      render: (_, r) => fmtPrice(r.official?.output),
    },
    {
      title: t('客户数'),
      dataIndex: 'customerCount',
      align: 'right',
      render: (v) => <Tag color='blue'>{v}</Tag>,
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      align: 'right',
      render: (_, r) => (
        <Button
          size='small'
          icon={<IconPlus />}
          onClick={() => openCreate(r.model_name)}
        >
          {t('为该模型添加折扣')}
        </Button>
      ),
    },
  ];

  // 内层：某模型下的客户明细
  const innerColumns = [
    {
      title: t('用户'),
      dataIndex: 'username',
      render: (v, r) => `${v || '-'} (ID: ${r.user_id})`,
    },
    {
      title: t('模型折扣'),
      dataIndex: 'discount',
      align: 'right',
      render: (v) => (v != null ? Number(v).toFixed(2) : '1.00'),
    },
    {
      title: t('输入价格') + ' ($/1M)',
      dataIndex: 'input_price',
      align: 'right',
      render: (v) => (v != null ? Number(v).toFixed(4) : '0'),
    },
    {
      title: t('输出价格') + ' ($/1M)',
      dataIndex: 'output_price',
      align: 'right',
      render: (v) => (v != null ? Number(v).toFixed(4) : '0'),
    },
    {
      title: t('次数价格') + ' ($/次)',
      dataIndex: 'request_price',
      align: 'right',
      render: (v) => (v != null ? Number(v).toFixed(4) : '0'),
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      align: 'right',
      render: (_, record) => (
        <Space>
          <Button size='small' onClick={() => openEdit(record)}>
            {t('编辑')}
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
    <div className='py-2'>
      <div className='flex items-center gap-3 mb-3 flex-wrap'>
        <Input
          prefix={<IconSearch />}
          placeholder={t('模型关键词筛选')}
          value={keyword}
          onChange={(v) => setKeyword(v)}
          showClear
          style={{ width: 220 }}
        />
        <Button icon={<IconRefresh />} loading={loading} onClick={fetchAll}>
          {t('刷新')}
        </Button>
        <Button type='primary' icon={<IconPlus />} onClick={() => openCreate('')}>
          {t('添加客户折扣')}
        </Button>
      </div>

      <Table
        loading={loading}
        columns={outerColumns}
        dataSource={groups}
        rowKey='key'
        pagination={false}
        empty={t('暂无客户折扣配置')}
        expandedRowRender={(record) => (
          <Table
            columns={innerColumns}
            dataSource={record.children}
            rowKey='id'
            pagination={false}
            size='small'
          />
        )}
      />

      <Modal
        title={
          editing
            ? t('编辑') + ' ' + t('客户折扣')
            : t('添加') + ' ' + t('客户折扣')
        }
        visible={modalVisible}
        onCancel={handleCloseModal}
        onOk={handleSubmit}
        okButtonProps={{ loading: submitLoading }}
        centered
      >
        <Form getFormApi={setFormApi} initValues={formValues}>
          {editing ? (
            <Form.Input field='user_id' label={t('用户')} disabled />
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
    </div>
  );
};

const PricingCenter = () => {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState({
    ModelPrice: '',
    ModelRatio: '',
    CompletionRatio: '',
  });
  const [loading, setLoading] = useState(false);

  const getOptions = async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (success) {
      const newInputs = {};
      data.forEach((item) => {
        let value = item.value;
        if (
          typeof value === 'string' &&
          (value.startsWith('{') || value.startsWith('['))
        ) {
          try {
            value = JSON.stringify(JSON.parse(value), null, 2);
          } catch (e) {
            // 保持原样
          }
        }
        newInputs[item.key] = value;
      });
      setInputs(newInputs);
    } else {
      showError(message);
    }
  };

  const onRefresh = async () => {
    try {
      setLoading(true);
      await getOptions();
    } catch (e) {
      showError(t('刷新失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    onRefresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className='mt-[60px] px-2'>
      <div className='flex items-center justify-between mb-3'>
        <Title heading={4}>{t('价格中心')}</Title>
      </div>
      <Spin spinning={loading} size='large'>
        <Card>
          <Tabs type='card'>
            <Tabs.TabPane tab={t('官方价格')} itemKey='official'>
              <OfficialPriceTab inputs={inputs} refresh={onRefresh} />
            </Tabs.TabPane>
            <Tabs.TabPane tab={t('客户折扣')} itemKey='discount'>
              <CustomerDiscountTab inputs={inputs} />
            </Tabs.TabPane>
          </Tabs>
        </Card>
      </Spin>
    </div>
  );
};

export default PricingCenter;
