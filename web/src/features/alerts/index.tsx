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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { PageFooterPortal, SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationPrevious,
  PaginationNext,
} from '@/components/ui/pagination'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { toIntlLocale } from '@/i18n/languages'
import { useAuthStore } from '@/stores/auth-store'

import { alertRequest, getAlertCatalog } from './api'
import { ChannelControls } from './components/channel-controls'
import { EnginePanel } from './components/engine-panel'
import { EventDetailDialog } from './components/event-detail'
import { ImportNotification } from './components/import-notification'
import { IncidentDetailDialog } from './components/incident-detail'
import { MigrationsPanel } from './components/migrations-panel'
import { RuleDetailDialog } from './components/rule-detail'
import { RuleEditor } from './components/rule-editor'
import { RulesList } from './components/rules-list'
import { TargetEditor } from './components/target-editor'
import { TargetsTable } from './components/targets-table'
import { formatAlertEventSummary } from './lib/event-summary'
import { alertLabel } from './lib/labels'
import type {
  AlertEvent,
  AlertIncident,
  AlertRule,
  AlertTarget,
  HoldResponse,
} from './types'

type Confirmation = {
  title: string
  description: string
  run: () => Promise<unknown>
}

export function AlertsPage(props: {
  initialEventID?: number
  initialChannelID?: number
  initialTokenID?: number
  initialOwnerID?: number
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const user = useAuthStore((state) => state.auth.user)
  const client = useQueryClient()
  const verification = useSecureVerification()
  const [tab, setTab] = useState('rules')
  const [all, setAll] = useState(true)
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [cursors, setCursors] = useState<number[]>([])
  const cursor = cursors.at(-1)
  const [viewingRule, setViewingRule] = useState<AlertRule>()
  const [editingRule, setEditingRule] = useState<AlertRule | null | undefined>()
  const [editingTarget, setEditingTarget] = useState<
    AlertTarget | null | undefined
  >()
  const [detail, setDetail] = useState<AlertEvent>()
  const [incidentDetail, setIncidentDetail] = useState<AlertIncident>()
  const [dismissedLink, setDismissedLink] = useState(false)
  const linkedEvent = useQuery({
    queryKey: ['alerts', 'event', props.initialEventID],
    queryFn: () =>
      alertRequest<{ event: AlertEvent }>(`events/${props.initialEventID}`),
    enabled: !!props.initialEventID && !dismissedLink,
  })
  const activeDetail =
    detail || (!dismissedLink ? linkedEvent.data?.event : undefined)
  const [confirmation, setConfirmation] = useState<Confirmation>()
  const catalog = useQuery({
    queryKey: ['alerts', 'catalog'],
    queryFn: getAlertCatalog,
  })
  const listParams = new URLSearchParams({ limit: '50' })
  if (all && catalog.data?.read_all && props.initialOwnerID === undefined) {
    listParams.set('scope', 'all')
  }
  if (props.initialOwnerID !== undefined) {
    listParams.set('owner_user_id', String(props.initialOwnerID))
  }
  if (
    props.initialChannelID &&
    ['rules', 'events', 'incidents', 'holds'].includes(tab)
  ) {
    listParams.set('channel_id', String(props.initialChannelID))
  }
  if (
    props.initialTokenID &&
    ['rules', 'events', 'incidents', 'holds'].includes(tab)
  ) {
    listParams.set('token_id', String(props.initialTokenID))
  }
  if (cursor) listParams.set('before_id', String(cursor))
  if (search && tab !== 'holds') listParams.set('q', search)
  const suffix = `?${listParams}`
  const detailRuleID = activeDetail?.rule_id ?? incidentDetail?.rule_id
  const detailRule = useQuery({
    queryKey: ['alerts', 'rule', detailRuleID],
    queryFn: () => alertRequest<AlertRule>(`rules/${detailRuleID}`),
    enabled: !!detailRuleID && !!catalog.data && !catalog.data.manage_platform,
    retry: false,
    meta: { errorToast: false },
  })
  const targets = useQuery({
    queryKey: ['alerts', 'target-options', catalog.data?.manage_platform],
    queryFn: async () => {
      const requests = [alertRequest<AlertTarget[]>('targets')]
      if (catalog.data?.manage_platform) {
        requests.push(alertRequest<AlertTarget[]>('targets?owner_user_id=0'))
      }
      return (await Promise.all(requests)).flat()
    },
    enabled: !!catalog.data,
  })
  const listedTargets = useQuery({
    queryKey: ['alerts', 'targets', all, suffix],
    queryFn: () => alertRequest<AlertTarget[]>(`targets${suffix}`),
    enabled: tab === 'targets',
  })
  const incidentRules = useQuery({
    queryKey: ['alerts', 'incident-permissions'],
    queryFn: () => alertRequest<AlertRule[]>('rules?limit=200'),
    enabled: tab === 'incidents',
  })
  const incidents = useQuery({
    queryKey: ['alerts', 'incidents', all, suffix],
    queryFn: () => alertRequest<AlertIncident[]>(`incidents${suffix}`),
    enabled: tab === 'incidents',
    refetchInterval: 10000,
  })
  const events = useQuery({
    queryKey: ['alerts', 'events', all, suffix],
    queryFn: () => alertRequest<AlertEvent[]>(`events${suffix}`),
    enabled: tab === 'events',
    refetchInterval: 15000,
  })
  const holds = useQuery({
    queryKey: ['alerts', 'holds', all, suffix],
    queryFn: () => alertRequest<HoldResponse[]>(`holds${suffix}`),
    enabled: tab === 'holds',
    refetchInterval: 15000,
  })
  const refresh = () => client.invalidateQueries({ queryKey: ['alerts'] })
  const operation = useMutation({
    mutationFn: async () => {
      await confirmation?.run()
    },
    onSuccess: () => {
      setConfirmation(undefined)
      void refresh()
    },
  })
  const canManage = (rule: AlertRule) =>
    catalog.data?.manage_platform ||
    (rule.owner_user_id === user?.id &&
      rule.config.management_mode === 'user_managed')
  const date = (stamp: number) =>
    new Intl.DateTimeFormat(locale, {
      dateStyle: 'short',
      timeStyle: 'medium',
    }).format(stamp)
  const handlePause = (rule: AlertRule) =>
    setConfirmation({
      title: t('Pause alert rule'),
      description: rule.config.name,
      run: () =>
        alertRequest(`rules/${rule.id}/pause`, 'POST', {
          expected_revision: rule.revision,
        }),
    })
  const closeEditor = (owner?: number) => {
    if (
      owner !== undefined &&
      owner !== user?.id &&
      catalog.data?.read_all &&
      props.initialOwnerID === undefined
    ) {
      setAll(true)
      setCursors([])
    }
    setEditingRule(undefined)
    setEditingTarget(undefined)
    void refresh()
  }
  if (catalog.isPending) return <LoadingState />
  if (catalog.isError || !catalog.data) {
    return <ErrorState onRetry={() => void catalog.refetch()} />
  }
  let pageRows: { id: number }[] = []
  let pageLoading = false
  if (tab === 'targets') {
    pageRows = listedTargets.data ?? []
    pageLoading = listedTargets.isFetching
  }
  if (tab === 'events') {
    pageRows = events.data ?? []
    pageLoading = events.isFetching
  }
  if (tab === 'incidents') {
    pageRows = incidents.data ?? []
    pageLoading = incidents.isFetching
  }
  if (tab === 'holds') {
    pageRows = (holds.data ?? []).map((item) => ({ id: item.hold.id }))
    pageLoading = holds.isFetching
  }
  const paged = ['targets', 'events', 'incidents', 'holds'].includes(tab)
  const views = ['rules', 'incidents', 'events', 'targets', 'holds']
  if (catalog.data.read_engine_health) views.push('engine')
  if (catalog.data.manage_platform) views.push('channels', 'migrations')
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Alert center')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {catalog.data.read_all &&
          props.initialOwnerID === undefined &&
          !props.initialChannelID &&
          !props.initialTokenID && (
            <Button
              variant='outline'
              onClick={() => {
                setAll((value) => !value)
                setCursors([])
              }}
            >
              {t(all ? 'All accounts' : 'My alerts')}
            </Button>
          )}
        <Button variant='outline' onClick={() => void refresh()}>
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Monitor usage, channel latency, incidents, and delivery results.'
            )}
          </p>
          {(props.initialOwnerID !== undefined ||
            props.initialChannelID ||
            props.initialTokenID) && (
            <p className='flex items-center gap-3 text-sm'>
              {t('Related alerts')}
              <Link
                className='text-primary underline underline-offset-4'
                to='/alerts'
                search={{}}
                onClick={() => setCursors([])}
              >
                {t('Clear filters')}
              </Link>
            </p>
          )}
          {catalog.data.shadow_mode && (
            <Badge variant='outline'>{t('Shadow mode')}</Badge>
          )}
          {!catalog.data.enabled && (
            <p role='status' className='rounded-lg border p-3 text-sm'>
              {t(
                'Alert collection is disabled. Existing isolation remains active.'
              )}
            </p>
          )}
          {!catalog.data.encryption_configured && (
            <p role='status' className='rounded-lg border p-3 text-sm'>
              {t(
                'Configure the notification encryption key ring before adding destinations.'
              )}
            </p>
          )}
          {paged && tab !== 'holds' && (
            <form
              className='flex gap-2'
              onSubmit={(event) => {
                event.preventDefault()
                setSearch(searchInput.trim())
                setCursors([])
              }}
            >
              <Input
                aria-label={t('Search by name')}
                placeholder={t('Search by name')}
                value={searchInput}
                maxLength={160}
                onChange={(event) => setSearchInput(event.target.value)}
              />
              <Button variant='outline' type='submit'>
                {t('Search')}
              </Button>
            </form>
          )}
          <Tabs
            value={tab}
            onValueChange={(value) => {
              setTab(value)
              setCursors([])
              setSearch('')
              setSearchInput('')
            }}
          >
            <TabsList className='flex h-auto w-full flex-wrap justify-start'>
              {views.map((view) => (
                <TabsTrigger key={view} value={view}>
                  {alertLabel(view, t)}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent value='rules' className='space-y-3'>
              <Button onClick={() => setEditingRule(null)}>
                {t('Create alert rule')}
              </Button>
              <RulesList
                key={listParams.toString()}
                baseParams={listParams.toString()}
                scopes={catalog.data.scopes}
                metrics={['legacy_notice', ...catalog.data.metrics]}
                canManage={canManage}
                onView={setViewingRule}
                onEdit={setEditingRule}
                onPause={handlePause}
                onDelete={(row) =>
                  setConfirmation({
                    title: t('Delete alert rule'),
                    description: t(
                      'Deleting a rule keeps its history and active automatic disables.'
                    ),
                    run: () =>
                      alertRequest(`rules/${row.id}`, 'DELETE', {
                        expected_revision: row.revision,
                      }),
                  })
                }
              />
            </TabsContent>
            <TabsContent value='targets' className='space-y-3'>
              <Button
                disabled={!catalog.data.encryption_configured}
                onClick={() => setEditingTarget(null)}
              >
                {t('Create notification target')}
              </Button>
              {listedTargets.isError && (
                <ErrorState onRetry={() => void listedTargets.refetch()} />
              )}
              <ImportNotification
                disabled={!catalog.data.encryption_configured}
              />
              <TargetsTable
                targets={listedTargets.data ?? []}
                canManage={(row) =>
                  !!catalog.data.manage_platform ||
                  row.owner_user_id === user?.id
                }
                onEdit={setEditingTarget}
                onTest={(row) =>
                  setConfirmation({
                    title: t('Send test notification'),
                    description: row.name,
                    run: () => alertRequest(`targets/${row.id}/test`, 'POST'),
                  })
                }
                onDelete={(row) =>
                  setConfirmation({
                    title: t('Delete notification target'),
                    description: row.name,
                    run: () =>
                      alertRequest(`targets/${row.id}`, 'DELETE', {
                        expected_revision: row.revision,
                      }),
                  })
                }
              />
            </TabsContent>
            <TabsContent value='incidents'>
              {incidents.isError ? (
                <ErrorState onRetry={() => void incidents.refetch()} />
              ) : (
                <StaticDataTable
                  data={incidents.data ?? []}
                  getRowKey={(row) => row.id}
                  columns={[
                    {
                      id: 'target',
                      header: t('Monitored object'),
                      cell: (row) => (
                        <Button
                          size='sm'
                          variant='link'
                          className='h-auto text-left whitespace-normal'
                          onClick={() => setIncidentDetail(row)}
                        >
                          {row.target_name}
                        </Button>
                      ),
                    },
                    {
                      id: 'status',
                      header: t('Status'),
                      cell: (row) => (
                        <>
                          <Badge>{row.severity}</Badge>{' '}
                          {alertLabel(row.status, t)} /{' '}
                          {alertLabel(row.handling_status, t)}
                        </>
                      ),
                    },
                    {
                      id: 'time',
                      header: t('Opened at'),
                      cell: (row) => date(row.opened_at),
                    },
                    {
                      id: 'ack',
                      header: t('Actions'),
                      cell: (row) => (
                        <Button
                          size='sm'
                          variant='outline'
                          disabled={
                            row.handling_status !== 'unacknowledged' ||
                            (!catalog.data?.manage_platform &&
                              !incidentRules.data?.some(
                                (rule) =>
                                  rule.id === row.rule_id && canManage(rule)
                              ))
                          }
                          onClick={() =>
                            setConfirmation({
                              title: t('Acknowledge incident'),
                              description: row.target_name,
                              run: () =>
                                alertRequest(
                                  `incidents/${row.id}/acknowledge`,
                                  'POST'
                                ),
                            })
                          }
                        >
                          {t('Acknowledge')}
                        </Button>
                      ),
                    },
                  ]}
                  emptyContent={t('No incidents')}
                />
              )}
            </TabsContent>
            <TabsContent value='events'>
              {events.isError ? (
                <ErrorState onRetry={() => void events.refetch()} />
              ) : (
                <StaticDataTable
                  data={events.data ?? []}
                  getRowKey={(row) => row.id}
                  columns={[
                    {
                      id: 'summary',
                      header: t('Event'),
                      cell: (row) => (
                        <div className='max-w-xl break-words'>
                          <span className='font-medium'>{row.target_name}</span>
                          <p className='text-muted-foreground text-sm'>
                            {formatAlertEventSummary(row.summary, t, locale)}
                          </p>
                        </div>
                      ),
                    },
                    {
                      id: 'time',
                      header: t('Time'),
                      cell: (row) => date(row.created_at),
                    },
                    {
                      id: 'details',
                      header: t('Details'),
                      cell: (row) => (
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setDetail(row)}
                        >
                          {t('Details')}
                        </Button>
                      ),
                    },
                  ]}
                  emptyContent={t('No alert events')}
                />
              )}
            </TabsContent>
            <TabsContent value='holds'>
              {holds.isError ? (
                <ErrorState onRetry={() => void holds.refetch()} />
              ) : (
                <StaticDataTable
                  data={holds.data ?? []}
                  getRowKey={(row) => row.hold.id}
                  columns={[
                    {
                      id: 'target',
                      header: t('Monitored object'),
                      cell: (row) => <span>{row.hold.target_name}</span>,
                    },
                    {
                      id: 'probe',
                      header: t('Recovery validation'),
                      cell: (row) =>
                        alertLabel(
                          row.hold.last_probe_status || 'Manual recovery',
                          t
                        ),
                    },
                    {
                      id: 'release',
                      header: t('Actions'),
                      cell: (row) => (
                        <Button
                          size='sm'
                          variant='outline'
                          disabled={
                            !catalog.data?.manage_platform &&
                            (row.hold.platform_enforced ||
                              row.hold.owner_user_id !== user?.id)
                          }
                          onClick={() =>
                            setConfirmation({
                              title: t('Release automatic disable'),
                              description: t(
                                'Other automatic disables and manual pauses remain effective.'
                              ),
                              run: async () => {
                                const proof =
                                  await verification.requestVerification({
                                    scope: 'alert.manage',
                                    context: row.verification_context,
                                  })
                                if (!proof) return
                                await alertRequest(
                                  `holds/${row.hold.id}/release`,
                                  'POST',
                                  {},
                                  proof.proof_token
                                )
                              },
                            })
                          }
                        >
                          {t('Release')}
                        </Button>
                      ),
                    },
                  ]}
                  emptyContent={t('No active automatic disable records')}
                />
              )}
            </TabsContent>
            {catalog.data.manage_platform && (
              <TabsContent value='migrations'>
                <MigrationsPanel catalog={catalog.data} />
              </TabsContent>
            )}
            {catalog.data.manage_platform && (
              <TabsContent value='channels'>
                <ChannelControls />
              </TabsContent>
            )}
            {catalog.data.read_engine_health && (
              <TabsContent value='engine'>
                <EnginePanel />
              </TabsContent>
            )}
          </Tabs>
          {paged && (
            <PageFooterPortal>
              <Pagination aria-label={t('Page')}>
                <PaginationContent>
                  <PaginationItem>
                    <PaginationPrevious
                      text={t('Previous')}
                      aria-label={t('Go to previous page')}
                      href='#'
                      aria-disabled={!cursors.length || pageLoading}
                      onClick={(event) => {
                        event.preventDefault()
                        if (cursors.length && !pageLoading) {
                          setCursors((current) => current.slice(0, -1))
                        }
                      }}
                    />
                  </PaginationItem>
                  <PaginationItem>
                    <span className='px-3 text-sm'>{cursors.length + 1}</span>
                  </PaginationItem>
                  <PaginationItem>
                    <PaginationNext
                      text={t('Next')}
                      aria-label={t('Go to next page')}
                      href='#'
                      aria-disabled={pageRows.length < 50 || pageLoading}
                      onClick={(event) => {
                        event.preventDefault()
                        const last = pageRows.at(-1)
                        if (last && pageRows.length === 50 && !pageLoading) {
                          setCursors((current) => [...current, last.id])
                        }
                      }}
                    />
                  </PaginationItem>
                </PaginationContent>
              </Pagination>
            </PageFooterPortal>
          )}

          {viewingRule && (
            <RuleDetailDialog
              rule={viewingRule}
              onClose={() => setViewingRule(undefined)}
            />
          )}
          {editingRule !== undefined && (
            <RuleEditor
              rule={editingRule ?? undefined}
              catalog={catalog.data}
              targets={targets.data ?? []}
              owner={props.initialOwnerID ?? user?.id ?? 0}
              initialChannelID={props.initialChannelID}
              initialTokenID={props.initialTokenID}
              onClose={() => setEditingRule(undefined)}
              onSaved={closeEditor}
            />
          )}
          {editingTarget !== undefined && (
            <TargetEditor
              target={editingTarget ?? undefined}
              catalog={catalog.data}
              onClose={() => setEditingTarget(undefined)}
              onSaved={closeEditor}
            />
          )}
          {activeDetail && (
            <EventDetailDialog
              event={activeDetail}
              canManage={
                !!catalog.data.manage_platform ||
                (activeDetail.rule_id > 0 &&
                  !!detailRule.data &&
                  detailRule.data.id === activeDetail.rule_id &&
                  canManage(detailRule.data))
              }
              onClose={() => {
                setDetail(undefined)
                setDismissedLink(true)
              }}
            />
          )}
          {incidentDetail && (
            <IncidentDetailDialog
              incident={incidentDetail}
              canManage={
                !!catalog.data.manage_platform ||
                (!!detailRule.data &&
                  detailRule.data.id === incidentDetail.rule_id &&
                  canManage(detailRule.data))
              }
              onClose={() => setIncidentDetail(undefined)}
              onEvent={setDetail}
            />
          )}
          <ConfirmDialog
            open={!!confirmation}
            onOpenChange={(open) => {
              if (!open && !operation.isPending) setConfirmation(undefined)
            }}
            title={confirmation?.title}
            desc={confirmation?.description ?? ''}
            isLoading={operation.isPending}
            handleConfirm={() => operation.mutate()}
          />
          <SecureVerificationDialog {...verification.dialogProps} />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
