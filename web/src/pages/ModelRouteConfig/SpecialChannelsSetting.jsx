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

import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Table,
  Button,
  Space,
  Tag,
  Typography,
  Card,
  Avatar,
  Popconfirm,
  Banner,
  Popover,
  Select,
  Spin,
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconEdit,
  IconDelete,
  IconServer,
  IconChevronDown,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../helpers';
import SpecialChannelEditModal from './SpecialChannelEditModal';

const { Text, Title } = Typography;

// 需要管理的 options 键。这些键的值均为 SpecailChannels JSON 数组：
// [{ model_name, channel_ids: [][]int, random_type }]
const CHANNEL_KEYS = [
  { key: 'OnlyTextChannels', descKey: '仅文本请求（无多模态标签）使用的渠道' },
  { key: 'VideoChannels', descKey: '视频请求（video 标签）使用的渠道' },
  { key: 'OnlyImageChannels', descKey: '仅图片请求（image 标签）使用的渠道' },
  { key: 'NoVideoChannels', descKey: '非视频多模态请求使用的渠道' },
];

const SpecialChannelsSetting = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [activeKey, setActiveKey] = useState(CHANNEL_KEYS[0].key);
  // { OnlyTextChannels: [rule, ...], ... }
  const [rulesMap, setRulesMap] = useState({});
  const [channelMap, setChannelMap] = useState({});
  const [loadingChannels, setLoadingChannels] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingIndex, setEditingIndex] = useState(-1); // -1 表示新增
  const [editingRule, setEditingRule] = useState(null);

  const loadChannels = async () => {
    setLoadingChannels(true);
    try {
      const res = await API.get('/api/channel/channel-name-list');
      const { success, data } = res.data;
      if (success && data) {
        const map = {};
        data.forEach((channel) => {
          map[channel.id] = channel.name;
        });
        setChannelMap(map);
      }
    } catch (error) {
      console.error('加载渠道列表失败:', error);
    } finally {
      setLoadingChannels(false);
    }
  };

  // 从 options 加载四个键的值
  const loadOptions = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      const optionObj = {};
      (data || []).forEach((item) => {
        optionObj[item.key] = item.value;
      });
      const parsed = {};
      CHANNEL_KEYS.forEach(({ key }) => {
        let arr = [];
        const raw = optionObj[key];
        if (raw) {
          try {
            const v = JSON.parse(raw);
            if (Array.isArray(v)) {
              arr = v;
            }
          } catch (e) {
            console.error(`解析 ${key} 失败:`, e);
          }
        }
        parsed[key] = arr;
      });
      setRulesMap(parsed);
    } catch (error) {
      showError(t('加载配置失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadChannels();
    loadOptions();
  }, []);

  // 保存某个键的规则数组到 options
  const saveKey = async (key, rules) => {
    const res = await API.put('/api/option/', {
      key,
      value: JSON.stringify(rules),
    });
    const { success, message } = res.data;
    if (!success) {
      throw new Error(message || t('保存失败'));
    }
  };

  const currentRules = rulesMap[activeKey] || [];

  const handleAdd = () => {
    setEditingIndex(-1);
    setEditingRule(null);
    setEditModalVisible(true);
  };

  const handleEdit = (rule, index) => {
    setEditingIndex(index);
    setEditingRule(rule);
    setEditModalVisible(true);
  };

  const handleModalSubmit = async (rule) => {
    const nextRules = [...currentRules];
    if (editingIndex >= 0) {
      nextRules[editingIndex] = rule;
    } else {
      nextRules.push(rule);
    }
    try {
      await saveKey(activeKey, nextRules);
      setRulesMap({ ...rulesMap, [activeKey]: nextRules });
      setEditModalVisible(false);
      setEditingRule(null);
      setEditingIndex(-1);
      showSuccess(t('保存成功'));
    } catch (error) {
      showError(error.message || t('保存失败'));
    }
  };

  const handleDelete = async (index) => {
    const nextRules = currentRules.filter((_, i) => i !== index);
    try {
      await saveKey(activeKey, nextRules);
      setRulesMap({ ...rulesMap, [activeKey]: nextRules });
      showSuccess(t('删除成功'));
    } catch (error) {
      showError(error.message || t('删除失败'));
    }
  };

  const renderChannelGroupsPopover = (channelGroups) => {
    if (!channelGroups || channelGroups.length === 0) {
      return <Text type='tertiary'>{t('无')}</Text>;
    }

    const content = (
      <div className='max-w-lg'>
        <div className='mb-2'>
          <Text strong>{t('渠道组配置')}</Text>
        </div>
        {loadingChannels ? (
          <div className='flex justify-center py-4'>
            <Spin />
          </div>
        ) : (
          <div className='space-y-3'>
            {channelGroups.map((group, groupIndex) => (
              <div key={groupIndex} className='p-3 bg-gray-50 rounded'>
                <div className='mb-2'>
                  <Tag color='blue' size='small'>
                    {t('渠道组')} {groupIndex + 1}
                  </Tag>
                  <Text type='tertiary' size='small' className='ml-2'>
                    {group.length} {t('个渠道')}
                  </Text>
                </div>
                <div className='flex flex-wrap gap-2'>
                  {group.map((channelId) => (
                    <Tag key={channelId} color='green' size='small'>
                      {channelMap[channelId] || `ID: ${channelId}`}
                    </Tag>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    );

    return (
      <Popover content={content} position='bottomLeft' trigger='click'>
        <Button
          size='small'
          theme='borderless'
          type='tertiary'
          icon={<IconChevronDown />}
          iconPosition='right'
        >
          {channelGroups.length} {t('组')}
        </Button>
      </Popover>
    );
  };

  const columns = [
    {
      title: t('模型名称'),
      dataIndex: 'model_name',
      render: (text) => <Text code>{text}</Text>,
    },
    {
      title: t('渠道组'),
      dataIndex: 'channel_ids',
      render: (groups) => renderChannelGroupsPopover(groups),
    },
    {
      title: t('选择模式'),
      dataIndex: 'random_type',
      width: 120,
      render: (type) => (
        <Tag color={type === 'order' ? 'cyan' : 'orange'}>
          {type === 'order' ? t('顺序') : t('随机')}
        </Tag>
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'actions',
      width: 150,
      render: (_, record, index) => (
        <Space>
          <Button
            theme='borderless'
            type='primary'
            size='small'
            icon={<IconEdit />}
            onClick={() => handleEdit(record, index)}
          >
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('确定删除此规则吗？')}
            content={t('删除后无法恢复')}
            onConfirm={() => handleDelete(index)}
            position='topRight'
          >
            <Button
              theme='borderless'
              type='danger'
              size='small'
              icon={<IconDelete />}
            >
              {t('删除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const currentDesc = CHANNEL_KEYS.find((k) => k.key === activeKey)?.descKey || '';

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      {/* Header */}
      <div className='flex items-center justify-between mb-6'>
        <div className='flex items-center'>
          <Avatar size='large' color='cyan' className='mr-3 shadow-md'>
            <IconServer size='extra-large' />
          </Avatar>
          <div>
            <Title heading={3} className='m-0'>
              {t('多模态渠道配置')}
            </Title>
            <Text type='tertiary' size='small'>
              {t('按请求模态（文本 / 图片 / 视频）为模型指定专用渠道')}
            </Text>
          </div>
        </div>
      </div>

      <Banner
        type='info'
        description={t(
          '根据请求携带的模态标签，系统会优先使用此处为对应模型配置的渠道，覆盖默认的渠道路由。模型名称支持正则匹配。',
        )}
        className='!rounded-lg mb-4'
      />

      {/* Key 选择 */}
      <div className='flex items-center justify-between mb-4'>
        <Space wrap align='center'>
          <Text strong>{t('渠道类别')}:</Text>
          <Select
            value={activeKey}
            onChange={(value) => setActiveKey(value)}
            style={{ width: 240 }}
            optionList={CHANNEL_KEYS.map(({ key }) => ({
              label: t(key),
              value: key,
            }))}
          />
          <Text type='tertiary' size='small'>
            {t(currentDesc)}
          </Text>
        </Space>
        <Button
          theme='solid'
          type='primary'
          icon={<IconPlus />}
          onClick={handleAdd}
        >
          {t('添加规则')}
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={currentRules.map((r, i) => ({ ...r, _idx: i }))}
        loading={loading}
        pagination={false}
        rowKey='_idx'
      />

      <SpecialChannelEditModal
        visible={editModalVisible}
        rule={editingRule}
        onClose={() => {
          setEditModalVisible(false);
          setEditingRule(null);
          setEditingIndex(-1);
        }}
        onSubmit={handleModalSubmit}
      />
    </Card>
  );
};

export default SpecialChannelsSetting;
