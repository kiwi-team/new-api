import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { Form, Button, Card, Toast, Typography } from '@douyinfe/semi-ui';
import api from '../utils/api';

const { Title, Text } = Typography;

export default function Register() {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (values) => {
    const { username, password, password2 } = values;
    if (!username || !password) {
      Toast.error('请填写所有必填项');
      return;
    }
    if (password !== password2) {
      Toast.error('两次输入的密码不一致');
      return;
    }
    if (password.length < 8) {
      Toast.error('密码长度至少为 8 位');
      return;
    }
    setLoading(true);
    try {
      const res = await api.post('/api/user/register', { username, password });
      const { success, message } = res.data;
      if (success) {
        Toast.success('注册成功，请登录');
        navigate('/portal/login');
      } else {
        Toast.error(message || '注册失败');
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
          <Title heading={3}>注册账号</Title>
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
            placeholder="请输入密码（至少 8 位）"
            mode="password"
            rules={[
              { required: true, message: '请输入密码' },
              { validator: (_, val) => val && val.length >= 8, message: '密码长度至少为 8 位' },
            ]}
            size="large"
          />
          <Form.Input
            field="password2"
            label="确认密码"
            placeholder="请再次输入密码"
            mode="password"
            rules={[{ required: true, message: '请确认密码' }]}
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
            注册
          </Button>
        </Form>
        <div style={{ textAlign: 'center', marginTop: 16 }}>
          <Text type="tertiary">
            已有账号？{' '}
            <Link to="/portal/login" style={{ color: 'var(--semi-color-link)' }}>
              返回登录
            </Link>
          </Text>
        </div>
      </Card>
    </div>
  );
}
