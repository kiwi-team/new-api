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

import React, { useState, useEffect } from 'react';
import CardPro from '../../common/ui/CardPro';
import { Table, Button, DatePicker, Space, Input, Tag } from '@douyinfe/semi-ui';
import { showError, API } from '../../../helpers';

const QuotaStatisticsTable = () => {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState([]);
  const [totalUSD, setTotalUSD] = useState(0);
  const getDefaultRange = () => {
    const now = new Date();
    const monthStart = new Date(now.getFullYear(), now.getMonth(), 1, 0, 0, 0, 0);
    const dayEnd = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59, 999);
    return [monthStart, dayEnd];
  };
  const [dateRange, setDateRange] = useState(getDefaultRange());
  const [modelName, setModelName] = useState('');
  const [expandModels, setExpandModels] = useState(false);
  const [expandDates, setExpandDates] = useState(false);
  const toioMode = (() => {
    try {
      const u = localStorage.getItem('user');
      if (u) {
        const parsed = JSON.parse(u);
        if (parsed?.toio_registered === 1) return true;
      }
    } catch {}
    return localStorage.getItem('is_toio') === 'true';
  })();
  const [clientUserId, setClientUserId] = useState('');

  const columns = (() => {
    const base = [
      { title: 'UID', dataIndex: 'client_user_id', key: 'client_user_id' },
    ];
    if (expandDates) {
      base.unshift({ title: 'Date', dataIndex: 'date', key: 'date' });
    }
    if (expandModels) {
      base.push({ title: 'Model Name', dataIndex: 'model_name', key: 'model_name' });
    } else {
      base.push({
        title: '月总预算',
        dataIndex: 'fixed_quota',
        key: 'month_budget',
        render: (_, record) => (
          <div>
            <span style={{ color: '#10b981' }}>月度固定预算: {record.fixed_quota}</span>
            {' + '}
            <span style={{ color: '#f59e0b' }}>临时预算: {record.temp_quota}</span>
          </div>
        ),
      });
    }
    base.push({ title: '请求次数', dataIndex: 'total_count', key: 'total_count' });
    base.push({ title: '消耗($)', dataIndex: 'total_quota', key: 'total_quota' });
    base.push({ title: '总PromptTokens', dataIndex: 'total_prompt', key: 'total_prompt' });
    base.push({ title: '总Completion Tokens', dataIndex: 'total_completion', key: 'total_completion' });
    return base;
  })();

  const fetchData = async () => {
    if (!dateRange || dateRange.length !== 2) {
        showError('Please select a date range first');
        return;
    }
    setLoading(true);
    const startTimestamp = Math.floor(dateRange[0].getTime() / 1000);
    const endTimestamp = Math.floor(dateRange[1].getTime() / 1000);
    try {
      const res = await API.get('/api/data/statistics', {
        params: {
          start_timestamp: startTimestamp,
          end_timestamp: endTimestamp,
          model_name: modelName,
          client_user_id: clientUserId,
          expand_models: expandModels,
          expand_dates: expandDates,
        },
      });
      const { success, message, data } = res.data;
      if (success) {
        setData(data);
        const sum = (data || []).reduce((acc, cur) => {
          const v = parseFloat(cur.total_quota) || 0;
          return acc + v;
        }, 0);
        setTotalUSD(sum);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [dateRange]);

  useEffect(() => {
    fetchData();
  }, [expandModels, expandDates]);

  useEffect(() => {
    if (toioMode) {
      setExpandModels(false);
      setExpandDates(false);
      setModelName('');
    }
  }, [toioMode]);

  const handleExport = async () => {
    if (!dateRange || dateRange.length !== 2) {
        showError('Please select a date range first');
        return;
    }
    const startTimestamp = Math.floor(dateRange[0].getTime() / 1000);
    const endTimestamp = Math.floor(dateRange[1].getTime() / 1000);
    
    try {
      const res = await API.get('/api/data/statistics/export', {
        params: {
          start_timestamp: startTimestamp,
          end_timestamp: endTimestamp,
          model_name: modelName,
          client_user_id: clientUserId,
          expand_models: expandModels,
          expand_dates: expandDates,
        },
        responseType: 'blob'
      });
      
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', 'quota_statistics.csv');
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (error) {
      showError('Export failed: ' + error.message);
    }
  };

  return (
    <CardPro
        type='type2'
        title="Quota Statistics"
        searchArea={
      <Space>
            <DatePicker 
                type="dateTimeRange" 
                value={dateRange} 
                onChange={setDateRange} 
            />
            <Tag color='white' shape='circle'>
              <span>总消耗: ${totalUSD.toFixed(2)}</span>
            </Tag>
        {!toioMode && (
          <>
            <Button
              type='tertiary'
              onClick={() => setExpandModels(!expandModels)}
            >
              {expandModels ? '按模型展开: 开' : '按模型展开: 关'}
            </Button>
            <Button
              type='tertiary'
              onClick={() => setExpandDates(!expandDates)}
            >
              {expandDates ? '按日期展开: 开' : '按日期展开: 关'}
            </Button>
            <Input 
                placeholder="Model Name" 
                value={modelName} 
                onChange={setModelName} 
                style={{ width: 200 }}
            />
          </>
        )}
        <Input 
            placeholder="UID" 
            value={clientUserId} 
            onChange={setClientUserId} 
            style={{ width: 200 }}
                />
                <Button theme='solid' onClick={fetchData} loading={loading}>Search</Button>
                <Button onClick={handleExport}>Export CSV</Button>
            </Space>
        }
    >
      <Table columns={columns} dataSource={data} loading={loading} pagination={{ pageSize: 20 }} />
    </CardPro>
  );
};

export default QuotaStatisticsTable;
