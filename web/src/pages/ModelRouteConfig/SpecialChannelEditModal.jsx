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

import React, { useEffect, useRef, useState } from 'react';
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
} from '@douyinfe/semi-ui';
import { IconSave, IconClose, IconServer, IconSetting } from '@douyinfe/semi-icons';
import { showInfo } from '../../helpers';
import ChannelGroupEditor from './ChannelGroupEditor';

const { Text, Title } = Typography;

// 编辑单条特殊渠道规则：{ model_name, channel_ids: [][]int, random_type }
const SpecialChannelEditModal = ({ visible, rule, onClose, onSubmit }) => {
  const { t } = useTranslation();
  const formApiRef = useRef(null);
  const [channelGroups, setChannelGroups] = useState([[]]);
  const [formKey, setFormKey] = useState(0);
  const isEdit = rule !== null;

  const getInitialValues = () => {
    if (isEdit && rule) {
      return {
        model_name: rule.model_name || '',
        random_type: rule.random_type || 'order',
      };
    }
    return { model_name: '', random_type: 'order' };
  };

  useEffect(() => {
    if (visible) {
      setFormKey((prev) => prev + 1);
      if (isEdit && rule) {
        setChannelGroups(
          rule.channel_ids && rule.channel_ids.length > 0 ? rule.channel_ids : [[]],
        );
      } else {
        setChannelGroups([[]]);
      }
      setTimeout(() => {
        if (formApiRef.current) {
          formApiRef.current.setValues(getInitialValues());
        }
      }, 100);
    }
  }, [visible, rule, isEdit]);

  const handleSubmit = (values) => {
    const modelName = (values.model_name || '').trim();
    if (!modelName) {
      showInfo(t('请输入模型名称'));
      return;
    }

    // 过滤空渠道组
    const cleanedGroups = (channelGroups || [])
      .map((group) => (group || []).filter((id) => id !== undefined && id !== null))
      .filter((group) => group.length > 0);

    if (cleanedGroups.length === 0) {
      showInfo(t('每个渠道组至少需要包含一个渠道'));
      return;
    }

    onSubmit({
      model_name: modelName,
      channel_ids: cleanedGroups,
      random_type: values.random_type || 'order',
    });
  };

  return (
    <Modal
      title={
        <Space>
          <Avatar size='small' color='cyan'>
            <IconServer />
          </Avatar>
          <Title heading={4} className='m-0'>
            {isEdit ? t('编辑规则') : t('新建规则')}
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

          <Banner
            type='info'
            description={t('模型名称支持正则表达式，将按此匹配请求的模型名。')}
            className='!rounded-lg mb-4'
          />

          <Form.Input
            field='model_name'
            label={t('模型名称')}
            placeholder={t('示例：gemini- 或 ^gemini-.*$')}
            rules={[{ required: true, message: t('请输入模型名称') }]}
            showClear
          />

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

        {/* 渠道组 */}
        <Card className='!rounded-lg shadow-sm border-0 mb-4'>
          <div className='flex items-center mb-4'>
            <Avatar size='small' color='purple' className='mr-2'>
              <IconServer size={16} />
            </Avatar>
            <Text strong>{t('渠道组配置')}</Text>
          </div>

          <ChannelGroupEditor value={channelGroups} onChange={setChannelGroups} />
        </Card>

        {/* Footer */}
        <div className='flex justify-end'>
          <Space>
            <Button
              theme='solid'
              type='primary'
              htmlType='submit'
              icon={<IconSave />}
            >
              {t('确定')}
            </Button>
            <Button theme='light' onClick={onClose} icon={<IconClose />}>
              {t('取消')}
            </Button>
          </Space>
        </div>
      </Form>
    </Modal>
  );
};

export default SpecialChannelEditModal;
