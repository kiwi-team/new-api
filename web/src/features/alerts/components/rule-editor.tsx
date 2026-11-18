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
import { useState, type ReactNode } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { SYSTEM_TASK_TYPE_LABEL } from '@/features/system-info/constants'
import { UserSelect } from '@/features/users/components/user-select'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { alertRequest, getAlertChannels, previewRule, saveRule } from '../api'
import { useTargetOptions } from '../hooks/use-target-options'
import { alertLabel } from '../lib/labels'
import {
  buildRule,
  canUseRuleTarget,
  isPercentageMetric,
  isPlatformScope,
  metricAllowed,
  metricDefaults,
  ruleDefaults,
  ruleFormSchema,
  type RuleFormValues,
} from '../lib/rule-form'
import { ruleTemplates } from '../lib/templates'
import type { AlertRule, AlertTarget, Catalog, Preview } from '../types'
import { RuleFieldHelp } from './rule-field-help'
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
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
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
  const isChannel =
    values.scope === 'channel' || values.scope === 'channel_model'
  const isBudget =
    values.scope === 'uid_budget' || values.scope === 'project_budget'
  const platform = isPlatformScope(values.scope)
  const ruleOwner = platform ? 0 : values.owner
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
      !platform &&
      values.owner > 0 &&
      values.owner !== props.owner,
  })
  const notificationOptions = useTargetOptions(props.catalog.manage_platform)
  const escalationOptions = useTargetOptions(props.catalog.manage_platform)
  const [chosenTargets, setChosenTargets] = useState<AlertTarget[]>([])
  const selectedIDs = [
    ...new Set([...values.targets, ...values.escalationTargets]),
  ]
    .map(Number)
    .sort((a, b) => a - b)
  const selectedTargets = useQuery({
    queryKey: ['alerts', 'targets', 'selected', selectedIDs],
    queryFn: async () => {
      const requests = []
      for (let offset = 0; offset < selectedIDs.length; offset += 20) {
        const ids = selectedIDs.slice(offset, offset + 20)
        requests.push(
          alertRequest<AlertTarget[] | null>(
            `targets?scope=all&ids=${ids.join(',')}`
          )
        )
      }
      return (await Promise.all(requests)).flatMap((targets) => targets ?? [])
    },
    enabled: props.catalog.manage_platform && selectedIDs.length > 0,
    meta: { errorToast: false },
  })
  const anotherOwner =
    props.catalog.manage_platform &&
    !platform &&
    values.owner > 0 &&
    values.owner !== props.owner
  const catalogForOwner = anotherOwner ? ownerCatalog.data : props.catalog
  const targetsByID = new Map(
    [
      ...props.targets,
      ...(notificationOptions.query.data?.pages.flat() ?? []),
      ...(escalationOptions.query.data?.pages.flat() ?? []),
      ...(selectedTargets.data ?? []),
      ...chosenTargets,
    ].map((target) => [target.id, target])
  )
  const inflight = values.metric === 'inflight_overdue_count'
  const percentage = isPercentageMetric(values.metric)
  const volumeChange = values.metric === 'request_volume_change'
  const traffic = ['request_started_count', 'request_volume_change'].includes(
    values.metric
  )
  const percentageMaximum =
    volumeChange && values.changeDirection === 'increase' ? 1000000 : 100
  const usesAggregation = ['duration_ms', 'ttft_ms'].includes(values.metric)
  const thresholdError = form.formState.errors.threshold
  const recoveryThresholdError = form.formState.errors.recoveryThreshold
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
    : (catalogForOwner?.api_keys ?? []).map((key) => ({
        value: String(key.id),
        label: key.name,
      }))
  const entityOptions = isBudget
    ? (props.catalog.budget_options ?? [])
        .filter((item) => item.type === values.scope)
        .map((item) => ({ value: String(item.id), label: item.name }))
    : ordinaryEntityOptions
  const modelOptions = [
    ...new Set(
      (channels.data ?? [])
        .filter(
          (channel) =>
            values.scope === 'model' ||
            values.entities.includes(String(channel.id))
        )
        .flatMap((channel) => channel.models.split(','))
    ),
  ]
    .filter(Boolean)
    .map((name) => ({ value: name, label: name }))
  const targetOptions = [...targetsByID.values()]
    .filter(
      (target) =>
        (target.enabled || !values.enabled) &&
        canUseRuleTarget(target, values, props.catalog.manage_platform)
    )
    .map((target) => ({
      value: String(target.id),
      label: target.enabled ? target.name : `${target.name} (${t('Disabled')})`,
      hint: props.catalog.manage_platform
        ? `${t('Owner user')}: #${target.owner_user_id} · ID: ${target.id}`
        : undefined,
    }))
  const validateNotificationTargets = (input: RuleFormValues) => {
    let valid = true
    for (const field of ['targets', 'escalationTargets'] as const) {
      let message = ''
      for (const id of input[field]) {
        const target = targetsByID.get(Number(id))
        if (
          !target ||
          !canUseRuleTarget(target, input, props.catalog.manage_platform)
        ) {
          message = 'Select notification targets available to this rule.'
          break
        }
        if (input.enabled && !target.enabled) {
          message =
            'Enable the selected notification target before enabling this rule.'
          break
        }
      }
      if (message) {
        form.setError(field, { type: 'validate', message })
        valid = false
      } else {
        form.clearErrors(field)
      }
    }
    return valid
  }
  const previewMutation = useMutation({
    mutationFn: async (input: RuleFormValues) =>
      previewRule(buildRule(input), ruleOwner, props.rule),
    onSuccess: (result, input) => {
      setPreview(result)
      setPreviewInput(JSON.stringify(input))
    },
  })
  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!preview || !previewCurrent) return false
      if (!validateNotificationTargets(values)) return false
      let proof: string | undefined
      if (preview.config.enabled && preview.config.actions?.length) {
        const result = await verification.requestVerification({
          scope: 'alert.manage',
          context: preview.verification_context,
        })
        if (!result) return false
        proof = result.proof_token
      }
      await saveRule(preview, ruleOwner, props.rule, proof)
      return true
    },
    onSuccess: (saved) => {
      if (saved) props.onSaved(ruleOwner)
    },
  })
  const [replayInput, setReplayInput] = useState('')
  const replayMutation = useMutation({
    mutationFn: (input: RuleFormValues) =>
      alertRequest<ReplayPoint[]>('rules/backtest', 'POST', {
        config: buildRule(input),
        owner_user_id: ruleOwner,
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
        'request_count',
        'request_started_count',
        'request_volume_change',
      ].includes(metric)
    ) {
      form.setValue('samples', 0)
    }
    if (['request_started_count', 'request_volume_change'].includes(metric)) {
      form.setValue('trigger', 'incident')
      form.setValue('window', 'rolling')
      form.setValue('action', '')
      form.setValue('autoRecovery', false)
    }
    if (metric === 'request_volume_change') form.setValue('recoveryHold', 120)
    if (metric === 'balance_remaining') {
      form.setValue('action', '')
      form.setValue('autoRecovery', false)
    }
    form.setValue('threshold', defaults.threshold)
    form.setValue('recoveryThreshold', defaults.recoveryThreshold)
    form.setValue('operator', defaults.operator)
  }
  const setScope = (scope: RuleFormValues['scope']) => {
    if (isPlatformScope(scope) !== platform) {
      form.setValue('targets', [])
      form.setValue('escalationTargets', [])
    }
    if (!isPlatformScope(scope) && values.owner === 0) {
      form.setValue('owner', props.owner)
    }
    if (scope === 'model') {
      form.setValue('allModels', false)
      form.setValue('futureModels', false)
    }
    form.setValue('scope', scope)
    form.setValue('entities', [])
    form.setValue('models', [])
    form.setValue('recoveryModels', [])
    form.setValue('action', '')
    if (!metricAllowed(values.metric, scope)) {
      let metric = 'duration_ms'
      if (scope === 'model') metric = 'request_started_count'
      if (['uid_budget', 'project_budget'].includes(scope)) {
        metric = 'budget_remaining_ratio'
      }
      setMetric(metric)
    }
  }
  const select = (
    name:
      | 'baselineMode'
      | 'changeDirection'
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
      <div className='flex items-center gap-1.5'>
        <Label htmlFor={`alert-${name}`}>{t(label)}</Label>
        <RuleFieldHelp
          field={name}
          label={label}
          metric={values.metric}
          direction={values.changeDirection}
          mode={values.trigger}
        />
      </div>
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
      | 'baselineCount'
      | 'baselineMinutes'
      | 'baselineMinimumCount'
      | 'minutes'
      | 'samples'
      | 'hold'
      | 'deadlineSeconds'
      | 'slowSeconds'
      | 'recoveryHold'
      | 'recoveryDuration'
      | 'recoveryTTFT'
      | 'repeat',
    label: string
  ) => (
    <div className='space-y-1'>
      <div className='flex items-center gap-1.5'>
        <Label htmlFor={`alert-${name}`}>{t(label)}</Label>
        <RuleFieldHelp
          field={name}
          label={label}
          metric={values.metric}
          direction={values.changeDirection}
          mode={values.trigger}
        />
      </div>
      <Input
        aria-invalid={!!form.formState.errors[name]}
        aria-describedby={
          form.formState.errors[name] ? `alert-${name}-error` : undefined
        }
        id={`alert-${name}`}
        type='number'
        readOnly={legacy}
        aria-disabled={legacy}
        min={0}
        {...form.register(name, { valueAsNumber: true })}
      />
      {form.formState.errors[name] && (
        <FieldError id={`alert-${name}-error`}>
          {form.formState.errors[name]?.type === 'custom'
            ? t(form.formState.errors[name]?.message || 'Enter a valid number.')
            : t('Enter a valid number.')}
        </FieldError>
      )}
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
          if (name === 'enforced' && !checked && !platform) {
            for (const field of ['targets', 'escalationTargets'] as const) {
              form.setValue(
                field,
                values[field].filter(
                  (id) =>
                    targetsByID.get(Number(id))?.owner_user_id === values.owner
                )
              )
            }
          }
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
    name:
      | 'entities'
      | 'targets'
      | 'models'
      | 'recoveryModels'
      | 'escalationTargets',
    label: string,
    options: { value: string; label: string; hint?: string }[]
  ) => {
    let source: ReturnType<typeof useTargetOptions> | undefined
    if (props.catalog.manage_platform && name === 'targets') {
      source = notificationOptions
    }
    if (props.catalog.manage_platform && name === 'escalationTargets') {
      source = escalationOptions
    }
    let emptyText: string | undefined
    let footer: ReactNode
    if (source) {
      const query = source.query
      if (source.pendingSearch || source.query.isPending) {
        emptyText = t('Loading...')
      }
      if (source.query.isError) {
        emptyText = t('Failed to load notification targets')
        footer = (
          <Button
            type='button'
            variant='ghost'
            className='w-full'
            onClick={() => void query.refetch()}
          >
            {t('Retry')}
          </Button>
        )
      } else if (source.query.hasNextPage) {
        footer = (
          <Button
            type='button'
            variant='ghost'
            className='w-full'
            disabled={source.query.isFetchingNextPage || source.pendingSearch}
            onClick={() => void query.fetchNextPage()}
          >
            {source.query.isFetchingNextPage
              ? t('Loading...')
              : t('Load more notification targets')}
          </Button>
        )
      }
    }
    return (
      <Controller
        control={form.control}
        name={name}
        render={({ field, fieldState }) => (
          <Field className='gap-1' data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor={`alert-${name}`}>{t(label)}</FieldLabel>
            <MultiSelect
              id={`alert-${name}`}
              aria-invalid={fieldState.invalid}
              aria-describedby={
                fieldState.error ? `alert-${name}-error` : undefined
              }
              options={options}
              selected={field.value}
              onChange={(ids) => {
                if (name === 'targets' || name === 'escalationTargets') {
                  const other =
                    name === 'targets'
                      ? values.escalationTargets
                      : values.targets
                  const chosen = [...new Set([...ids, ...other])].flatMap(
                    (id) => {
                      const target = targetsByID.get(Number(id))
                      return target ? [target] : []
                    }
                  )
                  setChosenTargets(chosen)
                  if (
                    props.catalog.manage_platform &&
                    chosen.some((target) => target.owner_user_id !== ruleOwner)
                  ) {
                    form.setValue('enforced', true)
                  }
                }
                field.onChange(ids)
              }}
              onSearchChange={source?.setSearch}
              emptyText={emptyText}
              footer={footer}
              placeholder={t('Select')}
            />
            {fieldState.error?.message && (
              <FieldError id={`alert-${name}-error`}>
                {t(fieldState.error.message)}
              </FieldError>
            )}
          </Field>
        )}
      />
    )
  }
  let thresholdLabel = 'Threshold (milliseconds or count)'
  if (['spend_net', 'balance_remaining'].includes(values.metric)) {
    thresholdLabel = 'Threshold (USD)'
  } else if (percentage) {
    thresholdLabel = volumeChange ? 'Change threshold (%)' : 'Threshold (%)'
  }
  let recoveryThresholdLabel = 'Recovery threshold'
  if (percentage) recoveryThresholdLabel = 'Recovery threshold (%)'
  if (volumeChange) recoveryThresholdLabel = 'Recovery change threshold (%)'
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
                  onClick={form.handleSubmit((input) => {
                    if (validateNotificationTargets(input)) {
                      replayMutation.mutate(input)
                    }
                  })}
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
          noValidate
          className='space-y-5'
          onChange={() => {
            setPreview(null)
            replayMutation.reset()
          }}
          onSubmit={form.handleSubmit((input) => {
            if (validateNotificationTargets(input)) {
              previewMutation.mutate(input)
            }
          })}
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
                  form.reset({
                    ...ruleDefaults(undefined, props.owner),
                    ...template?.values,
                    name: template ? t(template.name) : '',
                  })
                  setPreview(null)
                  replayMutation.reset()
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
                <Label htmlFor='alert-owner'>{t('Owner user')}</Label>
                <Controller
                  control={form.control}
                  name='owner'
                  render={({ field, fieldState }) => (
                    <UserSelect
                      id='alert-owner'
                      label={t('Owner user')}
                      value={field.value}
                      disabled={legacy}
                      invalid={fieldState.invalid}
                      onBlur={field.onBlur}
                      onValueChange={(owner) => {
                        if (owner === field.value) return
                        field.onChange(owner)
                        form.setValue('entities', [])
                        form.setValue('targets', [])
                        form.setValue('escalationTargets', [])
                        form.setValue('subscription', '')
                        setPreview(null)
                        replayMutation.reset()
                      }}
                    />
                  )}
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
            {values.scope === 'model' &&
              multi('models', 'Models', modelOptions)}
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
              'severity',
              'Severity',
              ['P0', 'P1', 'P2', 'P3'].map((value) => ({ value, label: value }))
            )}
            <FieldSet className='min-w-0 gap-4 rounded-lg border p-4 sm:col-span-2'>
              <FieldLegend variant='label'>{t('Alert condition')}</FieldLegend>
              <FieldGroup className='grid grid-cols-[minmax(0,0.7fr)_minmax(0,1fr)] items-end gap-3 sm:grid-cols-[minmax(0,1.2fr)_minmax(0,0.7fr)_minmax(0,1fr)]'>
                <div className='col-span-2 sm:col-span-1'>
                  {select(
                    'metric',
                    'Metric',
                    metrics.map((metric) => ({ value: metric, label: metric }))
                  )}
                </div>
                {select('operator', 'Comparison operator', [
                  { value: 'gt', label: '>' },
                  { value: 'gte', label: '≥' },
                  ...(!volumeChange
                    ? [
                        { value: 'lt', label: '<' },
                        { value: 'lte', label: '≤' },
                      ]
                    : []),
                ])}
                <Field className='gap-1' data-invalid={!!thresholdError}>
                  <div className='flex items-center gap-1.5'>
                    <FieldLabel htmlFor='alert-threshold'>
                      {t(thresholdLabel)}
                    </FieldLabel>
                    <RuleFieldHelp
                      field='threshold'
                      label={thresholdLabel}
                      metric={values.metric}
                      direction={values.changeDirection}
                      mode={values.trigger}
                    />
                  </div>
                  {percentage ? (
                    <InputGroup>
                      <InputGroupInput
                        id='alert-threshold'
                        type='number'
                        min={0}
                        max={percentageMaximum}
                        step={0.01}
                        inputMode='decimal'
                        readOnly={legacy}
                        aria-disabled={legacy}
                        aria-invalid={!!thresholdError}
                        aria-describedby={
                          thresholdError ? 'alert-threshold-error' : undefined
                        }
                        {...form.register('threshold')}
                      />
                      <InputGroupAddon align='inline-end' aria-hidden='true'>
                        %
                      </InputGroupAddon>
                    </InputGroup>
                  ) : (
                    <Input
                      id='alert-threshold'
                      readOnly={legacy}
                      aria-disabled={legacy}
                      aria-invalid={!!thresholdError}
                      inputMode='decimal'
                      {...form.register('threshold')}
                    />
                  )}
                  {percentage && thresholdError && (
                    <FieldError id='alert-threshold-error'>
                      {t(
                        thresholdError.message ||
                          'Enter a percentage from 0 to 100 with up to two decimal places.'
                      )}
                    </FieldError>
                  )}
                </Field>
              </FieldGroup>
              {volumeChange && (
                <FieldGroup className='grid gap-4 sm:grid-cols-2'>
                  {select('changeDirection', 'Change direction', [
                    { value: 'decrease', label: 'Decrease' },
                    { value: 'increase', label: 'Increase' },
                  ])}
                  {select('baselineMode', 'Comparison baseline', [
                    { value: 'historical', label: 'Historical average' },
                    { value: 'manual', label: 'Manual baseline' },
                  ])}
                  {values.baselineMode === 'manual' ? (
                    number('baselineCount', 'Baseline calls per window')
                  ) : (
                    <>
                      {number('baselineMinutes', 'Baseline history (minutes)')}
                      {number(
                        'baselineMinimumCount',
                        'Minimum baseline calls per window'
                      )}
                    </>
                  )}
                </FieldGroup>
              )}
              <FieldGroup className='grid gap-4 sm:grid-cols-2'>
                {values.metric === 'balance_remaining' && (
                  <>
                    {select('balancePool', 'Balance pool', [
                      { value: 'wallet', label: 'Wallet' },
                      { value: 'subscription', label: 'Subscription' },
                    ])}
                    {values.balancePool === 'subscription' &&
                      select('subscription', 'Subscription', [
                        { value: '', label: 'Select' },
                        ...(catalogForOwner?.subscriptions ?? []).map(
                          (subscription) => ({
                            value: String(subscription.id),
                            label: subscription.name,
                          })
                        ),
                      ])}
                  </>
                )}
                {['task_failure_count', 'task_overdue'].includes(
                  values.metric
                ) &&
                  select('taskType', 'Task type', [
                    { value: '', label: 'All' },
                    ...Object.entries(SYSTEM_TASK_TYPE_LABEL).map(
                      ([value, label]) => ({ value, label })
                    ),
                  ])}
                {values.metric.startsWith('slow_request_') &&
                  number('slowSeconds', 'Slow request threshold (seconds)')}
                {number('hold', 'Hold duration (seconds)')}
                {volumeChange &&
                  number('recoveryHold', 'Recovery hold (seconds)')}
                {!inflight && (
                  <Field
                    className='gap-1'
                    data-invalid={!!recoveryThresholdError}
                  >
                    <div className='flex items-center gap-1.5'>
                      <FieldLabel htmlFor='alert-recovery-threshold'>
                        {t(recoveryThresholdLabel)}
                      </FieldLabel>
                      <RuleFieldHelp
                        field='recoveryThreshold'
                        label={recoveryThresholdLabel}
                        metric={values.metric}
                        direction={values.changeDirection}
                        mode={values.trigger}
                      />
                    </div>
                    {percentage ? (
                      <InputGroup>
                        <InputGroupInput
                          id='alert-recovery-threshold'
                          type='number'
                          min={0}
                          max={percentageMaximum}
                          step={0.01}
                          inputMode='decimal'
                          readOnly={legacy}
                          aria-disabled={legacy}
                          aria-invalid={!!recoveryThresholdError}
                          aria-describedby={
                            recoveryThresholdError
                              ? 'alert-recovery-threshold-error'
                              : undefined
                          }
                          {...form.register('recoveryThreshold')}
                        />
                        <InputGroupAddon align='inline-end' aria-hidden='true'>
                          %
                        </InputGroupAddon>
                      </InputGroup>
                    ) : (
                      <Input
                        id='alert-recovery-threshold'
                        readOnly={legacy}
                        aria-disabled={legacy}
                        aria-invalid={!!recoveryThresholdError}
                        inputMode='decimal'
                        {...form.register('recoveryThreshold')}
                      />
                    )}
                    {percentage && recoveryThresholdError && (
                      <FieldError id='alert-recovery-threshold-error'>
                        {recoveryThresholdError.type === 'custom' &&
                        recoveryThresholdError.message
                          ? t(recoveryThresholdError.message)
                          : t(
                              'Enter a percentage from 0 to 100 with up to two decimal places.'
                            )}
                      </FieldError>
                    )}
                  </Field>
                )}
              </FieldGroup>
            </FieldSet>
            <FieldSet className='min-w-0 gap-4 rounded-lg border p-4 sm:col-span-2'>
              <FieldLegend variant='label'>
                {t('Calculation settings')}
              </FieldLegend>
              <FieldGroup className='grid gap-4 sm:grid-cols-2'>
                {traffic && (
                  <p className='text-muted-foreground text-sm sm:col-span-2'>
                    {t(
                      'Model traffic counts gateway requests; channel traffic counts upstream attempts, including retries.'
                    )}
                  </p>
                )}
                {values.scope === 'model' && (
                  <p className='text-muted-foreground text-sm sm:col-span-2'>
                    {t(
                      'Global model names match the model requested by the client.'
                    )}
                  </p>
                )}
                {inflight
                  ? select('deadline', 'Deadline', [
                      { value: 'first_output', label: 'No first output' },
                      { value: 'output_idle', label: 'Output stalled' },
                      { value: 'total', label: 'Total duration' },
                    ])
                  : select('window', 'Window', [
                      { value: 'rolling', label: 'Rolling window' },
                      ...(!traffic
                        ? [
                            { value: 'hour', label: 'Calendar hour' },
                            { value: 'day', label: 'Calendar day' },
                            { value: 'month', label: 'Calendar month' },
                            { value: 'cumulative', label: 'Since activation' },
                          ]
                        : []),
                    ])}
                {inflight || values.metric === 'task_overdue'
                  ? number('deadlineSeconds', 'Deadline (seconds)')
                  : values.window === 'rolling' &&
                    number('minutes', 'Window (minutes)')}
                {usesAggregation &&
                  select(
                    'aggregation',
                    'Aggregation',
                    ['mean', 'p95', 'p99'].map((value) => ({
                      value,
                      label: value,
                    }))
                  )}
                {!inflight &&
                  !traffic &&
                  values.metric !== 'request_count' &&
                  number('samples', 'Minimum valid samples')}
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
              </FieldGroup>
            </FieldSet>
            {!inflight &&
              !traffic &&
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
            {multi('targets', 'Notification targets', targetOptions)}
            {platform && (
              <p className='text-muted-foreground text-sm sm:col-span-2'>
                {t('Root can select notification targets from any user.')}
              </p>
            )}
            {multi('escalationTargets', 'Escalation targets', targetOptions)}
            {props.catalog.manage_platform && !platform && values.enforced && (
              <p className='text-muted-foreground text-sm sm:col-span-2'>
                {t(
                  'Rules using another user’s notification targets are platform enforced.'
                )}
              </p>
            )}
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
          {!inflight && !traffic && (
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
                        multi('recoveryModels', 'Models', modelOptions)}
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
                          'At least {{count}} qualifying probes over {{seconds}} seconds are required. Small probes do not verify full production capacity.',
                          {
                            count: formatNumber(
                              Math.max(3, values.recoverySuccesses),
                              locale
                            ),
                            seconds: formatNumber(
                              Math.max(120, values.recoveryObserveSeconds),
                              locale
                            ),
                          }
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
          <ReplayResults points={replayMutation.data} metric={values.metric} />
        )}
      </Dialog>
      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}
