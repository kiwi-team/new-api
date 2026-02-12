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
import {
  Modal,
  Button,
  Checkbox,
  CheckboxGroup,
  Typography,
  Spin,
  Banner,
  List,
  Tag,
  Input,
  Radio,
  RadioGroup,
  Table,
} from '@douyinfe/semi-ui';
import { IconTick, IconClose, IconSearch } from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const SyncModelPricesModal = ({ visible, onCancel, options = {} }) => {
  const { t } = useTranslation();
  const [step, setStep] = useState(1); // 1: 选择模型, 2: 选择环境, 3: 确认, 4: 结果
  const [environments, setEnvironments] = useState([]);
  const [selectedEnvIds, setSelectedEnvIds] = useState([]);
  const [loading, setLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [results, setResults] = useState([]);
  
  // 模型选择相关
  const [syncMode, setSyncMode] = useState('all'); // 'all' | 'selected'
  const [selectedModels, setSelectedModels] = useState([]);
  const [modelSearchText, setModelSearchText] = useState('');

  // 从 options 中获取所有模型
  const allModels = useMemo(() => {
    const modelSet = new Set();
    try {
      const modelPrice = JSON.parse(options.ModelPrice || '{}');
      const modelRatio = JSON.parse(options.ModelRatio || '{}');
      const completionRatio = JSON.parse(options.CompletionRatio || '{}');
      
      Object.keys(modelPrice).forEach(m => modelSet.add(m));
      Object.keys(modelRatio).forEach(m => modelSet.add(m));
      Object.keys(completionRatio).forEach(m => modelSet.add(m));
    } catch (e) {
      console.error('解析模型配置失败:', e);
    }
    return Array.from(modelSet).sort();
  }, [options]);

  // 过滤后的模型列表
  const filteredModels = useMemo(() => {
    if (!modelSearchText) return allModels;
    return allModels.filter(m => 
      m.toLowerCase().includes(modelSearchText.toLowerCase())
    );
  }, [allModels, modelSearchText]);

  // 重置状态
  useEffect(() => {
    if (visible) {
      fetchEnvironments();
      setStep(1);
      setSelectedEnvIds([]);
      setResults([]);
      setSyncMode('all');
      setSelectedModels([]);
      setModelSearchText('');
    }
  }, [visible]);

  const fetchEnvironments = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/sync/environments');
      if (res.data.success) {
        const enabledEnvs = (res.data.data || []).filter(env => env.status === 1);
        setEnvironments(enabledEnvs);
      } else {
        showError(res.data.message || t('获取环境列表失败'));
      }
    } catch (error) {
      showError(t('获取环境列表失败'));
    } finally {
      setLoading(false);
    }
  };

  const handleSync = async () => {
    if (selectedEnvIds.length === 0) {
      showError(t('请选择目标环境'));
      return;
    }

    setSyncing(true);
    try {
      const payload = {
        environment_ids: selectedEnvIds,
      };
      
      // 如果是选择性同步，添加选中的模型
      if (syncMode === 'selected' && selectedModels.length > 0) {
        payload.selected_models = selectedModels;
      }

      const res = await API.post('/api/sync/model-prices', payload);

      if (res.data.success) {
        setResults(res.data.results || []);
        setStep(4);
        
        const successCount = (res.data.results || []).filter(r => r.success).length;
        if (successCount === selectedEnvIds.length) {
          showSuccess(t('同步完成'));
        }
      } else {
        showError(res.data.message || t('同步失败'));
      }
    } catch (error) {
      showError(t('同步请求失败'));
    } finally {
      setSyncing(false);
    }
  };

  // 模型表格列定义
  const modelColumns = [
    {
      title: t('模型名称'),
      dataIndex: 'name',
      key: 'name',
    },
  ];

  const modelTableData = filteredModels.map(m => ({ key: m, name: m }));

  const renderStep1 = () => (
    <div>
      <Typography.Text style={{ marginBottom: 16, display: 'block' }}>
        {t('选择要同步的模型范围：')}
      </Typography.Text>
      
      <RadioGroup
        value={syncMode}
        onChange={e => setSyncMode(e.target.value)}
        direction="vertical"
        style={{ marginBottom: 16 }}
      >
        <Radio value="all">
          {t('同步全部模型价格配置')}
          <Typography.Text type="tertiary" size="small" style={{ marginLeft: 8 }}>
            ({allModels.length} {t('个模型')})
          </Typography.Text>
        </Radio>
        <Radio value="selected">
          {t('选择部分模型同步')}
        </Radio>
      </RadioGroup>

      {syncMode === 'selected' && (
        <div style={{ marginTop: 16 }}>
          <Input
            prefix={<IconSearch />}
            placeholder={t('搜索模型名称')}
            value={modelSearchText}
            onChange={setModelSearchText}
            style={{ marginBottom: 12, width: 300 }}
            showClear
          />
          
          <div style={{ maxHeight: 300, overflow: 'auto' }}>
            <Table
              columns={modelColumns}
              dataSource={modelTableData}
              pagination={false}
              size="small"
              rowSelection={{
                selectedRowKeys: selectedModels,
                onChange: (keys) => setSelectedModels(keys),
              }}
            />
          </div>
          
          <Typography.Text type="tertiary" size="small" style={{ marginTop: 8, display: 'block' }}>
            {t('已选择')} {selectedModels.length} {t('个模型')}
          </Typography.Text>
        </div>
      )}
    </div>
  );

  const renderStep2 = () => (
    <div>
      <Typography.Text style={{ marginBottom: 16, display: 'block' }}>
        {t('选择要同步到的目标环境：')}
      </Typography.Text>
      
      {loading ? (
        <div style={{ textAlign: 'center', padding: 20 }}>
          <Spin />
        </div>
      ) : environments.length === 0 ? (
        <Banner
          type="warning"
          description={t('暂无可用的同步环境，请先在环境管理中添加并启用环境')}
        />
      ) : (
        <CheckboxGroup
          direction="vertical"
          value={selectedEnvIds}
          onChange={setSelectedEnvIds}
        >
          {environments.map(env => (
            <Checkbox key={env.id} value={env.id}>
              <span>{env.name}</span>
              <Typography.Text type="tertiary" size="small" style={{ marginLeft: 8 }}>
                {env.api_url}
              </Typography.Text>
            </Checkbox>
          ))}
        </CheckboxGroup>
      )}
    </div>
  );

  const renderStep3 = () => {
    const selectedEnvs = environments.filter(env => selectedEnvIds.includes(env.id));
    const modelCount = syncMode === 'all' ? allModels.length : selectedModels.length;
    
    return (
      <div>
        <Banner
          type="warning"
          description={
            syncMode === 'all' 
              ? t('即将同步全部模型价格配置到目标环境，此操作将覆盖目标环境的配置')
              : t('即将同步选中模型的价格配置到目标环境，此操作将合并到目标环境的配置中')
          }
          style={{ marginBottom: 16 }}
        />
        
        <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>
          {t('同步内容：')}
        </Typography.Text>
        <ul style={{ marginBottom: 16, paddingLeft: 20 }}>
          <li>{t('模型倍率 (ModelRatio)')}</li>
          <li>{t('模型固定价格 (ModelPrice)')}</li>
          <li>{t('补全倍率 (CompletionRatio)')}</li>
        </ul>
        
        <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>
          {t('模型数量：')} {modelCount} {t('个')}
        </Typography.Text>
        
        {syncMode === 'selected' && selectedModels.length <= 10 && (
          <div style={{ marginBottom: 16 }}>
            {selectedModels.map(m => (
              <Tag key={m} style={{ marginRight: 4, marginBottom: 4 }}>{m}</Tag>
            ))}
          </div>
        )}
        
        <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>
          {t('目标环境：')}
        </Typography.Text>
        <List
          dataSource={selectedEnvs}
          renderItem={env => (
            <List.Item>
              <span>{env.name}</span>
              <Typography.Text type="tertiary" size="small" style={{ marginLeft: 8 }}>
                {env.api_url}
              </Typography.Text>
            </List.Item>
          )}
        />
      </div>
    );
  };

  const renderStep4 = () => (
    <div>
      <Typography.Text style={{ marginBottom: 16, display: 'block' }}>
        {t('同步结果：')}
      </Typography.Text>
      
      <List
        dataSource={results}
        renderItem={result => (
          <List.Item
            header={
              result.success ? (
                <Tag color="green" prefixIcon={<IconTick />}>{t('成功')}</Tag>
              ) : (
                <Tag color="red" prefixIcon={<IconClose />}>{t('失败')}</Tag>
              )
            }
            main={
              <div>
                <Typography.Text strong>{result.environment_name}</Typography.Text>
                {!result.success && result.error && (
                  <Typography.Text type="danger" size="small" style={{ display: 'block' }}>
                    {result.error}
                  </Typography.Text>
                )}
              </div>
            }
          />
        )}
      />
    </div>
  );

  const canProceedStep1 = () => {
    if (syncMode === 'all') return true;
    return selectedModels.length > 0;
  };

  const getFooter = () => {
    if (step === 1) {
      return (
        <>
          <Button onClick={onCancel}>{t('取消')}</Button>
          <Button
            type="primary"
            disabled={!canProceedStep1()}
            onClick={() => setStep(2)}
          >
            {t('下一步')}
          </Button>
        </>
      );
    }
    
    if (step === 2) {
      return (
        <>
          <Button onClick={() => setStep(1)}>{t('上一步')}</Button>
          <Button
            type="primary"
            disabled={selectedEnvIds.length === 0 || environments.length === 0}
            onClick={() => setStep(3)}
          >
            {t('下一步')}
          </Button>
        </>
      );
    }
    
    if (step === 3) {
      return (
        <>
          <Button onClick={() => setStep(2)}>{t('上一步')}</Button>
          <Button type="warning" loading={syncing} onClick={handleSync}>
            {t('确认同步')}
          </Button>
        </>
      );
    }
    
    return (
      <Button type="primary" onClick={onCancel}>
        {t('完成')}
      </Button>
    );
  };

  return (
    <Modal
      title={t('同步模型价格到其他环境')}
      visible={visible}
      onCancel={onCancel}
      footer={getFooter()}
      width={600}
      closeOnEsc={!syncing}
      maskClosable={!syncing}
    >
      {step === 1 && renderStep1()}
      {step === 2 && renderStep2()}
      {step === 3 && renderStep3()}
      {step === 4 && renderStep4()}
    </Modal>
  );
};

export default SyncModelPricesModal;
