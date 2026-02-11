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
  Select,
  InputNumber,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconDelete,
  IconPlus,
  IconSearch,
  IconSave,
  IconEdit,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';

// 默认汇率
const DEFAULT_USD_TO_CNY_RATE = 7.3;

// 价格单位选项
const PRICE_UNIT_OPTIONS = [
  { value: 'usd_per_1m', label: '$/1M tokens' },
  { value: 'usd_per_1k', label: '$/1K tokens' },
  { value: 'cny_per_1m', label: '¥/1M tokens' },
  { value: 'cny_per_1k', label: '¥/1K tokens' },
];

// 将各种价格单位转换为 $/1M tokens
const convertToUsdPer1M = (price, unit, exchangeRate) => {
  if (!price || isNaN(price)) return 0;
  const p = parseFloat(price);
  const rate = exchangeRate || DEFAULT_USD_TO_CNY_RATE;
  switch (unit) {
    case 'usd_per_1m':
      return p;
    case 'usd_per_1k':
      return p * 1000;
    case 'cny_per_1m':
      return p / rate;
    case 'cny_per_1k':
      return (p * 1000) / rate;
    default:
      return p;
  }
};


// 将 $/1M tokens 转换为系统倍率
// 系统基准：$0.002 / 1K tokens = $2 / 1M tokens，对应倍率 1
// 所以 ratio = price_per_1m / 2
const priceToRatio = (pricePerMillion) => {
  if (!pricePerMillion || isNaN(pricePerMillion)) return '';
  return (parseFloat(pricePerMillion) / 2).toFixed(6);
};

// 将系统倍率转换为 $/1M tokens
const ratioToPrice = (ratio) => {
  if (!ratio || isNaN(ratio)) return '';
  return (parseFloat(ratio) * 2).toFixed(6);
};

export default function HumanReadablePriceEditor(props) {
  const { t } = useTranslation();
  const [models, setModels] = useState([]);
  const [visible, setVisible] = useState(false);
  const [isEditMode, setIsEditMode] = useState(false);
  const [currentModel, setCurrentModel] = useState(null);
  const [searchText, setSearchText] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [exchangeRate, setExchangeRate] = useState(DEFAULT_USD_TO_CNY_RATE);
  const formRef = useRef(null);
  const pageSize = 10;

  // 表单状态
  const [priceUnit, setPriceUnit] = useState('usd_per_1m');

  // 获取汇率
  useEffect(() => {
    const fetchExchangeRate = async () => {
      try {
        const res = await API.get('/api/status');
        if (res.data?.data?.usd_exchange_rate) {
          setExchangeRate(res.data.data.usd_exchange_rate);
        }
      } catch (error) {
        console.error('获取汇率失败:', error);
      }
    };
    fetchExchangeRate();
  }, []);

  useEffect(() => {
    try {
      const modelPrice = JSON.parse(props.options.ModelPrice || '{}');
      const modelRatio = JSON.parse(props.options.ModelRatio || '{}');
      const completionRatio = JSON.parse(props.options.CompletionRatio || '{}');

      // 合并所有模型名称
      const modelNames = new Set([
        ...Object.keys(modelPrice),
        ...Object.keys(modelRatio),
        ...Object.keys(completionRatio),
      ]);

      const modelData = Array.from(modelNames).map((name) => {
        const price = modelPrice[name] === undefined ? '' : modelPrice[name];
        const ratio = modelRatio[name] === undefined ? '' : modelRatio[name];
        const comp = completionRatio[name] === undefined ? '' : completionRatio[name];

        // 计算显示用的价格（$/1M tokens）
        const inputPriceDisplay = ratio !== '' ? ratioToPrice(ratio) : '';
        const outputPriceDisplay = ratio !== '' && comp !== '' 
          ? (parseFloat(ratioToPrice(ratio)) * parseFloat(comp)).toFixed(6) 
          : '';

        return {
          name,
          fixedPrice: price,
          ratio,
          completionRatio: comp,
          inputPriceDisplay,
          outputPriceDisplay,
        };
      });

      setModels(modelData);
    } catch (error) {
      console.error('JSON解析错误:', error);
    }
  }, [props.options]);


  // 分页处理
  const getPagedData = (data, currentPage, pageSize) => {
    const start = (currentPage - 1) * pageSize;
    const end = start + pageSize;
    return data.slice(start, end);
  };

  const filteredModels = models.filter((model) => {
    return searchText ? model.name.toLowerCase().includes(searchText.toLowerCase()) : true;
  });

  const pagedData = getPagedData(filteredModels, currentPage, pageSize);

  // 保存数据
  const submitData = async () => {
    setLoading(true);
    const output = {
      ModelPrice: {},
      ModelRatio: {},
      CompletionRatio: {},
    };

    try {
      models.forEach((model) => {
        if (model.fixedPrice !== '' && model.fixedPrice !== undefined) {
          output.ModelPrice[model.name] = parseFloat(model.fixedPrice);
        } else {
          if (model.ratio !== '' && model.ratio !== undefined) {
            output.ModelRatio[model.name] = parseFloat(model.ratio);
          }
          if (model.completionRatio !== '' && model.completionRatio !== undefined) {
            output.CompletionRatio[model.name] = parseFloat(model.completionRatio);
          }
        }
      });

      const finalOutput = {
        ModelPrice: JSON.stringify(output.ModelPrice, null, 2),
        ModelRatio: JSON.stringify(output.ModelRatio, null, 2),
        CompletionRatio: JSON.stringify(output.CompletionRatio, null, 2),
      };

      const requestQueue = Object.entries(finalOutput).map(([key, value]) => {
        return API.put('/api/option/', { key, value });
      });

      const results = await Promise.all(requestQueue);

      for (const res of results) {
        if (!res.data.success) {
          return showError(res.data.message);
        }
      }

      showSuccess(t('保存成功'));
      props.refresh();
    } catch (error) {
      console.error('保存失败:', error);
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  // 删除模型
  const deleteModel = (name) => {
    setModels((prev) => prev.filter((model) => model.name !== name));
  };

  // 编辑模型
  const editModel = (record) => {
    setIsEditMode(true);
    setCurrentModel({ ...record });
    setPriceUnit('usd_per_1m');
    setVisible(true);

    setTimeout(() => {
      if (formRef.current) {
        formRef.current.setValues({
          name: record.name,
          fixedPrice: record.fixedPrice,
          inputPrice: record.inputPriceDisplay,
          outputPrice: record.outputPriceDisplay,
        });
      }
    }, 0);
  };


  // 添加或更新模型
  const addOrUpdateModel = (values) => {
    const { name, fixedPrice, inputPrice, outputPrice } = values;

    // 计算倍率
    let ratio = '';
    let completionRatio = '';
    let inputPriceDisplay = '';
    let outputPriceDisplay = '';

    if (!fixedPrice) {
      // 按量计费模式
      if (inputPrice) {
        const inputPriceUsd1M = convertToUsdPer1M(inputPrice, priceUnit, exchangeRate);
        ratio = priceToRatio(inputPriceUsd1M);
        inputPriceDisplay = inputPriceUsd1M.toFixed(6);
      }
      if (outputPrice && inputPrice) {
        const inputPriceUsd1M = convertToUsdPer1M(inputPrice, priceUnit, exchangeRate);
        const outputPriceUsd1M = convertToUsdPer1M(outputPrice, priceUnit, exchangeRate);
        if (inputPriceUsd1M > 0) {
          completionRatio = (outputPriceUsd1M / inputPriceUsd1M).toFixed(6);
          outputPriceDisplay = outputPriceUsd1M.toFixed(6);
        }
      }
    }

    const newModel = {
      name,
      fixedPrice: fixedPrice || '',
      ratio,
      completionRatio,
      inputPriceDisplay,
      outputPriceDisplay,
    };

    const existingIndex = models.findIndex((m) => m.name === name);
    if (existingIndex >= 0) {
      setModels((prev) =>
        prev.map((model, index) => (index === existingIndex ? newModel : model))
      );
      showSuccess(t('更新成功'));
    } else {
      setModels((prev) => [newModel, ...prev]);
      showSuccess(t('添加成功'));
    }

    setVisible(false);
    resetModalState();
  };

  const resetModalState = () => {
    setCurrentModel(null);
    setIsEditMode(false);
    setPriceUnit('usd_per_1m');
  };

  const columns = [
    {
      title: t('模型名称'),
      dataIndex: 'name',
      key: 'name',
      width: 250,
    },
    {
      title: t('模型固定价格'),
      dataIndex: 'fixedPrice',
      key: 'fixedPrice',
      width: 120,
      render: (text) => (text !== '' ? `$${text}/次` : '-'),
    },
    {
      title: t('输入价格'),
      dataIndex: 'inputPriceDisplay',
      key: 'inputPriceDisplay',
      width: 150,
      render: (text) => (text ? `$${text}/1M` : '-'),
    },
    {
      title: t('输出价格'),
      dataIndex: 'outputPriceDisplay',
      key: 'outputPriceDisplay',
      width: 150,
      render: (text) => (text ? `$${text}/1M` : '-'),
    },
    {
      title: t('模型倍率'),
      dataIndex: 'ratio',
      key: 'ratio',
      width: 100,
      render: (text) => (text !== '' ? text : '-'),
    },
    {
      title: t('补全倍率'),
      dataIndex: 'completionRatio',
      key: 'completionRatio',
      width: 100,
      render: (text) => (text !== '' ? text : '-'),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 120,
      render: (_, record) => (
        <Space>
          <Button
            type='primary'
            icon={<IconEdit />}
            size='small'
            onClick={() => editModel(record)}
          />
          <Button
            icon={<IconDelete />}
            type='danger'
            size='small'
            onClick={() => deleteModel(record.name)}
          />
        </Space>
      ),
    },
  ];


  return (
    <>
      <Space vertical align='start' style={{ width: '100%' }}>
        <Typography.Text type='secondary' style={{ marginBottom: 8 }}>
          {t('直接输入大模型官方价格（如 $3/1M tokens），系统自动转换为倍率。汇率：1 USD = ')} {exchangeRate} {t(' CNY')}
        </Typography.Text>
        <Space className='mt-2'>
          <Button
            icon={<IconPlus />}
            onClick={() => {
              resetModalState();
              setVisible(true);
            }}
          >
            {t('添加')}
          </Button>
          <Button type='primary' icon={<IconSave />} onClick={submitData} loading={loading}>
            {t('保存')}
          </Button>
          <Input
            prefix={<IconSearch />}
            placeholder={t('搜索模型名称')}
            value={searchText}
            onChange={(value) => {
              setSearchText(value);
              setCurrentPage(1);
            }}
            style={{ width: 200 }}
            showClear
          />
        </Space>
        <Table
          columns={columns}
          dataSource={pagedData}
          pagination={{
            currentPage: currentPage,
            pageSize: pageSize,
            total: filteredModels.length,
            onPageChange: (page) => setCurrentPage(page),
            showTotal: true,
            showSizeChanger: false,
          }}
          loading={loading}
        />
      </Space>

      <Modal
        title={isEditMode ? t('编辑模型价格') : t('添加模型价格')}
        visible={visible}
        onCancel={() => {
          resetModalState();
          setVisible(false);
        }}
        onOk={() => {
          if (formRef.current) {
            formRef.current.validate().then((values) => {
              addOrUpdateModel(values);
            }).catch(() => {
              showError(t('请检查输入'));
            });
          }
        }}
        width={500}
      >
        <Form getFormApi={(api) => (formRef.current = api)}>
          <Form.Input
            field='name'
            label={t('模型名称')}
            placeholder='gpt-4o'
            required
            disabled={isEditMode}
            rules={[{ required: true, message: t('请输入模型名称') }]}
          />

          <Form.Input
            field='fixedPrice'
            label={t('模型固定价格（按次计费）')}
            placeholder={t('留空则按量计费')}
            suffix='$/次'
          />

          <Typography.Text type='secondary' size='small' style={{ display: 'block', margin: '16px 0 8px' }}>
            {t('按量计费价格（与固定价格二选一）')}
          </Typography.Text>

          <Form.Slot label={t('价格单位')}>
            <Select
              value={priceUnit}
              onChange={setPriceUnit}
              optionList={PRICE_UNIT_OPTIONS}
              style={{ width: '100%' }}
            />
          </Form.Slot>

          <Form.Input
            field='inputPrice'
            label={t('输入价格')}
            placeholder='2.5'
          />

          <Form.Input
            field='outputPrice'
            label={t('输出价格')}
            placeholder='10'
          />
        </Form>
      </Modal>
    </>
  );
}
