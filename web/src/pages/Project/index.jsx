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

import React, { useEffect, useState, useMemo } from 'react';
import { API, showError, showSuccess, isAdmin, isRoot } from '../../helpers';
import {
  Button,
  Table,
  Modal,
  Form,
  Input,
  Space,
  Typography,
  Tag,
  Switch,
  Descriptions,
  Card,
  Popconfirm,
  Select,
  Tooltip,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const { Title, Text } = Typography;

const ProjectStatusEnabled = 1;
const ProjectStatusPaused = 2;

const formatBudget = (value, digits = 6) => {
  const amount = Number(value) || 0;
  return amount.toFixed(digits).replace(/\.?0+$/, '');
};

const ProjectPage = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [formApi, setFormApi] = useState(null);

  const initFormValues = {
    project_name: '',
    total_budget: 0,
  };
  const [formValues, setFormValues] = useState({ ...initFormValues });
  const [submitLoading, setSubmitLoading] = useState(false);

  // Plan management state
  const [planModalVisible, setPlanModalVisible] = useState(false);
  const [currentProject, setCurrentProject] = useState(null);
  const [plans, setPlans] = useState([]);
  const [plansLoading, setPlansLoading] = useState(false);
  const [planFormVisible, setPlanFormVisible] = useState(false);
  const [editingPlan, setEditingPlan] = useState(null);
  const [planFormApi, setPlanFormApi] = useState(null);
  const [planSubmitLoading, setPlanSubmitLoading] = useState(false);
  const initPlanFormValues = { plan_name: '', start_date: '', end_date: '' };
  const [planFormValues, setPlanFormValues] = useState({
    ...initPlanFormValues,
  });

  // Allocation management state (within a plan)
  const [allocationModalVisible, setAllocationModalVisible] = useState(false);
  const [currentPlan, setCurrentPlan] = useState(null);
  const [allocations, setAllocations] = useState([]);
  const [allocationPage, setAllocationPage] = useState(1);
  const [allocationPageSize, setAllocationPageSize] = useState(10);
  const [allocationTotal, setAllocationTotal] = useState(0);
  const [allocationLoading, setAllocationLoading] = useState(false);
  const [planAllocatedTotal, setPlanAllocatedTotal] = useState(0);
  const [planUsedTotal, setPlanUsedTotal] = useState(0);

  // Allocation form state
  const [allocationFormVisible, setAllocationFormVisible] = useState(false);
  const [editingAllocation, setEditingAllocation] = useState(null);
  const [allocationFormApi, setAllocationFormApi] = useState(null);
  const [allocationSubmitLoading, setAllocationSubmitLoading] = useState(false);
  const [selectedAllocationUid, setSelectedAllocationUid] = useState('');
  const initAllocationFormValues = { client_user_id: '', allocated_quota: 0 };
  const [allocationFormValues, setAllocationFormValues] = useState({
    ...initAllocationFormValues,
  });

  // Dashboard state
  const [dashboardData, setDashboardData] = useState(null);
  const [dashboardLoading, setDashboardLoading] = useState(false);

  // UID list for allocation form
  const [uidOptions, setUidOptions] = useState([]);
  const [uidSearchLoading, setUidSearchLoading] = useState(false);

  const fetchUidOptions = async (keyword = '') => {
    setUidSearchLoading(true);
    try {
      const params = { p: 1, page_size: 50 };
      let url = '/api/cliend_user_quota/';
      if (keyword && keyword.trim()) {
        url = '/api/cliend_user_quota/search';
        params.keyword = keyword.trim();
      }
      const res = await API.get(url, { params });
      const { success, data } = res.data;
      if (success && data?.items) {
        setUidOptions(
          data.items.map((item) => ({
            value: item.client_user_id,
            label: item.client_name
              ? `${item.client_name} · ${item.client_user_id}`
              : `未命名客户 · ${item.client_user_id}`,
            client_name: item.client_name || '',
            fixed_quota: item.fixed_quota || 0,
            temp_quota: item.temp_quota || 0,
          })),
        );
      }
    } catch (e) {
      // silently fail
    } finally {
      setUidSearchLoading(false);
    }
  };

  const fetchDashboard = async () => {
    setDashboardLoading(true);
    try {
      const res = await API.get('/api/project/dashboard');
      const { success, data } = res.data;
      if (success) {
        setDashboardData(data);
      }
    } catch (e) {
      // Silently fail for dashboard
    } finally {
      setDashboardLoading(false);
    }
  };

  const fetchData = async (
    pageNum = page,
    size = pageSize,
    keyword = searchKeyword,
  ) => {
    setLoading(true);
    try {
      const params = { p: pageNum, page_size: size };
      if (keyword && keyword.trim()) {
        params.keyword = keyword.trim();
      }
      const res = await API.get('/api/projects', { params });
      const { success, message, data } = res.data;
      if (success) {
        const items = data.items || [];
        setData(items);
        setCurrentProject((previous) => {
          if (!previous) return previous;
          return items.find((item) => item.id === previous.id) || previous;
        });
        setTotal(data.total || 0);
        setPage(data.p || pageNum);
        setPageSize(data.page_size || size);
      } else {
        showError(message || t('加载失败'));
      }
    } catch (e) {
      showError(t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData(1, pageSize, '');
    fetchDashboard();
  }, []);

  const openCreate = () => {
    setEditing(null);
    setFormValues({ ...initFormValues });
    setModalVisible(true);
  };

  const openEdit = (record) => {
    setEditing(record);
    setFormValues({
      project_name: record.project_name,
      total_budget: record.total_budget,
    });
    setModalVisible(true);
  };

  const handleStatusToggle = async (record) => {
    const newStatus =
      record.status === ProjectStatusEnabled
        ? ProjectStatusPaused
        : ProjectStatusEnabled;
    try {
      const res = await API.put(`/api/project/${record.id}/status`, {
        status: newStatus,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('状态更新成功'));
        fetchData(page, pageSize, searchKeyword);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  const handleSetActivePlan = async (projectId, planId) => {
    try {
      const res = await API.put(`/api/project/${projectId}/active-plan`, {
        plan_id: planId || 0,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('切换成功'));
        fetchData(page, pageSize, searchKeyword);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  const handleSubmit = async () => {
    setSubmitLoading(true);
    try {
      const payload = formApi ? formApi.getValues() : { ...formValues };
      let res;
      if (editing) {
        res = await API.put(`/api/project/${editing.id}`, payload);
      } else {
        res = await API.post('/api/project', payload);
      }
      const { success, message } = res.data;
      if (success) {
        showSuccess(editing ? t('更新成功') : t('创建成功'));
        setModalVisible(false);
        fetchData(page, pageSize, searchKeyword);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    } finally {
      setSubmitLoading(false);
    }
  };

  const handleCloseModal = () => {
    setModalVisible(false);
    formApi && formApi.reset();
    setFormValues({ ...initFormValues });
    setEditing(null);
  };

  useEffect(() => {
    if (modalVisible && formApi) {
      formApi.setValues(formValues);
    }
  }, [modalVisible, formValues, formApi]);

  // ==================== Plan Management Functions ====================

  const fetchPlans = async (projectId) => {
    setPlansLoading(true);
    try {
      const res = await API.get(`/api/project/${projectId}/plans`);
      const { success, data } = res.data;
      if (success) {
        setPlans(data || []);
      }
    } catch (e) {
      showError(t('加载失败'));
    } finally {
      setPlansLoading(false);
    }
  };

  const openPlanModal = (record) => {
    setCurrentProject(record);
    setPlanModalVisible(true);
    fetchPlans(record.id);
  };

  const closePlanModal = () => {
    setPlanModalVisible(false);
    setCurrentProject(null);
    setPlans([]);
  };

  const openPlanForm = (plan = null) => {
    setEditingPlan(plan);
    if (plan) {
      setPlanFormValues({
        plan_name: plan.plan_name,
        start_date: plan.start_date,
        end_date: plan.end_date,
      });
    } else {
      setPlanFormValues({ ...initPlanFormValues });
    }
    setPlanFormVisible(true);
  };

  const closePlanForm = () => {
    setPlanFormVisible(false);
    setEditingPlan(null);
    planFormApi && planFormApi.reset();
    setPlanFormValues({ ...initPlanFormValues });
  };

  const handlePlanSubmit = async () => {
    if (!currentProject) return;
    setPlanSubmitLoading(true);
    try {
      const payload = planFormApi
        ? planFormApi.getValues()
        : { ...planFormValues };
      let res;
      if (editingPlan) {
        res = await API.put(`/api/project/plan/${editingPlan.id}`, payload);
      } else {
        res = await API.post(`/api/project/${currentProject.id}/plan`, payload);
      }
      const { success, message } = res.data;
      if (success) {
        showSuccess(editingPlan ? t('更新成功') : t('创建成功'));
        closePlanForm();
        await Promise.all([
          fetchPlans(currentProject.id),
          fetchData(page, pageSize, searchKeyword),
        ]);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    } finally {
      setPlanSubmitLoading(false);
    }
  };

  const handleDeletePlan = async (planId) => {
    try {
      const res = await API.delete(`/api/project/plan/${planId}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('删除成功'));
        await Promise.all([
          fetchPlans(currentProject.id),
          fetchData(page, pageSize, searchKeyword),
        ]);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  useEffect(() => {
    if (planFormVisible && planFormApi) {
      planFormApi.setValues(planFormValues);
    }
  }, [planFormVisible, planFormValues, planFormApi]);

  // Format date string like 20260408 -> 2026-04-08
  const formatDate = (dateStr) => {
    if (!dateStr || dateStr.length !== 8) return dateStr;
    return `${dateStr.slice(0, 4)}-${dateStr.slice(4, 6)}-${dateStr.slice(6, 8)}`;
  };

  const planColumns = [
    {
      title: t('预算方案'),
      dataIndex: 'plan_name',
      width: 300,
      render: (value, record) => (
        <div>
          <Text strong>{value}</Text>
          <div style={{ marginTop: 4 }}>
            <Text type='tertiary' size='small'>
              {formatDate(record.start_date)} ~ {formatDate(record.end_date)}
            </Text>
          </div>
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'is_active',
      width: 110,
      render: (_, record) => {
        if (record.is_active) return <Tag color='green'>{t('启用中')}</Tag>;
        if (record.is_expired) return <Tag color='red'>{t('已过期')}</Tag>;
        return <Tag color='grey'>{t('未启用')}</Tag>;
      },
    },
    {
      title: t('分配概览'),
      dataIndex: 'allocations',
      width: 190,
      render: (_, record) => {
        const allocs = record.allocations || [];
        const total = allocs.reduce(
          (sum, allocation) => sum + (allocation.allocated_quota || 0),
          0,
        );
        return (
          <div style={{ lineHeight: 1.7 }}>
            <div>
              <Button
                theme='borderless'
                type='primary'
                size='small'
                style={{ paddingLeft: 0 }}
                onClick={() => openAllocationModalForPlan(record)}
              >
                {allocs.length > 0
                  ? t('查看 {{count}} 位客户', { count: allocs.length })
                  : t('添加客户分配')}
              </Button>
            </div>
            <div>
              <Text type='tertiary'>{t('已分配')}：</Text>
              <Text type={total > 0 ? 'success' : 'tertiary'}>
                ${formatBudget(total)}
              </Text>
            </div>
          </div>
        );
      },
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      width: 240,
      render: (_, record) => (
        <Space>
          <Button
            size='small'
            onClick={() => openAllocationModalForPlan(record)}
          >
            {t('管理分配')}
          </Button>
          <Button size='small' onClick={() => openPlanForm(record)}>
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('确认删除此计划及其所有分配？')}
            onConfirm={() => handleDeletePlan(record.id)}
          >
            <Button size='small' type='danger'>
              {t('删除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  // ==================== Allocation Management Functions (Plan-scoped) ====================

  const fetchAllocations = async (
    planId,
    pageNum = 1,
    size = allocationPageSize,
  ) => {
    setAllocationLoading(true);
    try {
      const params = { p: pageNum, page_size: size };
      const res = await API.get(`/api/project/plan/${planId}/allocations`, {
        params,
      });
      const { success, message, data } = res.data;
      if (success) {
        setAllocations(data.items || []);
        setAllocationTotal(data.total || 0);
        setAllocationPage(data.p || pageNum);
        setAllocationPageSize(data.page_size || size);
      } else {
        showError(message || t('加载失败'));
      }
    } catch (e) {
      showError(t('加载失败'));
    } finally {
      setAllocationLoading(false);
    }
  };

  const fetchPlanTotals = async (planId) => {
    try {
      const res = await API.get(`/api/project/plan/${planId}/allocations`, {
        params: { p: 1, page_size: 10000 },
      });
      const { success, data } = res.data;
      if (success) {
        const total = (data.items || []).reduce(
          (sum, item) => sum + (item.allocated_quota || 0),
          0,
        );
        const usedTotal = (data.items || []).reduce(
          (sum, item) => sum + (item.used_quota || 0) / 500000,
          0,
        );
        setPlanAllocatedTotal(total);
        setPlanUsedTotal(usedTotal);
      }
    } catch (e) {
      // Silently fail
    }
  };

  const openAllocationModalForPlan = (plan) => {
    const normalizedPlan = {
      ...plan,
      id: plan.id || plan.plan_id,
    };
    setCurrentPlan(normalizedPlan);
    setAllocationPage(1);
    setAllocationModalVisible(true);
    fetchAllocations(normalizedPlan.id, 1, allocationPageSize);
    fetchPlanTotals(normalizedPlan.id);
  };

  const openActivePlanAllocations = (project) => {
    const activePlan = (project.plans || []).find(
      (plan) => plan.plan_id === project.active_plan_id,
    );
    if (!activePlan) {
      showError(t('当前方案不存在，请刷新后重试'));
      return;
    }
    setCurrentProject(project);
    openAllocationModalForPlan(activePlan);
  };

  const closeAllocationModal = () => {
    setAllocationModalVisible(false);
    setCurrentPlan(null);
    setAllocations([]);
    setAllocationTotal(0);
    setPlanAllocatedTotal(0);
    setPlanUsedTotal(0);
  };

  const openAllocationForm = (allocation = null) => {
    setEditingAllocation(allocation);
    setSelectedAllocationUid(allocation?.client_user_id || '');
    if (allocation) {
      setAllocationFormValues({
        client_user_id: allocation.client_user_id,
        allocated_quota: allocation.allocated_quota,
      });
    } else {
      setAllocationFormValues({ ...initAllocationFormValues });
      fetchUidOptions();
    }
    setAllocationFormVisible(true);
  };

  const closeAllocationForm = () => {
    setAllocationFormVisible(false);
    setEditingAllocation(null);
    setSelectedAllocationUid('');
    allocationFormApi && allocationFormApi.reset();
    setAllocationFormValues({ ...initAllocationFormValues });
  };

  const handleAllocationSubmit = async () => {
    if (!currentPlan) return;
    setAllocationSubmitLoading(true);
    try {
      const payload = allocationFormApi
        ? allocationFormApi.getValues()
        : { ...allocationFormValues };
      const res = await API.post(
        `/api/project/plan/${currentPlan.id}/allocation`,
        payload,
      );
      const { success, message } = res.data;
      if (success) {
        showSuccess(editingAllocation ? t('更新成功') : t('创建成功'));
        closeAllocationForm();
        await Promise.all([
          fetchAllocations(currentPlan.id, allocationPage, allocationPageSize),
          fetchPlanTotals(currentPlan.id),
          fetchPlans(currentProject.id),
          fetchData(page, pageSize, searchKeyword),
        ]);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    } finally {
      setAllocationSubmitLoading(false);
    }
  };

  const handleClearBudget = async (allocationId) => {
    try {
      const res = await API.post(
        `/api/project/allocation/${allocationId}/clear`,
      );
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('已清空'));
        if (currentPlan && currentProject) {
          await Promise.all([
            fetchAllocations(
              currentPlan.id,
              allocationPage,
              allocationPageSize,
            ),
            fetchPlanTotals(currentPlan.id),
            fetchPlans(currentProject.id),
            fetchData(page, pageSize, searchKeyword),
          ]);
        }
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  useEffect(() => {
    if (allocationFormVisible && allocationFormApi) {
      allocationFormApi.setValues(allocationFormValues);
    }
  }, [allocationFormVisible, allocationFormValues, allocationFormApi]);

  const projectCumulativeRemaining = Math.max(
    (currentProject?.total_budget || 0) -
      (currentProject?.allocated_total || 0),
    0,
  );
  const allocationEditableMax =
    projectCumulativeRemaining + (editingAllocation?.allocated_quota || 0);
  const selectedUidInfo =
    uidOptions.find((option) => option.value === selectedAllocationUid) ||
    (editingAllocation
      ? {
          value: editingAllocation.client_user_id,
          client_name: editingAllocation.client_name || '',
        }
      : null);

  const allocationColumns = [
    {
      title: t('客户 / UID'),
      dataIndex: 'client_user_id',
      width: 360,
      render: (value, record) => (
        <div style={{ minWidth: 0 }}>
          <Text strong>{record.client_name || t('未命名客户')}</Text>
          <Tooltip content={value} position='topLeft'>
            <div
              style={{
                color: 'var(--semi-color-text-2)',
                fontFamily: 'monospace',
                fontSize: 12,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                marginTop: 2,
              }}
            >
              {value}
            </div>
          </Tooltip>
        </div>
      ),
    },
    {
      title: t('分配额度($)'),
      dataIndex: 'allocated_quota',
      width: 130,
      sorter: (a, b) =>
        (parseInt(a.allocated_quota, 10) || 0) -
        (parseInt(b.allocated_quota, 10) || 0),
      render: (value) => formatBudget(value),
    },
    {
      title: t('已使用($)'),
      dataIndex: 'used_quota',
      width: 130,
      sorter: (a, b) =>
        (parseInt(a.used_quota, 10) || 0) - (parseInt(b.used_quota, 10) || 0),
      render: (_, record) => formatBudget((record.used_quota || 0) / 500000),
    },
    {
      title: t('当前可用($)'),
      dataIndex: 'remaining',
      width: 140,
      render: (_, record) => {
        const remaining = Math.max(
          (record.allocated_quota || 0) - (record.used_quota / 500000 || 0),
          0,
        );
        return (
          <Text type={remaining > 0 ? 'success' : 'danger'}>
            {formatBudget(remaining)}
          </Text>
        );
      },
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      width: 180,
      render: (v) => (v ? new Date(v * 1000).toLocaleString() : '-'),
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      width: 180,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button size='small' onClick={() => openAllocationForm(record)}>
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t(
              '确认将该客户在本方案的分配额度设为 0？历史消耗不会被删除。',
            )}
            onConfirm={() => handleClearBudget(record.id)}
          >
            <Button size='small' type='danger'>
              {t('额度归零')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    {
      title: t('项目'),
      dataIndex: 'project_name',
      width: 280,
      render: (value, record) => (
        <div>
          <Text strong style={{ wordBreak: 'break-word' }}>
            {value}
          </Text>
          <div style={{ marginTop: 8 }}>
            <Space>
              <Tag
                color={
                  record.status === ProjectStatusEnabled ? 'green' : 'grey'
                }
              >
                {record.status === ProjectStatusEnabled
                  ? t('项目启用')
                  : t('项目暂停')}
              </Tag>
              <Switch
                checked={record.status === ProjectStatusEnabled}
                onChange={() => handleStatusToggle(record)}
                size='small'
              />
            </Space>
          </div>
        </div>
      ),
    },
    {
      title: t('项目累计额度($)'),
      dataIndex: 'total_budget',
      width: 190,
      render: (_, record) => {
        const remaining =
          (record.total_budget || 0) - (record.allocated_total || 0);
        return (
          <div style={{ lineHeight: 1.8 }}>
            <div>
              <Text type='tertiary'>{t('预算上限')}：</Text>
              <Text strong>{formatBudget(record.total_budget)}</Text>
            </div>
            <div>
              <Text type='tertiary'>{t('剩余可分配')}：</Text>
              <Text type={remaining > 0 ? 'success' : 'danger'}>
                {formatBudget(remaining)}
              </Text>
            </div>
          </div>
        );
      },
    },
    {
      title: t('当前选用方案'),
      dataIndex: 'active_plan_id',
      width: 340,
      render: (_, record) => {
        const plans = record.plans || [];
        const activePlans = plans.filter((p) => {
          const today = new Date().toISOString().slice(0, 10).replace(/-/g, '');
          return p.end_date >= today;
        });
        const options = [
          { value: 0, label: t('未启用') },
          ...activePlans.map((p) => ({
            value: p.plan_id,
            label: `${p.plan_name} (${formatDate(p.start_date)}~${formatDate(p.end_date)})`,
          })),
        ];
        let statusLabel = t('当前生效');
        let statusColor = 'green';
        if (!record.active_plan_id) {
          statusLabel = t('未选择方案');
          statusColor = 'grey';
        } else if (record.status === ProjectStatusPaused) {
          statusLabel = t('项目暂停');
          statusColor = 'orange';
        } else if (!record.active_plan_effective) {
          const today = new Date().toISOString().slice(0, 10).replace(/-/g, '');
          statusLabel =
            record.active_plan_start_date > today
              ? t('已选择·未开始')
              : t('已选择·已过期');
          statusColor = record.active_plan_start_date > today ? 'blue' : 'red';
        }
        return (
          <div>
            <div style={{ marginBottom: 8 }}>
              <Tag color={statusColor}>{statusLabel}</Tag>
              {record.active_plan_name && (
                <Text strong style={{ marginLeft: 6 }}>
                  {record.active_plan_name}
                </Text>
              )}
            </div>
            {record.active_plan_start_date && (
              <div style={{ marginBottom: 8 }}>
                <Text type='tertiary' size='small'>
                  {formatDate(record.active_plan_start_date)} ~{' '}
                  {formatDate(record.active_plan_end_date)}
                </Text>
              </div>
            )}
            <Select
              size='small'
              value={record.active_plan_id || 0}
              optionList={options}
              onChange={(val) => handleSetActivePlan(record.id, val)}
              style={{ width: '100%' }}
            />
          </div>
        );
      },
    },
    {
      title: t('当前方案预算($)'),
      dataIndex: 'active_allocated_total',
      width: 230,
      render: (_, record) => {
        if (!record.active_plan_id)
          return <Text type='tertiary'>{t('尚未选择方案')}</Text>;
        const used = (record.active_used_quota || 0) / 500000;
        const remaining = Math.max(
          (record.active_allocated_total || 0) - used,
          0,
        );
        return (
          <div style={{ lineHeight: 1.8 }}>
            <div>
              <Tooltip content={t('查看当前方案的客户分配详情')}>
                <Button
                  theme='borderless'
                  type='primary'
                  size='small'
                  style={{ paddingLeft: 0 }}
                  onClick={() => openActivePlanAllocations(record)}
                >
                  {t('已分配')}：${formatBudget(record.active_allocated_total)}
                </Button>
              </Tooltip>
            </div>
            <div>
              <Text type='tertiary'>{t('已使用')}：</Text>
              <Text>{formatBudget(used)}</Text>
            </div>
            <div>
              <Text type='tertiary'>{t('当前可用')}：</Text>
              {record.active_plan_effective ? (
                <Text type={remaining > 0 ? 'success' : 'danger'} strong>
                  {formatBudget(remaining)}
                </Text>
              ) : (
                <Tooltip content={t('项目或方案当前未生效')}>
                  <Text type='tertiary'>-</Text>
                </Tooltip>
              )}
            </div>
          </div>
        );
      },
    },
    {
      title: t('历史累计($)'),
      dataIndex: 'allocated_total',
      width: 210,
      render: (_, record) => (
        <div style={{ lineHeight: 1.8 }}>
          <div>
            <Text type='tertiary'>{t('已分配')}：</Text>
            <Text>{formatBudget(record.allocated_total)}</Text>
          </div>
          <div>
            <Text type='tertiary'>{t('已使用')}：</Text>
            <Text>{formatBudget((record.quota || 0) / 500000)}</Text>
          </div>
        </div>
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'op',
      width: 160,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button size='small' onClick={() => openEdit(record)}>
            {t('编辑')}
          </Button>
          <Button size='small' onClick={() => openPlanModal(record)}>
            {t('分配计划')}
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div className='mt-[60px] px-2'>
      {/* Dashboard Summary Cards */}
      {dashboardData && (
        <div className='grid grid-cols-2 md:grid-cols-4 gap-4 mb-4'>
          <Card title={t('项目总数')} loading={dashboardLoading}>
            <Text size='large' strong>
              {dashboardData.projects?.length || 0}
            </Text>
          </Card>
          <Card title={t('项目累计预算上限')} loading={dashboardLoading}>
            <Text size='large' strong>
              {dashboardData.projects?.reduce(
                (sum, p) => sum + (p.total_budget || 0),
                0,
              ) || 0}
            </Text>
          </Card>
          <Card title={t('历史累计已分配')} loading={dashboardLoading}>
            <Text size='large' strong>
              {dashboardData.projects?.reduce(
                (sum, p) => sum + (p.allocated_total || 0),
                0,
              ) || 0}
            </Text>
          </Card>
          <Card title={t('历史累计已使用')} loading={dashboardLoading}>
            <Text size='large' strong>
              {formatBudget(
                (dashboardData.projects?.reduce(
                  (sum, p) => sum + (p.used_total || 0),
                  0,
                ) || 0) / 500000,
              )}
            </Text>
          </Card>
        </div>
      )}
      <div className='flex items-center justify-between mb-3'>
        <div>
          <Title heading={4}>{t('项目管理')}</Title>
          <Text type='tertiary'>
            {t(
              '当前方案指标只统计已启用且在有效期内的方案；历史累计指标包含所有方案。',
            )}
          </Text>
        </div>
        <Space>
          <Input
            placeholder={t('搜索项目名称')}
            value={searchKeyword}
            onChange={(v) => setSearchKeyword(v)}
          />
          <Button onClick={() => fetchData(1, pageSize, searchKeyword)}>
            {t('搜索')}
          </Button>
          <Button type='primary' onClick={openCreate}>
            {t('新建')}
          </Button>
        </Space>
      </div>
      <Table
        loading={loading}
        columns={columns}
        dataSource={data}
        rowKey='id'
        scroll={{ x: 1470 }}
        pagination={{
          currentPage: page,
          pageSize,
          total,
          onPageChange: (p) => {
            setPage(p);
            fetchData(p, pageSize, searchKeyword);
          },
          onPageSizeChange: (size) => {
            setPageSize(size);
            fetchData(1, size, searchKeyword);
          },
        }}
      />

      {/* Project Create/Edit Modal */}
      <Modal
        title={editing ? t('编辑项目') : t('新建项目')}
        visible={modalVisible}
        onCancel={handleCloseModal}
        onOk={handleSubmit}
        okButtonProps={{ loading: submitLoading }}
        centered
      >
        <Form getFormApi={setFormApi} initValues={formValues}>
          <Form.Input
            field='project_name'
            label={t('项目名称')}
            placeholder={t('请输入项目名称')}
            disabled={!!editing}
            rules={[{ required: true, message: t('请输入项目名称') }]}
          />
          <Form.InputNumber
            field='total_budget'
            label={t('项目累计预算上限')}
            min={0}
            placeholder={t('请输入项目累计预算上限')}
          />
        </Form>
      </Modal>

      {/* Plan Management Modal */}
      <Modal
        title={
          currentProject
            ? `${t('分配计划')} - ${currentProject.project_name}`
            : t('分配计划')
        }
        visible={planModalVisible}
        onCancel={closePlanModal}
        footer={null}
        width={1040}
        centered
        bodyStyle={{ maxHeight: 'calc(100vh - 180px)', overflowY: 'auto' }}
      >
        {currentProject && (
          <>
            <Descriptions
              data={[
                {
                  key: t('项目累计预算上限'),
                  value: currentProject.total_budget || 0,
                },
                {
                  key: t('历史累计已分配'),
                  value: currentProject.allocated_total || 0,
                },
                {
                  key: t('累计上限剩余可分配'),
                  value: (
                    <Text
                      type={
                        (currentProject.total_budget || 0) -
                          (currentProject.allocated_total || 0) >=
                        0
                          ? 'success'
                          : 'danger'
                      }
                    >
                      {(currentProject.total_budget || 0) -
                        (currentProject.allocated_total || 0)}
                    </Text>
                  ),
                },
              ]}
              row
              style={{ marginBottom: 16 }}
            />
            <div className='flex items-center justify-between mb-3'>
              <Text strong>{t('分配计划列表')}</Text>
              <Button
                type='primary'
                size='small'
                onClick={() => openPlanForm()}
              >
                {t('新建计划')}
              </Button>
            </div>
            <Table
              loading={plansLoading}
              columns={planColumns}
              dataSource={plans}
              rowKey='id'
              size='small'
              pagination={false}
              scroll={{ x: 840, y: 'calc(100vh - 360px)' }}
            />
          </>
        )}
      </Modal>

      {/* Plan Create/Edit Form Modal */}
      <Modal
        title={editingPlan ? t('编辑计划') : t('新建计划')}
        visible={planFormVisible}
        onCancel={closePlanForm}
        onOk={handlePlanSubmit}
        okButtonProps={{ loading: planSubmitLoading }}
        centered
      >
        <Form getFormApi={setPlanFormApi} initValues={planFormValues}>
          <Form.Input
            field='plan_name'
            label={t('计划名称')}
            placeholder={t('请输入计划名称')}
            rules={[{ required: true, message: t('请输入计划名称') }]}
          />
          <Form.Input
            field='start_date'
            label={t('生效日期')}
            placeholder='20260408'
            rules={[{ required: true, message: t('请输入生效日期') }]}
            extraText={t('格式：YYYYMMDD，如 20260408')}
          />
          <Form.Input
            field='end_date'
            label={t('截止日期')}
            placeholder='20260430'
            rules={[{ required: true, message: t('请输入截止日期') }]}
            extraText={t('格式：YYYYMMDD，如 20260430')}
          />
        </Form>
      </Modal>

      {/* Allocation Management Modal (within a plan) */}
      <Modal
        title={
          currentPlan
            ? `${t('预算分配')} - ${currentPlan.plan_name} (${formatDate(currentPlan.start_date)} ~ ${formatDate(currentPlan.end_date)})`
            : t('预算分配')
        }
        visible={allocationModalVisible}
        onCancel={closeAllocationModal}
        footer={null}
        width={1180}
        centered
        bodyStyle={{ maxHeight: 'calc(100vh - 160px)', overflowY: 'auto' }}
      >
        {currentPlan && currentProject && (
          <>
            <Descriptions
              data={[
                {
                  key: t('项目累计预算上限'),
                  value: currentProject.total_budget || 0,
                },
                {
                  key: t('项目历史累计已分配'),
                  value: currentProject.allocated_total || 0,
                },
                {
                  key: t('项目累计剩余可分配'),
                  value: (
                    <Text
                      type={
                        projectCumulativeRemaining > 0 ? 'success' : 'danger'
                      }
                    >
                      {formatBudget(projectCumulativeRemaining)}
                    </Text>
                  ),
                },
                { key: t('本计划已分配'), value: planAllocatedTotal },
                {
                  key: t('本计划已使用'),
                  value: formatBudget(planUsedTotal),
                },
                {
                  key: t('本计划当前可用'),
                  value: (
                    <Text
                      type={
                        planAllocatedTotal - planUsedTotal > 0
                          ? 'success'
                          : 'danger'
                      }
                    >
                      {formatBudget(
                        Math.max(planAllocatedTotal - planUsedTotal, 0),
                      )}
                    </Text>
                  ),
                },
                {
                  key: t('计划状态'),
                  value: currentPlan.is_active ? (
                    <Tag color='green'>{t('启用中')}</Tag>
                  ) : currentPlan.is_expired ? (
                    <Tag color='red'>{t('已过期')}</Tag>
                  ) : (
                    <Tag color='grey'>{t('未启用')}</Tag>
                  ),
                },
              ]}
              row
              style={{ marginBottom: 16 }}
            />
            <div className='flex items-center justify-between mb-3'>
              <Text>{t('分配列表')}</Text>
              <Button
                type='primary'
                size='small'
                onClick={() => openAllocationForm()}
              >
                {t('新建分配')}
              </Button>
            </div>
            <Table
              loading={allocationLoading}
              columns={allocationColumns}
              dataSource={allocations}
              rowKey='id'
              size='small'
              scroll={{ x: 1100, y: 'calc(100vh - 430px)' }}
              pagination={{
                currentPage: allocationPage,
                pageSize: allocationPageSize,
                total: allocationTotal,
                onPageChange: (p) => {
                  setAllocationPage(p);
                  fetchAllocations(currentPlan.id, p, allocationPageSize);
                },
                onPageSizeChange: (size) => {
                  setAllocationPageSize(size);
                  fetchAllocations(currentPlan.id, 1, size);
                },
              }}
            />
          </>
        )}
      </Modal>

      {/* Allocation Create/Edit Form Modal */}
      <Modal
        title={editingAllocation ? t('编辑分配') : t('新建分配')}
        visible={allocationFormVisible}
        onCancel={closeAllocationForm}
        onOk={handleAllocationSubmit}
        okButtonProps={{ loading: allocationSubmitLoading }}
        width={680}
        centered
      >
        {currentProject && currentPlan && (
          <Card
            shadows='hover'
            style={{ marginBottom: 16 }}
            bodyStyle={{ padding: 14 }}
          >
            <Descriptions
              data={[
                {
                  key: t('项目'),
                  value: currentProject.project_name,
                },
                {
                  key: t('预算方案'),
                  value: currentPlan.plan_name,
                },
                {
                  key: t('有效期'),
                  value: `${formatDate(currentPlan.start_date)} ~ ${formatDate(currentPlan.end_date)}`,
                },
                {
                  key: t('项目累计剩余可分配'),
                  value: `$${formatBudget(projectCumulativeRemaining)}`,
                },
                {
                  key: editingAllocation
                    ? t('本次最多可调整至')
                    : t('本次最多可分配'),
                  value: `$${formatBudget(allocationEditableMax)}`,
                },
              ]}
              row
            />
          </Card>
        )}
        <Form
          getFormApi={setAllocationFormApi}
          initValues={allocationFormValues}
        >
          {editingAllocation ? (
            <Form.Input field='client_user_id' label={t('用户ID')} disabled />
          ) : (
            <Form.Select
              field='client_user_id'
              label={t('用户ID')}
              placeholder={t('搜索UID')}
              filter
              remote
              onSearch={(val) => fetchUidOptions(val)}
              loading={uidSearchLoading}
              optionList={uidOptions}
              onChange={(value) => setSelectedAllocationUid(value || '')}
              rules={[{ required: true, message: t('请选择用户ID') }]}
              showClear
              style={{ width: '100%' }}
            />
          )}
          {selectedUidInfo && (
            <div
              style={{
                marginTop: -8,
                marginBottom: 16,
                padding: '10px 12px',
                background: 'var(--semi-color-fill-0)',
                borderRadius: 6,
              }}
            >
              <Text strong>
                {selectedUidInfo.client_name || t('未命名客户')}
              </Text>
              <div
                style={{
                  marginTop: 3,
                  fontFamily: 'monospace',
                  fontSize: 12,
                  wordBreak: 'break-all',
                  color: 'var(--semi-color-text-2)',
                }}
              >
                {selectedUidInfo.value}
              </div>
            </div>
          )}
          <Form.InputNumber
            field='allocated_quota'
            label={t('本方案分配额度($)')}
            min={0}
            max={allocationEditableMax}
            placeholder={t('请输入本方案分配额度')}
            extraText={
              editingAllocation
                ? t(
                    '该客户本方案已使用 ${{used}}；降低额度不会删除历史消耗。',
                    {
                      used: formatBudget(
                        (editingAllocation.used_quota || 0) / 500000,
                      ),
                    },
                  )
                : t('额度会计入项目历史累计已分配，并受项目累计预算上限约束。')
            }
            rules={[{ required: true, message: t('请输入分配额度') }]}
          />
        </Form>
      </Modal>
    </div>
  );
};

export default ProjectPage;
