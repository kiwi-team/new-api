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
  Table,
  Button,
  Input,
  Select,
  Space,
  Typography,
  Popconfirm,
  Tag,
} from '@douyinfe/semi-ui';
import { IconSearch, IconRefresh, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const DEFAULT_RATE = 7.3;
const round6 = (n) => Math.round((Number(n) + Number.EPSILON) * 1e6) / 1e6;

/**
 * 双币种（美金/人民币）行内编辑单元：修改任一按汇率换算另一个，失焦/回车即提交。
 */
const DualPriceCell = ({ value, rate, disabled, onCommit }) => {
  const [usdStr, setUsdStr] = useState('');
  const [cnyStr, setCnyStr] = useState('');

  useEffect(() => {
    if (value === null || value === undefined || value === '') {
      setUsdStr('');
      setCnyStr('');
    } else {
      setUsdStr(String(value));
      setCnyStr(String(round6(value * rate)));
    }
  }, [value, rate]);

  if (disabled) {
    return <Typography.Text type='tertiary'>-</Typography.Text>;
  }

  const commitFromUsd = () => {
    const usd = usdStr === '' ? 0 : parseFloat(usdStr);
    const clean = isNaN(usd) ? 0 : usd;
    setCnyStr(String(round6(clean * rate)));
    if (round6(clean) !== round6(value || 0)) onCommit(round6(clean));
  };
  const commitFromCny = () => {
    const cny = cnyStr === '' ? 0 : parseFloat(cnyStr);
    const usd = (isNaN(cny) ? 0 : cny) / rate;
    setUsdStr(String(round6(usd)));
    if (round6(usd) !== round6(value || 0)) onCommit(round6(usd));
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      <Input
        size='small'
        prefix='$'
        value={usdStr}
        onChange={setUsdStr}
        onBlur={commitFromUsd}
        onEnterPress={commitFromUsd}
        style={{ width: 150 }}
      />
      <Input
        size='small'
        prefix='¥'
        value={cnyStr}
        onChange={setCnyStr}
        onBlur={commitFromCny}
        onEnterPress={commitFromCny}
        style={{ width: 150 }}
      />
    </div>
  );
};

/**
 * 官方价格（价格中心 Tab）：行内直接编辑、双币种同步、改后即时生效、带修改时间，默认每页 100 条。
 * props.inputs 含 ModelRatio / CompletionRatio / ModelPrice / ModelPriceUpdateTime（option JSON 串）。
 */
const OfficialPriceTab = ({ inputs, refresh }) => {
  const { t } = useTranslation();
  const [rate, setRate] = useState(DEFAULT_RATE);
  const [keyword, setKeyword] = useState('');
  const [savingModel, setSavingModel] = useState(null);

  // 新增模型行内表单
  const [newName, setNewName] = useState('');
  const [newMode, setNewMode] = useState('token'); // token | call

  useEffect(() => {
    const fetchRate = async () => {
      try {
        const res = await API.get('/api/status');
        const r = res.data?.data?.usd_exchange_rate;
        if (r && !isNaN(r)) setRate(parseFloat(r));
      } catch (e) {
        // 保持默认汇率
      }
    };
    fetchRate();
  }, []);

  // 由 option 解析行数据
  const rows = useMemo(() => {
    let modelRatio = {};
    let completionRatio = {};
    let modelPrice = {};
    let updateTime = {};
    try {
      modelRatio = JSON.parse(inputs?.ModelRatio || '{}');
    } catch (e) {}
    try {
      completionRatio = JSON.parse(inputs?.CompletionRatio || '{}');
    } catch (e) {}
    try {
      modelPrice = JSON.parse(inputs?.ModelPrice || '{}');
    } catch (e) {}
    try {
      updateTime = JSON.parse(inputs?.ModelPriceUpdateTime || '{}');
    } catch (e) {}

    const names = new Set([
      ...Object.keys(modelRatio),
      ...Object.keys(completionRatio),
      ...Object.keys(modelPrice),
    ]);
    let arr = Array.from(names).map((name) => {
      const isPerCall = modelPrice[name] !== undefined;
      const ratio = modelRatio[name];
      const comp = completionRatio[name];
      const inputUSD = ratio !== undefined ? round6(ratio * 2) : null;
      const outputUSD =
        ratio !== undefined
          ? round6(ratio * (comp !== undefined ? comp : 1) * 2)
          : null;
      return {
        key: name,
        model: name,
        isPerCall,
        inputUSD,
        outputUSD,
        perCallUSD: isPerCall ? modelPrice[name] : null,
        updatedAt: updateTime[name] || null,
      };
    });
    if (keyword.trim()) {
      const kw = keyword.trim().toLowerCase();
      arr = arr.filter((r) => r.model.toLowerCase().includes(kw));
    }
    // 修改时间倒序，其次按模型名
    arr.sort((a, b) => {
      if ((b.updatedAt || 0) !== (a.updatedAt || 0)) {
        return (b.updatedAt || 0) - (a.updatedAt || 0);
      }
      return a.model.localeCompare(b.model);
    });
    return arr;
  }, [inputs, keyword]);

  // 保存某模型（合并补丁后 PUT），成功后刷新 options
  const saveModel = async (row, patch = {}) => {
    const merged = { ...row, ...patch };
    setSavingModel(merged.model);
    try {
      const res = await API.put('/api/pricing/model', {
        model_name: merged.model,
        is_per_call: merged.isPerCall,
        input_price: merged.inputUSD || 0,
        output_price: merged.outputUSD || 0,
        per_call_price: merged.perCallUSD || 0,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('已保存'));
        await refresh();
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    } finally {
      setSavingModel(null);
    }
  };

  const handleDelete = async (model) => {
    try {
      const res = await API.delete('/api/pricing/model', {
        params: { model_name: model },
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('删除成功'));
        await refresh();
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  const handleAdd = async () => {
    const name = newName.trim();
    if (!name) {
      showError(t('模型名称不能为空'));
      return;
    }
    await saveModel(
      {
        model: name,
        isPerCall: newMode === 'call',
        inputUSD: 0,
        outputUSD: 0,
        perCallUSD: 0,
      },
      {},
    );
    setNewName('');
  };

  const columns = [
    { title: t('模型名称'), dataIndex: 'model', width: 240 },
    {
      title: t('计费方式'),
      dataIndex: 'isPerCall',
      width: 130,
      render: (_, r) => (
        <Select
          size='small'
          value={r.isPerCall ? 'call' : 'token'}
          style={{ width: 100 }}
          onChange={(v) => saveModel(r, { isPerCall: v === 'call' })}
          optionList={[
            { value: 'token', label: t('按量计费') },
            { value: 'call', label: t('按次计费') },
          ]}
        />
      ),
    },
    {
      title: t('输入价格') + ' ($/¥ /1M)',
      dataIndex: 'inputUSD',
      width: 180,
      render: (_, r) => (
        <DualPriceCell
          value={r.inputUSD}
          rate={rate}
          disabled={r.isPerCall}
          onCommit={(usd) => saveModel(r, { inputUSD: usd })}
        />
      ),
    },
    {
      title: t('输出价格') + ' ($/¥ /1M)',
      dataIndex: 'outputUSD',
      width: 180,
      render: (_, r) => (
        <DualPriceCell
          value={r.outputUSD}
          rate={rate}
          disabled={r.isPerCall}
          onCommit={(usd) => saveModel(r, { outputUSD: usd })}
        />
      ),
    },
    {
      title: t('次数价格') + ' ($/¥ /次)',
      dataIndex: 'perCallUSD',
      width: 180,
      render: (_, r) => (
        <DualPriceCell
          value={r.perCallUSD}
          rate={rate}
          disabled={!r.isPerCall}
          onCommit={(usd) => saveModel(r, { perCallUSD: usd })}
        />
      ),
    },
    {
      title: t('修改时间'),
      dataIndex: 'updatedAt',
      width: 180,
      render: (v) => (v ? new Date(v * 1000).toLocaleString() : '-'),
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      width: 100,
      fixed: 'right',
      render: (_, r) => (
        <Popconfirm title={t('确认删除')} onConfirm={() => handleDelete(r.model)}>
          <Button size='small' type='danger'>
            {t('删除')}
          </Button>
        </Popconfirm>
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
          onChange={setKeyword}
          showClear
          style={{ width: 220 }}
        />
        <Button icon={<IconRefresh />} onClick={refresh}>
          {t('刷新')}
        </Button>
        <Tag color='green'>
          {t('当前汇率')} 1$ = {rate}¥
        </Tag>
      </div>

      {/* 行内新增（非弹窗） */}
      <div className='flex items-center gap-2 mb-3 flex-wrap p-3 rounded-lg' style={{ background: 'var(--semi-color-fill-0)' }}>
        <Typography.Text type='secondary'>{t('新增模型')}：</Typography.Text>
        <Input
          placeholder={t('模型名称')}
          value={newName}
          onChange={setNewName}
          style={{ width: 240 }}
        />
        <Select
          value={newMode}
          onChange={setNewMode}
          style={{ width: 120 }}
          optionList={[
            { value: 'token', label: t('按量计费') },
            { value: 'call', label: t('按次计费') },
          ]}
        />
        <Button type='primary' icon={<IconPlus />} onClick={handleAdd}>
          {t('添加')}
        </Button>
        <Typography.Text type='tertiary' size='small'>
          {t('添加后可直接在表格中填写价格，改动即时生效')}
        </Typography.Text>
      </div>

      <Table
        columns={columns}
        dataSource={rows}
        rowKey='key'
        loading={!!savingModel}
        pagination={{ pageSize: 100, formatPageText: false }}
        empty={t('暂无已配置的价格')}
        scroll={{ x: 'max-content' }}
      />
    </div>
  );
};

export default OfficialPriceTab;
