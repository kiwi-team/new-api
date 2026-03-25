import React, { useEffect, useState } from 'react';
import { Modal, Select } from '@douyinfe/semi-ui';
import { API, getModelCategories, selectFilter } from '../../../../helpers';
import { useTranslation } from 'react-i18next';

const BatchAppendModelsModal = ({ visible, onCancel, onConfirm }) => {
  const { t } = useTranslation();
  const [groupOptions, setGroupOptions] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState('');
  const [modelOptions, setModelOptions] = useState([]);
  const [selectedModels, setSelectedModels] = useState([]);

  useEffect(() => {
    if (visible) {
      setSelectedGroup('');
      setSelectedModels([]);
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
      API.get('/api/user/models').then((res) => {
        const { success, data } = res.data;
        if (success) {
          const categories = getModelCategories(t);
          setModelOptions(
            (data || []).map((model) => {
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
            }),
          );
        }
      });
    }
  }, [visible, t]);

  return (
    <Modal
      title={t('批量添加模型')}
      visible={visible}
      onCancel={onCancel}
      onOk={() => onConfirm(selectedGroup, selectedModels)}
      okButtonProps={{
        disabled: !selectedGroup || selectedModels.length === 0,
      }}
    >
      <p style={{ marginBottom: 12 }}>
        {t('为指定分组下的所有令牌追加可用模型（不会覆盖已有模型）')}
      </p>
      <div style={{ marginBottom: 12 }}>
        <label style={{ display: 'block', marginBottom: 4 }}>
          {t('选择分组')}
        </label>
        <Select
          style={{ width: '100%' }}
          placeholder={t('请选择分组')}
          value={selectedGroup}
          onChange={(value) => setSelectedGroup(value)}
          optionList={groupOptions}
          filter
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 4 }}>
          {t('选择模型')}
        </label>
        <Select
          style={{ width: '100%' }}
          placeholder={t('请选择要添加的模型')}
          value={selectedModels}
          onChange={(value) => setSelectedModels(value)}
          optionList={modelOptions}
          multiple
          filter={selectFilter}
          maxTagCount={3}
        />
      </div>
    </Modal>
  );
};

export default BatchAppendModelsModal;
