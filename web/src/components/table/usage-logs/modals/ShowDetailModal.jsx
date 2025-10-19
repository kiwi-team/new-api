import React, { useState } from 'react';
import {
    copy,
    showSuccess,
} from '../../../../helpers';
import {
    Modal,
    Button,
} from '@douyinfe/semi-ui';
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

export default function ShowDetailModal({
    visible,
    onCancel,
    detailContent,
}) {

    const { t } = useTranslation();

    const copyText = async () => {
        if (await copy(detailContent)) {
            showSuccess(t('已复制：'));
        } else {
            Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: detailContent });
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
                    <Button
                        type='primary'
                        onClick={onCancel}
                        className='!rounded-full'
                    >
                        {t('关闭')}
                    </Button>
                </div>
            }
        >
            <div className='bg-gray-50 p-4 rounded-lg max-h-96 overflow-auto'>
                <pre className='whitespace-pre-wrap text-sm font-mono'>
                    {formatJsonContent(detailContent)}
                </pre>
            </div>
        </Modal>
    );
}
