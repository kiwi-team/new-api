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

import React from 'react';
import { Navigate } from 'react-router-dom';
import { history } from './history';

export function authHeader() {
  // return authorization header with jwt token
  let user = JSON.parse(localStorage.getItem('user'));

  if (user && user.token) {
    return { Authorization: 'Bearer ' + user.token };
  } else {
    return {};
  }
}

export const AuthRedirect = ({ children }) => {
  const user = localStorage.getItem('user');

  if (user) {
    return <Navigate to='/console' replace />;
  }

  return children;
};

function PrivateRoute({ children }) {
  if (!localStorage.getItem('user')) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  return children;
}

export function AdminRoute({ children }) {
  const raw = localStorage.getItem('user');
  if (!raw) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  try {
    const user = JSON.parse(raw);
    if (user && typeof user.role === 'number' && user.role >= 10) {
      return children;
    }
  } catch (e) {
    // ignore
  }
  return <Navigate to='/forbidden' replace />;
}

export function LeaderRoute({ children }) {
  const raw = localStorage.getItem('user');
  if (!raw) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  try {
    const user = JSON.parse(raw);
    // Leader 用户 (role >= 5) 可以访问
    if (user && typeof user.role === 'number' && user.role >= 5) {
      return children;
    }
  } catch (e) {
    // ignore
  }
  return <Navigate to='/forbidden' replace />;
}

export function RootRoute({ children }) {
  const raw = localStorage.getItem('user');
  if (!raw) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  try {
    const user = JSON.parse(raw);
    // 只有超级管理员 (role === 100) 可以访问
    if (user && typeof user.role === 'number' && user.role === 100) {
      return children;
    }
  } catch (e) {
    // ignore
  }
  return <Navigate to='/forbidden' replace />;
}

export { PrivateRoute };

/**
 * PageRoute - 组织标签系统的统一权限路由守卫(详见 org.md 第 7.2 节)
 *
 * 用法: <PageRoute pageKey="quota_statistics"><MyPage /></PageRoute>
 *
 * 行为:
 *   - 未登录 → 跳 /login
 *   - 系统 admin/root (role >= 10) → 直接放行(完整菜单)
 *   - 当前 userMenu.pages 含 pageKey → 放行
 *   - 否则 → 跳 /forbidden
 *
 * userMenu 由 UserContext 在登录后自动 fetch /api/user/menu 拿到,后端按 (org_code, org_role) 计算。
 * 在 menu 还没加载完成时(首次访问),按 user.role 兜底放行高权限用户,避免闪烁。
 */
export function PageRoute({ pageKey, children }) {
  const raw = localStorage.getItem('user');
  if (!raw) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  try {
    const user = JSON.parse(raw);
    // 系统 admin/root 始终通过(他们看完整菜单)
    if (user && typeof user.role === 'number' && user.role >= 10) {
      return children;
    }
    // 读 localStorage 缓存的 userMenu(UserContext 注入)
    const menuRaw = localStorage.getItem('user_menu');
    if (menuRaw) {
      const menu = JSON.parse(menuRaw);
      if (Array.isArray(menu?.pages) && menu.pages.includes(pageKey)) {
        return children;
      }
      // 已经拉到 menu 但 page 不在白名单 → 拒
      return <Navigate to='/forbidden' replace />;
    }
    // menu 还没拉到 → 暂时放行(避免闪烁);后端有 PageAuth 中间件兜底
    return children;
  } catch (e) {
    return <Navigate to='/forbidden' replace />;
  }
}
