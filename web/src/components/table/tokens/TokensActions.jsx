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

import React, { useRef, useState } from 'react';
import { Button, Space } from '@douyinfe/semi-ui';
import { showError } from '../../../helpers';
import { API, showSuccess, isRoot } from '../../../helpers';
import CopyTokensModal from './modals/CopyTokensModal';
import DeleteTokensModal from './modals/DeleteTokensModal';
import BatchSetGroupModal from './modals/BatchSetGroupModal';
import BatchAppendModelsModal from './modals/BatchAppendModelsModal';

const TokensActions = ({
  selectedKeys,
  setEditingToken,
  setShowEdit,
  batchCopyTokens,
  batchDeleteTokens,
  batchSetGroup,
  batchAppendModels,
  copyText,
  t,
}) => {
  // Modal states
  const [showCopyModal, setShowCopyModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [showGroupModal, setShowGroupModal] = useState(false);
  const [showAppendModelsModal, setShowAppendModelsModal] = useState(false);
  const fileInputRef = useRef(null);

  // Handle copy selected tokens with options
  const handleCopySelectedTokens = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }
    setShowCopyModal(true);
  };

  // Handle delete selected tokens with confirmation
  const handleDeleteSelectedTokens = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }
    setShowDeleteModal(true);
  };

  // Handle delete confirmation
  const handleConfirmDelete = () => {
    batchDeleteTokens();
    setShowDeleteModal(false);
  };

  // Handle batch set group
  const handleSetGroup = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }
    setShowGroupModal(true);
  };

  const handleConfirmSetGroup = (group) => {
    batchSetGroup(group);
    setShowGroupModal(false);
  };

  // Handle batch append models
  const handleConfirmAppendModels = (group, models) => {
    batchAppendModels(group, models);
    setShowAppendModelsModal(false);
  };

  return (
    <>
      <div className='flex flex-wrap gap-2 w-full md:w-auto order-2 md:order-1'>
        <Button
          type='primary'
          className='flex-1 md:flex-initial'
          onClick={() => {
            setEditingToken({
              id: undefined,
            });
            setShowEdit(true);
          }}
          size='small'
        >
          {t('添加令牌')}
        </Button>

        <Button
          type='tertiary'
          className='flex-1 md:flex-initial'
          onClick={handleCopySelectedTokens}
          size='small'
        >
          {t('复制所选令牌')}
        </Button>

        {isRoot() && (
          <Button
            type='tertiary'
            className='flex-1 md:flex-initial'
            onClick={async () => {
              try {
                const res = await API.get('/api/admin/export/tokens', { responseType: 'blob' });
                const url = window.URL.createObjectURL(new Blob([res.data]));
                const a = document.createElement('a');
                a.href = url;
                a.download = 'tokens.csv';
                a.click();
                window.URL.revokeObjectURL(url);
              } catch (e) {
                showError(t('导出失败'));
              }
            }}
            size='small'
          >
            {t('导出CSV')}
          </Button>
        )}

        <input
          type='file'
          accept='.csv'
          ref={fileInputRef}
          style={{ display: 'none' }}
          onChange={async (e) => {
            const f = e.target.files?.[0];
            if (!f) return;
            try {
              const formData = new FormData();
              formData.append('file', f);
              const res = await API.post('/api/admin/import/tokens', formData);
              const { success, message } = res.data;
              if (success) showSuccess(message || t('导入成功'));
              else showError(message);
            } catch (err) {
              showError(t('导入失败'));
            }
            if (fileInputRef.current) fileInputRef.current.value = '';
          }}
        />
        {isRoot() && (
          <Button
            type='tertiary'
            className='flex-1 md:flex-initial'
            onClick={() => {
              if (fileInputRef.current) fileInputRef.current.value = '';
              fileInputRef.current?.click();
            }}
            size='small'
          >
            {t('导入CSV')}
          </Button>
        )}

        <Button
          type='danger'
          className='w-full md:w-auto'
          onClick={handleDeleteSelectedTokens}
          size='small'
        >
          {t('删除所选令牌')}
        </Button>

        <Button
          type='tertiary'
          className='flex-1 md:flex-initial'
          onClick={handleSetGroup}
          size='small'
        >
          {t('设置分组')}
        </Button>

        <Button
          type='tertiary'
          className='flex-1 md:flex-initial'
          onClick={() => setShowAppendModelsModal(true)}
          size='small'
        >
          {t('批量添加模型')}
        </Button>
      </div>

      <CopyTokensModal
        visible={showCopyModal}
        onCancel={() => setShowCopyModal(false)}
        selectedKeys={selectedKeys}
        copyText={copyText}
        t={t}
      />

      <DeleteTokensModal
        visible={showDeleteModal}
        onCancel={() => setShowDeleteModal(false)}
        onConfirm={handleConfirmDelete}
        selectedKeys={selectedKeys}
        t={t}
      />

      <BatchSetGroupModal
        visible={showGroupModal}
        onCancel={() => setShowGroupModal(false)}
        onConfirm={handleConfirmSetGroup}
        selectedKeys={selectedKeys}
        t={t}
      />

      <BatchAppendModelsModal
        visible={showAppendModelsModal}
        onCancel={() => setShowAppendModelsModal(false)}
        onConfirm={handleConfirmAppendModels}
      />
    </>
  );
};

export default TokensActions;
