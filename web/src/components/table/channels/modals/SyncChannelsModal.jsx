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
  Select,
  Divider,
} from '@douyinfe/semi-ui';
import { IconAlertTriangle, IconTick, IconClose, IconChevronDown, IconChevronUp } from '@douyinfe/semi-icons';
import { API } from '../../../../helpers/api';

const { Text } = Typography;

const SyncChannelsModal = ({ visible, onCancel, selectedChannels, t }) => {
  const [environments, setEnvironments] = useState([]);
  const [selectedEnvIds, setSelectedEnvIds] = useState([]);
  const [loading, setLoading] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [syncResults, setSyncResults] = useState(null);
  const [previewData, setPreviewData] = useState(null); // preview API response
  // channelMapping: { envId: { sourceChannelId: targetChannelId } }
  // targetChannelId = 0 means create new
  const [channelMapping, setChannelMapping] = useState({});
  const [step, setStep] = useState('select'); // select, mapping, confirm, result
  const [expandedEnvs, setExpandedEnvs] = useState({});

  const _t = (key) => (t ? t(key) : key);

  // 加载已启用的环境列表
  const loadEnvironments = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/sync/environments/enabled');
      if (res.data.success) {
        setEnvironments(res.data.data || []);
      } else {
        Toast.error(res.data.message || _t('加载环境列表失败'));
      }
    } catch (error) {
      Toast.error(_t('加载环境列表失败'));
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
      setPreviewData(null);
      setChannelMapping({});
      setExpandedEnvs({});
    }
  }, [visible]);

  // 加载预览数据
  const loadPreview = async () => {
    setPreviewing(true);
    try {
      const res = await API.post('/api/sync/channels/preview', {
        channel_ids: selectedChannels.map((ch) => ch.id),
        environment_ids: selectedEnvIds,
      });
      if (res.data.success) {
        const previews = res.data.data || [];
        setPreviewData(previews);

        // 自动初始化 channelMapping
        const mapping = {};
        const expanded = {};
        for (const envPreview of previews) {
          const envId = envPreview.environment_id;
          mapping[envId] = {};
          expanded[envId] = true;
          for (const ch of envPreview.channels || []) {
            if (ch.matches && ch.matches.length === 1) {
              // 只有一个匹配，自动选中
              mapping[envId][ch.channel_id] = ch.matches[0].id;
            } else if (!ch.matches || ch.matches.length === 0) {
              // 无匹配，默认创建
              mapping[envId][ch.channel_id] = 0;
            } else {
              // 多个匹配，默认不选（需要用户选择）
              mapping[envId][ch.channel_id] = undefined;
            }
          }
        }
        setChannelMapping(mapping);
        setExpandedEnvs(expanded);
        setStep('mapping');
      } else {
        Toast.error(res.data.message || _t('预览失败'));
      }
    } catch (error) {
      Toast.error(_t('预览请求失败'));
    } finally {
      setPreviewing(false);
    }
  };

  // 检查映射是否完整（所有多匹配的渠道都已选择）
  const isMappingComplete = () => {
    if (!previewData) return false;
    for (const envPreview of previewData) {
      const envId = envPreview.environment_id;
      const envMapping = channelMapping[envId];
      if (!envMapping) return false;
      for (const ch of envPreview.channels || []) {
        if (envMapping[ch.channel_id] === undefined) return false;
      }
    }
    return true;
  };

  // 更新单个渠道的映射
  const updateMapping = (envId, sourceChannelId, targetChannelId) => {
    setChannelMapping((prev) => ({
      ...prev,
      [envId]: {
        ...prev[envId],
        [sourceChannelId]: targetChannelId,
      },
    }));
  };

  // 执行同步
  const handleSync = async () => {
    setSyncing(true);
    try {
      // 构建 channel_mapping: { envId: { sourceChId: targetChId } }
      // 过滤掉 undefined 值，将 number 类型传递
      const mappingPayload = {};
      for (const [envId, envMap] of Object.entries(channelMapping)) {
        mappingPayload[envId] = {};
        for (const [srcId, tgtId] of Object.entries(envMap)) {
          if (tgtId !== undefined) {
            mappingPayload[envId][srcId] = tgtId;
          }
        }
      }

      const res = await API.post('/api/sync/channels', {
        channel_ids: selectedChannels.map((ch) => ch.id),
        environment_ids: selectedEnvIds,
        channel_mapping: mappingPayload,
      });
      if (res.data.success) {
        setSyncResults(res.data.results);
        setStep('result');
      } else {
        Toast.error(res.data.message || _t('同步失败'));
      }
    } catch (error) {
      Toast.error(_t('同步请求失败'));
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
        description={_t('请选择要同步到的目标环境，同步操作将覆盖目标环境中相同key+类型的渠道配置')}
        style={{ marginBottom: 16 }}
      />

      <div style={{ marginBottom: 16 }}>
        <Text strong>{_t('已选择的渠道')}：</Text>
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
          description={_t('暂无可用的同步环境，请先在环境管理中添加并启用环境')}
        />
      ) : (
        <div>
          <Text strong>{_t('选择目标环境')}：</Text>
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

  // 渲染渠道映射步骤
  const renderMappingStep = () => {
    if (!previewData) return <Spin />;

    return (
      <div>
        <Banner
          type="info"
          description={_t('请为每个渠道选择要同步到的目标渠道。如果目标环境没有匹配的渠道，将自动创建新渠道。')}
          style={{ marginBottom: 16 }}
        />

        {previewData.map((envPreview) => {
          const envId = envPreview.environment_id;
          const isExpanded = expandedEnvs[envId] !== false;

          if (envPreview.error) {
            return (
              <div key={envId} style={{ marginBottom: 16 }}>
                <Text strong>{envPreview.environment_name}</Text>
                <Text type="danger" style={{ marginLeft: 8 }}>{envPreview.error}</Text>
              </div>
            );
          }

          return (
            <div key={envId} style={{ marginBottom: 16 }}>
              <div
                style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', marginBottom: 8 }}
                onClick={() => setExpandedEnvs((prev) => ({ ...prev, [envId]: !isExpanded }))}
              >
                {isExpanded ? <IconChevronUp size="small" /> : <IconChevronDown size="small" />}
                <Text strong style={{ marginLeft: 4 }}>{envPreview.environment_name}</Text>
                <Tag color="blue" size="small" style={{ marginLeft: 8 }}>
                  {(envPreview.channels || []).length} {_t('个渠道')}
                </Tag>
              </div>

              {isExpanded && (
                <div style={{ paddingLeft: 20 }}>
                  {(envPreview.channels || []).map((ch) => {
                    const hasMatches = ch.matches && ch.matches.length > 0;
                    const multipleMatches = ch.matches && ch.matches.length > 1;
                    const currentValue = channelMapping[envId]?.[ch.channel_id];

                    return (
                      <div
                        key={ch.channel_id}
                        style={{
                          display: 'flex',
                          alignItems: 'flex-start',
                          marginBottom: 12,
                          padding: '8px 12px',
                          backgroundColor: 'var(--semi-color-fill-0)',
                          borderRadius: 6,
                        }}
                      >
                        {/* 源渠道信息 */}
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <Text strong>{ch.channel_name}</Text>
                          <div style={{ marginTop: 4 }}>
                            <Text type="tertiary" size="small" style={{ wordBreak: 'break-all' }}>
                              {ch.models}
                            </Text>
                          </div>
                        </div>

                        <div style={{ margin: '0 12px', display: 'flex', alignItems: 'center', color: 'var(--semi-color-text-2)' }}>
                          →
                        </div>

                        {/* 目标渠道选择 */}
                        <div style={{ flex: 1, minWidth: 0 }}>
                          {!hasMatches ? (
                            <Tag color="green" size="small">{_t('新建渠道')}</Tag>
                          ) : multipleMatches ? (
                            <Select
                              size="small"
                              style={{ width: '100%' }}
                              value={currentValue}
                              onChange={(val) => updateMapping(envId, ch.channel_id, val)}
                              placeholder={_t('请选择目标渠道')}
                              renderOptionItem={({ disabled, selected, label, value, focused, className, style, onMouseEnter, onClick }) => {
                                // 从 matches 中查找模型信息
                                const matchInfo = value > 0 ? ch.matches.find((m) => m.id === value) : null;
                                return (
                                  <div
                                    style={{ ...style, padding: '6px 12px', cursor: disabled ? 'not-allowed' : 'pointer' }}
                                    className={className}
                                    onClick={() => !disabled && onClick()}
                                    onMouseEnter={() => onMouseEnter()}
                                  >
                                    <div>
                                      <Text strong={selected}>{label}</Text>
                                    </div>
                                    {matchInfo && matchInfo.models && (
                                      <Text type="tertiary" size="small" style={{ wordBreak: 'break-all' }}>
                                        {matchInfo.models}
                                      </Text>
                                    )}
                                  </div>
                                );
                              }}
                              optionList={[
                                { value: 0, label: `✨ ${_t('新建渠道')}` },
                                ...ch.matches.map((m) => ({
                                  value: m.id,
                                  label: m.name,
                                })),
                              ]}
                            />
                          ) : (
                            // 单个匹配，直接显示
                            <div>
                              <Tag color="blue" size="small">{_t('更新')}</Tag>
                              <Text size="small" style={{ marginLeft: 4 }}>{ch.matches[0].name}</Text>
                              <div style={{ marginTop: 4 }}>
                                <Text type="tertiary" size="small" style={{ wordBreak: 'break-all' }}>
                                  {ch.matches[0].models}
                                </Text>
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}

              <Divider margin="8px" />
            </div>
          );
        })}
      </div>
    );
  };

  // 渲染确认步骤
  const renderConfirmStep = () => {
    const selectedEnvs = environments.filter((env) =>
      selectedEnvIds.includes(env.id)
    );

    // 统计各环境的创建/更新数量
    const getEnvStats = (envId) => {
      const envMapping = channelMapping[envId] || {};
      let createCount = 0;
      let updateCount = 0;
      for (const tgtId of Object.values(envMapping)) {
        if (tgtId === 0) createCount++;
        else if (tgtId > 0) updateCount++;
      }
      return { createCount, updateCount };
    };

    return (
      <div>
        <Banner
          type="warning"
          icon={<IconAlertTriangle />}
          description={_t('请确认以下同步操作，确认后将立即执行')}
          style={{ marginBottom: 16 }}
        />

        <div style={{ marginBottom: 16 }}>
          <Text strong>{_t('将要同步的渠道')}：</Text>
          <div style={{ marginTop: 8 }}>
            {selectedChannels.map((ch) => (
              <Tag key={ch.id} color="blue" style={{ marginRight: 8, marginBottom: 4 }}>
                {ch.name}
              </Tag>
            ))}
          </div>
        </div>

        <div>
          <Text strong>{_t('目标环境')}：</Text>
          <div style={{ marginTop: 8 }}>
            {selectedEnvs.map((env) => {
              const stats = getEnvStats(env.id);
              return (
                <div key={env.id} style={{ marginBottom: 8 }}>
                  <Tag color="green" style={{ marginRight: 8 }}>
                    {env.name}
                  </Tag>
                  <Text type="tertiary" size="small">
                    {stats.createCount > 0 && `${_t('新建')} ${stats.createCount} ${_t('个')}`}
                    {stats.createCount > 0 && stats.updateCount > 0 && '，'}
                    {stats.updateCount > 0 && `${_t('更新')} ${stats.updateCount} ${_t('个')}`}
                  </Text>
                </div>
              );
            })}
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
                    {_t('同步成功')}，{_t('共同步')} {result.synced_count} {_t('个渠道')}
                  </Text>
                ) : (
                  <Text type="danger" style={{ marginLeft: 8 }}>
                    {_t('同步失败')}：{result.error}
                  </Text>
                )}
                {result.details && result.details.length > 0 && (
                  <div style={{ marginTop: 8 }}>
                    {result.details.map((detail, idx) => (
                      <div key={idx}>
                        <Text type={detail.success ? 'success' : 'danger'}>
                          {detail.channel_name}: {detail.action === 'created' ? _t('创建') : _t('更新')}
                          {detail.success ? _t('成功') : _t('失败')}
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
        return _t('同步渠道到其他环境');
      case 'mapping':
        return _t('渠道映射');
      case 'confirm':
        return _t('确认同步');
      case 'result':
        return _t('同步结果');
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
              {_t('取消')}
            </button>
            <button
              className="semi-button semi-button-primary"
              disabled={selectedEnvIds.length === 0 || previewing}
              onClick={loadPreview}
            >
              {previewing ? _t('加载中...') : _t('下一步')}
            </button>
          </Space>
        );
      case 'mapping':
        return (
          <Space>
            <button className="semi-button semi-button-tertiary" onClick={() => setStep('select')}>
              {_t('上一步')}
            </button>
            <button
              className="semi-button semi-button-primary"
              disabled={!isMappingComplete()}
              onClick={() => setStep('confirm')}
            >
              {_t('下一步')}
            </button>
          </Space>
        );
      case 'confirm':
        return (
          <Space>
            <button className="semi-button semi-button-tertiary" onClick={() => setStep('mapping')}>
              {_t('上一步')}
            </button>
            <button
              className="semi-button semi-button-warning"
              onClick={handleSync}
              disabled={syncing}
            >
              {syncing ? _t('同步中...') : _t('确认同步')}
            </button>
          </Space>
        );
      case 'result':
        return (
          <button className="semi-button semi-button-primary" onClick={onCancel}>
            {_t('完成')}
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
      onCancel={syncing ? undefined : onCancel}
      footer={getFooter()}
      width={700}
      maskClosable={false}
      closable={!syncing}
    >
      {step === 'select' && renderSelectStep()}
      {step === 'mapping' && renderMappingStep()}
      {step === 'confirm' && renderConfirmStep()}
      {step === 'result' && renderResultStep()}
    </Modal>
  );
};

export default SyncChannelsModal;
