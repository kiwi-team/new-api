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

import React, { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import { reducer, initialState } from './reducer';

export const UserContext = React.createContext({
  state: initialState,
  dispatch: () => null,
});

export const UserProvider = ({ children }) => {
  const [state, dispatch] = React.useReducer(reducer, initialState);
  const { i18n } = useTranslation();

  // Sync language preference when user data is loaded
  useEffect(() => {
    if (state.user?.setting) {
      try {
        const settings = JSON.parse(state.user.setting);
        if (settings.language && settings.language !== i18n.language) {
          i18n.changeLanguage(settings.language);
        }
      } catch (e) {
        // Ignore parse errors
      }
    }
  }, [state.user?.setting, i18n]);

  // 组织标签系统:登录后(user 变化)自动拉 /api/user/menu 缓存到 context + localStorage。
  // SiderBar/App.jsx/Headerbar 都消费 state.userMenu;
  // PageRoute 路由守卫读 localStorage('user_menu')(它在 React 渲染前判定)。
  // 详见 org.md 第 6.1 / 7.1 / 7.2 节。
  useEffect(() => {
    if (!state.user?.id) {
      localStorage.removeItem('user_menu');
      return;
    }
    (async () => {
      try {
        const res = await API.get('/api/user/menu');
        if (res.data?.success && res.data?.data) {
          dispatch({ type: 'setUserMenu', payload: res.data.data });
          localStorage.setItem('user_menu', JSON.stringify(res.data.data));
        }
      } catch (e) {
        // 静默失败:menu 为 undefined,消费方按"未授权"兜底
      }
    })();
  }, [state.user?.id]);

  return (
    <UserContext.Provider value={[state, dispatch]}>
      {children}
    </UserContext.Provider>
  );
};
