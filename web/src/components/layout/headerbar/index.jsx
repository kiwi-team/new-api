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
import { useHeaderBar } from '../../../hooks/common/useHeaderBar';
import { useNotifications } from '../../../hooks/common/useNotifications';
import { useNavigation } from '../../../hooks/common/useNavigation';
import NoticeModal from '../NoticeModal';
import MobileMenuButton from './MobileMenuButton';
import HeaderLogo from './HeaderLogo';
import Navigation from './Navigation';
import ActionButtons from './ActionButtons';

const HeaderBar = ({ onMobileMenuToggle, drawerOpen }) => {
  const {
    userState,
    statusState,
    isMobile,
    collapsed,
    logoLoaded,
    currentLang,
    isLoading,
    systemName,
    logo,
    isNewYear,
    isSelfUseMode,
    docsLink,
    isDemoSiteMode,
    isConsoleRoute,
    theme,
    headerNavModules,
    pricingRequireAuth,
    logout,
    handleLanguageChange,
    handleThemeToggle,
    handleMobileMenuToggle,
    navigate,
    t,
  } = useHeaderBar({ onMobileMenuToggle, drawerOpen });
  // 组织标签系统:消费 userMenu.topbar_mode (详见 org.md 6.1 / 7.3)。
  // 不再读 toio_registered / is_toio。
  const toioOnlyLogout = userState?.userMenu?.topbar_mode === 'logout_only';

  const {
    noticeVisible,
    unreadCount,
    handleNoticeOpen,
    handleNoticeClose,
    getUnreadKeys,
  } = useNotifications(statusState);

  const { mainNavLinks } = useNavigation(t, docsLink, headerNavModules);

  return (
    <header className='text-semi-color-text-0 sticky top-0 z-50 transition-colors duration-300 bg-white/75 dark:bg-zinc-900/75 backdrop-blur-lg'>
      <NoticeModal
        visible={noticeVisible}
        onClose={handleNoticeClose}
        isMobile={isMobile}
        defaultTab={unreadCount > 0 ? 'system' : 'inApp'}
        unreadKeys={getUnreadKeys()}
      />

      <div className='w-full px-2'>
        <div className='flex items-center justify-between h-16'>
          {toioOnlyLogout ? (
            <div className='flex-1 flex items-center justify-end gap-3'>
              {/* 砍光模式(mt 等):右上角只剩登出按钮时,补上当前账号 + 组织角色,
                  避免用户误以为不知道自己以什么身份登录 */}
              {userState?.user?.username && (
                <span className='text-semi-color-text-1 text-sm'>
                  {userState.user.display_name || userState.user.username}
                  {userState.user.org_role && (
                    <span className='ml-1 text-semi-color-text-2'>
                      ({userState.user.org_code
                        ? `${userState.user.org_code}-${userState.user.org_role}`
                        : userState.user.org_role})
                    </span>
                  )}
                </span>
              )}
              <ActionButtons
                isNewYear={isNewYear}
                unreadCount={unreadCount}
                onNoticeOpen={handleNoticeOpen}
                theme={theme}
                onThemeToggle={handleThemeToggle}
                currentLang={currentLang}
                onLanguageChange={handleLanguageChange}
                userState={userState}
                isLoading={isLoading}
                isMobile={isMobile}
                isSelfUseMode={isSelfUseMode}
                logout={logout}
                navigate={navigate}
                t={t}
                onlyLogout={true}
              />
            </div>
          ) : (
            <>
              <div className='flex items-center'>
                <MobileMenuButton
                  isConsoleRoute={isConsoleRoute}
                  isMobile={isMobile}
                  drawerOpen={drawerOpen}
                  collapsed={collapsed}
                  onToggle={handleMobileMenuToggle}
                  t={t}
                />

                <HeaderLogo
                  isMobile={isMobile}
                  isConsoleRoute={isConsoleRoute}
                  logo={logo}
                  logoLoaded={logoLoaded}
                  isLoading={isLoading}
                  systemName={systemName}
                  isSelfUseMode={isSelfUseMode}
                  isDemoSiteMode={isDemoSiteMode}
                  t={t}
                />
              </div>

              <Navigation
                mainNavLinks={mainNavLinks}
                isMobile={isMobile}
                isLoading={isLoading}
                userState={userState}
                pricingRequireAuth={pricingRequireAuth}
              />

              <ActionButtons
                isNewYear={isNewYear}
                unreadCount={unreadCount}
                onNoticeOpen={handleNoticeClose}
                theme={theme}
                onThemeToggle={handleThemeToggle}
                currentLang={currentLang}
                onLanguageChange={handleLanguageChange}
                userState={userState}
                isLoading={isLoading}
                isMobile={isMobile}
                isSelfUseMode={isSelfUseMode}
                logout={logout}
                navigate={navigate}
                t={t}
              />
            </>
          )}
        </div>
      </div>
    </header>
  );
};

export default HeaderBar;
