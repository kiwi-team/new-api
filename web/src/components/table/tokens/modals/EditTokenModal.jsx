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

import React, { useEffect, useState, useContext, useRef } from 'react';
import {
  API,
  showError,
  showSuccess,
  timestamp2string,
  renderGroupOption,
  renderQuotaWithPrompt,
  getModelCategories,
  selectFilter,
  isAdmin,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Avatar,
  Form,
  Col,
  Row,
  TagInput,
  InputNumber,
  Select,
  Input,
  AutoComplete,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconLink,
  IconSave,
  IconClose,
  IconKey,
  IconMenu,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { StatusContext } from '../../../../context/Status';

const { Text, Title } = Typography;

const EditTokenModal = (props) => {
  const { t } = useTranslation();
  const [statusState, statusDispatch] = useContext(StatusContext);
  const [loading, setLoading] = useState(false);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [models, setModels] = useState([]);
  const [modelNameList, setModelNameList] = useState([]);
  const [groups, setGroups] = useState([]);
  const [channelOptions, setChannelOptions] = useState([]);
  const [channelOptionMap, setChannelOptionMap] = useState(new Map());
  const getChannelStatusColor = (status) => {
    if (status === 1) return 'green';
    if (status === 2) return 'red';
    if (status === 3) return 'yellow';
    return 'grey';
  };
  const renderChannelOption = (option) => {
    const color = getChannelStatusColor(option.__status);
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          width: '100%',
        }}
      >
        <span>{option.label}</span>
        <Tag color={color} size='small' />
      </div>
    );
  };

  const dedupeOptions = (list) => {
    const map = new Map();
    for (const o of list) {
      const key = Number(o.value);
      const prev = map.get(key);
      if (!prev) map.set(key, o);
      else {
        const pUnknown = String(prev.label || '').startsWith('未知(');
        const cUnknown = String(o.label || '').startsWith('未知(');
        if (pUnknown && !cUnknown) map.set(key, o);
      }
    }
    return Array.from(map.values());
  };

  const rebuildChannelOptionMap = (opts) => {
    const m = new Map();
    for (const o of opts) m.set(Number(o.value), o);
    setChannelOptionMap(m);
  };

  const ensureOptionsForIds = (ids) => {
    const base = [...channelOptions];
    let changed = false;
    for (const id of ids) {
      const nid = Number(id);
      if (!channelOptionMap.has(nid)) {
        base.push({ label: `(${nid})未知`, value: nid, __status: 0 });
        changed = true;
      }
    }
    if (changed) {
      const deduped = dedupeOptions(base);
      setChannelOptions(deduped);
      rebuildChannelOptionMap(deduped);
    }
  };
  const isEdit = props.editingToken.id !== undefined;

  const [channelRulesList, setChannelRulesList] = useState([]);
  const [channelRulesJson, setChannelRulesJson] = useState('');

  const parseChannelRulesToUI = (raw) => {
    try {
      let obj = {};
      if (!raw) obj = {};
      else if (typeof raw === 'string') obj = JSON.parse(raw || '{}');
      else if (typeof raw === 'object') obj = raw || {};
      const list = Object.entries(obj).map(([modelKey, rule]) => {
        const channels = Array.isArray(rule?.channels) ? rule.channels : [];
        const mapped = channels.map((ch) => {
          const ids = Array.isArray(ch?.ids)
            ? ch.ids.map((v) => Number(v)).filter((v) => !isNaN(v))
            : Number(ch?.id || 0) > 0
              ? [Number(ch.id)]
              : [];
          const group_ratio =
            typeof ch?.group_ratio === 'object' && ch.group_ratio !== null
              ? ch.group_ratio
              : undefined;
          return group_ratio ? { ids, group_ratio } : { ids };
        });
        return {
          modelKey,
          retry: Number(rule?.retry || 0),
          random_type: rule?.random_type || 'order',
          disable_channels: Array.isArray(rule?.disable_channels)
            ? rule.disable_channels
                .map((v) => Number(v))
                .filter((v) => !isNaN(v))
            : [],
          channels: mapped,
        };
      });
      setChannelRulesList(list);
      updateChannelRulesJsonFromList(list);
    } catch (e) {
      setChannelRulesList([]);
      setChannelRulesJson('');
      formApiRef.current?.setValue('channel_rules', '');
    }
  };

  const updateChannelRulesJsonFromList = (list) => {
    const obj = {};
    list.forEach((item) => {
      const key = String(item.modelKey || '').trim();
      if (!key) return;
      const channels = Array.isArray(item.channels) ? item.channels : [];
      const mapped = channels
        .map((ch) => {
          const ids = Array.isArray(ch.ids)
            ? ch.ids.map((v) => Number(v)).filter((v) => !isNaN(v))
            : [];
          if (ids.length === 0) return null;
          const hasGroupRatio =
            typeof ch.group_ratio === 'object' && ch.group_ratio !== null;
          if (hasGroupRatio && ids.length === 1) {
            const gr = {};
            for (const [k, v] of Object.entries(ch.group_ratio)) {
              const num = Number(v);
              if (!isNaN(num)) gr[k] = num;
            }
            return { id: ids[0], group_ratio: gr };
          }
          return { ids };
        })
        .filter(Boolean);
      const disableChannels = Array.isArray(item.disable_channels)
        ? item.disable_channels.map((v) => Number(v)).filter((v) => !isNaN(v))
        : [];
      obj[key] = {
        retry: Number(item.retry || 0),
        random_type: item.random_type || 'order',
        disable_channels: disableChannels,
        channels: mapped,
      };
    });
    const json = Object.keys(obj).length > 0 ? JSON.stringify(obj) : '';
    setChannelRulesJson(json);
    formApiRef.current?.setValue('channel_rules', json);
  };

  const addRule = () => {
    const list = [
      ...channelRulesList,
      {
        modelKey: '',
        retry: 0,
        random_type: 'order',
        disable_channels: [],
        channels: [],
      },
    ];
    setChannelRulesList(list);
    updateChannelRulesJsonFromList(list);
  };

  const removeRule = (idx) => {
    const list = channelRulesList.filter((_, i) => i !== idx);
    setChannelRulesList(list);
    updateChannelRulesJsonFromList(list);
  };

  const updateRuleField = (idx, key, value) => {
    const list = channelRulesList.map((r, i) =>
      i === idx ? { ...r, [key]: value } : r,
    );
    setChannelRulesList(list);
    updateChannelRulesJsonFromList(list);
  };

  const addChannelItem = (ruleIdx) => {
    const list = channelRulesList.map((r, i) =>
      i === ruleIdx
        ? {
            ...r,
            channels: [...r.channels, { ids: [] }],
          }
        : r,
    );
    setChannelRulesList(list);
    updateChannelRulesJsonFromList(list);
  };

  const removeChannelItem = (ruleIdx, chIdx) => {
    const list = channelRulesList.map((r, i) =>
      i === ruleIdx
        ? { ...r, channels: r.channels.filter((_, j) => j !== chIdx) }
        : r,
    );
    setChannelRulesList(list);
    updateChannelRulesJsonFromList(list);
  };

  const updateChannelItem = (ruleIdx, chIdx, key, value) => {
    const list = channelRulesList.map((r, i) =>
      i === ruleIdx
        ? {
            ...r,
            channels: r.channels.map((ch, j) =>
              j === chIdx ? { ...ch, [key]: value } : ch,
            ),
          }
        : r,
    );
    setChannelRulesList(list);
    updateChannelRulesJsonFromList(list);
  };

  const getInitValues = () => ({
    name: '',
    remain_quota: 0,
    expired_time: -1,
    unlimited_quota: true,
    model_limits_enabled: false,
    model_limits: [],
    allow_ips: '',
    group: '',
    cross_group_retry: false,
    tokenCount: 1,
    channel_rules: '',
    channel_ratios: '',
  });

  const handleCancel = () => {
    props.handleClose();
  };

  const setExpiredTime = (month, day, hour, minute) => {
    let now = new Date();
    let timestamp = now.getTime() / 1000;
    let seconds = month * 30 * 24 * 60 * 60;
    seconds += day * 24 * 60 * 60;
    seconds += hour * 60 * 60;
    seconds += minute * 60;
    if (!formApiRef.current) return;
    if (seconds !== 0) {
      timestamp += seconds;
      formApiRef.current.setValue('expired_time', timestamp2string(timestamp));
    } else {
      formApiRef.current.setValue('expired_time', -1);
    }
  };

  const loadModels = async () => {
    let res = await API.get(`/api/user/models`);
    const { success, message, data } = res.data;
    if (success) {
      const categories = getModelCategories(t);
      let localModelOptions = data.map((model) => {
        let icon = null;
        for (const [key, category] of Object.entries(categories)) {
          if (key !== 'all' && category.filter({ model_name: model })) {
            icon = category.icon;
            break;
          }
        }
        return {
          label: (
            <span className='flex items-center gap-1'>
              {icon}
              {model}
            </span>
          ),
          value: model,
        };
      });
      setModels(localModelOptions);
      try {
        const names = Array.isArray(data)
          ? data.filter((m) => typeof m === 'string')
          : [];
        setModelNameList(names);
      } catch {}
    } else {
      showError(t(message));
    }
  };

  const loadGroups = async () => {
    let res = await API.get(`/api/user/self/groups`);
    const { success, message, data } = res.data;
    if (success) {
      let localGroupOptions = Object.entries(data).map(([group, info]) => ({
        label: info.desc,
        value: group,
        ratio: info.ratio,
      }));
      if (statusState?.status?.default_use_auto_group) {
        if (localGroupOptions.some((group) => group.value === 'auto')) {
          localGroupOptions.sort((a, b) => (a.value === 'auto' ? -1 : 1));
        }
      }
      setGroups(localGroupOptions);
      // if (statusState?.status?.default_use_auto_group && formApiRef.current) {
      //   formApiRef.current.setValue('group', 'auto');
      // }
    } else {
      showError(t(message));
    }
  };

  const loadAllChannels = async () => {
    try {
      const res = await API.get(
        //`/api/channel/?p=1&page_size=1000&id_sort=true&tag_mode=false`,
       '/api/channel/channel-name-list',
      );
      const { success, message, data } = res.data;
      if (success) {
        const items = data?.items || data || [];
        const opts = (Array.isArray(items) ? items : []).map((ch) => ({
          label: `(${ch.id})${ch.name}`,
          value: ch.id,
          __status: ch.status,
        }));
        const merged = dedupeOptions([...channelOptions, ...opts]);
        setChannelOptions(merged);
        rebuildChannelOptionMap(merged);
      } else {
        showError(t(message));
      }
    } catch (e) {
      // ignore
    }
  };

  const loadToken = async () => {
    setLoading(true);
    let res = await API.get(`/api/token/${props.editingToken.id}`);
    const { success, message, data } = res.data;
    if (success) {
      if (data.expired_time !== -1) {
        data.expired_time = timestamp2string(data.expired_time);
      }
      if (data.model_limits !== '') {
        data.model_limits = data.model_limits.split(',');
      } else {
        data.model_limits = [];
      }
      if (formApiRef.current) {
        formApiRef.current.setValues({ ...getInitValues(), ...data });
        try {
          const raw = data.channel_rules || '';
          parseChannelRulesToUI(raw);
        } catch {}
        try {
          const ids = [];
          channelRulesList.forEach((r) =>
            (r.channels || []).forEach((ch) =>
              (ch.ids || []).forEach((id) => ids.push(Number(id))),
            ),
          );
          ensureOptionsForIds(ids);
        } catch {}
      }
    } else {
      showError(message);
    }
    setLoading(false);
  };

  useEffect(() => {
    if (formApiRef.current) {
      if (!isEdit) {
        formApiRef.current.setValues(getInitValues());
      }
    }
    loadModels();
    loadGroups();
    loadAllChannels();
  }, [props.editingToken.id]);

  useEffect(() => {
    try {
      const ids = [];
      channelRulesList.forEach((r) =>
        (r.channels || []).forEach((ch) =>
          (ch.ids || []).forEach((id) => ids.push(Number(id))),
        ),
      );
      ensureOptionsForIds(ids);
    } catch {}
  }, [channelOptions, channelRulesList]);

  useEffect(() => {
    if (props.visiable) {
      if (isEdit) {
        loadToken();
      } else {
        formApiRef.current?.setValues(getInitValues());
        parseChannelRulesToUI('');
      }
    } else {
      formApiRef.current?.reset();
    }
  }, [props.visiable, props.editingToken.id]);

  const dragItem = useRef(null);
  const dragOverItem = useRef(null);

  const handleDragStart = (e, ruleIdx, chIdx) => {
    dragItem.current = { ruleIdx, chIdx };
    e.dataTransfer.effectAllowed = 'move';
    e.target.style.opacity = '0.5';
  };

  const handleDragEnd = (e) => {
    e.target.style.opacity = '1';
    const source = dragItem.current;
    const destination = dragOverItem.current;

    dragItem.current = null;
    dragOverItem.current = null;

    if (!source || !destination) return;
    if (source.ruleIdx !== destination.ruleIdx) return;
    if (source.chIdx === destination.chIdx) return;

    const ruleIdx = source.ruleIdx;
    const list = [...channelRulesList];
    const rule = { ...list[ruleIdx] };
    const channels = [...rule.channels];

    const [movedItem] = channels.splice(source.chIdx, 1);
    channels.splice(destination.chIdx, 0, movedItem);

    rule.channels = channels;
    list[ruleIdx] = rule;

    setChannelRulesList(list);
    updateChannelRulesJsonFromList(list);
  };

  const handleDragEnter = (e, ruleIdx, chIdx) => {
    if (dragItem.current?.ruleIdx !== ruleIdx) return;
    dragOverItem.current = { ruleIdx, chIdx };
  };

  const generateRandomSuffix = () => {
    const characters =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < 6; i++) {
      result += characters.charAt(
        Math.floor(Math.random() * characters.length),
      );
    }
    return result;
  };

  const submit = async (values) => {
    setLoading(true);
    if (isEdit) {
      let { tokenCount: _tc, ...localInputs } = values;
      localInputs.remain_quota = parseInt(localInputs.remain_quota);
      if (localInputs.expired_time !== -1) {
        let time = Date.parse(localInputs.expired_time);
        if (isNaN(time)) {
          showError(t('过期时间格式错误！'));
          setLoading(false);
          return;
        }
        localInputs.expired_time = Math.ceil(time / 1000);
      }
      localInputs.model_limits = localInputs.model_limits.join(',');
      localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
      localInputs.channel_rules = channelRulesJson || '';
      let res = await API.put(`/api/token/`, {
        ...localInputs,
        id: parseInt(props.editingToken.id),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('令牌更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showError(t(message));
      }
    } else {
      const count = parseInt(values.tokenCount, 10) || 1;
      let successCount = 0;
      for (let i = 0; i < count; i++) {
        let { tokenCount: _tc, ...localInputs } = values;
        const baseName =
          values.name.trim() === '' ? 'default' : values.name.trim();
        if (i !== 0 || values.name.trim() === '') {
          localInputs.name = `${baseName}-${generateRandomSuffix()}`;
        } else {
          localInputs.name = baseName;
        }
        localInputs.remain_quota = parseInt(localInputs.remain_quota);

        if (localInputs.expired_time !== -1) {
          let time = Date.parse(localInputs.expired_time);
          if (isNaN(time)) {
            showError(t('过期时间格式错误！'));
            setLoading(false);
            break;
          }
          localInputs.expired_time = Math.ceil(time / 1000);
        }
        localInputs.model_limits = localInputs.model_limits.join(',');
        localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
        localInputs.channel_rules = channelRulesJson || '';
        let res = await API.post(`/api/token/`, localInputs);
        const { success, message } = res.data;
        if (success) {
          successCount++;
        } else {
          showError(t(message));
          break;
        }
      }
      if (successCount > 0) {
        showSuccess(t('令牌创建成功，请在列表页面点击复制获取令牌！'));
        props.refresh();
        props.handleClose();
      }
    }
    setLoading(false);
    formApiRef.current?.setValues(getInitValues());
  };

  return (
    <SideSheet
      placement={isEdit ? 'right' : 'left'}
      title={
        <Space>
          {isEdit ? (
            <Tag color='blue' shape='circle'>
              {t('更新')}
            </Tag>
          ) : (
            <Tag color='green' shape='circle'>
              {t('新建')}
            </Tag>
          )}
          <Title heading={4} className='m-0'>
            {isEdit ? t('更新令牌信息') : t('创建新的令牌')}
          </Title>
        </Space>
      }
      bodyStyle={{ padding: '0' }}
      visible={props.visiable}
      width={isMobile ? '100%' : 600}
      footer={
        <div className='flex justify-end bg-white'>
          <Space>
            <Button
              theme='solid'
              className='!rounded-lg'
              onClick={() => formApiRef.current?.submitForm()}
              icon={<IconSave />}
              loading={loading}
            >
              {t('提交')}
            </Button>
            <Button
              theme='light'
              className='!rounded-lg'
              type='primary'
              onClick={handleCancel}
              icon={<IconClose />}
            >
              {t('取消')}
            </Button>
          </Space>
        </div>
      }
      closeIcon={null}
      onCancel={() => handleCancel()}
    >
      <Spin spinning={loading}>
        <Form
          key={isEdit ? 'edit' : 'new'}
          initValues={getInitValues()}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={submit}
        >
          {({ values }) => (
            <div className='p-2'>
              {/* 基本信息 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='blue' className='mr-2 shadow-md'>
                    <IconKey size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('基本信息')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的基本信息')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Input
                      field='name'
                      label={t('名称')}
                      placeholder={t('请输入名称')}
                      rules={[{ required: true, message: t('请输入名称') }]}
                      showClear
                    />
                  </Col>
                  <Col span={24}>
                    {groups.length > 0 ? (
                      <Form.Select
                        field='group'
                        label={t('令牌分组')}
                        placeholder={t('令牌分组，默认为用户的分组')}
                        optionList={groups}
                        renderOptionItem={renderGroupOption}
                        showClear
                        style={{ width: '100%' }}
                      />
                    ) : (
                      <Form.Select
                        placeholder={t('管理员未设置用户可选分组')}
                        disabled
                        label={t('令牌分组')}
                        style={{ width: '100%' }}
                      />
                    )}
                  </Col>
                  <Col span={24} style={{ display: values.group === 'auto' ? 'block' : 'none' }}>
                    <Form.Switch
                      field='cross_group_retry'
                      label={t('跨分组重试')}
                      size='default'
                      extraText={t(
                        '开启后，当前分组渠道失败时会按顺序尝试下一个分组的渠道',
                      )}
                    />
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={10} xl={10}>
                    <Form.DatePicker
                      field='expired_time'
                      label={t('过期时间')}
                      type='dateTime'
                      placeholder={t('请选择过期时间')}
                      rules={[
                        { required: true, message: t('请选择过期时间') },
                        {
                          validator: (rule, value) => {
                            // 允许 -1 表示永不过期，也允许空值在必填校验时被拦截
                            if (value === -1 || !value)
                              return Promise.resolve();
                            const time = Date.parse(value);
                            if (isNaN(time)) {
                              return Promise.reject(t('过期时间格式错误！'));
                            }
                            if (time <= Date.now()) {
                              return Promise.reject(
                                t('过期时间不能早于当前时间！'),
                              );
                            }
                            return Promise.resolve();
                          },
                        },
                      ]}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={14} xl={14}>
                    <Form.Slot label={t('过期时间快捷设置')}>
                      <Space wrap>
                        <Button
                          theme='light'
                          type='primary'
                          onClick={() => setExpiredTime(0, 0, 0, 0)}
                        >
                          {t('永不过期')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(1, 0, 0, 0)}
                        >
                          {t('一个月')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(0, 1, 0, 0)}
                        >
                          {t('一天')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(0, 0, 1, 0)}
                        >
                          {t('一小时')}
                        </Button>
                      </Space>
                    </Form.Slot>
                  </Col>
                  {!isEdit && (
                    <Col span={24}>
                      <Form.InputNumber
                        field='tokenCount'
                        label={t('新建数量')}
                        min={1}
                        extraText={t('批量创建时会在名称后自动添加随机后缀')}
                        rules={[
                          { required: true, message: t('请输入新建数量') },
                        ]}
                        style={{ width: '100%' }}
                      />
                    </Col>
                  )}
                </Row>
              </Card>

              {/* 额度设置 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='green' className='mr-2 shadow-md'>
                    <IconCreditCard size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('额度设置')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌可用额度和数量')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.AutoComplete
                      field='remain_quota'
                      label={t('额度')}
                      placeholder={t('请输入额度')}
                      type='number'
                      disabled={values.unlimited_quota}
                      extraText={renderQuotaWithPrompt(values.remain_quota)}
                      rules={
                        values.unlimited_quota
                          ? []
                          : [{ required: true, message: t('请输入额度') }]
                      }
                      data={[
                        { value: 500000, label: '1$' },
                        { value: 5000000, label: '10$' },
                        { value: 25000000, label: '50$' },
                        { value: 50000000, label: '100$' },
                        { value: 250000000, label: '500$' },
                        { value: 500000000, label: '1000$' },
                      ]}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.Switch
                      field='unlimited_quota'
                      label={t('无限额度')}
                      size='default'
                      extraText={t(
                        '令牌的额度仅用于限制令牌本身的最大额度使用量，实际的使用受到账户的剩余额度限制',
                      )}
                    />
                  </Col>
                </Row>
              </Card>

              {/* 访问限制 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar
                    size='small'
                    color='purple'
                    className='mr-2 shadow-md'
                  >
                    <IconLink size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('访问限制')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的访问限制')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Select
                      field='model_limits'
                      label={t('模型限制列表')}
                      placeholder={t(
                        '请选择该令牌支持的模型，留空支持所有模型',
                      )}
                      multiple
                      optionList={models}
                      extraText={t('非必要，不建议启用模型限制')}
                      filter={selectFilter}
                      autoClearSearchValue={false}
                      searchPosition='dropdown'
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.TextArea
                      field='allow_ips'
                      label={t('IP白名单（支持CIDR表达式）')}
                      placeholder={t('允许的IP，一行一个，不填写则不限制')}
                      autosize
                      rows={1}
                      extraText={t('请勿过度信任此功能，IP可能被伪造，请配合nginx和cdn等网关使用')}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24} className={isAdmin() ? '' : 'tableHiddle'}>
                    <Form.Slot label={t('设置渠道规则')}>
                      <Card className='!rounded-2xl shadow-sm border-0'>
                        <Row gutter={12}>
                          <Col span={24}>
                            <Space>
                              <Button
                                type='tertiary'
                                onClick={addRule}
                                size='small'
                              >
                                {t('添加规则')}
                              </Button>
                            </Space>
                          </Col>
                          {channelRulesList.map((rule, idx) => (
                            <Col span={24} key={`rule_${idx}`}>
                              <Card className='!rounded-xl border-0'>
                                <Row gutter={8}>
                                  <Col span={12}>
                                    <Form.Slot label={t('模型关键字')}>
                                      <AutoComplete
                                        value={rule.modelKey}
                                        data={modelNameList.map((m) => ({
                                          value: m,
                                          label: m,
                                        }))}
                                        onChange={(v) =>
                                          updateRuleField(idx, 'modelKey', v)
                                        }
                                        onSelect={(item) =>
                                          updateRuleField(
                                            idx,
                                            'modelKey',
                                            item?.value || '',
                                          )
                                        }
                                        placeholder={t(
                                          '输入以搜索模型名称或前缀',
                                        )}
                                        style={{ width: '100%' }}
                                        showClear
                                      />
                                    </Form.Slot>
                                  </Col>
                                  <Col span={6}>
                                    <InputNumber
                                      value={rule.retry}
                                      onChange={(v) =>
                                        updateRuleField(
                                          idx,
                                          'retry',
                                          Number(v || 0),
                                        )
                                      }
                                      style={{ width: '100%' }}
                                    />
                                  </Col>
                                  <Col span={6}>
                                    <Select
                                      value={rule.random_type}
                                      optionList={[
                                        { label: 'order', value: 'order' },
                                        { label: 'random', value: 'random' },
                                      ]}
                                      onChange={(v) =>
                                        updateRuleField(idx, 'random_type', v)
                                      }
                                      style={{ width: '100%' }}
                                    />
                                  </Col>
                                  <Col span={24}>
                                    <Form.Slot label={t('禁用渠道')}>
                                      <TagInput
                                        value={(
                                          rule.disable_channels || []
                                        ).map((n) => String(n))}
                                        onChange={(arr) =>
                                          updateRuleField(
                                            idx,
                                            'disable_channels',
                                            (arr || [])
                                              .map((v) => Number(v))
                                              .filter((v) => !isNaN(v)),
                                          )
                                        }
                                        addOnBlur={true}
                                        separator={[',', '，']}
                                        hideCopy
                                        style={{ width: '100%' }}
                                      />
                                    </Form.Slot>
                                  </Col>
                                  <Col span={24}>
                                    <Space>
                                      <Button
                                        type='tertiary'
                                        size='small'
                                        onClick={() => addChannelItem(idx)}
                                      >
                                        {t('添加渠道组')}
                                      </Button>
                                      <Button
                                        type='danger'
                                        theme='borderless'
                                        size='small'
                                        onClick={() => removeRule(idx)}
                                      >
                                        {t('删除规则')}
                                      </Button>
                                    </Space>
                                  </Col>
                                  {rule.channels.map((ch, j) => (
                                    <Col
                                      span={24}
                                      key={`ch_${idx}_${j}`}
                                      draggable
                                      onDragStart={(e) =>
                                        handleDragStart(e, idx, j)
                                      }
                                      onDragEnter={(e) =>
                                        handleDragEnter(e, idx, j)
                                      }
                                      onDragEnd={handleDragEnd}
                                      onDragOver={(e) => e.preventDefault()}
                                    >
                                      <Card className='!rounded-lg border-0'>
                                        <Row
                                          gutter={8}
                                          type='flex'
                                          align='middle'
                                        >
                                          <Col
                                            span={2}
                                            style={{
                                              cursor: 'move',
                                              display: 'flex',
                                              justifyContent: 'center',
                                            }}
                                          >
                                            <IconMenu className='text-gray-400' />
                                          </Col>
                                          <Col span={18}>
                                            <Select
                                              multiple
                                              optionList={channelOptions}
                                              value={(ch.ids || []).map((n) =>
                                                Number(n),
                                              )}
                                              onChange={(vals) =>
                                                updateChannelItem(
                                                  idx,
                                                  j,
                                                  'ids',
                                                  (vals || [])
                                                    .map((v) => Number(v))
                                                    .filter((v) => !isNaN(v)),
                                                )
                                              }
                                              filter
                                              autoClearSearchValue={false}
                                              searchPosition='dropdown'
                                              showClear
                                              style={{ width: '100%' }}
                                            />
                                          </Col>
                                          <Col span={4}>
                                            <Button
                                              type='danger'
                                              theme='borderless'
                                              onClick={() =>
                                                removeChannelItem(idx, j)
                                              }
                                            >
                                              {t('删除')}
                                            </Button>
                                          </Col>
                                        </Row>
                                      </Card>
                                    </Col>
                                  ))}
                                </Row>
                              </Card>
                            </Col>
                          ))}
                          <Col span={24}>
                            <Form.Input
                              field='channel_rules'
                              value={channelRulesJson}
                              style={{ display: 'none' }}
                            />
                          </Col>
                        </Row>
                      </Card>
                    </Form.Slot>
                  </Col>
                  <Col span={24} className={isAdmin() ? '' : 'tableHiddle'}>
                    <Form.TextArea
                      field='channel_ratios'
                      label={t('设置渠道倍率')}
                      autosize
                      rows={3}
                      extraText={t('请核对配置信息')}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>
              </Card>
            </div>
          )}
        </Form>
      </Spin>
    </SideSheet>
  );
};

export default EditTokenModal;
