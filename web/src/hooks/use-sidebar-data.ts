/*
Copyright (C) 2023-2026 QuantumNous

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
import {
  Activity,
  Box,
  Boxes,
  ChartColumn,
  ChartLine,
  ClipboardList,
  Coins,
  CreditCard,
  FileText,
  FlaskConical,
  FolderKanban,
  Key,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  Network,
  PlugZap,
  Radio,
  Receipt,
  ReceiptText,
  Route,
  ServerCog,
  Settings,
  ShieldAlert,
  ShieldCheck,
  Tags,
  Ticket,
  TriangleAlert,
  User,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { SidebarData } from '@/components/layout/types'
import { ROLE } from '@/lib/roles'

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()

  return {
    navGroups: [
      {
        id: 'chat',
        title: t('Chat'),
        items: [
          {
            title: t('Playground'),
            url: '/playground',
            icon: FlaskConical,
            pageKeys: ['playground'],
          },
          {
            title: t('Chat'),
            icon: MessageSquare,
            type: 'chat-presets',
            pageKeys: ['chat'],
          },
        ],
      },
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
            pageKeys: ['detail'],
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
            pageKeys: ['detail'],
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
            pageKeys: ['token'],
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
            pageKeys: ['log'],
          },
          {
            title: t('Audit Logs'),
            url: '/usage-logs/audit',
            icon: ClipboardList,
          },
          {
            title: t('Error Logs'),
            url: '/error-logs',
            icon: TriangleAlert,
            requiredRole: ROLE.SUPER_ADMIN,
            pageKeys: ['errorlog'],
          },
          {
            title: t('Task Logs'),
            url: '/usage-logs/task',
            activeUrls: ['/usage-logs/drawing'],
            configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
            icon: ListTodo,
            pageKeys: ['task', 'midjourney'],
          },
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Wallet'),
            url: '/wallet',
            icon: Wallet,
            pageKeys: ['topup'],
          },
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
            pageKeys: ['personal'],
          },
          {
            title: t('Security & Access'),
            url: '/security',
            icon: ShieldCheck,
          },
        ],
      },
      {
        /**
         * Billing pages are not role-gated as a group: org-tagged accounts
         * with `role = common` can be granted individual pages through
         * `/api/user/menu`, so visibility is decided per item.
         */
        id: 'billing',
        title: t('Billing'),
        items: [
          {
            title: t('Model Usage Analysis'),
            url: '/model-usage-analysis',
            icon: ChartLine,
            pageKeys: ['modelUsageAnalysis'],
          },
          {
            title: t('Quota Statistics'),
            url: '/quota-statistics',
            icon: ChartColumn,
            pageKeys: ['quota_statistics'],
          },
          {
            title: t('Bill'),
            url: '/bill',
            icon: ReceiptText,
            pageKeys: ['bill', 'bill_self'],
          },
          {
            title: t('UID Budgets'),
            url: '/client-user-quota',
            icon: Coins,
            pageKeys: ['client_user_quota'],
          },
          {
            title: t('Project Budgets'),
            url: '/project-budget',
            icon: FolderKanban,
            pageKeys: ['project'],
          },
          {
            title: t('Settlement Prices'),
            url: '/settlement-config',
            icon: Receipt,
            pageKeys: ['settlement_config_readonly', 'settlement_config'],
          },
          {
            title: t('Pricing Center'),
            url: '/pricing-center',
            icon: Tags,
            requiredRole: ROLE.SUPER_ADMIN,
          },
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          {
            title: t('Channels'),
            url: '/channels',
            icon: Radio,
            pageKeys: ['channel'],
          },
          {
            title: t('Models'),
            url: '/models/metadata',
            icon: Box,
            pageKeys: ['models'],
          },
          {
            title: t('Model Deployments'),
            url: '/models/deployments',
            icon: Boxes,
            pageKeys: ['deployment'],
          },
          {
            title: t('Users'),
            url: '/users',
            icon: Users,
            pageKeys: ['user'],
          },
          {
            title: t('Redemption Codes'),
            url: '/redemption-codes',
            icon: Ticket,
            pageKeys: ['redemption'],
          },
          {
            title: t('Subscriptions'),
            url: '/subscriptions',
            icon: CreditCard,
            pageKeys: ['subscription'],
          },
          {
            title: t('Model Channel Monitor'),
            url: '/model-channel-monitor',
            icon: Activity,
            requiredRole: ROLE.SUPER_ADMIN,
            pageKeys: ['modelChannelMonitor'],
          },
          {
            title: t('Internal Channel Monitor'),
            url: '/internal-channel-monitor',
            icon: ShieldAlert,
            requiredRole: ROLE.SUPER_ADMIN,
            pageKeys: ['internalChannelMonitor'],
          },
          {
            title: t('Model Route Config'),
            url: '/model-route-config',
            icon: Route,
            requiredRole: ROLE.SUPER_ADMIN,
            pageKeys: ['model_route_config'],
          },
          {
            title: t('Environments'),
            url: '/sync-environments',
            icon: Network,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('System Info'),
            url: '/system-info',
            icon: ServerCog,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('Task Plugins'),
            url: '/task-plugins',
            icon: PlugZap,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('System Settings'),
            url: '/system-settings/site',
            activeUrls: ['/system-settings'],
            icon: Settings,
            pageKeys: ['setting'],
          },
        ],
      },
    ],
  }
}
