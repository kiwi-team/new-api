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
  Input,
  Tag,
  Modal,
  Typography,
  Card,
  Avatar,
  Popconfirm,
  Switch,
  Tooltip,
  Banner,
  Popover,
  Spin,
  Select,
} from '@douyinfe/semi-ui';
import {
  IconSearch,
  IconPlus,
  IconEdit,
  IconDelete,
  IconRefresh,
  IconRoute,
  IconSetting,
  IconChevronDown,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess, showInfo } from '../../helpers';
import EditModal from './EditModal';

const { Text, Title } = Typography;

const ModelRouteConfig = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [configs, setConfigs] = useState([]);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [modelKeyword, setModelKeyword] = useState('');
  const [selectedChannelId, setSelectedChannelId] = useState(null);
  const [pagination, setPagination] = useState({
    currentPage: 1,
    pageSize: 20,
    total: 0,
  });
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingConfig, setEditingConfig] = useState(null);
  const [channelMap, setChannelMap] = useState({}); // 渠道 ID -> 名称映射
  const [channelOptions, setChannelOptions] = useState([]); // 渠道选项列表
  const [loadingChannels, setLoadingChannels] = useState(false);

  // 加载渠道列表
  const loadChannels = async () => {
    setLoadingChannels(true);
    try {
      const res = await API.get('/api/channel/channel-name-list');
      const { success, data } = res.data;
      
      if (success && data) {
        // 创建 ID -> 名称的映射
        const map = {};
        const options = [];
        data.forEach(channel => {
          map[channel.id] = channel.name;
          options.push({
            label: `${channel.name} (ID: ${channel.id})`,
            value: channel.id,
          });
        });
        setChannelMap(map);
        setChannelOptions(options);
      }
    } catch (error) {
      console.error('加载渠道列表失败:', error);
    } finally {
      setLoadingChannels(false);
    }
  };

  // 加载配置列表
  const loadConfigs = async (page = 1, keyword = '', modelKw = '', channelId = null) => {
    setLoading(true);
    try {
      // 构建查询参数
      const params = new URLSearchParams({
        p: page,
        page_size: pagination.pageSize,
      });
      
      if (keyword) {
        params.append('keyword', keyword);
      }
      if (modelKw) {
        params.append('model_keyword', modelKw);
      }
      if (channelId) {
        params.append('channel_id', channelId);
      }
      
      const url = `/api/model_route_config/search?${params.toString()}`;
      
      const res = await API.get(url);
      const { success, message, data } = res.data;
      
      if (success) {
        setConfigs(data.items || []);
        setPagination({
          ...pagination,
          currentPage: page,
          total: data.total || 0,
        });
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('加载配置列表失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadChannels();
    loadConfigs();
  }, []);

  // 搜索
  const handleSearch = () => {
    loadConfigs(1, searchKeyword, modelKeyword, selectedChannelId);
  };

  // 刷新
  const handleRefresh = () => {
    setSearchKeyword('');
    setModelKeyword('');
    setSelectedChannelId(null);
    loadConfigs(1, '', '', null);
  };

  // 添加配置
  const handleAdd = () => {
    setEditingConfig(null);
    setEditModalVisible(true);
  };

  // 编辑配置
  const handleEdit = (config) => {
    setEditingConfig(config);
    setEditModalVisible(true);
  };

  // 删除配置
  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/model_route_config/${id}`);
      const { success, message } = res.data;
      
      if (success) {
        showSuccess(t('删除成功'));
        loadConfigs(pagination.currentPage, searchKeyword, modelKeyword, selectedChannelId);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('删除失败'));
    }
  };

  // 切换启用状态
  const handleToggleStatus = async (config) => {
    try {
      const res = await API.post('/api/model_route_config/status', {
        id: config.id,
        enabled: config.enabled === 1 ? 0 : 1,
      });
      const { success, message } = res.data;
      
      if (success) {
        showSuccess(t('状态更新成功'));
        loadConfigs(pagination.currentPage, searchKeyword, modelKeyword, selectedChannelId);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('状态更新失败'));
    }
  };

  // 渲染匹配规则详情
  const renderPatternsPopover = (patterns, title, color) => {
    if (!patterns || patterns.length === 0) {
      return <Text type='tertiary'>{t('无')}</Text>;
    }

    const content = (
      <div className='max-w-md'>
        <div className='mb-2'>
          <Text strong>{title}</Text>
        </div>
        <div className='space-y-1'>
          {patterns.map((pattern, index) => (
            <div key={index} className='p-2 bg-gray-50 rounded'>
              <Text code>{pattern}</Text>
            </div>
          ))}
        </div>
      </div>
    );

    return (
      <Popover
        content={content}
        position='bottomLeft'
        trigger='click'
      >
        <Tag 
          color={color} 
          size='small' 
          className='cursor-pointer hover:opacity-80'
        >
          {title}: {patterns.length}
          <IconChevronDown className='ml-1' size='small' />
        </Tag>
      </Popover>
    );
  };

  // 渲染渠道组详情
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
      <Popover
        content={content}
        position='bottomLeft'
        trigger='click'
      >
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

  // 表格列定义
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: t('配置名称'),
      dataIndex: 'name',
      render: (text, record) => (
        <div>
          <Text strong>{text}</Text>
          {record.enabled === 0 && (
            <Tag color='red' size='small' className='ml-2'>
              {t('已禁用')}
            </Tag>
          )}
        </div>
      ),
    },
    {
      title: t('匹配规则'),
      dataIndex: 'rules',
      render: (_, record) => {
        const hasRules = 
          (record.model_patterns && record.model_patterns.length > 0) ||
          (record.body_patterns && record.body_patterns.length > 0) ||
          (record.url_patterns && record.url_patterns.length > 0);

        if (!hasRules) {
          return <Text type='tertiary'>{t('无')}</Text>;
        }

        return (
          <Space wrap>
            {record.model_patterns && record.model_patterns.length > 0 && 
              renderPatternsPopover(record.model_patterns, t('模型'), 'blue')}
            {record.body_patterns && record.body_patterns.length > 0 && 
              renderPatternsPopover(record.body_patterns, t('Body'), 'green')}
            {record.url_patterns && record.url_patterns.length > 0 && 
              renderPatternsPopover(record.url_patterns, t('URL'), 'purple')}
          </Space>
        );
      },
    },
    {
      title: t('渠道组'),
      dataIndex: 'channel_groups',
      render: (groups) => renderChannelGroupsPopover(groups),
    },
    {
      title: t('选择模式'),
      dataIndex: 'random_type',
      render: (type) => (
        <Tag color={type === 'order' ? 'cyan' : 'orange'}>
          {type === 'order' ? t('顺序') : t('随机')}
        </Tag>
      ),
    },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      sorter: (a, b) => a.priority - b.priority,
      render: (priority) => (
        <Tag color='violet'>{priority}</Tag>
      ),
    },
    {
      title: t('重试次数'),
      dataIndex: 'max_retry',
      width: 100,
    },
    {
      title: t('状态'),
      dataIndex: 'enabled',
      width: 100,
      render: (enabled, record) => (
        <Switch
          checked={enabled === 1}
          onChange={() => handleToggleStatus(record)}
        />
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'actions',
      width: 150,
      render: (_, record) => (
        <Space>
          <Button
            theme='borderless'
            type='primary'
            size='small'
            icon={<IconEdit />}
            onClick={() => handleEdit(record)}
          >
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('确定删除此配置吗？')}
            content={t('删除后无法恢复')}
            onConfirm={() => handleDelete(record.id)}
            position='topRight'
            style={{ width: 300 }}
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

  return (
    <div className='mt-[60px] px-4'>
      <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
        {/* Header */}
        <div className='flex items-center justify-between mb-6'>
          <div className='flex items-center'>
            <Avatar
              size='large'
              color='blue'
              className='mr-3 shadow-md'
            >
              <IconRoute size='extra-large' />
            </Avatar>
            <div>
              <Title heading={3} className='m-0'>
                {t('模型路由配置')}
              </Title>
              <Text type='tertiary' size='small'>
                {t('配置模型请求的渠道路由规则')}
              </Text>
            </div>
          </div>
        </div>

        {/* Banner */}
        <Banner
          type='info'
          description={t('模型路由配置允许您根据模型名称、请求体内容、URL路径等条件，将请求路由到指定的渠道组。配置按优先级从高到低匹配。')}
          className='!rounded-lg mb-4'
        />

        {/* Toolbar */}
        <div className='mb-4'>
          <div className='flex items-center justify-between mb-3'>
            <Space wrap>
              <Input
                prefix={<IconSearch />}
                placeholder={t('搜索配置名称')}
                value={searchKeyword}
                onChange={(value) => setSearchKeyword(value)}
                onEnterPress={handleSearch}
                style={{ width: 200 }}
                showClear
              />
              <Input
                placeholder={t('模型关键词')}
                value={modelKeyword}
                onChange={(value) => setModelKeyword(value)}
                onEnterPress={handleSearch}
                style={{ width: 200 }}
                showClear
              />
              <Select
                placeholder={t('选择渠道')}
                value={selectedChannelId}
                onChange={(value) => setSelectedChannelId(value)}
                style={{ width: 250 }}
                filter
                showClear
                optionList={channelOptions}
                loading={loadingChannels}
              />
              <Button
                type='primary'
                onClick={handleSearch}
                icon={<IconSearch />}
              >
                {t('搜索')}
              </Button>
              <Button
                onClick={handleRefresh}
                icon={<IconRefresh />}
              >
                {t('重置')}
              </Button>
            </Space>
            <Button
              theme='solid'
              type='primary'
              icon={<IconPlus />}
              onClick={handleAdd}
            >
              {t('添加配置')}
            </Button>
          </div>
        </div>

        {/* Table */}
        <Table
          columns={columns}
          dataSource={configs}
          loading={loading}
          pagination={{
            currentPage: pagination.currentPage,
            pageSize: pagination.pageSize,
            total: pagination.total,
            onPageChange: (page) => loadConfigs(page, searchKeyword, modelKeyword, selectedChannelId),
          }}
          rowKey='id'
        />
      </Card>

      {/* Edit Modal */}
      <EditModal
        visible={editModalVisible}
        config={editingConfig}
        onClose={() => {
          setEditModalVisible(false);
          setEditingConfig(null);
        }}
        onSuccess={() => {
          setEditModalVisible(false);
          setEditingConfig(null);
          loadConfigs(pagination.currentPage, searchKeyword, modelKeyword, selectedChannelId);
        }}
      />
    </div>
  );
};

export default ModelRouteConfig;
