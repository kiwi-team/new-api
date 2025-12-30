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
import { Table, Button, DatePicker, Space, Input } from '@douyinfe/semi-ui';
import { showError, API } from '../../../helpers';

const QuotaStatisticsTable = () => {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState([]);
  const [dateRange, setDateRange] = useState([
    new Date(new Date().getTime() - 6 * 24 * 60 * 60 * 1000),
    new Date()
  ]);
  const [modelName, setModelName] = useState('');
  const [clientUserId, setClientUserId] = useState('');

  const columns = [
    { title: 'Date', dataIndex: 'date', key: 'date' },
    { title: 'Client User ID', dataIndex: 'client_user_id', key: 'client_user_id' },
    { title: 'Model Name', dataIndex: 'model_name', key: 'model_name' },
    { title: 'Total Count', dataIndex: 'total_count', key: 'total_count' },
    { title: 'Total Quota', dataIndex: 'total_quota', key: 'total_quota' },
    { title: 'Total Prompt Tokens', dataIndex: 'total_prompt', key: 'total_prompt' },
    { title: 'Total Completion Tokens', dataIndex: 'total_completion', key: 'total_completion' },
  ];

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
        },
      });
      const { success, message, data } = res.data;
      if (success) {
        setData(data);
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
                <Input 
                    placeholder="Model Name" 
                    value={modelName} 
                    onChange={setModelName} 
                    style={{ width: 200 }}
                />
                <Input 
                    placeholder="Client User ID" 
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
