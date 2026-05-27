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
import { useTranslation } from 'react-i18next';
import {
  Modal,
  Form,
  Button,
  Space,
  Typography,
  Card,
  Avatar,
  Banner,
  TagInput,
  InputNumber,
  Switch,
  Divider,
} from '@douyinfe/semi-ui';
import {
  IconSave,
  IconClose,
  IconRoute,
  IconSetting,
  IconServer,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess, showInfo } from '../../helpers';
import ChannelGroupEditor from './ChannelGroupEditor';

const { Text, Title } = Typography;

const EditModal = ({ visible, config, onClose, onSuccess }) => {
  const { t } = useTranslation();
  const formApiRef = useRef(null);
  const [loading, setLoading] = useState(false);
  const [channelGroups, setChannelGroups] = useState([[]]);
  const [formKey, setFormKey] = useState(0); // 添加 key 来强制重新渲染
  const isEdit = config !== null;

  const defaultValues = {
    name: '',
    model_patterns: [],
    body_patterns: [],
    url_patterns: [],
    channel_groups: [[]],
    random_type: 'order',
    max_retry: 3,
    priority: 0,
    enabled: true,
  };

  // 准备初始值
  const getInitialValues = () => {
    if (isEdit && config) {
      return {
        name: config.name || '',
        model_patterns: config.model_patterns || [],
        body_patterns: config.body_patterns || [],
        url_patterns: config.url_patterns || [],
        random_type: config.random_type || 'order',
        max_retry: config.max_retry || 3,
        priority: config.priority || 0,
        enabled: config.enabled === 1,
      };
    }
    return defaultValues;
  };

  useEffect(() => {
    if (visible) {
      console.log('EditModal: visible changed', { isEdit, config });
      console.log('Config details:', {
        name: config?.name,
        model_patterns: config?.model_patterns,
        body_patterns: config?.body_patterns,
        url_patterns: config?.url_patterns,
        channel_groups: config?.channel_groups,
        random_type: config?.random_type,
        max_retry: config?.max_retry,
        priority: config?.priority,
        enabled: config?.enabled,
      });
      
      // 重置表单 key 以强制重新渲染
      setFormKey(prev => {
        const newKey = prev + 1;
        console.log('Form key updated:', prev, '->', newKey);
        return newKey;
      });
      
      // 设置渠道组
      if (isEdit && config) {
        console.log('Setting channel groups:', config.channel_groups);
        setChannelGroups(config.channel_groups || [[]]);
      } else {
        console.log('Resetting channel groups to default');
        setChannelGroups([[]]);
      }
      
      // 延迟设置表单值，确保 Form 组件已经重新渲染
      setTimeout(() => {
        if (formApiRef.current && isEdit && config) {
          const values = getInitialValues();
          console.log('Setting form values via formApi:', values);
          formApiRef.current.setValues(values);
        }
      }, 100);
    }
  }, [visible, config, isEdit]);

  const handleSubmit = async (values) => {
    console.log('handleSubmit called with values:', values);
    
    // 使用 state 中的 channelGroups
    const submitValues = {
      ...values,
      channel_groups: channelGroups,
    };
    
    console.log('submitValues:', submitValues);
    
    // 验证至少配置一个匹配规则
    // 使用更健壮的检查方式，处理 undefined 和空数组的情况
    const hasModelPattern = Array.isArray(submitValues.model_patterns) && submitValues.model_patterns.length > 0;
    const hasBodyPattern = Array.isArray(submitValues.body_patterns) && submitValues.body_patterns.length > 0;
    const hasUrlPattern = Array.isArray(submitValues.url_patterns) && submitValues.url_patterns.length > 0;

    console.log('Validation:', {
      hasModelPattern,
      hasBodyPattern,
      hasUrlPattern,
      model_patterns: submitValues.model_patterns,
      body_patterns: submitValues.body_patterns,
      url_patterns: submitValues.url_patterns,
    });

    if (!hasModelPattern && !hasBodyPattern && !hasUrlPattern) {
      showInfo(t('请至少配置一个匹配规则（模型名称、请求体关键词或URL路径）'));
      return;
    }

    // 验证渠道组配置
    if (!channelGroups || channelGroups.length === 0) {
      showInfo(t('请至少配置一个渠道组'));
      return;
    }

    // 验证每个渠道组至少有一个渠道
    const hasEmptyGroup = channelGroups.some(group => !group || group.length === 0);
    if (hasEmptyGroup) {
      showInfo(t('每个渠道组至少需要包含一个渠道'));
      return;
    }

    setLoading(true);
    try {
      const payload = {
        ...submitValues,
        enabled: submitValues.enabled ? 1 : 0,
      };

      if (isEdit) {
        payload.id = config.id;
      }

      const res = isEdit
        ? await API.put('/api/model_route_config', payload)
        : await API.post('/api/model_route_config', payload);

      const { success, message } = res.data;

      if (success) {
        showSuccess(isEdit ? t('更新成功') : t('创建成功'));
        onSuccess();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(isEdit ? t('更新失败') : t('创建失败'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={
        <Space>
          <Avatar size='small' color='blue'>
            <IconRoute />
          </Avatar>
          <Title heading={4} className='m-0'>
            {isEdit ? t('编辑路由配置') : t('新建路由配置')}
          </Title>
        </Space>
      }
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={800}
      bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
    >
      <Form
        key={formKey}
        getFormApi={(api) => {
          formApiRef.current = api;
          console.log('Form API initialized, key:', formKey);
          console.log('Form initial values:', getInitialValues());
        }}
        onSubmit={handleSubmit}
        labelPosition='left'
        labelAlign='right'
        labelWidth={120}
        initValues={getInitialValues()}
      >
        {/* 基本信息 */}
        <Card className='!rounded-lg shadow-sm border-0 mb-4'>
          <div className='flex items-center mb-4'>
            <Avatar size='small' color='blue' className='mr-2'>
              <IconSetting size={16} />
            </Avatar>
            <Text strong>{t('基本信息')}</Text>
          </div>

          <Form.Input
            field='name'
            label={t('配置名称')}
            placeholder={t('请输入配置名称')}
            rules={[{ required: true, message: t('请输入配置名称') }]}
            showClear
          />

          <Form.InputNumber
            field='priority'
            label={t('优先级')}
            placeholder={t('数字越大优先级越高')}
            rules={[{ required: true, message: t('请输入优先级') }]}
            min={0}
            style={{ width: '100%' }}
          />

          <Form.Switch
            field='enabled'
            label={t('启用状态')}
          />
        </Card>

        {/* 匹配规则 */}
        <Card className='!rounded-lg shadow-sm border-0 mb-4'>
          <div className='flex items-center mb-4'>
            <Avatar size='small' color='green' className='mr-2'>
              <IconRoute size={16} />
            </Avatar>
            <Text strong>{t('匹配规则')}</Text>
            <Text type='tertiary' size='small' className='ml-2'>
              {t('（至少配置一个）')}
            </Text>
          </div>

          <Banner
            type='info'
            description={t('所有配置的匹配规则都必须满足（AND关系）。支持正则表达式。')}
            className='!rounded-lg mb-4'
          />

          <Form.TagInput
            field='model_patterns'
            label={t('模型名称匹配')}
            placeholder={t('输入正则表达式，按回车添加')}
            allowDuplicates={false}
            showClear
            helpText={t('示例：^gemini-.*-pro$')}
          />

          <Form.TagInput
            field='body_patterns'
            label={t('请求体关键词')}
            placeholder={t('输入关键词，按回车添加')}
            allowDuplicates={false}
            showClear
            helpText={t('示例："thinking":true')}
          />

          <Form.TagInput
            field='url_patterns'
            label={t('URL路径匹配')}
            placeholder={t('输入URL路径，按回车添加')}
            allowDuplicates={false}
            showClear
            helpText={t('示例：/v1/chat/completions')}
          />
        </Card>

        {/* 渠道配置 */}
        <Card className='!rounded-lg shadow-sm border-0 mb-4'>
          <div className='flex items-center mb-4'>
            <Avatar size='small' color='purple' className='mr-2'>
              <IconServer size={16} />
            </Avatar>
            <Text strong>{t('渠道配置')}</Text>
          </div>

          <div className='mb-4'>
            <Text strong className='block mb-2'>{t('渠道组配置')}</Text>
            <ChannelGroupEditor 
              value={channelGroups} 
              onChange={setChannelGroups} 
            />
          </div>

          <Form.RadioGroup
            field='random_type'
            label={t('选择模式')}
            type='button'
            options={[
              { label: t('顺序选择'), value: 'order' },
              { label: t('随机选择'), value: 'random' },
            ]}
          />
        </Card>

        {/* 高级设置 */}
        <Card className='!rounded-lg shadow-sm border-0 mb-4'>
          <div className='flex items-center mb-4'>
            <Avatar size='small' color='orange' className='mr-2'>
              <IconSetting size={16} />
            </Avatar>
            <Text strong>{t('高级设置')}</Text>
          </div>

          <Form.InputNumber
            field='max_retry'
            label={t('最大重试次数')}
            placeholder={t('请求失败时的最大重试次数')}
            rules={[{ required: true, message: t('请输入最大重试次数') }]}
            min={0}
            max={100}
            style={{ width: '100%' }}
          />
        </Card>

        {/* Footer */}
        <div className='flex justify-end'>
          <Space>
            <Button
              theme='solid'
              type='primary'
              htmlType='submit'
              loading={loading}
              icon={<IconSave />}
            >
              {t('提交')}
            </Button>
            <Button
              theme='light'
              onClick={onClose}
              icon={<IconClose />}
            >
              {t('取消')}
            </Button>
          </Space>
        </div>
      </Form>
    </Modal>
  );
};

export default EditModal;
