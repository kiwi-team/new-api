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

import React, { useEffect, useState, useMemo, useCallback } from 'react';
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
  Tooltip,
} from '@douyinfe/semi-ui';

const { Title, Text } = Typography;

const formatPlanDate = (value) => {
  if (!value || value.length !== 8) return value || '-';
  return `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
};

const formatBudget = (value, digits = 6) => {
  const amount = Number(value) || 0;
  return amount.toFixed(digits).replace(/\.?0+$/, '');
};

const BudgetBreakdown = ({ budget, used, available, clickable = false }) => (
  <div style={{ lineHeight: 1.55 }}>
    <div>
      <Text
        type={clickable ? 'primary' : 'tertiary'}
        style={clickable ? { color: 'var(--semi-color-primary)' } : undefined}
      >
        额度：
      </Text>
      <Text
        style={clickable ? { color: 'var(--semi-color-primary)' } : undefined}
      >
        ${formatBudget(budget)}
      </Text>
    </div>
    <div>
      <Text
        type={clickable ? 'primary' : 'tertiary'}
        style={clickable ? { color: 'var(--semi-color-primary)' } : undefined}
      >
        已用：
      </Text>
      <Text
        style={clickable ? { color: 'var(--semi-color-primary)' } : undefined}
      >
        ${formatBudget(used)}
      </Text>
    </div>
    <div>
      <Text
        type={clickable ? 'primary' : 'tertiary'}
        style={clickable ? { color: 'var(--semi-color-primary)' } : undefined}
      >
        可用：
      </Text>
      <Text
        type={clickable ? 'primary' : available > 0 ? 'success' : 'danger'}
        style={clickable ? { color: 'var(--semi-color-primary)' } : undefined}
        strong
      >
        ${formatBudget(available)}
      </Text>
    </div>
  </div>
);

const NonProjectBudgetBreakdown = ({ record, projectSummary }) => {
  const totalUsed = (Number(record.used_quota) || 0) / 500000;
  const monthlyProjectUsed =
    Number(projectSummary?.monthly_project_used_usd) || 0;
  const nonProjectUsed = Math.max(totalUsed - monthlyProjectUsed, 0);
  const fixedBudget = Number(record.fixed_quota) || 0;
  const tempBudget = Number(record.temp_quota) || 0;
  const isTempExpired =
    record.expired_at > 0 && record.expired_at <= Date.now() / 1000;
  const effectiveTempBudget = isTempExpired ? 0 : tempBudget;
  const available = Math.max(
    fixedBudget + effectiveTempBudget - nonProjectUsed,
    0,
  );
  const tempExpiry =
    tempBudget <= 0
      ? '-'
      : record.expired_at
        ? `${new Date(record.expired_at * 1000).toLocaleString()} 到期`
        : '长期有效';

  return (
    <div style={{ lineHeight: 1.65 }}>
      <div>
        <Text type='tertiary'>月度固定预算：</Text>
        <Text>${formatBudget(fixedBudget)}</Text>
      </div>
      <div>
        <Text type='tertiary'>临时预算：</Text>
        <Text>${formatBudget(tempBudget)}</Text>
        <Text type={isTempExpired ? 'danger' : 'secondary'}>
          {' '}
          （{tempExpiry}）
        </Text>
        {isTempExpired && (
          <Tag color='red' size='small' style={{ marginLeft: 6 }}>
            已过期
          </Tag>
        )}
      </div>
      <div>
        <Text type='tertiary'>本月已使用非项目预算：</Text>
        <Text>${formatBudget(nonProjectUsed)}</Text>
      </div>
      <div>
        <Text type='tertiary'>可用：</Text>
        <Text type={available > 0 ? 'success' : 'danger'} strong>
          ${formatBudget(available)}
        </Text>
      </div>
    </div>
  );
};

const renderAllocationStatus = (record) => {
  if (record.is_current_effective) return <Tag color='green'>当前生效</Tag>;
  if (record.project_status === 2) return <Tag color='orange'>项目暂停</Tag>;
  if (record.is_active_plan && !record.is_in_date_range) {
    const today = new Date().toISOString().slice(0, 10).replace(/-/g, '');
    return record.start_date > today ? (
      <Tag color='blue'>未开始</Tag>
    ) : (
      <Tag color='red'>已过期</Tag>
    );
  }
  return <Tag color='grey'>历史方案</Tag>;
};

const CliendUserQuotaPage = () => {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(100);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [formApi, setFormApi] = useState(null);

  // 判断是否为管理员或root用户
  const isAdminOrRoot = useMemo(() => isAdmin() || isRoot(), []);

  const initFormValues = {
    client_user_id: '',
    client_name: '',
    fixed_quota: 0,
    temp_quota: 0,
    remark: '',
    expired_at: null,
  };
  const [formValues, setFormValues] = useState({
    ...initFormValues,
  });
  const [submitLoading, setSubmitLoading] = useState(false);

  // 项目预算弹窗
  const [projectModalVisible, setProjectModalVisible] = useState(false);
  const [projectAllocations, setProjectAllocations] = useState([]);
  const [projectModalLoading, setProjectModalLoading] = useState(false);
  const [projectModalUid, setProjectModalUid] = useState('');
  // 项目预算汇总（列表内联显示）
  const [projectBudgetMap, setProjectBudgetMap] = useState({});

  const fetchBatchProjectBudget = useCallback(async (items) => {
    if (!items || items.length === 0) return;
    const uids = items.map((r) => r.client_user_id).join(',');
    try {
      const res = await API.get('/api/cliend_user_quota/batch-project-budget', {
        params: { uids },
      });
      const { success, data } = res.data;
      if (success && data) {
        setProjectBudgetMap(data);
      }
    } catch (e) {
      // silent
    }
  }, []);

  const fetchProjectAllocations = async (clientUserId) => {
    setProjectModalUid(clientUserId);
    setProjectModalVisible(true);
    setProjectModalLoading(true);
    try {
      const res = await API.get('/api/cliend_user_quota/project-allocations', {
        params: { client_user_id: clientUserId },
      });
      const { success, data } = res.data;
      if (success) {
        setProjectAllocations(Array.isArray(data) ? data : []);
      } else {
        setProjectAllocations([]);
      }
    } catch (e) {
      setProjectAllocations([]);
    } finally {
      setProjectModalLoading(false);
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
      let url = '/api/cliend_user_quota/';
      if (keyword && keyword.trim()) {
        url = '/api/cliend_user_quota/search';
        params.keyword = keyword.trim();
      }
      const res = await API.get(url, { params });
      const { success, message, data } = res.data;
      if (success) {
        const items = data.items || [];
        setData(items);
        setTotal(data.total || 0);
        setPage(data.p || pageNum);
        setPageSize(data.page_size || size);
        fetchBatchProjectBudget(items);
      } else {
        showError(message || '加载失败');
      }
    } catch (e) {
      showError('加载失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData(1, pageSize, '');
  }, []);

  const openCreate = () => {
    setEditing(null);
    const now = new Date();
    const firstNext = new Date(
      now.getFullYear(),
      now.getMonth() + 1,
      1,
      0,
      0,
      0,
    );
    const endMonth = new Date(firstNext.getTime() - 1000);
    setFormValues({
      ...initFormValues,
      expired_at: endMonth,
    });
    setModalVisible(true);
  };

  const openEdit = (record) => {
    setEditing(record);
    setFormValues({
      client_user_id: record.client_user_id,
      client_name: record.client_name || '',
      fixed_quota: record.fixed_quota,
      temp_quota: record.temp_quota,
      remark: record.remark || '',
      expired_at: record.expired_at ? new Date(record.expired_at * 1000) : null,
    });
    setModalVisible(true);
  };

  const handleDelete = async (record) => {
    Modal.confirm({
      title: '确认删除',
      content: `确认删除 ${record.client_user_id} 的预算记录？`,
      onOk: async () => {
        try {
          const res = await API.delete(`/api/cliend_user_quota/${record.id}`);
          const { success, message } = res.data;
          if (success) {
            showSuccess('删除成功');
            fetchData(page, pageSize, searchKeyword);
          } else {
            showError(message || '删除失败');
          }
        } catch (e) {
          showError('删除失败');
        }
      },
    });
  };

  const handleSubmit = async () => {
    setSubmitLoading(true);
    try {
      const payload = formApi ? formApi.getValues() : { ...formValues };
      if (payload.client_user_id) {
        payload.client_user_id = payload.client_user_id.trim();
      }
      if (payload.expired_at instanceof Date) {
        payload.expired_at = Math.floor(payload.expired_at.getTime() / 1000);
      }
      const api = editing ? API.put : API.post;
      const res = await api('/api/cliend_user_quota/', payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess(editing ? '更新成功' : '创建成功');
        setModalVisible(false);
        fetchData(page, pageSize, searchKeyword);
      } else {
        showError(message || '操作失败');
      }
    } catch (e) {
      showError('操作失败');
    } finally {
      setSubmitLoading(false);
    }
  };

  const handleCloseModal = () => {
    setModalVisible(false);
    formApi && formApi.reset();
    setFormValues({
      ...initFormValues,
    });
    setEditing(null);
  };

  useEffect(() => {
    if (modalVisible && formApi) {
      formApi.setValues(formValues);
    }
  }, [modalVisible, formValues, formApi]);

  const columns = [
    //{ title: 'ID', dataIndex: 'id', width: 80 },
    { title: 'Client UID', dataIndex: 'client_user_id', width: 220 },
    // 只有管理员或root用户才能看到ClientName列
    ...(isAdminOrRoot
      ? [{ title: 'Client Name', dataIndex: 'client_name', width: 150 }]
      : []),
    {
      title: '月度非项目预算($)',
      dataIndex: 'non_project_budget',
      width: 300,
      render: (_, record) => (
        <NonProjectBudgetBreakdown
          record={record}
          projectSummary={projectBudgetMap[record.client_user_id]}
        />
      ),
      sorter: (a, b) =>
        (parseInt(a.fixed_quota, 10) || 0) +
        (parseInt(a.temp_quota, 10) || 0) -
        ((parseInt(b.fixed_quota, 10) || 0) +
          (parseInt(b.temp_quota, 10) || 0)),
    },
    {
      title: '当前项目预算($)',
      dataIndex: 'project_budget',
      width: 220,
      render: (_, record) => {
        const summary = projectBudgetMap[record.client_user_id];
        if (!summary || !summary.projects || summary.projects.length === 0) {
          return (
            <Button
              theme='borderless'
              type='primary'
              size='small'
              onClick={() => fetchProjectAllocations(record.client_user_id)}
            >
              无当前生效预算
            </Button>
          );
        }
        const tooltipContent = (
          <div>
            {summary.projects.map((p) => (
              <div key={p.allocation_id} style={{ marginBottom: 4 }}>
                <div>
                  {p.project_name} / {p.plan_name}
                </div>
                <div>
                  分配 ${formatBudget(p.allocated_quota)}，已使用 $
                  {formatBudget(p.used_quota_usd)}，可用 $
                  {formatBudget(p.remaining_quota_usd)}
                </div>
              </div>
            ))}
          </div>
        );
        return (
          <Tooltip content={tooltipContent} position='top'>
            <Button
              theme='borderless'
              type='primary'
              size='small'
              onClick={() => fetchProjectAllocations(record.client_user_id)}
            >
              <BudgetBreakdown
                budget={summary.total_allocated}
                used={summary.total_used_usd}
                available={summary.total_remaining_usd}
                clickable
              />
            </Button>
          </Tooltip>
        );
      },
    },
    {
      title: '本月总使用($)',
      dataIndex: 'used_quota',
      width: 140,
      render: (v) => formatBudget((parseInt(v, 10) || 0) / 500000),
      sorter: (a, b) =>
        (parseInt(a.used_quota, 10) || 0) - (parseInt(b.used_quota, 10) || 0),
    },
    {
      title: '操作',
      dataIndex: 'op',
      width: 220,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button onClick={() => openEdit(record)}>编辑</Button>
          <Button type='danger' onClick={() => handleDelete(record)}>
            删除
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div className='mt-[60px] px-2'>
      <div className='flex items-center justify-between mb-3'>
        <div>
          <Title heading={4}>UID 预算管理</Title>
          <Text type='tertiary'>
            项目与非项目消费分别统计；非项目可用预算为月度固定预算与有效临时预算之和，扣除本月非项目使用金额。
          </Text>
        </div>
        <Space>
          <Input
            placeholder='搜索 Client UID'
            value={searchKeyword}
            onChange={(v) => setSearchKeyword(v)}
          />
          <Button onClick={() => fetchData(1, pageSize, searchKeyword)}>
            搜索
          </Button>
          {isAdminOrRoot && (
            <Button
              type='tertiary'
              onClick={async () => {
                try {
                  const res = await API.get('/api/cliend_user_quota/export', {
                    responseType: 'blob',
                  });
                  const url = window.URL.createObjectURL(new Blob([res.data]));
                  const a = document.createElement('a');
                  a.href = url;
                  a.download = 'cliend_user_quota.csv';
                  a.click();
                  window.URL.revokeObjectURL(url);
                } catch (e) {
                  showError('导出失败');
                }
              }}
            >
              导出CSV
            </Button>
          )}
          <Button type='primary' onClick={openCreate}>
            新建
          </Button>
        </Space>
      </div>
      <Table
        loading={loading}
        columns={columns}
        dataSource={data}
        scroll={{ x: 1300 }}
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

      <Modal
        title={editing ? '编辑预算' : '新建预算'}
        visible={modalVisible}
        onCancel={handleCloseModal}
        onOk={handleSubmit}
        okButtonProps={{ loading: submitLoading }}
        centered
      >
        <Form getFormApi={setFormApi} initValues={formValues}>
          <Form.Input
            field='client_user_id'
            label='Client UID'
            disabled={!!editing}
          />
          {isAdminOrRoot && (
            <Form.Input
              field='client_name'
              label='Client Name'
              placeholder='客户名称（仅管理员可见）'
            />
          )}
          <Form.InputNumber field='fixed_quota' label='月度固定预算' />
          <Form.InputNumber field='temp_quota' label='临时预算' />
          <Form.DatePicker
            field='expired_at'
            type='dateTime'
            label='临时预算过期时间'
          />
          <Form.Input field='remark' label='备注' />
        </Form>
      </Modal>

      <Modal
        title={`项目预算详情（含历史）- ${projectModalUid}`}
        visible={projectModalVisible}
        onCancel={() => setProjectModalVisible(false)}
        footer={null}
        centered
        width={1100}
      >
        <div style={{ marginBottom: 12, color: 'var(--semi-color-text-2)' }}>
          “当前生效”需同时满足：项目已启用、方案被选为启用方案，且当天在预算有效期内。
        </div>
        <Table
          loading={projectModalLoading}
          dataSource={projectAllocations}
          rowKey='allocation_id'
          pagination={false}
          size='small'
          columns={[
            {
              title: '项目名称',
              dataIndex: 'project_name',
              key: 'project_name',
              width: 220,
            },
            {
              title: '预算方案',
              dataIndex: 'plan_name',
              key: 'plan_name',
              width: 140,
            },
            {
              title: '状态',
              key: 'status',
              width: 100,
              render: (_, record) => renderAllocationStatus(record),
            },
            {
              title: '有效期',
              key: 'validity',
              width: 200,
              render: (_, record) =>
                `${formatPlanDate(record.start_date)} ~ ${formatPlanDate(record.end_date)}`,
            },
            {
              title: '分配预算($)',
              dataIndex: 'allocated_quota',
              key: 'allocated_quota',
              width: 120,
            },
            {
              title: '方案已消耗($)',
              dataIndex: 'used_quota_usd',
              key: 'used_quota_usd',
              width: 140,
              render: (v) => {
                const val = parseFloat(v) || 0;
                return val.toFixed(6);
              },
            },
            {
              title: '方案剩余($)',
              dataIndex: 'remaining_quota_usd',
              key: 'remaining_quota_usd',
              width: 130,
              render: (v) => formatBudget(v),
            },
          ]}
          empty={<span style={{ color: '#999' }}>暂无项目预算分配</span>}
        />
      </Modal>
    </div>
  );
};

export default CliendUserQuotaPage;
