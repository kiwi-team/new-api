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

import { useState, useCallback, useEffect } from 'react';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import {
  modelColorMap,
  renderNumber,
  renderQuota,
  modelToColor,
  getQuotaWithUnit,
  API,
  showError,
} from '../../helpers';
import {
  processRawData,
  calculateTrendData,
  aggregateDataByTimeAndModel,
  generateChartTimePoints,
  updateChartSpec,
  updateMapValue,
  initializeMaps,
} from '../../helpers/dashboard';

export const useDashboardCharts = (
  dataExportDefaultTime,
  setTrendData,
  setConsumeQuota,
  setTimes,
  setConsumeTokens,
  setPieData,
  setLineData,
  setModelColors,
  t,
) => {
  // ========== 图表规格状态 ==========
  const [spec_pie, setSpecPie] = useState({
    type: 'pie',
    data: [
      {
        id: 'id0',
        values: [{ type: 'null', value: '0' }],
      },
    ],
    outerRadius: 0.8,
    innerRadius: 0.5,
    padAngle: 0.6,
    valueField: 'value',
    categoryField: 'type',
    pie: {
      style: {
        cornerRadius: 10,
      },
      state: {
        hover: {
          outerRadius: 0.85,
          stroke: '#000',
          lineWidth: 1,
        },
        selected: {
          outerRadius: 0.85,
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    title: {
      visible: true,
      text: t('模型调用次数占比'),
      subtext: `${t('总计')}：${renderNumber(0)}`,
    },
    legends: {
      visible: true,
      orient: 'left',
    },
    label: {
      visible: true,
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['type'],
            value: (datum) => renderNumber(datum['value']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  const [spec_line, setSpecLine] = useState({
    type: 'bar',
    data: [
      {
        id: 'barData',
        values: [],
      },
    ],
    xField: 'Time',
    yField: 'Usage',
    seriesField: 'Model',
    stack: true,
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: true,
      text: t('模型消耗分布'),
      subtext: `${t('总计')}：${renderQuota(0, 2)}`,
    },
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => datum['rawQuota'] || 0,
          },
        ],
        updateContent: (array) => {
          array.sort((a, b) => b.value - a.value);
          let sum = 0;
          for (let i = 0; i < array.length; i++) {
            if (array[i].key == '其他') {
              continue;
            }
            let value = parseFloat(array[i].value);
            if (isNaN(value)) {
              value = 0;
            }
            if (array[i].datum && array[i].datum.TimeSum) {
              sum = array[i].datum.TimeSum;
            }
            array[i].value = renderQuota(value, 4);
          }
          array.unshift({
            key: t('总计'),
            value: renderQuota(sum, 4),
          });
          return array;
        },
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  // 模型消耗趋势折线图
  const [spec_model_line, setSpecModelLine] = useState({
    type: 'line',
    data: [
      {
        id: 'lineData',
        values: [],
      },
    ],
    xField: 'Time',
    yField: 'Count',
    seriesField: 'Model',
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: true,
      text: t('模型消耗趋势'),
      subtext: '',
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  // 模型调用次数排行柱状图
  const [spec_rank_bar, setSpecRankBar] = useState({
    type: 'bar',
    data: [
      {
        id: 'rankData',
        values: [],
      },
    ],
    xField: 'Model',
    yField: 'Count',
    seriesField: 'Model',
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: true,
      text: t('模型调用次数排行'),
      subtext: '',
    },
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  // ========== 渠道消耗统计 ==========
  const [channelStatData, setChannelStatData] = useState([]);
  const [channelList, setChannelList] = useState([]);
  const [selectedChannelId, setSelectedChannelId] = useState(null);
  const [channelStatLoading, setChannelStatLoading] = useState(false);

  // 渠道消耗饼图
  const [spec_channel_pie, setSpecChannelPie] = useState({
    type: 'pie',
    data: [
      {
        id: 'channelPieData',
        values: [{ type: 'null', value: '0' }],
      },
    ],
    outerRadius: 0.8,
    innerRadius: 0.5,
    padAngle: 0.6,
    valueField: 'value',
    categoryField: 'type',
    pie: {
      style: {
        cornerRadius: 10,
      },
      state: {
        hover: {
          outerRadius: 0.85,
          stroke: '#000',
          lineWidth: 1,
        },
        selected: {
          outerRadius: 0.85,
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    title: {
      visible: true,
      text: t('渠道模型消耗分布'),
      subtext: `${t('总计')}：${renderQuota(0, 2)}`,
    },
    legends: {
      visible: true,
      orient: 'left',
    },
    label: {
      visible: true,
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['type'],
            value: (datum) => renderQuota(datum['value'], 4),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  // 加载渠道消耗统计数据
  const loadChannelStatistics = useCallback(
    async (startTimestamp, endTimestamp) => {
      setChannelStatLoading(true);
      try {
        const url = `/api/data/channel-statistics?start_timestamp=${startTimestamp}&end_timestamp=${endTimestamp}`;
        const res = await API.get(url);
        const { success, message, data } = res.data;
        if (success && data) {
          setChannelStatData(data);

          // 按渠道汇总消耗，生成渠道列表（按消耗从大到小排序）
          const channelQuotaMap = new Map();
          data.forEach((item) => {
            const existing = channelQuotaMap.get(item.channel_id) || {
              channel_id: item.channel_id,
              channel_name: item.channel_name || `渠道 ${item.channel_id}`,
              total_quota: 0,
            };
            existing.total_quota += item.total_quota;
            channelQuotaMap.set(item.channel_id, existing);
          });

          const sortedChannels = Array.from(channelQuotaMap.values()).sort(
            (a, b) => b.total_quota - a.total_quota,
          );
          setChannelList(sortedChannels);

          // 默认选择消耗最高的渠道
          if (sortedChannels.length > 0 && selectedChannelId === null) {
            setSelectedChannelId(sortedChannels[0].channel_id);
            updateChannelPieChart(data, sortedChannels[0].channel_id);
          } else if (selectedChannelId !== null) {
            updateChannelPieChart(data, selectedChannelId);
          }
        } else {
          showError(message || t('获取渠道统计数据失败'));
        }
      } catch (error) {
        showError(t('获取渠道统计数据失败'));
      }
      setChannelStatLoading(false);
    },
    [selectedChannelId, t],
  );

  // 更新渠道饼图数据
  const updateChannelPieChart = useCallback(
    (data, channelId) => {
      const channelData = data.filter((item) => item.channel_id === channelId);
      const pieData = channelData
        .map((item) => ({
          type: item.model_name,
          value: item.total_quota,
        }))
        .sort((a, b) => b.value - a.value);

      const totalQuota = pieData.reduce((sum, item) => sum + item.value, 0);
      const channelName =
        channelData[0]?.channel_name || `渠道 ${channelId}`;

      // 生成颜色映射
      const newModelColors = {};
      pieData.forEach((item) => {
        newModelColors[item.type] =
          modelColorMap[item.type] || modelToColor(item.type);
      });

      setSpecChannelPie((prev) => ({
        ...prev,
        data: [
          {
            id: 'channelPieData',
            values: pieData.length > 0 ? pieData : [{ type: 'null', value: 0 }],
          },
        ],
        title: {
          ...prev.title,
          text: `${channelName} - ${t('模型消耗分布')}`,
          subtext: `${t('总计')}：${renderQuota(totalQuota, 2)}`,
        },
        color: {
          specified: { ...modelColorMap, ...newModelColors },
        },
      }));
    },
    [t],
  );

  // 切换渠道
  const handleChannelChange = useCallback(
    (channelId) => {
      setSelectedChannelId(channelId);
      updateChannelPieChart(channelStatData, channelId);
    },
    [channelStatData, updateChannelPieChart],
  );

  // ========== 数据处理函数 ==========
  const generateModelColors = useCallback((uniqueModels, modelColors) => {
    const newModelColors = {};
    Array.from(uniqueModels).forEach((modelName) => {
      newModelColors[modelName] =
        modelColorMap[modelName] ||
        modelColors[modelName] ||
        modelToColor(modelName);
    });
    return newModelColors;
  }, []);

  const updateChartData = useCallback(
    (data) => {
      const processedData = processRawData(
        data,
        dataExportDefaultTime,
        initializeMaps,
        updateMapValue,
      );

      const {
        totalQuota,
        totalTimes,
        totalTokens,
        uniqueModels,
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
      } = processedData;

      const trendDataResult = calculateTrendData(
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
        dataExportDefaultTime,
      );
      setTrendData(trendDataResult);

      const newModelColors = generateModelColors(uniqueModels, {});
      setModelColors(newModelColors);

      const aggregatedData = aggregateDataByTimeAndModel(
        data,
        dataExportDefaultTime,
      );

      const modelTotals = new Map();
      for (let [_, value] of aggregatedData) {
        updateMapValue(modelTotals, value.model, value.count);
      }

      const newPieData = Array.from(modelTotals)
        .map(([model, count]) => ({
          type: model,
          value: count,
        }))
        .sort((a, b) => b.value - a.value);

      const chartTimePoints = generateChartTimePoints(
        aggregatedData,
        data,
        dataExportDefaultTime,
      );

      let newLineData = [];

      chartTimePoints.forEach((time) => {
        let timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          return {
            Time: time,
            Model: model,
            rawQuota: aggregated?.quota || 0,
            Usage: aggregated?.quota
              ? getQuotaWithUnit(aggregated.quota, 4)
              : 0,
          };
        });

        const timeSum = timeData.reduce((sum, item) => sum + item.rawQuota, 0);
        timeData.sort((a, b) => b.rawQuota - a.rawQuota);
        timeData = timeData.map((item) => ({ ...item, TimeSum: timeSum }));
        newLineData.push(...timeData);
      });

      newLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      updateChartSpec(
        setSpecPie,
        newPieData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'id0',
      );

      updateChartSpec(
        setSpecLine,
        newLineData,
        `${t('总计')}：${renderQuota(totalQuota, 2)}`,
        newModelColors,
        'barData',
      );

      // ===== 模型调用次数折线图 =====
      let modelLineData = [];
      chartTimePoints.forEach((time) => {
        const timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          return {
            Time: time,
            Model: model,
            Count: aggregated?.count || 0,
          };
        });
        modelLineData.push(...timeData);
      });
      modelLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      // ===== 模型调用次数排行柱状图 =====
      const rankData = Array.from(modelTotals)
        .map(([model, count]) => ({
          Model: model,
          Count: count,
        }))
        .sort((a, b) => b.Count - a.Count);

      updateChartSpec(
        setSpecModelLine,
        modelLineData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'lineData',
      );

      updateChartSpec(
        setSpecRankBar,
        rankData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'rankData',
      );

      setPieData(newPieData);
      setLineData(newLineData);
      setConsumeQuota(totalQuota);
      setTimes(totalTimes);
      setConsumeTokens(totalTokens);
    },
    [
      dataExportDefaultTime,
      setTrendData,
      generateModelColors,
      setModelColors,
      setPieData,
      setLineData,
      setConsumeQuota,
      setTimes,
      setConsumeTokens,
      t,
    ],
  );

  // ========== 初始化图表主题 ==========
  useEffect(() => {
    initVChartSemiTheme({
      isWatchingThemeSwitch: true,
    });
  }, []);

  return {
    // 图表规格
    spec_pie,
    spec_line,
    spec_model_line,
    spec_rank_bar,
    spec_channel_pie,

    // 渠道统计相关
    channelList,
    selectedChannelId,
    channelStatLoading,
    loadChannelStatistics,
    handleChannelChange,

    // 函数
    updateChartData,
    generateModelColors,
  };
};
