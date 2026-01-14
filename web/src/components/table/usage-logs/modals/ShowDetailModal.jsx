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
import React, { useState } from 'react';
import { copy, showSuccess } from '../../../../helpers';
import { Modal, Button, Space, Tag, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

// 格式化JSON内容
const formatJsonContent = (content) => {
  try {
    const parsed = JSON.parse(content);
    return JSON.stringify(parsed, null, 2);
  } catch (e) {
    return content;
  }
};

export default function ShowDetailModal({ visible, onCancel, detailContent }) {
  const { t } = useTranslation();

  const renderMultiModelByObject = (request) => {
    try {
      request = JSON.parse(request);
      let models = [];
      if (request.tools && request.tools.length > 0) {
        models.push('tools');
      }
      if (Array.isArray(request.messages)) {
        for (let message of request.messages) {
          if (!Array.isArray(message.content)) {
            continue;
          }
          for (let item of message.content) {
            if (item.type === 'image_url') {
              models.push('image');
            } else if (item.type === 'audio_url') {
              models.push('audio');
            } else if (item.type === 'video_url') {
              models.push('video');
            }
          }
        }
      }
      models = [...new Set(models)];
      return models.join(',');
    } catch (err) {
      return '';
    }
  };
  const multi = renderMultiModelByObject(detailContent);

  const copyText = async () => {
    if (await copy(detailContent)) {
      showSuccess(t('已复制：'));
    } else {
      Modal.error({
        title: t('无法复制到剪贴板，请手动复制'),
        content: detailContent,
      });
    }
  };

  return (
    <Modal
      title={t('详情')}
      visible={true}
      onCancel={onCancel}
      width={800}
      footer={
        <div className='flex justify-end gap-2'>
          <Button
            theme='light'
            onClick={async (e) => {
              await copyText(e);
            }}
            className='!rounded-full'
          >
            {t('复制')}
          </Button>
          <Button type='primary' onClick={onCancel} className='!rounded-full'>
            {t('关闭')}
          </Button>
        </div>
      }
    >
      {multi && (
        <div className='mb-3'>
          <Space align='center'>
            <Typography.Text strong>{t('多模态')}:</Typography.Text>
            {multi.split(',').map((m) => (
              <Tag key={m} color='white' shape='circle'>
                {m}
              </Tag>
            ))}
          </Space>
        </div>
      )}
      <div className='bg-gray-50 p-4 rounded-lg max-h-96 overflow-auto'>
        <pre className='whitespace-pre-wrap text-sm font-mono'>
          {formatJsonContent(detailContent)}
        </pre>
      </div>
    </Modal>
  );
}
