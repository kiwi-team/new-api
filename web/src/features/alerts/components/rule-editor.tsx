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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { SYSTEM_TASK_TYPE_LABEL } from '@/features/system-info/constants'

import { alertRequest, getAlertChannels, previewRule, saveRule } from '../api'
import { alertLabel } from '../lib/labels'
import {
  buildRule,
  metricAllowed,
  metricDefaults,
  ruleDefaults,
  ruleFormSchema,
  type RuleFormValues,
} from '../lib/rule-form'
import { ruleTemplates } from '../lib/templates'
import type { AlertRule, AlertTarget, Catalog, Preview } from '../types'
import { PreviewResults } from './rule-preview'
import { ReplayResults, type ReplayPoint } from './rule-replay'

type Props = {
  rule?: AlertRule
  catalog: Catalog
  targets: AlertTarget[]
  owner: number
  initialChannelID?: number
  initialTokenID?: number
  onClose: () => void
  onSaved: (owner?: number) => void
}

export function RuleEditor(props: Props) {
  const { t } = useTranslation()
  const verification = useSecureVerification()
  const defaults = ruleDefaults(props.rule, props.owner)
  if (!props.rule && props.initialChannelID) {
    Object.assign(defaults, {
      scope: 'channel_model',
      owner: 0,
      enforced: true,
      groupMode: 'per_entity',
      entities: [String(props.initialChannelID)],
      allModels: true,
      futureModels: true,
    })
  } else if (!props.rule && props.initialTokenID) {
    Object.assign(defaults, {
      scope: 'api_key',
      entities: [String(props.initialTokenID)],
    })
  }
  const form = useForm<RuleFormValues>({
    resolver: zodResolver(ruleFormSchema),
    defaultValues: defaults,
  })
  const values = form.watch()
  const [preview, setPreview] = useState<Preview | null>(null)
  const [previewInput, setPreviewInput] = useState('')
  // Compare normalized values: resolver output has schema key order, while
  // defaults and imported templates may have a different insertion order.
  const normalizedInput = ruleFormSchema.safeParse(values)
  const inputFingerprint = normalizedInput.success
    ? JSON.stringify(normalizedInput.data)
    : ''
  const previewCurrent = !!inputFingerprint && previewInput === inputFingerprint
  const channels = useQuery({
    queryKey: ['alerts', 'channels'],
    queryFn: getAlertChannels,
    enabled: props.catalog.manage_platform,
  })
  const ownerCatalog = useQuery({
    queryKey: ['alerts', 'catalog', values.owner],
    queryFn: () =>
      alertRequest<Catalog>(`catalog?owner_user_id=${values.owner}`),
    enabled:
      props.catalog.manage_platform &&
      values.owner > 0 &&
      values.owner !== props.owner,
  })
  const ownerTargets = useQuery({
    queryKey: ['alerts', 'targets', 'owner', values.owner],
    queryFn: () =>
      alertRequest<AlertTarget[]>(`targets?owner_user_id=${values.owner}`),
    enabled:
      props.catalog.manage_platform &&
      values.owner > 0 &&
      values.owner !== props.owner,
  })
  const isChannel =
    values.scope === 'channel' || values.scope === 'channel_model'
  const isBudget =
    values.scope === 'uid_budget' || values.scope === 'project_budget'
  const platform = isChannel || values.scope === 'system' || isBudget
  const inflight = values.metric === 'inflight_overdue_count'
  const legacy = !!values.legacySignal
  const metrics = [
    ...props.catalog.metrics,
    ...(values.legacySignal ? ['legacy_notice'] : []),
  ].filter((metric) => metricAllowed(metric, values.scope))
  const ordinaryEntityOptions = isChannel
    ? (channels.data ?? []).map((channel) => ({
        value: String(channel.id),
        label: channel.name,
      }))
    : (ownerCatalog.data?.api_keys ?? props.catalog.api_keys ?? []).map(
        (key) => ({
          value: String(key.id),
          label: key.name,
        })
      )
  const entityOptions = isBudget
    ? (props.catalog.budget_options ?? [])
        .filter((item) => item.type === values.scope)
        .map((item) => ({ value: String(item.id), label: item.name }))
    : ordinaryEntityOptions
  const modelOptions = [
    ...new Set(
      (channels.data ?? [])
        .filter((channel) => values.entities.includes(String(channel.id)))
        .flatMap((channel) => channel.models.split(','))
    ),
  ]
    .filter(Boolean)
    .map((name) => ({ value: name, label: name }))
  const targetOptions = (ownerTargets.data ?? props.targets)
    .filter(
      (target) =>
        target.enabled &&
        (target.owner_user_id === values.owner ||
          (platform && target.owner_user_id === 0))
    )
    .map((target) => ({ value: String(target.id), label: target.name }))
  const previewMutation = useMutation({
    mutationFn: async (input: RuleFormValues) =>
      previewRule(buildRule(input), platform ? 0 : input.owner, props.rule),
    onSuccess: (result, input) => {
      setPreview(result)
      setPreviewInput(JSON.stringify(input))
    },
  })
  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!preview || !previewCurrent) return false
      let proof: string | undefined
      if (preview.config.enabled && preview.config.actions.length) {
        const result = await verification.requestVerification({
          scope: 'alert.manage',
          context: preview.verification_context,
        })
        if (!result) return false
        proof = result.proof_token
      }
      await saveRule(preview, platform ? 0 : values.owner, props.rule, proof)
      return true
    },
    onSuccess: (saved) => {
      if (saved) props.onSaved(platform ? 0 : values.owner)
    },
  })
  const [replayInput, setReplayInput] = useState('')
  const replayMutation = useMutation({
    mutationFn: (input: RuleFormValues) =>
      alertRequest<ReplayPoint[]>('rules/backtest', 'POST', {
        config: buildRule(input),
        owner_user_id: platform ? 0 : input.owner,
        rule_id: props.rule?.id ?? 0,
      }),
    onSuccess: (_, input) => setReplayInput(JSON.stringify(input)),
  })
  const busy =
    previewMutation.isPending ||
    saveMutation.isPending ||
    replayMutation.isPending
  const setMetric = (metric: string) => {
    const defaults = metricDefaults(metric)
    form.setValue('metric', defaults.metric)
    if (
      [
        'spend_net',
        'balance_remaining',
        'budget_remaining_ratio',
        'inflight_overdue_count',
      ].includes(metric)
    ) {
      form.setValue('samples', 0)
    }
    if (metric === 'balance_remaining') {
      form.setValue('action', '')
      form.setValue('autoRecovery', false)
    }
    form.setValue('threshold', defaults.threshold)
    form.setValue('recoveryThreshold', defaults.recoveryThreshold)
    form.setValue('operator', defaults.operator)
  }
  const setScope = (scope: RuleFormValues['scope']) => {
    form.setValue('scope', scope)
    form.setValue('entities', [])
    form.setValue('models', [])
    form.setValue('action', '')
    if (!metricAllowed(values.metric, scope)) {
      setMetric(
        ['uid_budget', 'project_budget'].includes(scope)
          ? 'budget_remaining_ratio'
          : 'duration_ms'
      )
    }
  }
  const select = (
    name:
      | 'balancePool'
      | 'subscription'
      | 'taskType'
      | 'metric'
      | 'operator'
      | 'window'
      | 'severity'
      | 'aggregation'
      | 'deadline'
      | 'trigger'
      | 'groupMode'
      | 'action'
      | 'noAlternative'
      | 'recoveryProtocol'
      | 'protocol'
      | 'stream',
    label: string,
    options: { value: string; label: string }[]
  ) => (
    <div className='space-y-1'>
      <Label htmlFor={`alert-${name}`}>{t(label)}</Label>
      <NativeSelect
        id={`alert-${name}`}
        className='w-full'
        value={values[name]}
        disabled={legacy && name !== 'severity'}
        onChange={(event) => {
          if (name === 'metric') {
            setMetric(event.target.value)
            return
          }
          if (name === 'operator') {
            form.setValue(
              'operator',
              event.target.value as RuleFormValues['operator']
            )
            return
          }
          form.setValue(name, event.target.value)
          if (name === 'trigger' && event.target.value === 'milestone') {
            setMetric('spend_net')
            form.setValue('window', 'cumulative')
          }
          if (name === 'trigger' && event.target.value === 'schedule_report') {
            setMetric('spend_net')
            form.setValue('window', 'day')
            form.setValue('action', '')
          }
        }}
      >
        {options.map((option) => (
          <NativeSelectOption key={option.value} value={option.value}>
            {alertLabel(option.label, t)}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </div>
  )
  const number = (
    name:
      | 'minutes'
      | 'samples'
      | 'hold'
      | 'deadlineSeconds'
      | 'slowSeconds'
      | 'recoveryDuration'
      | 'recoveryTTFT'
      | 'repeat',
    label: string
  ) => (
    <div className='space-y-1'>
      <Label htmlFor={`alert-${name}`}>{t(label)}</Label>
      <Input
        id={`alert-${name}`}
        type='number'
        readOnly={legacy}
        aria-disabled={legacy}
        min={0}
        {...form.register(name, { valueAsNumber: true })}
      />
    </div>
  )
  const toggle = (
    name:
      | 'enabled'
      | 'allOwned'
      | 'includeFuture'
      | 'autoRecovery'
      | 'enforced'
      | 'acknowledge'
      | 'allModels'
      | 'futureModels'
      | 'reportZero'
      | 'reportRevision'
      | 'reportFirstPartial',
    label: string
  ) => (
    <div className='flex items-center gap-2'>
      <Switch
        id={`alert-${name}`}
        checked={values[name]}
        disabled={legacy && name !== 'enabled'}
        onCheckedChange={(checked) => {
          form.setValue(name, checked)
          if (name === 'allModels') {
            form.setValue('models', [])
            if (!checked) form.setValue('futureModels', false)
          }
        }}
      />
      <Label htmlFor={`alert-${name}`}>{t(label)}</Label>
    </div>
  )
  const multi = (
    name: 'entities' | 'targets' | 'models' | 'escalationTargets',
    label: string,
    options: { value: string; label: string }[]
  ) => (
    <div className='space-y-1'>
      <Label htmlFor={`alert-${name}`}>{t(label)}</Label>
      <Controller
        control={form.control}
        name={name}
        render={({ field }) => (
          <MultiSelect
            id={`alert-${name}`}
            options={options}
            selected={field.value}
            onChange={field.onChange}
            placeholder={t('Select')}
          />
        )}
      />
    </div>
  )
  let thresholdLabel = 'Threshold (milliseconds or count)'
  if (['spend_net', 'balance_remaining'].includes(values.metric)) {
    thresholdLabel = 'Threshold (USD)'
  } else if (
    values.metric.endsWith('_rate') ||
    values.metric === 'budget_remaining_ratio'
  ) {
    thresholdLabel = 'Threshold (basis points)'
  }
  const actionOptions = [{ value: '', label: 'Notify only' }]
  if (values.scope === 'api_key' && values.trigger !== 'schedule_report') {
    actionOptions.push({
      value: 'disable_api_key',
      label: 'Disable matched API keys',
    })
  }
  if (
    values.scope === 'account' &&
    values.metric !== 'balance_remaining' &&
    values.trigger !== 'schedule_report'
  ) {
    actionOptions.push({
      value: 'disable_account_keys',
      label: 'Disable all account API keys',
    })
  }
  if (values.scope === 'channel_model') {
    actionOptions.push({
      value: 'disable_channel_model',
      label: 'Isolate matched channel model',
    })
  }
  if (values.scope === 'channel') {
    actionOptions.push({
      value: 'disable_channel',
      label: 'Disable whole channel',
    })
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open && !busy) props.onClose()
        }}
        title={t(props.rule ? 'Edit alert rule' : 'Create alert rule')}
        description={t(
          'Preview verifies data coverage and action scope before saving.'
        )}
        contentClassName='sm:max-w-4xl'
        footer={
          <>
            <Button variant='outline' onClick={props.onClose} disabled={busy}>
              {t('Cancel')}
            </Button>
            {!inflight &&
              !legacy &&
              !isBudget &&
              values.metric !== 'balance_remaining' &&
              values.window !== 'cumulative' &&
              values.scope !== 'system' && (
                <Button
                  variant='outline'
                  disabled={busy}
                  onClick={form.handleSubmit((input) =>
                    replayMutation.mutate(input)
                  )}
                >
                  {t('Replay last hour')}
                </Button>
              )}
            <Button type='submit' form='alert-rule-form' disabled={busy}>
              {t('Preview')}
            </Button>
            <Button
              disabled={!preview || !previewCurrent || busy}
              onClick={() => saveMutation.mutate()}
            >
              {t('Save')}
            </Button>
          </>
        }
      >
        <form
          id='alert-rule-form'
          className='space-y-5'
          onChange={() => {
            setPreview(null)
            replayMutation.reset()
          }}
          onSubmit={form.handleSubmit((input) => previewMutation.mutate(input))}
        >
          {!props.rule && (
            <div className='space-y-1'>
              <Label htmlFor='alert-template'>{t('Rule template')}</Label>
              <NativeSelect
                id='alert-template'
                defaultValue=''
                onChange={(event) => {
                  const template = ruleTemplates.find(
                    (item) => item.name === event.target.value
                  )
                  if (template) {
                    form.reset({
                      ...ruleDefaults(undefined, props.owner),
                      ...template.values,
                      name: t(template.name),
                    })
                    setPreview(null)
                    replayMutation.reset()
                  }
                }}
              >
                <NativeSelectOption value=''>
                  {t('Custom rule')}
                </NativeSelectOption>
                {ruleTemplates
                  .filter(
                    (item) => !item.platform || props.catalog.manage_platform
                  )
                  .map((item) => (
                    <NativeSelectOption key={item.name} value={item.name}>
                      {t(item.name)}
                    </NativeSelectOption>
                  ))}
              </NativeSelect>
            </div>
          )}
          {legacy && (
            <p className='text-muted-foreground text-sm'>
              {t('Legacy trigger retained')}
            </p>
          )}
          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='space-y-1'>
              <Label htmlFor='alert-name'>{t('Name')}</Label>
              <Input
                id='alert-name'
                {...form.register('name')}
                aria-invalid={!!form.formState.errors.name}
              />
            </div>
            <div className='space-y-1'>
              <Label htmlFor='alert-scope'>{t('Monitored object')}</Label>
              <NativeSelect
                id='alert-scope'
                disabled={legacy}
                className='w-full'
                value={values.scope}
                onChange={(event) =>
                  setScope(event.target.value as RuleFormValues['scope'])
                }
              >
                {props.catalog.scopes.map((scope) => (
                  <NativeSelectOption key={scope} value={scope}>
                    {alertLabel(scope, t)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            {props.catalog.manage_platform && !platform && (
              <div className='space-y-1'>
                <Label htmlFor='alert-owner'>{t('Owner user ID')}</Label>
                <Input
                  id='alert-owner'
                  readOnly={legacy}
                  aria-disabled={legacy}
                  type='number'
                  {...form.register('owner', { valueAsNumber: true })}
                />
              </div>
            )}
            {(values.scope === 'api_key' || isChannel || isBudget) &&
              multi(
                'entities',
                isChannel ? 'Channels' : alertLabel(values.scope, t),
                entityOptions
              )}
            {values.scope === 'api_key' && (
              <div className='space-y-2'>
                {toggle('allOwned', 'All owned API keys')}
                {values.allOwned &&
                  toggle('includeFuture', 'Include future API keys')}
                {select('groupMode', 'Grouping', [
                  {
                    value: 'per_entity',
                    label: 'Evaluate each object separately',
                  },
                  { value: 'aggregate', label: 'Combine selected API keys' },
                ])}
              </div>
            )}
            {values.scope === 'channel_model' && (
              <>
                <div className='flex flex-wrap gap-4'>
                  {toggle('allModels', 'All configured models')}
                  {values.allModels &&
                    toggle('futureModels', 'Include future models')}
                </div>
                {!values.allModels && multi('models', 'Models', modelOptions)}
              </>
            )}
            {select(
              'metric',
              'Metric',
              metrics.map((metric) => ({ value: metric, label: metric }))
            )}
            {values.metric === 'balance_remaining' && (
              <>
                {select('balancePool', 'Balance pool', [
                  { value: 'wallet', label: 'Wallet' },
                  { value: 'subscription', label: 'Subscription' },
                ])}
                {values.balancePool === 'subscription' &&
                  select('subscription', 'Subscription', [
                    { value: '', label: 'Select' },
                    ...(
                      ownerCatalog.data?.subscriptions ??
                      props.catalog.subscriptions ??
                      []
                    ).map((subscription) => ({
                      value: String(subscription.id),
                      label: subscription.name,
                    })),
                  ])}
              </>
            )}
            {['task_failure_count', 'task_overdue'].includes(values.metric) &&
              select('taskType', 'Task type', [
                { value: '', label: 'All' },
                ...Object.entries(SYSTEM_TASK_TYPE_LABEL).map(
                  ([value, label]) => ({ value, label })
                ),
              ])}
            {select('operator', 'Operator', [
              { value: 'gt', label: '>' },
              { value: 'gte', label: '≥' },
              { value: 'lt', label: '<' },
              { value: 'lte', label: '≤' },
            ])}
            {select(
              'severity',
              'Severity',
              ['P0', 'P1', 'P2', 'P3'].map((value) => ({ value, label: value }))
            )}
            {inflight
              ? select('deadline', 'Deadline', [
                  { value: 'first_output', label: 'No first output' },
                  { value: 'output_idle', label: 'Output stalled' },
                  { value: 'total', label: 'Total duration' },
                ])
              : select('window', 'Window', [
                  { value: 'rolling', label: 'Rolling window' },
                  { value: 'hour', label: 'Calendar hour' },
                  { value: 'day', label: 'Calendar day' },
                  { value: 'month', label: 'Calendar month' },
                  { value: 'cumulative', label: 'Since activation' },
                ])}
            {inflight || values.metric === 'task_overdue'
              ? number('deadlineSeconds', 'Deadline (seconds)')
              : values.window === 'rolling' &&
                number('minutes', 'Window (minutes)')}
            {!inflight && number('samples', 'Minimum valid samples')}
            {select(
              'aggregation',
              'Aggregation',
              ['mean', 'p95', 'p99'].map((value) => ({ value, label: value }))
            )}
            <div className='space-y-1'>
              <Label htmlFor='alert-threshold'>{t(thresholdLabel)}</Label>
              <Input
                id='alert-threshold'
                readOnly={legacy}
                aria-disabled={legacy}
                inputMode='decimal'
                {...form.register('threshold')}
              />
            </div>
            {!inflight && (
              <div className='space-y-1'>
                <Label htmlFor='alert-recovery-threshold'>
                  {t('Recovery threshold')}
                </Label>
                <Input
                  id='alert-recovery-threshold'
                  readOnly={legacy}
                  aria-disabled={legacy}
                  inputMode='decimal'
                  {...form.register('recoveryThreshold')}
                />
              </div>
            )}
            {values.metric.startsWith('slow_request_') &&
              number('slowSeconds', 'Slow request threshold (seconds)')}
            {number('hold', 'Hold duration (seconds)')}
            {!inflight &&
              select('trigger', 'Trigger', [
                { value: 'incident', label: 'Incident lifecycle' },
                { value: 'once', label: 'Once' },
                { value: 'once_per_period', label: 'Once per calendar period' },
                { value: 'milestone', label: 'Cumulative milestones' },
                { value: 'schedule_report', label: 'Scheduled report' },
              ])}
            {values.trigger === 'milestone' && (
              <div className='space-y-1'>
                <Label htmlFor='alert-step'>{t('Milestone step (USD)')}</Label>
                <Input id='alert-step' {...form.register('stepUSD')} />
              </div>
            )}
            {values.trigger === 'schedule_report' && (
              <div className='space-y-1'>
                <Label htmlFor='alert-send-at'>{t('Report time')}</Label>
                <Input
                  id='alert-send-at'
                  type='time'
                  {...form.register('sendAt')}
                />
              </div>
            )}
            {!inflight && (
              <div className='space-y-1'>
                <Label htmlFor='alert-timezone'>{t('Timezone')}</Label>
                <Input
                  id='alert-timezone'
                  readOnly={legacy}
                  aria-disabled={legacy}
                  {...form.register('timezone')}
                />
              </div>
            )}
            {select('protocol', 'Protocol', [
              { value: '', label: 'All' },
              ...[
                'openai',
                'claude',
                'gemini',
                'openai_responses',
                'task',
                'realtime',
              ].map((value) => ({ value, label: value })),
            ])}
            {select('stream', 'Streaming', [
              { value: '', label: 'All' },
              { value: 'true', label: 'Streaming' },
              { value: 'false', label: 'Non-streaming' },
            ])}
            {multi('targets', 'Notification targets', targetOptions)}
            {multi('escalationTargets', 'Escalation targets', targetOptions)}
            {number('repeat', 'Repeat interval (seconds)')}
          </div>
          <div className='flex flex-wrap gap-4'>
            {toggle('enabled', 'Enabled')}
            {toggle('acknowledge', 'Require acknowledgement')}
            {values.trigger === 'schedule_report' && (
              <>
                {toggle('reportZero', 'Send zero usage reports')}
                {toggle('reportRevision', 'Revise incomplete reports')}
                {toggle('reportFirstPartial', 'Allow first partial period')}
              </>
            )}
            {props.catalog.manage_platform &&
              !platform &&
              toggle('enforced', 'Platform enforced')}
          </div>
          {!inflight && (
            <fieldset
              className='space-y-3 rounded-lg border p-4'
              disabled={legacy}
            >
              <legend className='px-1 text-sm font-medium'>
                {t('Automatic actions')}
              </legend>
              {select('action', 'Action', actionOptions)}
              {!!values.action && (
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Actions affect new requests. Spending can exceed a threshold while requests are in flight.'
                  )}
                </p>
              )}
              {isChannel && !!values.action && (
                <>
                  {select('noAlternative', 'When no alternative is available', [
                    { value: 'notify_only', label: 'Notify only' },
                    {
                      value: 'isolate_anyway',
                      label: 'Isolate even without alternatives',
                    },
                  ])}
                  {toggle('autoRecovery', 'Recover after independent probes')}
                  {values.autoRecovery && (
                    <div className='grid gap-3 sm:grid-cols-2'>
                      {select('recoveryProtocol', 'Probe protocol', [
                        { value: '', label: 'Select' },
                        { value: 'openai', label: 'OpenAI' },
                        { value: 'claude', label: 'Claude' },
                        { value: 'gemini', label: 'Gemini' },
                        {
                          value: 'openai_responses',
                          label: 'OpenAI Responses',
                        },
                      ])}
                      {values.scope === 'channel' &&
                        multi('models', 'Models', modelOptions)}
                      {number(
                        'recoveryDuration',
                        'Probe duration limit (seconds)'
                      )}
                      {number(
                        'recoveryTTFT',
                        'Probe first output limit (seconds)'
                      )}
                      <p className='text-muted-foreground text-sm sm:col-span-2'>
                        {t(
                          'Three qualifying probes over at least two minutes are required. Small probes do not verify full production capacity.'
                        )}
                      </p>
                    </div>
                  )}
                </>
              )}
            </fieldset>
          )}
          {Object.keys(form.formState.errors).length > 0 && (
            <p role='alert' className='text-destructive text-sm'>
              {t('Check the rule fields before continuing.')}
            </p>
          )}
        </form>
        {preview && previewCurrent && <PreviewResults preview={preview} />}
        {replayMutation.data && replayInput === inputFingerprint && (
          <ReplayResults points={replayMutation.data} />
        )}
      </Dialog>
      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}
