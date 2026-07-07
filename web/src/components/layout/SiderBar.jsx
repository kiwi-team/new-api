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

import React, { useContext, useEffect, useMemo, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getLucideIcon } from '../../helpers/render';
import { ChevronLeft } from 'lucide-react';
import { useSidebarCollapsed } from '../../hooks/common/useSidebarCollapsed';
import { useSidebar } from '../../hooks/common/useSidebar';
import { useMinimumLoadingTime } from '../../hooks/common/useMinimumLoadingTime';
import { isAdmin, isLeader, isRoot, isMixRouter, showError } from '../../helpers';
import { UserContext } from '../../context/User';
import SkeletonWrapper from './components/SkeletonWrapper';

import { Nav, Divider, Button } from '@douyinfe/semi-ui';

// 把 SiderBar 的 itemKey 映射到 service.GetUserMenu 返回的 page key。
// 一个 itemKey 可以对应多个候选 page(任一命中即可见)。
// 详见 org.md service/org_view.go 中的 page 常量。
const ITEM_KEY_TO_PAGES = {
  cuquota: ['client_user_quota'],
  quotaStatistics: ['quota_statistics'],
  modelRouteConfig: ['model_route_config'],
  settlementConfig: ['settlement_config_readonly', 'settlement_config'],
  // bill: 既包含普通用户的 bill_self,也包含 wl-admin 的全组织 bill
  bill: ['bill', 'bill_self'],
};

const routerMap = {
  home: '/',
  channel: '/console/channel',
  token: '/console/token',
  redemption: '/console/redemption',
  topup: '/console/topup',
  user: '/console/user',
  subscription: '/console/subscription',
  log: '/console/log',
  errorlog: '/console/errorlog',
  midjourney: '/console/midjourney',
  setting: '/console/setting',
  modelRouteConfig: '/console/model-route-config',
  modelChannelMonitor: '/console/model-channel-monitor',
  internalChannelMonitor: '/console/internal-channel-monitor',
  modelUsageAnalysis: '/console/model-usage-analysis',
  about: '/about',
  detail: '/console',
  pricing: '/pricing',
  task: '/console/task',
  models: '/console/models',
  //channelByModel: '/console/channel/model',
  deployment: '/console/deployment',
  playground: '/console/playground',
  personal: '/console/personal',
  quotaStatistics: '/console/quota-statistics',
  bill: '/console/bill',
  cuquota: '/console/client-user-quota',
  project: '/console/project',
  // Settlement config route
  settlementConfig: '/console/settlement-config',
  // Debug module routes
  debugExecutor: '/console/debug/executor',
  debugTemplates: '/console/debug/templates',
  debugLogs: '/console/debug/logs',
  // Sync module routes
  sync: '/console/sync',
};

const SiderBar = ({ onNavigate = () => {} }) => {
  const { t } = useTranslation();
  const [collapsed, toggleCollapsed] = useSidebarCollapsed();
  const {
    isModuleVisible,
    hasSectionVisibleModules,
    loading: sidebarLoading,
  } = useSidebar();

  const showSkeleton = useMinimumLoadingTime(sidebarLoading, 200);

  const [selectedKeys, setSelectedKeys] = useState(['home']);
  const [chatItems, setChatItems] = useState([]);
  const [openedKeys, setOpenedKeys] = useState([]);
  const location = useLocation();
  const [routerMapState, setRouterMapState] = useState(routerMap);
  // 组织标签系统:消费 UserContext.userMenu(由 UserContext 在登录后从 /api/user/menu 拉)。
  // SiderBar 只渲染 userMenu.pages 里的菜单(系统 admin 看完整菜单)。详见 org.md。
  const [userContextState] = useContext(UserContext);
  const userMenu = userContextState?.userMenu;
  // 砍光模式:顶栏只剩登出 + 菜单严格白名单(mt 砍光)
  const isWhitelistMode = userMenu?.topbar_mode === 'logout_only' && !isAdmin();
  // 判定一个 itemKey 是否在当前用户的 userMenu.pages 内(系统 admin 默认放行)
  const itemAllowedByMenu = (itemKey) => {
    if (isAdmin() || isRoot()) return true;
    if (!userMenu?.pages) return true; // menu 还没拉到时不拦截,避免闪烁
    const candidates = ITEM_KEY_TO_PAGES[itemKey] || [itemKey];
    return candidates.some((k) => userMenu.pages.includes(k));
  };

  const workspaceItems = useMemo(() => {
    // 组织标签:角色相关的可见性(errorlog/quotaStatistics)交给 itemAllowedByMenu 过滤,
    // 不再用 tableHiddle CSS hide(否则 mt-admin 等 role=common 但有 org 加成的用户会被
    // 错误地藏掉菜单项)。仅保留 feature flag 类的 className(detail/midjourney/task)。
    const items = [
      { text: t('数据看板'), itemKey: 'detail', to: '/detail', className: localStorage.getItem('enable_data_export') === 'true' ? '' : 'tableHiddle' },
      { text: t('令牌管理'), itemKey: 'token', to: '/token' },
      { text: t('使用日志'), itemKey: 'log', to: '/log' },
      { text: t('错误日志'), itemKey: 'errorlog', to: '/errorlog' },
      { text: t('绘图日志'), itemKey: 'midjourney', to: '/midjourney', className: localStorage.getItem('enable_drawing') === 'true' ? '' : 'tableHiddle' },
      { text: t('任务日志'), itemKey: 'task', to: '/task', className: localStorage.getItem('enable_task') === 'true' ? '' : 'tableHiddle' },
      { text: t('消耗统计'), itemKey: 'quotaStatistics', to: '/quota-statistics' },
      { text: t('用量分析'), itemKey: 'modelUsageAnalysis', to: '/console/model-usage-analysis' },
      { text: t('账单查询'), itemKey: 'bill', to: '/console/bill' },
    ];

    // 组织标签:userMenu.pages 决定可见项(系统 admin bypass)
    // tableHiddle/isModuleVisible 是后端 menu 之外的额外细分(数据看板的功能开关等)
    const filteredItems = items.filter((item) => {
      // 账单查询:普通用户(仅 bill_self)隐藏,仅系统 admin / 组织管理员(org 级 bill 授权)可见
      if (item.itemKey === 'bill') {
        if (isAdmin() || isRoot()) return true;
        return !!userMenu?.pages?.includes('bill');
      }
      if (!itemAllowedByMenu(item.itemKey)) return false;
      if (item.itemKey === 'quotaStatistics') return true;
      const configVisible = isModuleVisible('console', item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [
    localStorage.getItem('enable_data_export'),
    localStorage.getItem('enable_drawing'),
    localStorage.getItem('enable_task'),
    t,
    isModuleVisible,
    userMenu?.pages,
  ]);

  const financeItems = useMemo(() => {
    const items = [
      {
        text: t('钱包管理'),
        itemKey: 'topup',
        to: '/topup',
      },
      {
        text: t('个人设置'),
        itemKey: 'personal',
        to: '/personal',
      },
    ];

    // 根据配置过滤项目
    const filteredItems = items.filter((item) => {
      const configVisible = isModuleVisible('personal', item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [t, isModuleVisible]);

  const adminItems = useMemo(() => {
    const items = [
      { text: t('渠道管理'), itemKey: 'channel', to: '/channel', className: isRoot() ? '' : 'tableHiddle' },
      { text: t('模型管理'), itemKey: 'models', to: '/console/models', className: isAdmin() ? '' : 'tableHiddle' },
      { text: t('模型部署'), itemKey: 'deployment', to: '/deployment', className: isAdmin() ? '' : 'tableHiddle' },
      { text: t('模型渠道监控'), itemKey: 'modelChannelMonitor', to: '/console/model-channel-monitor', className: isRoot() ? '' : 'tableHiddle' },
      { text: t('内部渠道监控'), itemKey: 'internalChannelMonitor', to: '/console/internal-channel-monitor', className: isRoot() ? '' : 'tableHiddle' },
      { text: t('兑换码管理'), itemKey: 'redemption', to: '/redemption', className: isAdmin() ? '' : 'tableHiddle' },
      {
        text: t('订阅管理'),
        itemKey: 'subscription',
        to: '/subscription',
        className: isAdmin() ? '' : 'tableHiddle',
      },
      { text: t('用户管理'), itemKey: 'user', to: '/user', className: isAdmin() ? '' : 'tableHiddle' },
      { text: t('UID预算管理'), itemKey: 'cuquota', to: '/console/client-user-quota' },
      { text: t('项目预算管理'), itemKey: 'project', to: '/console/project' },
      { text: t('模型路由配置'), itemKey: 'modelRouteConfig', to: '/console/model-route-config', className: isRoot() ? '' : 'tableHiddle' },
      { text: t('系统设置'), itemKey: 'setting', to: '/setting', className: isRoot() ? '' : 'tableHiddle' },
      { text: t('结算价格管理'), itemKey: 'settlementConfig', to: '/console/settlement-config' },
    ];

    // 组织标签:userMenu.pages 决定可见项(系统 admin bypass)
    const filteredItems = items.filter((item) => {
      if (!itemAllowedByMenu(item.itemKey)) return false;
      const configVisible = isModuleVisible('admin', item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [isAdmin(), isRoot(), t, isModuleVisible, userMenu?.pages]);

  // Debug module items - only for root users
  const debugItems = useMemo(() => {
    if (!isRoot()) return [];
    
    return [
      { text: t('调试器'), itemKey: 'debugExecutor', to: '/console/debug/executor' },
      { text: t('测试数据'), itemKey: 'debugTemplates', to: '/console/debug/templates' },
      { text: t('调试日志'), itemKey: 'debugLogs', to: '/console/debug/logs' },
    ];
  }, [isRoot(), t]);

  // Sync module items - only for root users
  const syncItems = useMemo(() => {
    if (!isRoot()) return [];
    
    return [
      { text: t('环境管理'), itemKey: 'sync', to: '/console/sync' },
    ];
  }, [isRoot(), t]);

  const chatMenuItems = useMemo(() => {
    const items = [
      {
        text: t('操练场'),
        itemKey: 'playground',
        to: '/playground',
      },
      {
        text: t('聊天'),
        itemKey: 'chat',
        items: chatItems,
      },
    ];

    // 组织标签:userMenu.pages 决定可见项(wl 用户菜单里没有 playground/chat,自动过滤掉);
    // isModuleVisible 是另一层管理员可配置的功能开关。
    const filteredItems = items.filter((item) => {
      if (!itemAllowedByMenu(item.itemKey)) return false;
      const configVisible = isModuleVisible('chat', item.itemKey);
      return configVisible;
    });

    return filteredItems;
  }, [chatItems, t, isModuleVisible, userMenu?.pages]);

  // 更新路由映射，添加聊天路由
  const updateRouterMapWithChats = (chats) => {
    const newRouterMap = { ...routerMap };

    if (Array.isArray(chats) && chats.length > 0) {
      for (let i = 0; i < chats.length; i++) {
        newRouterMap['chat' + i] = '/console/chat/' + i;
      }
    }

    setRouterMapState(newRouterMap);
    return newRouterMap;
  };

  // 加载聊天项
  useEffect(() => {
    let chats = localStorage.getItem('chats');
    if (chats) {
      try {
        chats = JSON.parse(chats);
        if (Array.isArray(chats)) {
          let chatItems = [];
          for (let i = 0; i < chats.length; i++) {
            let shouldSkip = false;
            let chat = {};
            for (let key in chats[i]) {
              let link = chats[i][key];
              if (typeof link !== 'string') continue; // 确保链接是字符串
              if (link.startsWith('fluent')) {
                shouldSkip = true;
                break; // 跳过 Fluent Read
              }
              chat.text = key;
              chat.itemKey = 'chat' + i;
              chat.to = '/console/chat/' + i;
            }
            if (shouldSkip || !chat.text) continue; // 避免推入空项
            chatItems.push(chat);
          }
          setChatItems(chatItems);
          updateRouterMapWithChats(chats);
        }
      } catch (e) {
        showError('聊天数据解析失败');
      }
    }
  }, []);

  // 根据当前路径设置选中的菜单项
  useEffect(() => {
    const currentPath = location.pathname;
    let matchingKey = Object.keys(routerMapState).find(
      (key) => routerMapState[key] === currentPath,
    );

    // 处理聊天路由
    if (!matchingKey && currentPath.startsWith('/console/chat/')) {
      const chatIndex = currentPath.split('/').pop();
      if (!isNaN(chatIndex)) {
        matchingKey = 'chat' + chatIndex;
      } else {
        matchingKey = 'chat';
      }
    }

    // 如果找到匹配的键，更新选中的键
    if (matchingKey) {
      setSelectedKeys([matchingKey]);
    }
  }, [location.pathname, routerMapState]);

  // 监控折叠状态变化以更新 body class
  useEffect(() => {
    if (collapsed) {
      document.body.classList.add('sidebar-collapsed');
    } else {
      document.body.classList.remove('sidebar-collapsed');
    }
  }, [collapsed]);

  // 选中高亮颜色（统一）
  const SELECTED_COLOR = 'var(--semi-color-primary)';

  // 渲染自定义菜单项
  const renderNavItem = (item) => {
    // 跳过隐藏的项目
    if (item.className === 'tableHiddle') return null;

    const isSelected = selectedKeys.includes(item.itemKey);
    const textColor = isSelected ? SELECTED_COLOR : 'inherit';

    return (
      <Nav.Item
        key={item.itemKey}
        itemKey={item.itemKey}
        text={
          <span
            className='truncate font-medium text-sm'
            style={{ color: textColor }}
          >
            {item.text}
          </span>
        }
        icon={
          <div className='sidebar-icon-container flex-shrink-0'>
            {getLucideIcon(item.itemKey, isSelected)}
          </div>
        }
        className={item.className}
      />
    );
  };

  // 渲染子菜单项
  const renderSubItem = (item) => {
    if (item.items && item.items.length > 0) {
      const isSelected = selectedKeys.includes(item.itemKey);
      const textColor = isSelected ? SELECTED_COLOR : 'inherit';

      return (
        <Nav.Sub
          key={item.itemKey}
          itemKey={item.itemKey}
          text={
            <span
              className='truncate font-medium text-sm'
              style={{ color: textColor }}
            >
              {item.text}
            </span>
          }
          icon={
            <div className='sidebar-icon-container flex-shrink-0'>
              {getLucideIcon(item.itemKey, isSelected)}
            </div>
          }
        >
          {item.items.map((subItem) => {
            const isSubSelected = selectedKeys.includes(subItem.itemKey);
            const subTextColor = isSubSelected ? SELECTED_COLOR : 'inherit';

            return (
              <Nav.Item
                key={subItem.itemKey}
                itemKey={subItem.itemKey}
                text={
                  <span
                    className='truncate font-medium text-sm'
                    style={{ color: subTextColor }}
                  >
                    {subItem.text}
                  </span>
                }
              />
            );
          })}
        </Nav.Sub>
      );
    } else {
      return renderNavItem(item);
    }
  };

  return (
    <div
      className='sidebar-container'
      style={{
        width: 'var(--sidebar-current-width)',
      }}
    >
      <SkeletonWrapper
        loading={showSkeleton}
        type='sidebar'
        className=''
        collapsed={collapsed}
        showAdmin={isAdmin()}
      >
        <Nav
          className='sidebar-nav'
          defaultIsCollapsed={collapsed}
          isCollapsed={collapsed}
          onCollapseChange={toggleCollapsed}
          selectedKeys={selectedKeys}
          itemStyle='sidebar-nav-item'
          hoverStyle='sidebar-nav-item:hover'
          selectedStyle='sidebar-nav-item-selected'
          renderWrapper={({ itemElement, props }) => {
            const to =
              routerMapState[props.itemKey] || routerMap[props.itemKey];

            // 如果没有路由，直接返回元素
            if (!to) return itemElement;

            return (
              <Link
                style={{ textDecoration: 'none' }}
                to={to}
                onClick={onNavigate}
              >
                {itemElement}
              </Link>
            );
          }}
          onSelect={(key) => {
            // 如果点击的是已经展开的子菜单的父项，则收起子菜单
            if (openedKeys.includes(key.itemKey)) {
              setOpenedKeys(openedKeys.filter((k) => k !== key.itemKey));
            }

            setSelectedKeys([key.itemKey]);
          }}
          openKeys={openedKeys}
          onOpenChange={(data) => {
            setOpenedKeys(data.openKeys);
          }}
        >
          {/* 聊天区域:砍光模式(mt)不渲染;wl 用户 chatMenuItems 已经被 userMenu 过滤空,
              用 chatMenuItems.length 兜底也藏掉空标题 */}
          {!isWhitelistMode &&
            hasSectionVisibleModules('chat') &&
            chatMenuItems.length > 0 && (
              <div className='sidebar-section'>
                {!collapsed && (
                  <div className='sidebar-group-label'>{t('聊天')}</div>
                )}
                {chatMenuItems.map((item) => renderSubItem(item))}
              </div>
            )}

          {/* 控制台区域 */}
          {hasSectionVisibleModules('console') && (
            <>
              <Divider className='sidebar-divider' />
              <div>
                {!collapsed && (
                  <div className='sidebar-group-label'>{t('控制台')}</div>
                )}
                {workspaceItems.map((item) => renderNavItem(item))}
              </div>
            </>
          )}

          {/* 个人中心区域:砍光模式(mt)不渲染 */}
          {!isWhitelistMode && hasSectionVisibleModules('personal') && (
            <>
              <Divider className='sidebar-divider' />
              <div>
                {!collapsed && (
                  <div className='sidebar-group-label'>{t('个人中心')}</div>
                )}
                {financeItems.map((item) => renderNavItem(item))}
              </div>
            </>
          )}

          {/* 管理员区域:系统 admin 看完整 + 组织 admin(如 mt-admin)看部分。
              判断依据是 adminItems 非空 OR isAdmin。adminItems 已经按 userMenu 过滤过了。 */}
          {(isAdmin() && hasSectionVisibleModules('admin')) || adminItems.length > 0 ? (
            <>
              <Divider className='sidebar-divider' />
              <div>
                {!collapsed && (
                  <div className='sidebar-group-label'>{t('管理员')}</div>
                )}
                {adminItems.map((item) => renderNavItem(item))}
              </div>
            </>
          ) : null}

          {/* 调试模块区域 - 仅超级管理员可见 */}
          {isRoot() && debugItems.length > 0 && (
            <>
              <Divider className='sidebar-divider' />
              <div>
                {!collapsed && (
                  <div className='sidebar-group-label'>{t('调试模块')}</div>
                )}
                {debugItems.map((item) => renderNavItem(item))}
              </div>
            </>
          )}

          {/* 环境同步区域 - 仅超级管理员可见 */}
          {isRoot() && syncItems.length > 0 && (
            <>
              <Divider className='sidebar-divider' />
              <div>
                {!collapsed && (
                  <div className='sidebar-group-label'>{t('环境同步')}</div>
                )}
                {syncItems.map((item) => renderNavItem(item))}
              </div>
            </>
          )}
        </Nav>
      </SkeletonWrapper>

      {/* 底部折叠按钮 */}
      <div className='sidebar-collapse-button'>
        <SkeletonWrapper
          loading={showSkeleton}
          type='button'
          width={collapsed ? 36 : 156}
          height={24}
          className='w-full'
        >
          <Button
            theme='outline'
            type='tertiary'
            size='small'
            icon={
              <ChevronLeft
                size={16}
                strokeWidth={2.5}
                color='var(--semi-color-text-2)'
                style={{
                  transform: collapsed ? 'rotate(180deg)' : 'rotate(0deg)',
                }}
              />
            }
            onClick={toggleCollapsed}
            icononly={collapsed}
            style={
              collapsed
                ? { width: 36, height: 24, padding: 0 }
                : { padding: '4px 12px', width: '100%' }
            }
          >
            {!collapsed ? t('收起侧边栏') : null}
          </Button>
        </SkeletonWrapper>
      </div>
    </div>
  );
};

export default SiderBar;
