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
import { Button } from '@douyinfe/semi-ui';
import { API, showError, showSuccess, isRoot } from '../../../helpers';

const UsersActions = ({ setShowAddUser, t }) => {
  // Add new user
  const handleAddUser = () => {
    setShowAddUser(true);
  };
  const fileInputRef = useRef(null);
  const handleExport = async () => {
    try {
      const res = await API.get('/api/admin/export/users', { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const a = document.createElement('a');
      a.href = url;
      a.download = 'users.csv';
      a.click();
      window.URL.revokeObjectURL(url);
    } catch (e) {
      showError(t('导出失败'));
    }
  };
  const handleImport = async (file) => {
    try {
      const formData = new FormData();
      formData.append('file', file);
      const res = await API.post('/api/admin/import/users', formData);
      const { success, message } = res.data;
      if (success) showSuccess(message || t('导入成功'));
      else showError(message);
    } catch (e) {
      showError(t('导入失败'));
    }
  };

  return (
    <div className='flex gap-2 w-full md:w-auto order-2 md:order-1'>
      <Button className='w-full md:w-auto' onClick={handleAddUser} size='small'>
        {t('添加用户')}
      </Button>
      {isRoot() && (
        <Button className='w-full md:w-auto' onClick={handleExport} size='small' type='tertiary'>
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
          className='w-full md:w-auto'
          size='small'
          onClick={() => {
            if (fileInputRef.current) fileInputRef.current.value = '';
            fileInputRef.current?.click();
          }}
        >
          {t('导入CSV')}
        </Button>
      )}
    </div>
  );
};

export default UsersActions;
