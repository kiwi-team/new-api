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
import { Table, Button, DatePicker, Space, Input, Tag, Select } from '@douyinfe/semi-ui';
import { showError, API, isRoot } from '../../../helpers';

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
  const [clientScenairos, setClientScenairos] = useState([]);
  const [selectedUserId, setSelectedUserId] = useState(null);
  const [projectName, setProjectName] = useState(null);
  const [projectList, setProjectList] = useState([]);
  const [projectListLoading, setProjectListLoading] = useState(false);

  const scenairoOptions = [
    { value: 'PersonalExperiment', label: '个人实验(含未标记)' },
    { value: 'ReleaseEvaluation', label: '发版评测' },
    { value: 'DailyExternalModelEvaluation', label: '日常外部模型评测' },
    { value: 'Other', label: '其他' },
  ];
  const [userList, setUserList] = useState([]);
  const [userListLoading, setUserListLoading] = useState(false);

  // 获取用户列表（仅root用户可用）
  const fetchUserList = async () => {
    if (!isRoot()) return;
    setUserListLoading(true);
    try {
      const res = await API.get('/api/user/', {
        params: {
          page: 1,
          page_size: 1000, // 获取足够多的用户
        },
      });
      const { success, data } = res.data;
      if (success && data?.items) {
        const options = data.items.map((user) => ({
          value: user.id,
          label: `${user.username} (ID: ${user.id})`,
        }));
        setUserList([{ value: 0, label: '全部用户' }, ...options]);
      }
    } catch (error) {
      console.error('Failed to fetch user list:', error);
    } finally {
      setUserListLoading(false);
    }
  };

  // 初始化时获取用户列表
  useEffect(() => {
    if (isRoot()) {
      fetchUserList();
    }
  }, []);

  // 获取项目名称列表
  const fetchProjectNames = async () => {
    setProjectListLoading(true);
    try {
      const res = await API.get('/api/data/project-names');
      const { success, data } = res.data;
      if (success && Array.isArray(data)) {
        const options = data.map((name) => ({
          value: name,
          label: name,
        }));
        setProjectList(options);
      }
    } catch (error) {
      console.error('Failed to fetch project names:', error);
    } finally {
      setProjectListLoading(false);
    }
  };

  useEffect(() => {
    fetchProjectNames();
  }, []);

  const columns = (() => {
    const base = [
      { title: 'UID', dataIndex: 'client_user_id', key: 'client_user_id', sorter: (a, b) => String(a.client_user_id).localeCompare(String(b.client_user_id)) },
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
        render: (_, record) => {
          const fixed = parseInt(record.fixed_quota || 0, 10);
          const temp = parseInt(record.temp_quota || 0, 10);
          const total = fixed + temp;
          return (
            <span>
              {total}
              <span style={{ color: '#888', fontSize: '12px', marginLeft: 4 }}>
                (固定{fixed}+临时{temp})
              </span>
            </span>
          );
        },
      });
    }
    // 消耗列，根据月总预算显示不同颜色
    base.push({
      title: '消耗($)',
      dataIndex: 'total_quota',
      key: 'total_quota',
      sorter: (a, b) => (parseFloat(a.total_quota) || 0) - (parseFloat(b.total_quota) || 0),
      render: (value, record) => {
        const quota = parseFloat(value) || 0;
        const fixed = parseInt(record.fixed_quota || 0, 10);
        const temp = parseInt(record.temp_quota || 0, 10);
        const monthBudget = fixed + temp;
        let color = 'inherit';
        if (monthBudget > 0) {
          if (quota >= monthBudget) {
            color = '#ef4444'; // 红色：超过或等于月总预算
          } else if (quota >= monthBudget * 0.5) {
            color = '#f59e0b'; // 黄色：超过50%
          }
        }
        return <span style={{ color, fontWeight: color !== 'inherit' ? 'bold' : 'normal' }}>{quota.toFixed(6)}</span>;
      },
    });
    // 请求次数列放到消耗后面
    base.push({ title: '请求次数', dataIndex: 'total_count', key: 'total_count' });
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
      const params = {
        start_timestamp: startTimestamp,
        end_timestamp: endTimestamp,
        model_name: modelName,
        client_user_id: clientUserId,
        client_scenairos: clientScenairos.join(','),
        expand_models: expandModels,
        expand_dates: expandDates,
      };
      // 仅root用户可以传递user_id参数
      if (isRoot() && selectedUserId && selectedUserId > 0) {
        params.user_id = selectedUserId;
      }
      if (projectName) {
        params.project_name = projectName;
      }
      const res = await API.get('/api/data/statistics', { params });
      const { success, message, data } = res.data;
      if (success) {
        const list = Array.isArray(data) ? data : [];
        setData(list);
        const sum = (list || []).reduce((acc, cur) => {
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
    fetchData();
  }, [clientUserId, modelName, clientScenairos]);

  useEffect(() => {
    fetchData();
  }, [selectedUserId, projectName]);

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
      const params = {
        start_timestamp: startTimestamp,
        end_timestamp: endTimestamp,
        model_name: modelName,
        client_user_id: clientUserId,
        client_scenairos: clientScenairos.join(','),
        expand_models: expandModels,
        expand_dates: expandDates,
      };
      // 仅root用户可以传递user_id参数
      if (isRoot() && selectedUserId && selectedUserId > 0) {
        params.user_id = selectedUserId;
      }
      if (projectName) {
        params.project_name = projectName;
      }
      const res = await API.get('/api/data/statistics/export', {
        params,
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
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
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
              </>
            )}
        </Space>
        <Space>
            {!toioMode && (
              <Input 
                  placeholder="Model Name" 
                  value={modelName} 
                  onChange={setModelName} 
                  style={{ width: 150 }}
              />
            )}
            <Input 
                placeholder="UID(模糊)" 
                value={clientUserId} 
                onChange={setClientUserId} 
                style={{ width: 150 }}
            />
            <Select
                placeholder="项目名称"
                style={{ width: 180 }}
                optionList={projectList}
                value={projectName}
                onChange={setProjectName}
                loading={projectListLoading}
                filter
                showClear
            />
            <Select
                placeholder="Scenairo"
                multiple
                style={{ width: 220 }}
                optionList={scenairoOptions}
                value={clientScenairos}
                onChange={setClientScenairos}
                showClear
            />
            {isRoot() && (
              <Select
                placeholder="选择用户"
                style={{ width: 180 }}
                optionList={userList}
                value={selectedUserId}
                onChange={setSelectedUserId}
                loading={userListLoading}
                filter
                showClear
              />
            )}
            <Button theme='solid' onClick={fetchData} loading={loading}>Search</Button>
            <Button onClick={handleExport}>Export CSV</Button>
        </Space>
      </div>
        }
    >
      <Table columns={columns} dataSource={data} loading={loading} pagination={{ pageSize: 20 }} />
    </CardPro>
  );
};

export default QuotaStatisticsTable;
