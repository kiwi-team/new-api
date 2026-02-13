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

import React from 'react';
import { Card, Tabs, TabPane, Select, Spin } from '@douyinfe/semi-ui';
import { PieChart } from 'lucide-react';
import { VChart } from '@visactor/react-vchart';
import { renderQuota } from '../../helpers';

const ChartsPanel = ({
  activeChartTab,
  setActiveChartTab,
  spec_line,
  spec_model_line,
  spec_pie,
  spec_rank_bar,
  spec_channel_pie,
  channelList,
  selectedChannelId,
  channelStatLoading,
  handleChannelChange,
  CARD_PROPS,
  CHART_CONFIG,
  FLEX_CENTER_GAP2,
  hasApiInfoPanel,
  t,
}) => {
  return (
    <Card
      {...CARD_PROPS}
      className={`!rounded-2xl ${hasApiInfoPanel ? 'lg:col-span-3' : ''}`}
      title={
        <div className='flex flex-col lg:flex-row lg:items-center lg:justify-between w-full gap-3'>
          <div className={FLEX_CENTER_GAP2}>
            <PieChart size={16} />
            {t('模型数据分析')}
          </div>
          <Tabs
            type='slash'
            activeKey={activeChartTab}
            onChange={setActiveChartTab}
          >
            <TabPane tab={<span>{t('消耗分布')}</span>} itemKey='1' />
            <TabPane tab={<span>{t('消耗趋势')}</span>} itemKey='2' />
            <TabPane tab={<span>{t('调用次数分布')}</span>} itemKey='3' />
            <TabPane tab={<span>{t('调用次数排行')}</span>} itemKey='4' />
            <TabPane tab={<span>{t('渠道消耗统计')}</span>} itemKey='5' />
          </Tabs>
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <div className='h-96 p-2'>
        {activeChartTab === '1' && (
          <VChart spec={spec_line} option={CHART_CONFIG} />
        )}
        {activeChartTab === '2' && (
          <VChart spec={spec_model_line} option={CHART_CONFIG} />
        )}
        {activeChartTab === '3' && (
          <VChart spec={spec_pie} option={CHART_CONFIG} />
        )}
        {activeChartTab === '4' && (
          <VChart spec={spec_rank_bar} option={CHART_CONFIG} />
        )}
        {activeChartTab === '5' && (
          <div className='h-full flex flex-col'>
            <div className='flex items-center gap-2 mb-2 px-2'>
              <span className='text-sm'>{t('选择渠道')}:</span>
              <Select
                value={selectedChannelId}
                onChange={handleChannelChange}
                style={{ width: 280 }}
                placeholder={t('请选择渠道')}
                optionList={channelList.map((ch) => ({
                  value: ch.channel_id,
                  label: `${ch.channel_name} (${renderQuota(ch.total_quota, 2)})`,
                }))}
                loading={channelStatLoading}
              />
            </div>
            <div className='flex-1'>
              {channelStatLoading ? (
                <div className='h-full flex items-center justify-center'>
                  <Spin size='large' />
                </div>
              ) : (
                <VChart spec={spec_channel_pie} option={CHART_CONFIG} />
              )}
            </div>
          </div>
        )}
      </div>
    </Card>
  );
};

export default ChartsPanel;
