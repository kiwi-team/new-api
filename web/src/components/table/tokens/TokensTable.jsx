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

import React, { useEffect, useMemo, useState } from 'react';
import { Empty } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { getTokensColumns } from './TokensColumnDefs';
import { API, isRoot } from '../../../helpers';

const TokensTable = (tokensData) => {
  const {
    tokens,
    loading,
    activePage,
    pageSize,
    tokenCount,
    compactMode,
    handlePageChange,
    handlePageSizeChange,
    rowSelection,
    handleRow,
    showKeys,
    setShowKeys,
    copyText,
    manageToken,
    onOpenLink,
    setEditingToken,
    setShowEdit,
    refresh,
    t,
  } = tokensData;

  const [channelNameMap, setChannelNameMap] = useState(new Map());

  useEffect(() => {
    // 渠道名映射只对 root 用户拉(后端 /api/channel/* 已经升级为 RootAuth)。
    // 非 root 用户既看不到渠道列也用不到这个映射,跳过请求避免 403/无谓网络。
    // 详见 org.md 全系统级约束。
    if (!isRoot()) return;
    let mounted = true;
    (async () => {
      try {
        const res = await API.get(
          //'/api/channel/?p=1&page_size=1000&id_sort=true&tag_mode=false',
          '/api/channel/channel-name-list',
          { disableDuplicate: true },
        );
        const items = res?.data || [];
        const map = new Map();
        items.forEach((ch) => {
          const id = Number(ch.id);
          if (!isNaN(id) && id > 0) map.set(id, String(ch.name || ''));
        });
        if (mounted) setChannelNameMap(map);
      } catch {}
    })();
    return () => {
      mounted = false;
    };
  }, []);

  // Get all columns
  const columns = useMemo(() => {
    return getTokensColumns({
      t,
      showKeys,
      setShowKeys,
      copyText,
      manageToken,
      onOpenLink,
      setEditingToken,
      setShowEdit,
      refresh,
      channelNameMap,
    });
  }, [
    t,
    showKeys,
    setShowKeys,
    copyText,
    manageToken,
    onOpenLink,
    setEditingToken,
    setShowEdit,
    refresh,
    channelNameMap,
  ]);

  // Handle compact mode by removing fixed positioning
  const tableColumns = useMemo(() => {
    return compactMode
      ? columns.map((col) => {
          if (col.dataIndex === 'operate') {
            const { fixed, ...rest } = col;
            return rest;
          }
          return col;
        })
      : columns;
  }, [compactMode, columns]);

  return (
    <CardTable
      columns={tableColumns}
      dataSource={tokens}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      pagination={{
        currentPage: activePage,
        pageSize: pageSize,
        total: tokenCount,
        showSizeChanger: true,
        pageSizeOptions: [10, 20, 50, 100],
        onPageSizeChange: handlePageSizeChange,
        onPageChange: handlePageChange,
      }}
      hidePagination={true}
      loading={loading}
      rowSelection={rowSelection}
      onRow={handleRow}
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('搜索无结果')}
          style={{ padding: 30 }}
        />
      }
      className='rounded-xl overflow-hidden'
      size='middle'
    />
  );
};

export default TokensTable;
