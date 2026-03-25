import React, { useEffect, useState } from 'react';
import { Modal, Select } from '@douyinfe/semi-ui';
import { API } from '../../../../helpers';

const BatchSetGroupModal = ({ visible, onCancel, onConfirm, selectedKeys, t }) => {
  const [groupOptions, setGroupOptions] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState('');

  useEffect(() => {
    if (visible) {
      setSelectedGroup('');
      API.get('/api/user/self/groups').then((res) => {
        const { success, data } = res.data;
        if (success) {
          setGroupOptions(
            Object.entries(data).map(([group, info]) => ({
              label: info.desc || group,
              value: group,
            })),
          );
        }
      });
    }
  }, [visible]);

  return (
    <Modal
      title={t('批量设置分组')}
      visible={visible}
      onCancel={onCancel}
      onOk={() => onConfirm(selectedGroup)}
      okButtonProps={{ disabled: selectedGroup === '' }}
    >
      <p style={{ marginBottom: 12 }}>
        {t('为所选的 {{count}} 个令牌设置分组', {
          count: selectedKeys.length,
        })}
      </p>
      <Select
        style={{ width: '100%' }}
        placeholder={t('请选择分组')}
        value={selectedGroup}
        onChange={(value) => setSelectedGroup(value)}
        optionList={groupOptions}
        filter
      />
    </Modal>
  );
};

export default BatchSetGroupModal;
