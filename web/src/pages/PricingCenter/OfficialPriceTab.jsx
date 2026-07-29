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

import React, { useEffect, useMemo, useState, useRef } from 'react';
import {
  Table,
  Button,
  Input,
  Select,
  Space,
  Typography,
  Popconfirm,
  Tag,
  Modal,
  Form,
} from '@douyinfe/semi-ui';
import { IconSearch, IconRefresh, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const DEFAULT_RATE = 7.3;
const round6 = (n) => Math.round((Number(n) + Number.EPSILON) * 1e6) / 1e6;

/**
 * 判断两个价格是否「实质相同」。
 *
 * 存库的美金价是 6 位量化值、倍率写入时还会做一次消噪吸附，因此从人民币侧反推出来的值
 * 与库里的值往往在末位有极小差异。用严格 !== 判断会导致仅仅点一下输入框再失焦
 * 就触发一次无意义的写库（甚至反复来回写），这里用容差兜住。
 *
 * 相对容差 5e-5 相当于 0.005%，远小于任何有意义的调价；
 * 绝对下限取 1.5e-6，略大于存储侧 6 位量化的步长——差异小于一个存储步长时根本无法落库，
 * 判为「无变化」才是正确语义（否则录入 ¥0.123456 这类多位小数后会多出一次无谓的保存）。
 */
const NO_OP_ABS_EPS = 1.5e-6;
const nearlyEqual = (a, b) => {
  const x = Number(a) || 0;
  const y = Number(b) || 0;
  if (x === y) return true;
  const scale = Math.max(Math.abs(x), Math.abs(y));
  if (scale === 0) return true;
  return Math.abs(x - y) <= Math.max(scale * 5e-5, NO_OP_ABS_EPS);
};

/**
 * 价格展示格式：默认两位小数，但只在「与真值几乎相等」时才收缩位数。
 *
 * - 浮点噪声被抹平：1.99999 / 2.00002 / 19.9999968 都显示成 2.00 / 20.00
 * - 真实价格不被歪曲：0.075 仍显示 0.075，不会变成 0.08（那会让人误以为涨价了）
 * - 极小价格不被抹成 0：0.002 显示 0.002 而不是 0.00
 *
 * 容差取「相对 1e-5，至少 5e-6，但不超过自身的 1%」——绝对下限是为了兼顾
 * ¥0.01 这类小额价格（美金侧 6 位量化后的噪声相对值会偏大），
 * 1% 上限则防止极小价格被吸附到量级完全不同的数字上。
 */
const formatPrice = (n) => {
  const num = Number(n);
  if (n === null || n === undefined || n === '' || !isFinite(num)) return '';
  if (num === 0) return '0.00';
  const abs = Math.abs(num);
  const tol = Math.min(Math.max(abs * 1e-5, 5e-6), abs * 1e-2);
  for (let d = 2; d <= 6; d++) {
    const s = num.toFixed(d);
    const v = Number(s);
    if (v !== 0 && Math.abs(v - num) <= tol) return s;
  }
  return String(num);
};

const formatTokens = (n) => {
  if (n >= 1_000_000)
    return `${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(n % 1_000 === 0 ? 0 : 1)}K`;
  return String(n);
};

/**
 * 双币种（美金/人民币）行内编辑单元：修改任一按汇率换算另一个，失焦/回车即提交。
 *
 * 精度约定：
 * - 存储侧沿用 6 位小数量化，计费不受影响
 * - 展示侧走 formatPrice（两位小数为主），把换算往返产生的浮点噪声挡在页面之外
 * - 聚焦时展开完整精度，保证要编辑时看到的是真值而不是展示近似值
 * - 未实际编辑过的输入框失焦不提交，避免「点一下就改价」
 */
const DualPriceCell = ({ value, rate, disabled, onCommit }) => {
  const [usdStr, setUsdStr] = useState('');
  const [cnyStr, setCnyStr] = useState('');
  // 是否被用户真正编辑过（而不仅仅是聚焦/失焦）
  const usdDirty = useRef(false);
  const cnyDirty = useRef(false);

  const hasValue = !(value === null || value === undefined || value === '');
  const usdExact = hasValue ? round6(value) : null;
  const cnyExact = hasValue ? round6(value * rate) : null;

  useEffect(() => {
    setUsdStr(hasValue ? formatPrice(usdExact) : '');
    setCnyStr(hasValue ? formatPrice(cnyExact) : '');
    usdDirty.current = false;
    cnyDirty.current = false;
  }, [value, rate]);

  if (disabled) {
    return <Typography.Text type='tertiary'>-</Typography.Text>;
  }

  const commitFromUsd = () => {
    const parsed = usdStr === '' ? 0 : parseFloat(usdStr);
    const usd = round6(isNaN(parsed) ? 0 : parsed);
    usdDirty.current = false;
    setUsdStr(formatPrice(usd));
    setCnyStr(formatPrice(round6(usd * rate)));
    if (!nearlyEqual(usd, value || 0)) onCommit(usd);
  };
  const commitFromCny = () => {
    const parsed = cnyStr === '' ? 0 : parseFloat(cnyStr);
    const cny = isNaN(parsed) ? 0 : parsed;
    const usd = round6(cny / rate);
    cnyDirty.current = false;
    setUsdStr(formatPrice(usd));
    setCnyStr(formatPrice(round6(usd * rate)));
    if (!nearlyEqual(usd, value || 0)) onCommit(usd);
  };

  // 聚焦展开真值，失焦若未编辑则只恢复展示格式、不写库
  const handleUsdFocus = () => setUsdStr(hasValue ? String(usdExact) : '');
  const handleUsdBlur = () => {
    if (!usdDirty.current) {
      setUsdStr(hasValue ? formatPrice(usdExact) : '');
      return;
    }
    commitFromUsd();
  };
  const handleCnyFocus = () => setCnyStr(hasValue ? String(cnyExact) : '');
  const handleCnyBlur = () => {
    if (!cnyDirty.current) {
      setCnyStr(hasValue ? formatPrice(cnyExact) : '');
      return;
    }
    commitFromCny();
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      <Input
        size='small'
        prefix='$'
        value={usdStr}
        onChange={(v) => {
          usdDirty.current = true;
          setUsdStr(v);
        }}
        onFocus={handleUsdFocus}
        onBlur={handleUsdBlur}
        onEnterPress={commitFromUsd}
        style={{ width: 150 }}
      />
      <Input
        size='small'
        prefix='¥'
        value={cnyStr}
        onChange={(v) => {
          cnyDirty.current = true;
          setCnyStr(v);
        }}
        onFocus={handleCnyFocus}
        onBlur={handleCnyBlur}
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
  const [newMode, setNewMode] = useState('token'); // token | call | tiered

  // 阶梯档位编辑弹窗状态
  const [tierModalModel, setTierModalModel] = useState(null); // 正在编辑档位的模型名
  const [tierEditIndex, setTierEditIndex] = useState(-1); // -1=新增档位
  const tierFormRef = useRef(null);

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
    let cacheRatio = {};
    let createCacheRatio = {};
    let tieredPrice = {};
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
      cacheRatio = JSON.parse(inputs?.CacheRatio || '{}');
    } catch (e) {}
    try {
      createCacheRatio = JSON.parse(inputs?.CreateCacheRatio || '{}');
    } catch (e) {}
    try {
      tieredPrice = JSON.parse(inputs?.TieredPrice || '{}');
    } catch (e) {}
    try {
      updateTime = JSON.parse(inputs?.ModelPriceUpdateTime || '{}');
    } catch (e) {}

    const names = new Set([
      ...Object.keys(modelRatio),
      ...Object.keys(completionRatio),
      ...Object.keys(modelPrice),
      ...Object.keys(tieredPrice),
    ]);
    let arr = Array.from(names).map((name) => {
      // 计费方式三态：阶梯 > 按次 > 按量
      let billingMode = 'token';
      if (Array.isArray(tieredPrice[name])) billingMode = 'tiered';
      else if (modelPrice[name] !== undefined) billingMode = 'call';

      const ratio = modelRatio[name];
      const comp = completionRatio[name];
      const inputUSD = ratio !== undefined ? round6(ratio * 2) : null;
      const outputUSD =
        ratio !== undefined
          ? round6(ratio * (comp !== undefined ? comp : 1) * 2)
          : null;
      // 缓存绝对价 = 缓存倍率 * 输入价（$/1M）
      const cr = cacheRatio[name];
      const ccr = createCacheRatio[name];
      const cacheReadUSD =
        cr !== undefined && inputUSD ? round6(cr * inputUSD) : null;
      const cacheCreateUSD =
        ccr !== undefined && inputUSD ? round6(ccr * inputUSD) : null;
      return {
        key: name,
        model: name,
        billingMode,
        inputUSD,
        outputUSD,
        perCallUSD: billingMode === 'call' ? modelPrice[name] : null,
        cacheReadUSD,
        cacheCreateUSD,
        tiers: Array.isArray(tieredPrice[name]) ? tieredPrice[name] : [],
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

  // 保存某模型（按量/按次，合并补丁后 PUT），成功后刷新 options
  const saveModel = async (row, patch = {}) => {
    const merged = { ...row, ...patch };
    setSavingModel(merged.model);
    try {
      const res = await API.put('/api/pricing/model', {
        model_name: merged.model,
        is_per_call: merged.billingMode === 'call',
        input_price: merged.inputUSD || 0,
        output_price: merged.outputUSD || 0,
        per_call_price: merged.perCallUSD || 0,
        cache_read_price: merged.cacheReadUSD || 0,
        cache_create_price: merged.cacheCreateUSD || 0,
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

  // 读取当前全量阶梯价 JSON（模型名 → tiers[]）
  const readTieredMap = () => {
    try {
      const parsed = JSON.parse(inputs?.TieredPrice || '{}');
      return parsed && typeof parsed === 'object' ? parsed : {};
    } catch (e) {
      return {};
    }
  };

  // 整体写回阶梯价 option（tiers 为空则删除该模型键）
  const saveTiered = async (modelName, tiers) => {
    setSavingModel(modelName);
    try {
      const map = readTieredMap();
      if (Array.isArray(tiers) && tiers.length > 0) {
        map[modelName] = [...tiers].sort((a, b) => a.max_tokens - b.max_tokens);
      } else {
        delete map[modelName];
      }
      const res = await API.put('/api/option/', {
        key: 'TieredPrice',
        value: JSON.stringify(map, null, 2),
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

  // 切换计费方式：清理另一套配置的残留，保证一个模型只有一份
  const switchBillingMode = async (row, mode) => {
    if (mode === row.billingMode) return;
    if (mode === 'tiered') {
      // 从 按量/按次 切到 阶梯：先清 ModelRatio/ModelPrice，再写入初始档位
      setSavingModel(row.model);
      try {
        await API.delete('/api/pricing/model', {
          params: { model_name: row.model },
        });
      } catch (e) {}
      setSavingModel(null);
      const initTiers = row.tiers?.length
        ? row.tiers
        : [{ max_tokens: 128000, input_price: 0, output_price: 0 }];
      await saveTiered(row.model, initTiers);
    } else if (row.billingMode === 'tiered') {
      // 从 阶梯 切回 按量/按次：先清阶梯键，再按 saveModel 落一份基础价
      await saveTiered(row.model, []);
      await saveModel(
        { model: row.model, billingMode: mode },
        { inputUSD: 0, outputUSD: 0, perCallUSD: 0 },
      );
    } else {
      // 按量 <-> 按次
      await saveModel(row, { billingMode: mode });
    }
  };

  const handleDelete = async (row) => {
    try {
      if (row.billingMode === 'tiered') {
        await saveTiered(row.model, []);
        return;
      }
      const res = await API.delete('/api/pricing/model', {
        params: { model_name: row.model },
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
    if (newMode === 'tiered') {
      await saveTiered(name, [
        { max_tokens: 128000, input_price: 0, output_price: 0 },
      ]);
    } else {
      await saveModel(
        {
          model: name,
          billingMode: newMode,
          inputUSD: 0,
          outputUSD: 0,
          perCallUSD: 0,
        },
        {},
      );
    }
    setNewName('');
  };

  // ===== 阶梯档位编辑弹窗 =====
  const openTierEditor = (modelName, tierIndex = -1) => {
    setTierModalModel(modelName);
    setTierEditIndex(tierIndex);
    const row = rows.find((r) => r.model === modelName);
    const tier = tierIndex >= 0 ? row?.tiers?.[tierIndex] : null;
    setTimeout(() => {
      tierFormRef.current?.setValues(
        tier
          ? {
              max_tokens: tier.max_tokens,
              input_price: tier.input_price,
              output_price: tier.output_price,
              cached_input_price: tier.cached_input_price,
              cache_write_price: tier.cache_write_price,
            }
          : {
              max_tokens: undefined,
              input_price: undefined,
              output_price: undefined,
              cached_input_price: undefined,
              cache_write_price: undefined,
            },
      );
    }, 0);
  };

  const handleSaveTier = async (values) => {
    const {
      max_tokens,
      input_price,
      output_price,
      cached_input_price,
      cache_write_price,
    } = values;
    const row = rows.find((r) => r.model === tierModalModel);
    const tiers = row ? [...row.tiers] : [];
    const tier = { max_tokens, input_price, output_price };
    // 缓存价可选，未填不写入（配合后端 omitempty 与 0=回退倍率语义）
    if (cached_input_price) tier.cached_input_price = cached_input_price;
    if (cache_write_price) tier.cache_write_price = cache_write_price;
    if (tierEditIndex >= 0) {
      tiers[tierEditIndex] = tier;
    } else {
      if (tiers.some((t) => t.max_tokens === max_tokens)) {
        showError(t('该阈值已存在'));
        return;
      }
      tiers.push(tier);
    }
    setTierModalModel(null);
    await saveTiered(row.model, tiers);
  };

  const handleDeleteTier = async (modelName, tierIndex) => {
    const row = rows.find((r) => r.model === modelName);
    if (!row) return;
    const tiers = row.tiers.filter((_, i) => i !== tierIndex);
    await saveTiered(modelName, tiers);
  };

  const columns = [
    { title: t('模型名称'), dataIndex: 'model', width: 240 },
    {
      title: t('计费方式'),
      dataIndex: 'billingMode',
      width: 130,
      render: (_, r) => (
        <Select
          size='small'
          value={r.billingMode}
          style={{ width: 110 }}
          onChange={(v) => switchBillingMode(r, v)}
          optionList={[
            { value: 'token', label: t('按量计费') },
            { value: 'call', label: t('按次计费') },
            { value: 'tiered', label: t('阶梯计费') },
          ]}
        />
      ),
    },
    {
      title: t('输入价格') + ' ($/¥ /1M)',
      dataIndex: 'inputUSD',
      width: 200,
      render: (_, r) => {
        if (r.billingMode === 'tiered') {
          return (
            <Button
              size='small'
              theme='light'
              type='tertiary'
              onClick={() => openTierEditor(r.model)}
            >
              {t('编辑档位')}（{r.tiers.length}）
            </Button>
          );
        }
        return (
          <DualPriceCell
            value={r.inputUSD}
            rate={rate}
            disabled={r.billingMode !== 'token'}
            onCommit={(usd) => saveModel(r, { inputUSD: usd })}
          />
        );
      },
    },
    {
      title: t('输出价格') + ' ($/¥ /1M)',
      dataIndex: 'outputUSD',
      width: 200,
      render: (_, r) => {
        if (r.billingMode === 'tiered') {
          // 阶梯模式：以档位标签形式展示各档，点击编辑
          return (
            <Space wrap>
              {r.tiers.map((tier, idx) => (
                <Tag
                  key={idx}
                  color='blue'
                  closable
                  onClose={() => handleDeleteTier(r.model, idx)}
                  onClick={() => openTierEditor(r.model, idx)}
                  style={{ cursor: 'pointer' }}
                >
                  ≤{formatTokens(tier.max_tokens)}: $
                  {formatPrice(tier.input_price)}/$
                  {formatPrice(tier.output_price)}
                  {tier.cached_input_price
                    ? ` ${t('缓存读')} $${formatPrice(tier.cached_input_price)}`
                    : ''}
                  {tier.cache_write_price
                    ? ` ${t('缓存写')} $${formatPrice(tier.cache_write_price)}`
                    : ''}
                </Tag>
              ))}
              <Button
                icon={<IconPlus />}
                size='small'
                theme='borderless'
                onClick={() => openTierEditor(r.model)}
              />
            </Space>
          );
        }
        return (
          <DualPriceCell
            value={r.outputUSD}
            rate={rate}
            disabled={r.billingMode !== 'token'}
            onCommit={(usd) => saveModel(r, { outputUSD: usd })}
          />
        );
      },
    },
    {
      title: t('缓存读取价') + ' ($/¥ /1M)',
      dataIndex: 'cacheReadUSD',
      width: 180,
      render: (_, r) => (
        <DualPriceCell
          value={r.cacheReadUSD}
          rate={rate}
          disabled={r.billingMode !== 'token'}
          onCommit={(usd) => saveModel(r, { cacheReadUSD: usd })}
        />
      ),
    },
    {
      title: t('缓存创建价') + ' ($/¥ /1M)',
      dataIndex: 'cacheCreateUSD',
      width: 180,
      render: (_, r) => (
        <DualPriceCell
          value={r.cacheCreateUSD}
          rate={rate}
          disabled={r.billingMode !== 'token'}
          onCommit={(usd) => saveModel(r, { cacheCreateUSD: usd })}
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
          disabled={r.billingMode !== 'call'}
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
        <Popconfirm title={t('确认删除')} onConfirm={() => handleDelete(r)}>
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
      <div
        className='flex items-center gap-2 mb-3 flex-wrap p-3 rounded-lg'
        style={{ background: 'var(--semi-color-fill-0)' }}
      >
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
            { value: 'tiered', label: t('阶梯计费') },
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

      {/* 阶梯档位 新增/编辑 弹窗 */}
      <Modal
        title={tierEditIndex >= 0 ? t('编辑价格档位') : t('添加价格档位')}
        visible={!!tierModalModel}
        onCancel={() => setTierModalModel(null)}
        onOk={() => {
          tierFormRef.current
            ?.validate()
            .then((values) => handleSaveTier(values))
            .catch(() => showError(t('请检查输入')));
        }}
        width={450}
      >
        <Typography.Text
          type='tertiary'
          style={{ display: 'block', marginBottom: 8 }}
        >
          {t('模型')}：{tierModalModel}
        </Typography.Text>
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
          <Form.InputNumber
            field='cached_input_price'
            label={t('缓存读取价（$/1M tokens）')}
            placeholder={t('留空则回退缓存倍率')}
            min={0}
            step={0.01}
            style={{ width: '100%' }}
          />
          <Form.InputNumber
            field='cache_write_price'
            label={t('缓存创建价（$/1M tokens）')}
            placeholder={t('留空则回退缓存创建倍率')}
            min={0}
            step={0.01}
            style={{ width: '100%' }}
          />
        </Form>
      </Modal>
    </div>
  );
};

export default OfficialPriceTab;
