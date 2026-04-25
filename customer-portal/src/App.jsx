import React from 'react';
import { Routes, Route, Navigate, useLocation, useNavigate, Link } from 'react-router-dom';
import { Nav, Layout, Button, Avatar, Dropdown, Typography } from '@douyinfe/semi-ui';
import {
  IconArticle,
  IconPriceTag,
  IconUser,
  IconExit,
} from '@douyinfe/semi-icons';
import { getPortalUser, clearPortalUser } from './utils/api';
import Login from './pages/Login';
import Register from './pages/Register';
import Prices from './pages/Prices';
import Bill from './pages/Bill';

const { Header, Content } = Layout;
const { Text } = Typography;

// Route guard: redirect unauthenticated users to login
function RequireAuth({ children }) {
  const user = getPortalUser();
  const location = useLocation();
  if (!user) {
    return <Navigate to="/portal/login" state={{ from: location }} replace />;
  }
  return children;
}

// Layout with top navigation bar
function PortalLayout({ children }) {
  const navigate = useNavigate();
  const location = useLocation();
  const user = getPortalUser();

  const handleLogout = () => {
    clearPortalUser();
    navigate('/portal/login');
  };

  // Determine active nav key from current path
  const getSelectedKey = () => {
    if (location.pathname.includes('/portal/bill')) return 'bill';
    if (location.pathname.includes('/portal/prices')) return 'prices';
    return 'bill';
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header
        style={{
          background: 'var(--semi-color-bg-1)',
          borderBottom: '1px solid var(--semi-color-border)',
          padding: '0 16px',
        }}
      >
        <Nav
          mode="horizontal"
          selectedKeys={[getSelectedKey()]}
          onSelect={({ itemKey }) => {
            if (itemKey === 'bill') navigate('/portal/bill');
            if (itemKey === 'prices') navigate('/portal/prices');
          }}
          header={{
            text: 'Customer Portal',
            style: { fontSize: 16, fontWeight: 600 },
          }}
          items={[
            { itemKey: 'bill', text: '账单查询', icon: <IconArticle /> },
            { itemKey: 'prices', text: '结算价格', icon: <IconPriceTag /> },
          ]}
          footer={
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Dropdown
                trigger="click"
                position="bottomRight"
                render={
                  <Dropdown.Menu>
                    <Dropdown.Item disabled>
                      <Text type="tertiary" size="small">
                        {user?.username || '用户'}
                      </Text>
                    </Dropdown.Item>
                    <Dropdown.Divider />
                    <Dropdown.Item icon={<IconExit />} onClick={handleLogout}>
                      退出登录
                    </Dropdown.Item>
                  </Dropdown.Menu>
                }
              >
                <Avatar size="small" color="blue" style={{ cursor: 'pointer' }}>
                  {(user?.username || 'U').charAt(0).toUpperCase()}
                </Avatar>
              </Dropdown>
            </div>
          }
        />
      </Header>
      <Content
        style={{
          background: 'var(--semi-color-bg-0)',
          flex: 1,
        }}
      >
        {children}
      </Content>
    </Layout>
  );
}

export default function App() {
  return (
    <Routes>
      {/* Public routes */}
      <Route path="/portal/login" element={<Login />} />
      <Route path="/portal/register" element={<Register />} />

      {/* Protected routes with layout */}
      <Route
        path="/portal/bill"
        element={
          <RequireAuth>
            <PortalLayout>
              <Bill />
            </PortalLayout>
          </RequireAuth>
        }
      />
      <Route
        path="/portal/prices"
        element={
          <RequireAuth>
            <PortalLayout>
              <Prices />
            </PortalLayout>
          </RequireAuth>
        }
      />

      {/* Default redirect */}
      <Route path="/portal" element={<Navigate to="/portal/bill" replace />} />
      <Route path="/portal/*" element={<Navigate to="/portal/bill" replace />} />
    </Routes>
  );
}
