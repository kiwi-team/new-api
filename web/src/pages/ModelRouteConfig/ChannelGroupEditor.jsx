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
  Button,
  Space,
  Card,
  Typography,
  Select,
  Tag,
  Banner,
  Spin,
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconDelete,
  IconChevronUp,
  IconChevronDown,
} from '@douyinfe/semi-icons';
import { API, showError } from '../../helpers';

const { Text } = Typography;

const ChannelGroupEditor = ({ value = [[]], onChange }) => {
  const { t } = useTranslation();
  const [channelOptions, setChannelOptions] = useState([]);
  const [loading, setLoading] = useState(false);
  const [groups, setGroups] = useState(value || [[]]);

  console.log('ChannelGroupEditor rendered:', { value, groups, channelOptions: channelOptions.length });

  // 加载渠道列表
  useEffect(() => {
    loadChannels();
  }, []);

  // 同步外部value变化
  useEffect(() => {
    console.log('Value changed:', value);
    if (value && Array.isArray(value)) {
      const valueStr = JSON.stringify(value);
      const groupsStr = JSON.stringify(groups);
      if (valueStr !== groupsStr) {
        console.log('Updating groups from value:', value);
        setGroups(value);
      }
    }
  }, [value]); // 移除 groups 依赖，避免循环

  const loadChannels = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/channel/channel-name-list');
      const { success, message, data } = res.data;
      
      if (success && data) {
        // 转换为 Select 组件需要的格式
        // API 返回的字段是 id (小写) 和 name
        const options = data.map(channel => ({
          label: `${channel.name} (ID: ${channel.id})`,
          value: channel.id,
          name: channel.name,
          id: channel.id,
        }));
        setChannelOptions(options);
      } else {
        showError(message || t('加载渠道列表失败'));
      }
    } catch (error) {
      console.error('加载渠道列表失败:', error);
      showError(t('加载渠道列表失败'));
    } finally {
      setLoading(false);
    }
  };

  // 更新groups并通知父组件
  const updateGroups = (newGroups) => {
    setGroups(newGroups);
    if (onChange) {
      onChange(newGroups);
    }
  };

  // 添加渠道组
  const handleAddGroup = () => {
    console.log('添加渠道组按钮被点击');
    updateGroups([...groups, []]);
  };

  // 删除渠道组
  const handleRemoveGroup = (groupIndex) => {
    if (groups.length <= 1) {
      showError(t('至少需要保留一个渠道组'));
      return;
    }
    const newGroups = groups.filter((_, index) => index !== groupIndex);
    updateGroups(newGroups);
  };

  // 添加渠道到组
  const handleAddChannel = (groupIndex, channelIds) => {
    const newGroups = [...groups];
    newGroups[groupIndex] = channelIds;
    updateGroups(newGroups);
  };

  // 移动渠道组
  const handleMoveGroup = (groupIndex, direction) => {
    const newGroups = [...groups];
    const targetIndex = direction === 'up' ? groupIndex - 1 : groupIndex + 1;
    
    if (targetIndex < 0 || targetIndex >= newGroups.length) {
      return;
    }
    
    [newGroups[groupIndex], newGroups[targetIndex]] = [newGroups[targetIndex], newGroups[groupIndex]];
    updateGroups(newGroups);
  };

  // 获取渠道名称
  const getChannelName = (channelId) => {
    const channel = channelOptions.find(opt => opt.id === channelId);
    return channel ? channel.name : `ID: ${channelId}`;
  };

  if (loading) {
    return (
      <div className='flex justify-center items-center py-8'>
        <Spin size='large' />
      </div>
    );
  }

  return (
    <div className='w-full'>
      <Banner
        type='info'
        description={
          <div>
            <Text>{t('渠道组内的渠道是等效的（随机选择），渠道组之间按配置的顺序或随机选择。')}</Text>
            <br />
            <Text type='tertiary' size='small'>
              {t('提示：可以使用搜索功能快速查找渠道')}
            </Text>
          </div>
        }
        className='!rounded-lg mb-4'
      />

      <div className='space-y-4'>
        {groups.map((group, groupIndex) => (
          <Card
            key={groupIndex}
            className='!rounded-lg shadow-sm border border-gray-200 w-full'
            bodyStyle={{ padding: '16px' }}
          >
            <div className='flex items-start justify-between mb-3'>
              <div className='flex items-center'>
                <Tag color='blue' size='large'>
                  {t('渠道组')} {groupIndex + 1}
                </Tag>
                <Text type='tertiary' size='small' className='ml-2'>
                  {group.length} {t('个渠道')}
                </Text>
              </div>
              <Space>
                <Button
                  size='small'
                  type='tertiary'
                  icon={<IconChevronUp />}
                  disabled={groupIndex === 0}
                  onClick={() => handleMoveGroup(groupIndex, 'up')}
                />
                <Button
                  size='small'
                  type='tertiary'
                  icon={<IconChevronDown />}
                  disabled={groupIndex === groups.length - 1}
                  onClick={() => handleMoveGroup(groupIndex, 'down')}
                />
                <Button
                  size='small'
                  type='danger'
                  icon={<IconDelete />}
                  onClick={() => handleRemoveGroup(groupIndex)}
                  disabled={groups.length <= 1}
                >
                  {t('删除组')}
                </Button>
              </Space>
            </div>

            <Select
              multiple
              filter
              placeholder={t('请选择渠道')}
              value={group}
              onChange={(value) => handleAddChannel(groupIndex, value)}
              optionList={channelOptions}
              style={{ width: '100%' }}
              maxTagCount={3}
              renderSelectedItem={(optionNode) => {
                const option = channelOptions.find(opt => opt.id === optionNode.value);
                return (
                  <Tag
                    key={optionNode.value}
                    color='cyan'
                    closable
                    onClose={() => {
                      const newChannels = group.filter(id => id !== optionNode.value);
                      handleAddChannel(groupIndex, newChannels);
                    }}
                  >
                    {option ? option.name : `ID: ${optionNode.value}`}
                  </Tag>
                );
              }}
            />

            {group.length > 0 && (
              <div className='mt-3'>
                <Text type='tertiary' size='small'>
                  {t('已选择的渠道')}:
                </Text>
                <div className='flex flex-wrap gap-2 mt-2'>
                  {group.map((channelId) => (
                    <Tag key={channelId} color='green' size='small'>
                      {getChannelName(channelId)} (ID: {channelId})
                    </Tag>
                  ))}
                </div>
              </div>
            )}
          </Card>
        ))}
      </div>

      <Button
        type='primary'
        theme='light'
        icon={<IconPlus />}
        onClick={handleAddGroup}
        className='mt-4 w-full'
        size='large'
      >
        {t('添加渠道组')}
      </Button>
    </div>
  );
};

export default ChannelGroupEditor;
