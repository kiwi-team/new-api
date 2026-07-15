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
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  copy,
  getTodayStartTimestamp,
  isAdmin,
  isRoot,
  showError,
  showSuccess,
  timestamp2string,
} from '../../helpers';

import {
  Button,
  Descriptions,
  Empty,
  Modal,
  Table,
  Tag,
  Tooltip,
  Checkbox,
  Card,
  Form,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { ITEMS_PER_PAGE } from '../../constants';
import {
  IconSetting,
  IconSearch,
  IconHelpCircle,
  IconEyeOpened,
  IconCopy,
  IconDownload,
} from '@douyinfe/semi-icons';

const ErrorLogsTable = () => {
  const { t } = useTranslation();

  // Define column keys for selection
  const COLUMN_KEYS = {
    ID: 'id',
    TOKEN_ID: 'token_id',
    USERID: 'user_id',
    CREATEDAT: 'created_at',
    CHANNELID: 'channel_id',
    CHANNELNAME: 'channel_name',
    MODELNAME: 'model_name',
    MESSAGE: 'message',
    TYPE: 'type',
    PARAM: 'param',
    CODE: 'code',
    REQUESTID: 'request_id',
    STATUSCODE: 'status_code',
    USETIME: 'use_time_ms',
    IP: 'ip',
    BODY: 'body',
    HEADER: 'header',
    USER_CLIENT_ID: 'user_client_id',
  };

  // State for column visibility
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);
  const [showDetailModal, setShowDetailModal] = useState(false);
  const [detailContent, setDetailContent] = useState('');
  const [loadingBodyId, setLoadingBodyId] = useState(null);
  const [loadingHeaderId, setLoadingHeaderId] = useState(null);
  const [exporting, setExporting] = useState(false);

  // Load saved column preferences from localStorage
  useEffect(() => {
    const savedColumns = localStorage.getItem('error-logs-table-columns');
    if (savedColumns) {
      try {
        const parsed = JSON.parse(savedColumns);
        // Make sure all columns are accounted for
        const defaults = getDefaultColumnVisibility();
        const merged = { ...defaults, ...parsed };
        // HEADER 列仅 root 可见，对非 root 强制关闭，防止本地存储里残留 true 状态泄露入口
        if (!isRoot()) {
          merged[COLUMN_KEYS.HEADER] = false;
        }
        setVisibleColumns(merged);
      } catch (e) {
        console.error('Failed to parse saved column preferences', e);
        initDefaultColumns();
      }
    } else {
      initDefaultColumns();
    }
  }, []);

  // Get default column visibility based on user role
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.ID]: true,
      [COLUMN_KEYS.TOKEN_ID]: true,
      [COLUMN_KEYS.USERID]: true,
      [COLUMN_KEYS.CHANNELID]: isAdminUser,
      [COLUMN_KEYS.CHANNELNAME]: true,
      [COLUMN_KEYS.CREATEDAT]: true,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.MODELNAME]: true,
      [COLUMN_KEYS.MESSAGE]: true,
      [COLUMN_KEYS.BODY]: true,
      [COLUMN_KEYS.PARAM]: true,
      [COLUMN_KEYS.CODE]: true,
      [COLUMN_KEYS.REQUESTID]: true,
      [COLUMN_KEYS.STATUSCODE]: isAdminUser,
      [COLUMN_KEYS.USETIME]: true,
      [COLUMN_KEYS.IP]: true,
      [COLUMN_KEYS.USER_CLIENT_ID]: true,
      // HEADER 默认仅对 root 可见，其它角色完全看不到入口
      [COLUMN_KEYS.HEADER]: isRoot(),
    };
  };

  // Initialize default column visibility
  const initDefaultColumns = () => {
    const defaults = getDefaultColumnVisibility();
    setVisibleColumns(defaults);
    localStorage.setItem('error-logs-table-columns', JSON.stringify(defaults));
  };

  // Handle column visibility change
  const handleColumnVisibilityChange = (columnKey, checked) => {
    const updatedColumns = { ...visibleColumns, [columnKey]: checked };
    setVisibleColumns(updatedColumns);
  };

  // Handle "Select All" checkbox
  const handleSelectAll = (checked) => {
    const allKeys = Object.keys(COLUMN_KEYS).map((key) => COLUMN_KEYS[key]);
    const updatedColumns = {};

    allKeys.forEach((key) => {
      // For admin-only columns, only enable them if user is admin
      if (
        (key === COLUMN_KEYS.CHANNELID ||
          key === COLUMN_KEYS.USERID ||
          key === COLUMN_KEYS.REQUESTID) &&
        !isAdminUser
      ) {
        updatedColumns[key] = false;
      } else if (key === COLUMN_KEYS.HEADER && !isRoot()) {
        updatedColumns[key] = false;
      } else {
        updatedColumns[key] = checked;
      }
    });

    setVisibleColumns(updatedColumns);
  };

  // Define all columns
  const allColumns = [
    {
      key: COLUMN_KEYS.ID,
      title: t('ID'),
      dataIndex: 'id',
    },
    {
      key: COLUMN_KEYS.USER_CLIENT_ID,
      title: t('Client USER ID'),
      dataIndex: 'client_user_id',
    },
    {
      key: COLUMN_KEYS.TOKEN_ID,
      title: t('Token ID'),
      dataIndex: 'token_id',
      render: (text, record, index) => {
        return (
          <>
            {t(record.token_name)}({text})
          </>
        );
      },
    },
    {
      key: COLUMN_KEYS.CREATEDAT,
      title: t('时间'),
      dataIndex: 'timestamp2string',
    },
    {
      key: COLUMN_KEYS.CHANNELNAME,
      title: t('渠道'),
      dataIndex: 'channel_name',
      className: 'tableShow',
      render: (text, record, index) => {
        return (
          <>
            {t(text)}({record.channel_id})
          </>
        );
      },
    },
    {
      key: COLUMN_KEYS.USERID,
      title: t('用户ID'),
      dataIndex: 'user_id',
      className: 'tableShow',
      render: (text, record, index) => {
        return <>{t(text)}</>;
      },
    },
    {
      key: COLUMN_KEYS.MESSAGE,
      title: t('Message'),
      dataIndex: 'message',
      render: (text, record, index) => {
        return (
          <div className='flex items-center gap-2'>
            <div className='max-w-[200px] overflow-auto truncate'>
              {t(text)}
            </div>
            <div className='flex gap-1'>
              <Button
                theme='borderless'
                type='tertiary'
                size='small'
                icon={<IconEyeOpened />}
                onClick={(e) => {
                  e.stopPropagation();
                  showDetailDialog(text, false);
                }}
              />
              <Button
                theme='borderless'
                type='tertiary'
                size='small'
                icon={<IconCopy />}
                onClick={(e) => copyBodyContent(e, text)}
              />
            </div>
          </div>
        );
      },
    },
    {
      key: COLUMN_KEYS.BODY,
      title: t('Body'),
      dataIndex: 'body',
      render: (text, record, index) => {
        return (
          <div className='flex items-center gap-2'>
            <Button
              theme='borderless'
              type='tertiary'
              size='small'
              icon={<IconEyeOpened />}
              loading={loadingBodyId === record.id}
              onClick={(e) => {
                e.stopPropagation();
                fetchAndShowBody(record.id);
              }}
            />
          </div>
        );
      },
    },
    {
      // 请求头列：仅 root 可见，调用 /api/log/error-logs/:id/header（后端 RootAuth 兜底）
      key: COLUMN_KEYS.HEADER,
      title: t('请求头'),
      dataIndex: 'header',
      className: isRoot() ? '' : 'tableHiddle',
      render: (text, record, index) => {
        if (!isRoot()) return null;
        return (
          <div className='flex items-center gap-2'>
            <Button
              theme='borderless'
              type='tertiary'
              size='small'
              icon={<IconEyeOpened />}
              loading={loadingHeaderId === record.id}
              onClick={(e) => {
                e.stopPropagation();
                fetchAndShowHeader(record.id);
              }}
            />
          </div>
        );
      },
    },
    {
      key: COLUMN_KEYS.MODELNAME,
      title: t('模型'),
      dataIndex: 'model_name',
      render: (text, record, index) => {
        return <>{t(text)}</>;
      },
    },
    {
      key: COLUMN_KEYS.PARAM,
      title: t('param'),
      dataIndex: 'param',
      render: (text, record, index) => {
        return <>{t(text)}</>;
      },
    },
    {
      key: COLUMN_KEYS.IP,
      title: (
        <div className='flex items-center gap-1'>
          {t('IP')}
          <Tooltip
            content={t(
              '只有当用户设置开启IP记录时，才会进行请求和错误类型日志的IP记录',
            )}
          >
            <IconHelpCircle className='text-gray-400 cursor-help' />
          </Tooltip>
        </div>
      ),
      dataIndex: 'ip',
      render: (text, record, index) => {
        return (
          <Tooltip content={text}>
            <Tag
              color='orange'
              size='large'
              shape='circle'
              onClick={(event) => {
                copyText(event, text);
              }}
            >
              {text}
            </Tag>
          </Tooltip>
        );
      },
    },
    {
      key: COLUMN_KEYS.REQUESTID,
      title: t('requestID'),
      dataIndex: 'request_id',
      className: isAdmin() ? 'tableShow' : 'tableHiddle',
      render: (text, record, index) => {
        return <>{t(text)}</>;
      },
    },
    {
      key: COLUMN_KEYS.STATUSCODE,
      title: t('status_code'),
      dataIndex: 'status_code',
      fixed: 'right',
      render: (text, record, index) => {
        return <>{t(text)}</>;
      },
    },
    {
      key: COLUMN_KEYS.USETIME,
      title: t('耗时'),
      dataIndex: 'use_time_ms',
      fixed: 'right',
      render: (text, record, index) => {
        const ms = Number(text);
        if (!ms || ms <= 0) {
          return <>-</>;
        }
        return <>{`${parseFloat((ms / 1000).toFixed(2))}s`}</>;
      },
    },
  ];

  // Update table when column visibility changes
  useEffect(() => {
    if (Object.keys(visibleColumns).length > 0) {
      // Save to localStorage
      localStorage.setItem(
        'error-logs-table-columns',
        JSON.stringify(visibleColumns),
      );
    }
  }, [visibleColumns]);

  // Filter columns based on visibility settings
  const getVisibleColumns = () => {
    return allColumns.filter((column) => visibleColumns[column.key]);
  };

  // Column selector modal
  const renderColumnSelector = () => {
    return (
      <Modal
        title={t('列设置')}
        visible={showColumnSelector}
        onCancel={() => setShowColumnSelector(false)}
        footer={
          <div className='flex justify-end'>
            <Button
              theme='light'
              onClick={() => initDefaultColumns()}
              className='!rounded-full'
            >
              {t('重置')}
            </Button>
            <Button
              theme='light'
              onClick={() => setShowColumnSelector(false)}
              className='!rounded-full'
            >
              {t('取消')}
            </Button>
            <Button
              type='primary'
              onClick={() => setShowColumnSelector(false)}
              className='!rounded-full'
            >
              {t('确定')}
            </Button>
          </div>
        }
      >
        <div style={{ marginBottom: 20 }}>
          <Checkbox
            checked={Object.values(visibleColumns).every((v) => v === true)}
            indeterminate={
              Object.values(visibleColumns).some((v) => v === true) &&
              !Object.values(visibleColumns).every((v) => v === true)
            }
            onChange={(e) => handleSelectAll(e.target.checked)}
          >
            {t('全选')}
          </Checkbox>
        </div>
        <div
          className='flex flex-wrap max-h-96 overflow-y-auto rounded-lg p-4'
          style={{ border: '1px solid var(--semi-color-border)' }}
        >
          {allColumns.map((column) => {
            // Skip admin-only columns for non-admin users
            if (
              !isAdminUser &&
              (column.key === COLUMN_KEYS.CHANNELID ||
                column.key === COLUMN_KEYS.USERID ||
                column.key === COLUMN_KEYS.REQUESTID)
            ) {
              return null;
            }

            return (
              <div key={column.key} className='w-1/2 mb-4 pr-2'>
                <Checkbox
                  checked={!!visibleColumns[column.key]}
                  onChange={(e) =>
                    handleColumnVisibilityChange(column.key, e.target.checked)
                  }
                >
                  {column.title}
                </Checkbox>
              </div>
            );
          })}
        </div>
      </Modal>
    );
  };

  const [logs, setLogs] = useState([]);
  const [expandData, setExpandData] = useState({});
  const [showStat, setShowStat] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingStat, setLoadingStat] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [logCount, setLogCount] = useState(ITEMS_PER_PAGE);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [logType, setLogType] = useState(0);
  const isAdminUser = isAdmin();
  const isRootUser = isRoot();
  let now = new Date();

  // 渠道 / 令牌下拉选项(仅 root 用户使用,精确匹配);其他用户维持输入框
  const [channelOptions, setChannelOptions] = useState([]);
  const [tokenIdOptions, setTokenIdOptions] = useState([]);

  // 渠道列表(root only):后端按 channel_id 精确匹配,选项值为渠道 ID
  const fetchChannelOptions = async () => {
    try {
      const res = await API.get('/api/channel/channel-name-list');
      const { success, data } = res.data;
      if (success && Array.isArray(data)) {
        const options = data.map((ch) => ({
          value: ch.id,
          label: `${ch.name || '[未知]'} (ID: ${ch.id})`,
        }));
        setChannelOptions(options);
      }
    } catch (error) {
      console.error('Failed to fetch channel options:', error);
    }
  };

  // 令牌列表:后端按 token_id 精确匹配,选项值为令牌 ID
  const fetchTokenIdOptions = async () => {
    try {
      const res = await API.get('/api/data/token-list');
      const { success, data } = res.data;
      if (success && Array.isArray(data)) {
        const options = data.map((token) => ({
          value: token.id,
          label: `${token.name || '[未命名]'} (ID: ${token.id})`,
        }));
        setTokenIdOptions(options);
      }
    } catch (error) {
      console.error('Failed to fetch token options:', error);
    }
  };

  // 仅 root 用户加载下拉选项;其他用户维持输入框,无需请求
  useEffect(() => {
    if (isRootUser) {
      fetchChannelOptions();
      fetchTokenIdOptions();
    }
  }, [isRootUser]);

  // Form 初始值
  const formInitValues = {
    p: 1,
    page_size: 10,
    channel: 0,
    request_id: '',
    model_name: '',
    channel: '',
    token_id: '',
    client_user_id:'',
    mt_session_id: '',
    trace_id: '',
    traj_id: '',
    session_id: '',
    dateRange: [
      timestamp2string(now.getTime() / 1000 - 3600),
      timestamp2string(now.getTime() / 1000 + 3600),
    ],
  };

  const [stat, setStat] = useState({
    quota: 0,
    token: 0,
  });

  // Form API 引用
  const [formApi, setFormApi] = useState(null);

  // 获取表单值的辅助函数，确保所有值都是字符串
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};

    // 处理时间范围
    let start_timestamp = timestamp2string(now.getTime() / 1000 - 3600);
    let end_timestamp = timestamp2string(now.getTime() / 1000 + 3600);

    if (
      formValues.dateRange &&
      Array.isArray(formValues.dateRange) &&
      formValues.dateRange.length === 2
    ) {
      start_timestamp = formValues.dateRange[0];
      end_timestamp = formValues.dateRange[1];
    }

    return {
      model_name: formValues.model_name || '',
      start_timestamp,
      end_timestamp,
      channel: formValues.channel || 0,
      request_id: formValues.request_id,
      p: formValues.p || 1,
      page_size: formValues.page_size || 10,
      token_id: formValues.token_id || 0,
      client_user_id: formValues.client_user_id || '',
      // 提交到后台前去掉首尾空白；URLSearchParams 已正确编码 + 等特殊字符
      mt_session_id: (formValues.mt_session_id || '').trim(),
      trace_id: (formValues.trace_id || '').trim(),
      traj_id: (formValues.traj_id || '').trim(),
      session_id: (formValues.session_id || '').trim(),
    };
  };

  const getErrorLogStat = async () => {
    const {
      request_id,
      p,
      page_size,
      model_name,
      start_timestamp,
      end_timestamp,
      channel,
      token_id,
      client_user_id,
    } = getFormValues();
    const currentLogType = formLogType !== undefined ? formLogType : logType;
    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    let res = await API.get("/api/log/error-logs",{
      params: {
        model_name,
        start_timestamp: localStartTimestamp,
        end_timestamp: localEndTimestamp,
        channel,
        request_id,
        p,
        page_size,
        token_id,
        client_user_id,
      },
    });
    const { success, message, data } = res.data;
    if (success) {
      setStat(data);
    } else {
      showError(message);
    }
  };

  const handleEyeClick = async () => {
    return;
    if (loadingStat) {
      return;
    }
    setLoadingStat(true);
    //await getErrorLogStat();
    if (isAdminUser) {
    } else {
      //await getLogSelfStat();
    }
    setShowStat(true);
    setLoadingStat(false);
  };

  const setLogsFormat = (logs) => {
    for (let i = 0; i < logs.length; i++) {
      logs[i].timestamp2string = timestamp2string(logs[i].created_at);
      logs[i].key = logs[i].id;
    }
    setLogs(logs);
  };

  const loadLogs = async (startIdx, pageSize, customLogType = null) => {
    setLoading(true);

    let url = '';
    const { model_name, start_timestamp, end_timestamp, channel, request_id, token_id, client_user_id, mt_session_id, trace_id, traj_id, session_id } =
      getFormValues();

    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    const params = new URLSearchParams({
      p: String(startIdx),
      page_size: String(pageSize),
      model_name: model_name || '',
      start_timestamp: String(localStartTimestamp),
      end_timestamp: String(localEndTimestamp),
      channel: channel ? String(channel) : '',
      request_id: request_id || '',
      token_id: token_id ? String(token_id) : '',
      client_user_id: client_user_id || '',
      mt_session_id: mt_session_id || '',
      trace_id: trace_id || '',
      traj_id: traj_id || '',
      session_id: session_id || '',
    });
    // 错误日志页为 root only,统一走管理员接口
    url = `/api/log/error-logs?${params.toString()}`;
    const res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      const newPageData = data.items;
      setActivePage(data.page);
      setPageSize(data.page_size);
      setLogCount(data.total);

      setLogsFormat(newPageData);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const handlePageChange = (page) => {
    setActivePage(page);
    loadLogs(page, pageSize).then((r) => {}); // 不传入logType，让其从表单获取最新值
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    loadLogs(activePage, size)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  };

  const refresh = async () => {
    setActivePage(1);
    //handleEyeClick();
    await loadLogs(1, pageSize); // 不传入logType，让其从表单获取最新值
  };

  // 导出错误日志为 CSV（仅 root，时间范围 ≤ 24 小时）
  const handleExportErrorLogs = async () => {
    if (!isRoot()) {
      showError(t('仅超级管理员可导出错误日志'));
      return;
    }

    const {
      model_name,
      start_timestamp,
      end_timestamp,
      channel,
      request_id,
      token_id,
      client_user_id,
      mt_session_id,
      trace_id,
      traj_id,
    } = getFormValues();

    const localStartTimestamp = Date.parse(start_timestamp) / 1000;
    const localEndTimestamp = Date.parse(end_timestamp) / 1000;

    if (
      !Number.isFinite(localStartTimestamp) ||
      !Number.isFinite(localEndTimestamp) ||
      !localStartTimestamp ||
      !localEndTimestamp
    ) {
      showError(t('请选择导出时间范围'));
      return;
    }
    if (localEndTimestamp < localStartTimestamp) {
      showError(t('结束时间必须晚于开始时间'));
      return;
    }
    const maxRangeSeconds = 24 * 3600;
    if (localEndTimestamp - localStartTimestamp > maxRangeSeconds) {
      showError(t('导出时间范围不能超过 24 小时'));
      return;
    }

    setExporting(true);
    try {
      const params = new URLSearchParams({
        model_name: model_name || '',
        start_timestamp: String(localStartTimestamp),
        end_timestamp: String(localEndTimestamp),
        channel: channel ? String(channel) : '',
        request_id: request_id || '',
        token_id: token_id ? String(token_id) : '',
        client_user_id: client_user_id || '',
        mt_session_id: mt_session_id || '',
        trace_id: trace_id || '',
        traj_id: traj_id || '',
      });
      const res = await API.get(
        `/api/log/error-logs/export?${params.toString()}`,
        { responseType: 'blob' },
      );

      // 校验/权限失败时后端返回 JSON（status 200, success=false）
      const contentType =
        (res.headers &&
          (res.headers['content-type'] || res.headers['Content-Type'])) ||
        '';
      if (contentType.includes('application/json')) {
        const text = await res.data.text();
        try {
          const errBody = JSON.parse(text);
          showError(errBody.message || t('导出失败'));
        } catch (e) {
          showError(t('导出失败'));
        }
        return;
      }

      const pad = (n) => String(n).padStart(2, '0');
      const fmtTs = (ts) => {
        const d = new Date(ts * 1000);
        return `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(
          d.getDate(),
        )}-${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`;
      };
      const filename = `error-log-${fmtTs(localStartTimestamp)}-${fmtTs(
        localEndTimestamp,
      )}.csv`;

      const url = window.URL.createObjectURL(new Blob([res.data]));
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
      showSuccess(t('导出成功'));
    } catch (e) {
      showError(t('导出失败'));
    } finally {
      setExporting(false);
    }
  };

  const copyText = async (e, text) => {
    e.stopPropagation();
    if (await copy(text)) {
      showSuccess('已复制');
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  // 格式化JSON内容
  const formatJsonContent = (content) => {
    try {
      const parsed = JSON.parse(content);
      return JSON.stringify(parsed, null, 2);
    } catch (e) {
      return content;
    }
  };

  // 显示详情弹框
  const showDetailDialog = (content, isJson = true) => {
    setDetailContent(isJson ? formatJsonContent(content) : content);
    setShowDetailModal(true);
  };

  // 获取并显示body内容
  const fetchAndShowBody = async (id) => {
    setLoadingBodyId(id);
    try {
      const res = await API.get(`/api/log/error-logs/${id}/body`);
      const { success, message, data } = res.data;
      if (success) {
        showDetailDialog(data, true);
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message || t('获取Body失败'));
    } finally {
      setLoadingBodyId(null);
    }
  };

  // 复制body内容
  const copyBodyContent = async (e, content) => {
    await copyText(e, content);
  };

  // 获取并显示 header 内容（仅 root）。后端 RootAuth 会兜底，前端入口同样按 root 隐藏。
  const fetchAndShowHeader = async (id) => {
    if (!isRoot()) return;
    setLoadingHeaderId(id);
    try {
      const res = await API.get(`/api/log/error-logs/${id}/header`);
      const { success, message, data } = res.data;
      if (success) {
        showDetailDialog(data?.content || '', true);
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message || t('获取Header失败'));
    } finally {
      setLoadingHeaderId(null);
    }
  };

  useEffect(() => {
    const localPageSize =
      parseInt(localStorage.getItem('page-size')) || ITEMS_PER_PAGE;
    setPageSize(localPageSize);
    loadLogs(activePage, localPageSize)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  }, []);

  // 当 formApi 可用时，初始化统计
  useEffect(() => {
    if (formApi) {
      //handleEyeClick();
    }
  }, [formApi]);

  const expandRowRender = (record, index) => {
    return <Descriptions data={expandData[record.key]} />;
  };

  // 检查是否有任何记录有展开内容
  const hasExpandableRows = () => {
    return logs.some(
      (log) => expandData[log.key] && expandData[log.key].length > 0,
    );
  };

  // 渲染详情弹框
  const renderDetailModal = () => {
    return (
      <Modal
        title={t('详情')}
        visible={showDetailModal}
        onCancel={() => setShowDetailModal(false)}
        width={800}
        footer={
          <div className='flex justify-end gap-2'>
            <Button
              theme='light'
              onClick={async (e) => {
                await copyText(e, detailContent);
              }}
              className='!rounded-full'
            >
              {t('复制')}
            </Button>
            <Button
              type='primary'
              onClick={() => setShowDetailModal(false)}
              className='!rounded-full'
            >
              {t('关闭')}
            </Button>
          </div>
        }
      >
        <div className='bg-gray-50 p-4 rounded-lg max-h-96 overflow-auto'>
          <pre className='whitespace-pre-wrap text-sm font-mono'>
            {detailContent}
          </pre>
        </div>
      </Modal>
    );
  };

  return (
    <div className='mt-[64px]'>
      {renderColumnSelector()}
      {renderDetailModal()}
      <Card
        className='!rounded-2xl mb-4'
        title={
          <div className='flex flex-col w-full'>
            {/* <Divider margin='12px' /> */}

            {/* 搜索表单区域 */}
            <Form
              initValues={formInitValues}
              getFormApi={(api) => setFormApi(api)}
              onSubmit={refresh}
              allowEmpty={true}
              autoComplete='off'
              layout='vertical'
              trigger='change'
              stopValidateWithError={false}
            >
              <div className='flex flex-col gap-4'>
                <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4'>
                  {/* 时间选择器 */}
                  <div className='col-span-1 lg:col-span-2'>
                    <Form.DatePicker
                      field='dateRange'
                      className='w-full'
                      type='dateTimeRange'
                      placeholder={[t('开始时间'), t('结束时间')]}
                      showClear
                      pure
                    />
                  </div>

                  {/* 其他搜索字段 */}
                  <Form.Input
                    field='request_id'
                    prefix={<IconSearch />}
                    placeholder={t('requestID')}
                    className='!rounded-full'
                    showClear
                    pure
                  />

                  <Form.Input
                    field='model_name'
                    prefix={<IconSearch />}
                    placeholder={t('modelName')}
                    className='!rounded-full'
                    showClear
                    pure
                  />

                  {/* 渠道 / 令牌:root 用户下拉+模糊搜索(精确匹配),其他用户维持输入框 */}
                  {isRootUser ? (
                    <Form.Select
                      field='channel'
                      prefix={<IconSearch />}
                      placeholder={t('channelID')}
                      optionList={channelOptions}
                      filter
                      className='w-full !rounded-full'
                      showClear
                      pure
                    />
                  ) : (
                    <Form.Input
                      field='channel'
                      prefix={<IconSearch />}
                      placeholder={t('channelID')}
                      className='!rounded-full'
                      showClear
                      pure
                    />
                  )}
                  {isRootUser ? (
                    <Form.Select
                      field='token_id'
                      prefix={<IconSearch />}
                      placeholder={t('tokenID')}
                      optionList={tokenIdOptions}
                      filter
                      className='w-full !rounded-full'
                      showClear
                      pure
                    />
                  ) : (
                    <Form.Input
                      field='token_id'
                      prefix={<IconSearch />}
                      placeholder={t('tokenID')}
                      className='!rounded-full'
                      showClear
                      pure
                    />
                  )}
                   <Form.Input
                    field='client_user_id'
                    prefix={<IconSearch />}
                    placeholder={t('clientUserID')}
                    className='!rounded-full'
                    showClear
                    pure
                  />
                   <Form.Input
                    field='mt_session_id'
                    prefix={<IconSearch />}
                    placeholder={t('MT Session ID')}
                    className='!rounded-full'
                    showClear
                    pure
                  />
                   <Form.Input
                    field='trace_id'
                    prefix={<IconSearch />}
                    placeholder={t('Trace ID')}
                    className='!rounded-full'
                    showClear
                    pure
                  />
                   <Form.Input
                    field='traj_id'
                    prefix={<IconSearch />}
                    placeholder={t('Traj ID')}
                    className='!rounded-full'
                    showClear
                    pure
                  />
                   <Form.Input
                    field='session_id'
                    prefix={<IconSearch />}
                    placeholder={t('Session ID')}
                    className='!rounded-full'
                    showClear
                    pure
                  />
                </div>

                {/* 操作按钮区域 */}
                <div className='flex flex-col sm:flex-row justify-between items-start sm:items-center gap-3'>
                  {/* 日志类型选择器 */}

                  <div className='flex gap-2 w-full sm:w-auto justify-end'>
                    <Button
                      type='primary'
                      htmlType='submit'
                      loading={loading}
                      className='!rounded-full'
                    >
                      {t('查询')}
                    </Button>
                    <Button
                      theme='light'
                      onClick={() => {
                        if (formApi) {
                          formApi.reset();
                          //setLogType(0);
                          // 重置后立即查询，使用setTimeout确保表单重置完成
                          setTimeout(() => {
                            refresh();
                          }, 100);
                        }
                      }}
                      className='!rounded-full'
                    >
                      {t('重置')}
                    </Button>
                    <Button
                      theme='light'
                      type='tertiary'
                      icon={<IconSetting />}
                      onClick={() => setShowColumnSelector(true)}
                      className='!rounded-full'
                    >
                      {t('列设置')}
                    </Button>
                    {isRoot() && (
                      <Button
                        theme='light'
                        type='secondary'
                        icon={<IconDownload />}
                        loading={exporting}
                        onClick={handleExportErrorLogs}
                        className='!rounded-full'
                      >
                        {t('导出CSV')}
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            </Form>
          </div>
        }
        shadows='always'
        bordered={false}
      >
        <Table
          columns={getVisibleColumns()}
          {...(hasExpandableRows() && {
            expandedRowRender: expandRowRender,
            expandRowByClick: true,
            rowExpandable: (record) =>
              expandData[record.key] && expandData[record.key].length > 0,
          })}
          dataSource={logs}
          rowKey='key'
          loading={loading}
          scroll={{ x: 'max-content' }}
          className='rounded-xl overflow-hidden'
          size='middle'
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('搜索无结果')}
              style={{ padding: 30 }}
            />
          }
          pagination={{
            formatPageText: (page) =>
              t('第 {{start}} - {{end}} 条，共 {{total}} 条', {
                start: page.currentStart,
                end: page.currentEnd,
                total: logCount,
              }),
            currentPage: activePage,
            pageSize: pageSize,
            total: logCount,
            pageSizeOptions: [10, 20, 50, 100],
            showSizeChanger: true,
            onPageSizeChange: (size) => {
              handlePageSizeChange(size);
            },
            onPageChange: handlePageChange,
          }}
        />
      </Card>
    </div>
  );
};

export default ErrorLogsTable;
