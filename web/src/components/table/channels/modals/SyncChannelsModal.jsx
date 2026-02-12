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
  Modal,
  Checkbox,
  CheckboxGroup,
  Typography,
  Tag,
  Space,
  Toast,
  Spin,
  Banner,
  List,
} from '@douyinfe/semi-ui';
import { IconAlertTriangle, IconTick, IconClose } from '@douyinfe/semi-icons';
import { API } from '../../../../helpers/api';

const { Text, Title } = Typography;

const SyncChannelsModal = ({ visible, onCancel, selectedChannels, t }) => {
  const [environments, setEnvironments] = useState([]);
  const [selectedEnvIds, setSelectedEnvIds] = useState([]);
  const [loading, setLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [syncResults, setSyncResults] = useState(null);
  const [step, setStep] = useState('select'); // select, confirm, result

  // 加载已启用的环境列表
  const loadEnvironments = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/sync/environments/enabled');
      if (res.data.success) {
        setEnvironments(res.data.data || []);
      } else {
        Toast.error(res.data.message || '加载环境列表失败');
      }
    } catch (error) {
      Toast.error('加载环境列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible) {
      loadEnvironments();
      setStep('select');
      setSelectedEnvIds([]);
      setSyncResults(null);
    }
  }, [visible]);

  // 执行同步
  const handleSync = async () => {
    setSyncing(true);
    try {
      const res = await API.post('/api/sync/channels', {
        channel_ids: selectedChannels.map((ch) => ch.id),
        environment_ids: selectedEnvIds,
      });
      if (res.data.success) {
        setSyncResults(res.data.results);
        setStep('result');
      } else {
        Toast.error(res.data.message || '同步失败');
      }
    } catch (error) {
      Toast.error('同步请求失败');
    } finally {
      setSyncing(false);
    }
  };

  // 渲染选择环境步骤
  const renderSelectStep = () => (
    <div>
      <Banner
        type="warning"
        icon={<IconAlertTriangle />}
        description={t ? t('请选择要同步到的目标环境，同步操作将覆盖目标环境中相同key+类型的渠道配置') : '请选择要同步到的目标环境，同步操作将覆盖目标环境中相同key+类型的渠道配置'}
        style={{ marginBottom: 16 }}
      />
      
      <div style={{ marginBottom: 16 }}>
        <Text strong>{t ? t('已选择的渠道') : '已选择的渠道'}：</Text>
        <div style={{ marginTop: 8 }}>
          {selectedChannels.map((ch) => (
            <Tag key={ch.id} style={{ marginRight: 8, marginBottom: 4 }}>
              {ch.name}
            </Tag>
          ))}
        </div>
      </div>

      {loading ? (
        <Spin />
      ) : environments.length === 0 ? (
        <Banner
          type="info"
          description={t ? t('暂无可用的同步环境，请先在环境管理中添加并启用环境') : '暂无可用的同步环境，请先在环境管理中添加并启用环境'}
        />
      ) : (
        <div>
          <Text strong>{t ? t('选择目标环境') : '选择目标环境'}：</Text>
          <CheckboxGroup
            value={selectedEnvIds}
            onChange={setSelectedEnvIds}
            direction="vertical"
            style={{ marginTop: 8 }}
          >
            {environments.map((env) => (
              <Checkbox key={env.id} value={env.id}>
                <Space>
                  <Text>{env.name}</Text>
                  <Text type="tertiary" size="small">
                    ({env.api_url})
                  </Text>
                </Space>
              </Checkbox>
            ))}
          </CheckboxGroup>
        </div>
      )}
    </div>
  );

  // 渲染确认步骤
  const renderConfirmStep = () => {
    const selectedEnvs = environments.filter((env) =>
      selectedEnvIds.includes(env.id)
    );
    return (
      <div>
        <Banner
          type="warning"
          icon={<IconAlertTriangle />}
          description={t ? t('请确认以下同步操作，确认后将立即执行') : '请确认以下同步操作，确认后将立即执行'}
          style={{ marginBottom: 16 }}
        />

        <div style={{ marginBottom: 16 }}>
          <Text strong>{t ? t('将要同步的渠道') : '将要同步的渠道'}：</Text>
          <div style={{ marginTop: 8 }}>
            {selectedChannels.map((ch) => (
              <Tag key={ch.id} color="blue" style={{ marginRight: 8, marginBottom: 4 }}>
                {ch.name}
              </Tag>
            ))}
          </div>
        </div>

        <div>
          <Text strong>{t ? t('目标环境') : '目标环境'}：</Text>
          <div style={{ marginTop: 8 }}>
            {selectedEnvs.map((env) => (
              <Tag key={env.id} color="green" style={{ marginRight: 8, marginBottom: 4 }}>
                {env.name}
              </Tag>
            ))}
          </div>
        </div>
      </div>
    );
  };

  // 渲染结果步骤
  const renderResultStep = () => (
    <div>
      <List
        dataSource={syncResults || []}
        renderItem={(result) => (
          <List.Item
            header={
              result.success ? (
                <IconTick style={{ color: 'var(--semi-color-success)' }} />
              ) : (
                <IconClose style={{ color: 'var(--semi-color-danger)' }} />
              )
            }
            main={
              <div>
                <Text strong>{result.environment_name}</Text>
                {result.success ? (
                  <Text type="success" style={{ marginLeft: 8 }}>
                    {t ? t('同步成功') : '同步成功'}，{t ? t('共同步') : '共同步'} {result.synced_count} {t ? t('个渠道') : '个渠道'}
                  </Text>
                ) : (
                  <Text type="danger" style={{ marginLeft: 8 }}>
                    {t ? t('同步失败') : '同步失败'}：{result.error}
                  </Text>
                )}
                {result.details && result.details.length > 0 && (
                  <div style={{ marginTop: 8 }}>
                    {result.details.map((detail, idx) => (
                      <div key={idx}>
                        <Text type={detail.success ? 'success' : 'danger'}>
                          {detail.channel_name}: {detail.action === 'created' ? (t ? t('创建') : '创建') : (t ? t('更新') : '更新')}
                          {detail.success ? (t ? t('成功') : '成功') : (t ? t('失败') : '失败')}
                          {detail.error && ` - ${detail.error}`}
                        </Text>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            }
          />
        )}
      />
    </div>
  );

  // 获取弹窗标题
  const getTitle = () => {
    switch (step) {
      case 'select':
        return t ? t('同步渠道到其他环境') : '同步渠道到其他环境';
      case 'confirm':
        return t ? t('确认同步') : '确认同步';
      case 'result':
        return t ? t('同步结果') : '同步结果';
      default:
        return '';
    }
  };

  // 获取弹窗底部按钮
  const getFooter = () => {
    switch (step) {
      case 'select':
        return (
          <Space>
            <button className="semi-button semi-button-tertiary" onClick={onCancel}>
              {t ? t('取消') : '取消'}
            </button>
            <button
              className="semi-button semi-button-primary"
              disabled={selectedEnvIds.length === 0}
              onClick={() => setStep('confirm')}
            >
              {t ? t('下一步') : '下一步'}
            </button>
          </Space>
        );
      case 'confirm':
        return (
          <Space>
            <button className="semi-button semi-button-tertiary" onClick={() => setStep('select')}>
              {t ? t('上一步') : '上一步'}
            </button>
            <button
              className="semi-button semi-button-warning"
              onClick={handleSync}
              disabled={syncing}
            >
              {syncing ? (t ? t('同步中...') : '同步中...') : (t ? t('确认同步') : '确认同步')}
            </button>
          </Space>
        );
      case 'result':
        return (
          <button className="semi-button semi-button-primary" onClick={onCancel}>
            {t ? t('完成') : '完成'}
          </button>
        );
      default:
        return null;
    }
  };

  return (
    <Modal
      title={getTitle()}
      visible={visible}
      onCancel={step === 'result' ? onCancel : undefined}
      footer={getFooter()}
      width={600}
      closable={step !== 'confirm' || !syncing}
    >
      {step === 'select' && renderSelectStep()}
      {step === 'confirm' && renderConfirmStep()}
      {step === 'result' && renderResultStep()}
    </Modal>
  );
};

export default SyncChannelsModal;
