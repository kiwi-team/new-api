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

import React, { useRef } from 'react';
import {
  Button,
  Dropdown,
  Modal,
  Switch,
  Typography,
  Select,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, isRoot } from '../../../helpers';
import CompactModeToggle from '../../common/ui/CompactModeToggle';

const ChannelsActions = ({
  enableBatchDelete,
  batchDeleteChannels,
  setShowBatchSetTag,
  testAllChannels,
  fixChannelsAbilities,
  updateAllChannelsBalance,
  deleteAllDisabledChannels,
  compactMode,
  setCompactMode,
  idSort,
  setIdSort,
  setEnableBatchDelete,
  enableTagMode,
  setEnableTagMode,
  statusFilter,
  setStatusFilter,
  getFormValues,
  loadChannels,
  searchChannels,
  activeTypeKey,
  activePage,
  pageSize,
  setActivePage,
  showSyncModal,
  selectedChannels,
  t,
}) => {
  const fileInputRef = useRef(null);
  const handleExport = async () => {
    try {
      const res = await API.get('/api/admin/export/channels', { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const a = document.createElement('a');
      a.href = url;
      a.download = 'channels.csv';
      a.click();
      window.URL.revokeObjectURL(url);
    } catch (e) {
      console.log(e);
      showError(t('导出失败'));
    }
  };
  const handleImport = async (file) => {
    try {
      const formData = new FormData();
      formData.append('file', file);
      const res = await API.post('/api/admin/import/channels', formData);
      const { success, message } = res.data;
      if (success) {
        showSuccess(message || t('导入成功'));
        const { searchKeyword, searchGroup, searchModel } = getFormValues();
        if (searchKeyword === '' && searchGroup === '' && searchModel === '') {
          await loadChannels(activePage, pageSize, idSort, enableTagMode);
        } else {
          await searchChannels(
            enableTagMode,
            activeTypeKey,
            statusFilter,
            activePage,
            pageSize,
            idSort,
          );
        }
      } else showError(message);
    } catch (e) {
      console.log(e);
      showError(t('导入失败'));
    }
  };
  return (
    <div className='flex flex-col gap-2'>
      {/* 第一行：批量操作按钮 + 设置开关 */}
      <div className='flex flex-col md:flex-row justify-between gap-2'>
        {/* 左侧：批量操作按钮 */}
        <div className='flex flex-wrap md:flex-nowrap items-center gap-2 w-full md:w-auto order-2 md:order-1'>
          <Button
            size='small'
            disabled={!enableBatchDelete}
            type='danger'
            className='w-full md:w-auto'
            onClick={() => {
              Modal.confirm({
                title: t('确定是否要删除所选通道？'),
                content: t('此修改将不可逆'),
                onOk: () => batchDeleteChannels(),
              });
            }}
          >
            {t('删除所选通道')}
          </Button>

          <Button
            size='small'
            disabled={!enableBatchDelete}
            type='tertiary'
            onClick={() => setShowBatchSetTag(true)}
            className='w-full md:w-auto'
          >
            {t('批量设置标签')}
          </Button>

          {isRoot() && (
            <Button
              size='small'
              disabled={!enableBatchDelete || !selectedChannels || selectedChannels.length === 0}
              type='tertiary'
              onClick={showSyncModal}
              className='w-full md:w-auto'
            >
              {t('同步到其他环境')}
            </Button>
          )}

          <Dropdown
            size='small'
            trigger='click'
            render={
              <Dropdown.Menu>
                <Dropdown.Item>
                  <Button
                    size='small'
                    type='tertiary'
                    className='w-full'
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定？'),
                        content: t('确定要测试所有通道吗？'),
                        onOk: () => testAllChannels(),
                        size: 'small',
                        centered: true,
                      });
                    }}
                  >
                    {t('测试所有通道')}
                  </Button>
                </Dropdown.Item>
                <Dropdown.Item>
                  <Button
                    size='small'
                    className='w-full'
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定是否要修复数据库一致性？'),
                        content: t(
                          '进行该操作时，可能导致渠道访问错误，请仅在数据库出现问题时使用',
                        ),
                        onOk: () => fixChannelsAbilities(),
                        size: 'sm',
                        centered: true,
                      });
                    }}
                  >
                    {t('修复数据库一致性')}
                  </Button>
                </Dropdown.Item>
                <Dropdown.Item>
                  <Button
                    size='small'
                    type='secondary'
                    className='w-full'
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定？'),
                        content: t('确定要更新所有已启用通道余额吗？'),
                        onOk: () => updateAllChannelsBalance(),
                        size: 'sm',
                        centered: true,
                      });
                    }}
                  >
                    {t('更新所有已启用通道余额')}
                  </Button>
                </Dropdown.Item>
                <Dropdown.Item>
                  <Button
                    size='small'
                    type='danger'
                    className='w-full'
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定是否要删除禁用通道？'),
                        content: t('此修改将不可逆'),
                        onOk: () => deleteAllDisabledChannels(),
                        size: 'sm',
                        centered: true,
                      });
                    }}
                  >
                    {t('删除禁用通道')}
                  </Button>
                </Dropdown.Item>
              </Dropdown.Menu>
            }
          >
            <Button
              size='small'
              theme='light'
              type='tertiary'
              className='w-full md:w-auto'
            >
              {t('批量操作')}
            </Button>
          </Dropdown>

          {isRoot() && (
            <Button
              size='small'
              type='tertiary'
              className='w-full md:w-auto'
              onClick={handleExport}
            >
              {t('导出CSV')}
            </Button>
          )}
          <input
            type='file'
            accept='.csv'
            ref={fileInputRef}
            style={{ display: 'none' }}
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) handleImport(f);
              if (fileInputRef.current) fileInputRef.current.value = '';
            }}
          />
          {isRoot() && (
            <Button
              size='small'
              className='w-full md:w-auto'
              onClick={() => {
                if (fileInputRef.current) fileInputRef.current.value = '';
                fileInputRef.current?.click();
              }}
            >
              {t('导入CSV')}
            </Button>
          )}

          <CompactModeToggle
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        </div>

        {/* 右侧：设置开关区域 */}
        <div className='flex flex-col md:flex-row items-start md:items-center gap-2 w-full md:w-auto order-1 md:order-2'>
          <div className='flex items-center justify-between w-full md:w-auto'>
            <Typography.Text strong className='mr-2'>
              {t('使用ID排序')}
            </Typography.Text>
            <Switch
              size='small'
              checked={idSort}
              onChange={(v) => {
                localStorage.setItem('id-sort', v + '');
                setIdSort(v);
                const { searchKeyword, searchGroup, searchModel } =
                  getFormValues();
                if (
                  searchKeyword === '' &&
                  searchGroup === '' &&
                  searchModel === ''
                ) {
                  loadChannels(activePage, pageSize, v, enableTagMode);
                } else {
                  searchChannels(
                    enableTagMode,
                    activeTypeKey,
                    statusFilter,
                    activePage,
                    pageSize,
                    v,
                  );
                }
              }}
            />
          </div>

          <div className='flex items-center justify-between w-full md:w-auto'>
            <Typography.Text strong className='mr-2'>
              {t('开启批量操作')}
            </Typography.Text>
            <Switch
              size='small'
              checked={enableBatchDelete}
              onChange={(v) => {
                localStorage.setItem('enable-batch-delete', v + '');
                setEnableBatchDelete(v);
              }}
            />
          </div>

          <div className='flex items-center justify-between w-full md:w-auto'>
            <Typography.Text strong className='mr-2'>
              {t('标签聚合模式')}
            </Typography.Text>
            <Switch
              size='small'
              checked={enableTagMode}
              onChange={(v) => {
                localStorage.setItem('enable-tag-mode', v + '');
                setEnableTagMode(v);
                setActivePage(1);
                loadChannels(1, pageSize, idSort, v);
              }}
            />
          </div>

          <div className='flex items-center justify-between w-full md:w-auto'>
            <Typography.Text strong className='mr-2'>
              {t('状态筛选')}
            </Typography.Text>
            <Select
              size='small'
              value={statusFilter}
              onChange={(v) => {
                localStorage.setItem('channel-status-filter', v);
                setStatusFilter(v);
                setActivePage(1);
                loadChannels(
                  1,
                  pageSize,
                  idSort,
                  enableTagMode,
                  activeTypeKey,
                  v,
                );
              }}
            >
              <Select.Option value='all'>{t('全部')}</Select.Option>
              <Select.Option value='enabled'>{t('已启用')}</Select.Option>
              <Select.Option value='disabled'>{t('已禁用')}</Select.Option>
              <Select.Option value='auto_disabled'>
                {t('自动禁用')}
              </Select.Option>
            </Select>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ChannelsActions;
