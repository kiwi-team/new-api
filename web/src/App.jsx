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

import React, { lazy, Suspense, useContext, useMemo, useEffect } from 'react';
import { Route, Routes, useLocation, useNavigate, Navigate } from 'react-router-dom';
import Loading from './components/common/ui/Loading';
import User from './pages/User';
import { AuthRedirect, PrivateRoute, AdminRoute, LeaderRoute, RootRoute, PageRoute, isRoot } from './helpers';
import { isMixRouter } from './helpers';
import RegisterForm from './components/auth/RegisterForm';
// ToioRegisterForm import 已删除(组织标签系统替代)
import LoginForm from './components/auth/LoginForm';
// ToioLoginForm import 已删除(组织标签系统替代)
import NotFound from './pages/NotFound';
import Forbidden from './pages/Forbidden';
import Setting from './pages/Setting';
import { StatusContext } from './context/Status';

import PasswordResetForm from './components/auth/PasswordResetForm';
import PasswordResetConfirm from './components/auth/PasswordResetConfirm';
import Channel from './pages/Channel';
import Token from './pages/Token';
import Redemption from './pages/Redemption';
import TopUp from './pages/TopUp';
import Log from './pages/Log';
import ErrorLog from './pages/ErrorLog';
import Chat from './pages/Chat';
import Chat2Link from './pages/Chat2Link';
import Midjourney from './pages/Midjourney';
import Pricing from './pages/Pricing';
import Task from './pages/Task';
import ModelPage from './pages/Model';
import ModelDeploymentPage from './pages/ModelDeployment';
import Playground from './pages/Playground';
import Subscription from './pages/Subscription';
import OAuth2Callback from './components/auth/OAuth2Callback';
import PersonalSetting from './components/settings/PersonalSetting';
import Setup from './pages/Setup';
import SetupCheck from './components/layout/SetupCheck';
import ChannelByModel from './pages/Channel/ChannelByModel.js';
import EditChannel from './pages/Channel/EditChannel.js';
import QuotaStatistics from './pages/QuotaStatistics';
import Bill from './pages/Bill';
import CliendUserQuotaPage from './pages/CliendUserQuota';
import ProjectPage from './pages/Project';
import ModelRouteConfig from './pages/ModelRouteConfig';
import SettlementConfig from './pages/SettlementConfig';
import ModelChannelMonitor from './pages/ModelChannelMonitor';
import ModelUsageAnalysis from './pages/ModelUsageAnalysis';

// Debug module pages
import DebugExecutor from './pages/Debug/Executor';
import DebugTemplates from './pages/Debug/Templates';
import DebugLogs from './pages/Debug/LogPage';

// Sync module pages
import SyncEnvironment from './pages/SyncEnvironment';

const Home = lazy(() => import('./pages/Home'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const About = lazy(() => import('./pages/About'));
const UserAgreement = lazy(() => import('./pages/UserAgreement'));
const PrivacyPolicy = lazy(() => import('./pages/PrivacyPolicy'));

// 组织标签:砍光模式(mt)下,只允许的 URL 白名单 — 按 service.GetUserMenu 的 page key 映射。
// 不在白名单的访问被重定向到首个允许页。详见 org.md 7.2。
const PAGE_KEY_TO_URL = {
  log: '/console/log',
  quota_statistics: '/console/quota-statistics',
  client_user_quota: '/console/client-user-quota',
  project: '/console/project',
  bill: '/console/bill',
  settlement_config_readonly: '/console/settlement-config',
};

function App() {
  const location = useLocation();
  const navigate = useNavigate();
  const [statusState] = useContext(StatusContext);

  // 组织标签:消费 localStorage.user_menu(UserContext 在登录后写入)
  // 系统 admin 跳过砍光逻辑,他们看完整菜单
  const userMenu = (() => {
    try {
      const raw = localStorage.getItem('user_menu');
      return raw ? JSON.parse(raw) : null;
    } catch {
      return null;
    }
  })();
  const isWhitelistMode =
    userMenu?.topbar_mode === 'logout_only' && !isRoot();

  // 获取模型广场权限配置
  const pricingRequireAuth = useMemo(() => {
    const headerNavModulesConfig = statusState?.status?.HeaderNavModules;
    if (headerNavModulesConfig) {
      try {
        const modules = JSON.parse(headerNavModulesConfig);

        // 处理向后兼容性：如果pricing是boolean，默认不需要登录
        if (typeof modules.pricing === 'boolean') {
          return false; // 默认不需要登录鉴权
        }

        // 如果是对象格式，使用requireAuth配置
        return modules.pricing?.requireAuth === true;
      } catch (error) {
        console.error('解析顶栏模块配置失败:', error);
        return false; // 默认不需要登录
      }
    }
    return false; // 默认不需要登录
  }, [statusState?.status?.HeaderNavModules]);

  return (
    <SetupCheck>
      {isWhitelistMode && (() => {
        // 砍光模式:白名单从 userMenu.pages 计算,落地为允许的 URL 列表;
        // 当前路径不在白名单则跳到第一个允许页
        const allowed = (userMenu?.pages || [])
          .map((p) => PAGE_KEY_TO_URL[p])
          .filter(Boolean);
        if (allowed.length === 0) return null;
        if (!allowed.some((u) => location.pathname.startsWith(u))) {
          return <Navigate to={allowed[0]} replace />;
        }
        return null;
      })()}
      <Routes>
        <Route
          path='/'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <Home />
            </Suspense>
          }
        />
        <Route
          path='/setup'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <Setup />
            </Suspense>
          }
        />
        <Route path='/forbidden' element={<Forbidden />} />
        <Route
          path='/console/models'
          element={
            <AdminRoute>
              <ModelPage />
            </AdminRoute>
          }
        />
        <Route
          path='/console/deployment'
          element={
            <AdminRoute>
              <ModelDeploymentPage />
            </AdminRoute>
          }
        />
        <Route
          path='/console/subscription'
          element={
            <AdminRoute>
              <Subscription />
            </AdminRoute>
          }
        />
        <Route
          path='/console/channel'
          element={
            <AdminRoute>
              <Channel />
            </AdminRoute>
          }
        />
        <Route
          path='/console/channel/model'
          element={
            <PrivateRoute>
              <ChannelByModel />
            </PrivateRoute>
          }
        />
        <Route
          path='/console/channel/edit/:id'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <EditChannel />
            </Suspense>
          }
        />
        <Route
          path='/console/channel/add'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <EditChannel />
            </Suspense>
          }
        />
        <Route
          path='/console/token'
          element={
            <PrivateRoute>
              <Token />
            </PrivateRoute>
          }
        />
        <Route
          path='/console/playground'
          element={
            <PrivateRoute>
              <Playground />
            </PrivateRoute>
          }
        />
        <Route
          path='/console/redemption'
          element={
            <AdminRoute>
              <Redemption />
            </AdminRoute>
          }
        />
        <Route
          path='/console/quota-statistics'
          element={
            // 组织标签:消耗统计在 (mt-leader/admin, wl-admin) menu 里;系统 admin 始终通过
            <PageRoute pageKey='quota_statistics'>
              <QuotaStatistics />
            </PageRoute>
          }
        />
        <Route
          path='/console/bill'
          element={
            <PrivateRoute>
              <Bill />
            </PrivateRoute>
          }
        />
        <Route
          path='/console/client-user-quota'
          element={
            // 组织标签:mt-admin 在 menu 里有此页;系统 admin 始终通过
            <PageRoute pageKey='client_user_quota'>
              <CliendUserQuotaPage />
            </PageRoute>
          }
        />
        <Route
          path='/console/project'
          element={
            // 组织标签:mt-admin 在 menu 里有此页;系统 admin 始终通过
            <PageRoute pageKey='project'>
              <ProjectPage />
            </PageRoute>
          }
        />
        <Route
          path='/console/user'
          element={
            <AdminRoute>
              <User />
            </AdminRoute>
          }
        />
        <Route
          path='/user/reset'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <PasswordResetConfirm />
            </Suspense>
          }
        />
        <Route
          path='/login'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <AuthRedirect>
                <LoginForm />
              </AuthRedirect>
            </Suspense>
          }
        />
        {/* /toio/login + /toio/register 路由已删除(组织标签系统替代,详见 org.md)。
            老 toio 用户登录走统一 /login,所属组织由 root 手动打 tag。 */}
        <Route
          path='/register'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <AuthRedirect>
                <RegisterForm />
              </AuthRedirect>
            </Suspense>
          }
        />
        <Route
          path='/reset'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <PasswordResetForm />
            </Suspense>
          }
        />
        <Route
          path='/oauth/github'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <OAuth2Callback type='github'></OAuth2Callback>
            </Suspense>
          }
        />
        <Route
          path='/oauth/discord'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <OAuth2Callback type='discord'></OAuth2Callback>
            </Suspense>
          }
        />
        <Route
          path='/oauth/oidc'
          element={
            <Suspense fallback={<Loading></Loading>}>
              <OAuth2Callback type='oidc'></OAuth2Callback>
            </Suspense>
          }
        />
        <Route
          path='/oauth/linuxdo'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <OAuth2Callback type='linuxdo'></OAuth2Callback>
            </Suspense>
          }
        />
        <Route
          path='/console/setting'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Setting />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/model-route-config'
          element={
            <AdminRoute>
              <ModelRouteConfig />
            </AdminRoute>
          }
        />
        <Route
          path='/console/model-channel-monitor'
          element={
            <RootRoute>
              <ModelChannelMonitor />
            </RootRoute>
          }
        />
        <Route
          path='/console/internal-channel-monitor'
          element={
            <RootRoute>
              <ModelChannelMonitor internalView />
            </RootRoute>
          }
        />
        <Route
          path='/console/model-usage-analysis'
          element={
            <RootRoute>
              <ModelUsageAnalysis />
            </RootRoute>
          }
        />
        {/* Debug Module Routes - Root Only */}
        <Route
          path='/console/debug/executor'
          element={
            <RootRoute>
              <DebugExecutor />
            </RootRoute>
          }
        />
        <Route
          path='/console/debug/templates'
          element={
            <RootRoute>
              <DebugTemplates />
            </RootRoute>
          }
        />
        <Route
          path='/console/debug/logs'
          element={
            <RootRoute>
              <DebugLogs />
            </RootRoute>
          }
        />
        {/* Sync Module Routes - Root Only */}
        <Route
          path='/console/settlement-config'
          element={
            <RootRoute>
              <SettlementConfig />
            </RootRoute>
          }
        />
        <Route
          path='/console/sync'
          element={
            <RootRoute>
              <SyncEnvironment />
            </RootRoute>
          }
        />
        <Route
          path='/console/personal'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <PersonalSetting />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/topup'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <TopUp />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/log'
          element={
            <PrivateRoute>
              <Log />
            </PrivateRoute>
          }
        />
        <Route
          path='/console/errorlog'
          element={
            <PrivateRoute>
              <ErrorLog />
            </PrivateRoute>
          }
        />
        <Route
          path='/console'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Dashboard />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/midjourney'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Midjourney />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/task'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Task />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/pricing'
          element={
            pricingRequireAuth ? (
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Pricing />
                </Suspense>
              </PrivateRoute>
            ) : (
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Pricing />
              </Suspense>
            )
          }
        />
        <Route
          path='/about'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <About />
            </Suspense>
          }
        />
        <Route
          path='/user-agreement'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <UserAgreement />
            </Suspense>
          }
        />
        <Route
          path='/privacy-policy'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <PrivacyPolicy />
            </Suspense>
          }
        />
        <Route
          path='/console/chat/:id?'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <Chat />
            </Suspense>
          }
        />
        {/* 方便使用chat2link直接跳转聊天... */}
        <Route
          path='/chat2link'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Chat2Link />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route path='*' element={<NotFound />} />
      </Routes>
    </SetupCheck>
  );
}

export default App;
