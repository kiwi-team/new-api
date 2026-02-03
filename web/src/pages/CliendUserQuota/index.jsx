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
import { Button, Table, Modal, Form, Input, Space, Typography } from '@douyinfe/semi-ui';

const { Title } = Typography;

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
  }
  const [formValues, setFormValues] = useState({
    ...initFormValues, 
  });
  const [submitLoading, setSubmitLoading] = useState(false);

  const fetchData = async (pageNum = page, size = pageSize, keyword = searchKeyword) => {
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
        setData(data.items || []);
        setTotal(data.total || 0);
        setPage(data.p || pageNum);
        setPageSize(data.page_size || size);
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
    const firstNext = new Date(now.getFullYear(), now.getMonth() + 1, 1, 0, 0, 0);
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
    })
    setEditing(null);
  }

  useEffect(() => {
    if (modalVisible && formApi) {
      formApi.setValues(formValues);
    }
  }, [modalVisible, formValues, formApi]);

  const columns = [
    //{ title: 'ID', dataIndex: 'id', width: 80 },
    { title: 'Client UID', dataIndex: 'client_user_id', width: 220 },
    // 只有管理员或root用户才能看到ClientName列
    ...(isAdminOrRoot ? [{ title: 'Client Name', dataIndex: 'client_name', width: 150 }] : []),
    { 
      title: '月度固定预算', 
      dataIndex: 'fixed_quota', 
      width: 130,
      sorter: (a, b) => (parseInt(a.fixed_quota, 10) || 0) - (parseInt(b.fixed_quota, 10) || 0),
    },
    { 
      title: '临时预算', 
      dataIndex: 'temp_quota', 
      width: 100,
      sorter: (a, b) => (parseInt(a.temp_quota, 10) || 0) - (parseInt(b.temp_quota, 10) || 0),
    },
    { 
      title: '本月已使用($)', 
      dataIndex: 'used_quota', 
      width: 160, 
      render: (v) => ((parseInt(v, 10) || 0) / 500000).toFixed(6),
      sorter: (a, b) => (parseInt(a.used_quota, 10) || 0) - (parseInt(b.used_quota, 10) || 0),
    },
    { title: '临时预算过期时间', dataIndex: 'expired_at', width: 200, render: (v) => (v ? new Date(v * 1000).toLocaleString() : '-') },
    {
      title: '操作',
      dataIndex: 'op',
      width: 220,
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
        <Title heading={4}>UID 预算管理</Title>
        <Space>
          <Input
            placeholder='搜索 Client UID'
            value={searchKeyword}
            onChange={(v) => setSearchKeyword(v)}
          />
          <Button onClick={() => fetchData(1, pageSize, searchKeyword)}>搜索</Button>
          <Button type='primary' onClick={openCreate}>
            新建
          </Button>
        </Space>
      </div>
      <Table
        loading={loading}
        columns={columns}
        dataSource={data}
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
              <Form.InputNumber
                field='fixed_quota'
                label='月度固定预算'
              />
              <Form.InputNumber
                field='temp_quota'
                label='临时预算'
              />
              <Form.DatePicker
                field='expired_at'
                type='dateTime'
                label='临时预算过期时间'
              />
              <Form.Input
                field='remark'
                label='备注'
              />
            </Form>
          </Modal>
        
    </div>
  );
};

export default CliendUserQuotaPage;
