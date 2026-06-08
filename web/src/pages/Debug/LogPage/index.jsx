/*
Copyright (C) 2025 QuantumNous
*/

import React from 'react';
import { useTranslation } from 'react-i18next';
import { Banner, Button, Card, Empty, Space, Typography } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import { ScrollText } from 'lucide-react';

const { Text, Title } = Typography;

const DebugLogs = () => {
  const { t } = useTranslation();

  return (
    <div className='p-2 md:p-6'>
      <div className='mb-4'>
        <Title heading={3}>{t('调试日志')}</Title>
        <Text type='tertiary'>
          {t('查看调试器请求、响应与执行记录')}
        </Text>
      </div>

      <Banner
        type='info'
        description={t('调试日志页面入口已启用，后续可在这里接入调试器执行记录。')}
        className='mb-4'
      />

      <Card>
        <Empty
          image={<ScrollText size={48} />}
          title={t('暂无调试日志')}
          description={t('执行调试请求后，日志记录将展示在这里')}
        >
          <Space>
            <Button icon={<IconRefresh />} onClick={() => window.location.reload()}>
              {t('刷新')}
            </Button>
          </Space>
        </Empty>
      </Card>
    </div>
  );
};

export default DebugLogs;
