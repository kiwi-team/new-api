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

import React, { useEffect, useState } from 'react';
import {
  Button,
  TextArea,
  Space,
  Spin,
  RadioGroup,
  Radio,
  Typography,
} from '@douyinfe/semi-ui';
import { IconSave } from '@douyinfe/semi-icons';
import { API, showError, showSuccess, verifyJSON } from '../../../helpers';
import { useTranslation } from 'react-i18next';
import TieredPriceVisualEditor from './TieredPriceVisualEditor';

export default function TieredPriceSettings({ options, refresh }) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState('visual');
  const [tieredPrice, setTieredPrice] = useState('');

  useEffect(() => {
    if (options?.TieredPrice !== undefined) {
      setTieredPrice(options.TieredPrice);
    }
  }, [options]);

  const handleSave = async () => {
    if (mode === 'json' && tieredPrice && !verifyJSON(tieredPrice)) {
      return showError(t('不是合法的 JSON 字符串'));
    }
    setLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'TieredPrice',
        value: tieredPrice || '{}',
      });
      if (res.data.success) {
        showSuccess(t('保存成功'));
        refresh();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Spin spinning={loading}>
      <Space vertical align='start' style={{ width: '100%' }}>
        <Space style={{ marginBottom: 12 }}>
          <RadioGroup
            type='button'
            value={mode}
            onChange={(e) => setMode(e.target.value)}
          >
            <Radio value='visual'>{t('可视化')}</Radio>
            <Radio value='json'>JSON</Radio>
          </RadioGroup>
        </Space>

        {mode === 'visual' ? (
          <TieredPriceVisualEditor
            value={tieredPrice}
            onChange={setTieredPrice}
            onSave={handleSave}
            loading={loading}
          />
        ) : (
          <>
            <Typography.Text type='secondary' size='small'>
              {t('按输入 Token 数量分档计费，优先级高于固定价格和倍率')}
            </Typography.Text>
            <TextArea
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为价格档位数组，例如 {"qwen-long": [{"max_tokens": 500000, "input_price": 0.5, "output_price": 2.0}, {"max_tokens": 1000000, "input_price": 1.0, "output_price": 4.0}]}',
              )}
              value={tieredPrice}
              autosize={{ minRows: 6, maxRows: 20 }}
              onChange={setTieredPrice}
              style={{ width: '100%' }}
            />
            <Button
              type='primary'
              icon={<IconSave />}
              onClick={handleSave}
              loading={loading}
            >
              {t('保存')}
            </Button>
          </>
        )}
      </Space>
    </Spin>
  );
}
