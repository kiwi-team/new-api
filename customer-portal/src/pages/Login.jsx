import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { Form, Button, Card, Toast, Typography } from '@douyinfe/semi-ui';
import api, { setPortalUser } from '../utils/api';

const { Title, Text } = Typography;

export default function Login() {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (values) => {
    const { username, password } = values;
    if (!username || !password) {
      Toast.error('请输入用户名和密码');
      return;
    }
    setLoading(true);
    try {
      const res = await api.post('/api/user/login', { username, password });
      const { success, message, data } = res.data;
      if (success) {
        // Handle 2FA requirement
        if (data && data.require_2fa) {
          Toast.warning('该账号需要两步验证，请使用管理后台登录');
          return;
        }
        setPortalUser(data);
        Toast.success('登录成功');
        navigate('/portal/bill');
      } else {
        Toast.error(message || '登录失败');
      }
    } catch (err) {
      const msg =
        err.response?.data?.message || err.message || '网络错误，请稍后重试';
      Toast.error(msg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        minHeight: '100vh',
        background: 'var(--semi-color-fill-0)',
        padding: '16px',
      }}
    >
      <Card
        style={{ width: '100%', maxWidth: 400 }}
        bodyStyle={{ padding: '32px 24px' }}
      >
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Title heading={3}>客户门户登录</Title>
          <Text type="tertiary">Customer Portal</Text>
        </div>
        <Form onSubmit={handleSubmit} labelPosition="top">
          <Form.Input
            field="username"
            label="用户名"
            placeholder="请输入用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
            size="large"
          />
          <Form.Input
            field="password"
            label="密码"
            placeholder="请输入密码"
            mode="password"
            rules={[{ required: true, message: '请输入密码' }]}
            size="large"
          />
          <Button
            type="primary"
            htmlType="submit"
            loading={loading}
            block
            size="large"
            style={{ marginTop: 12 }}
          >
            登录
          </Button>
        </Form>
        <div style={{ textAlign: 'center', marginTop: 16 }}>
          <Text type="tertiary">
            还没有账号？{' '}
            <Link to="/portal/register" style={{ color: 'var(--semi-color-link)' }}>
              立即注册
            </Link>
          </Text>
        </div>
      </Card>
    </div>
  );
}
