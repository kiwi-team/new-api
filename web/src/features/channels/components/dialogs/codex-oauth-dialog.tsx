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
import { ExternalLink } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import { completeCodexOAuth, startCodexOAuth } from '../../api'

type CodexOAuthDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /**
   * Existing channel to re-authorise. When omitted the flow is for a channel
   * being created, and the resulting key is handed back via `onAuthorized`.
   */
  channelId?: number
  /** Receives the generated key when authorising a not-yet-saved channel. */
  onAuthorized?: (key: string) => void
}

/**
 * Codex authorization: open the provider URL, then paste the redirect back.
 *
 * The redirect target is not reachable by this deployment, so the user copies
 * the resulting URL (or bare code) manually — the backend parses both shapes.
 * The flow is one-shot and expires after 15 minutes server-side.
 */
export function CodexOAuthDialog({
  open,
  onOpenChange,
  channelId,
  onAuthorized,
}: CodexOAuthDialogProps) {
  const { t } = useTranslation()
  const [authorizeUrl, setAuthorizeUrl] = useState('')
  const [input, setInput] = useState('')
  const [isStarting, setIsStarting] = useState(false)
  const [isCompleting, setIsCompleting] = useState(false)

  useEffect(() => {
    if (!open) {
      setAuthorizeUrl('')
      setInput('')
    }
  }, [open])

  const handleStart = async () => {
    setIsStarting(true)
    try {
      const res = await startCodexOAuth(channelId)
      if (!res.success || !res.data?.authorize_url) {
        toast.error(res.message || t('Failed to start authorization'))
        return
      }
      setAuthorizeUrl(res.data.authorize_url)
      window.open(res.data.authorize_url, '_blank', 'noopener,noreferrer')
    } finally {
      setIsStarting(false)
    }
  }

  const handleComplete = async () => {
    const trimmed = input.trim()
    if (!trimmed) {
      toast.warning(t('Paste the redirect URL or authorization code'))
      return
    }
    setIsCompleting(true)
    try {
      const res = await completeCodexOAuth(trimmed, channelId)
      if (!res.success) {
        toast.error(res.message || t('Authorization failed'))
        return
      }
      const email = res.data?.email
      toast.success(
        email
          ? t('Authorized as {{email}}', { email })
          : t('Authorization succeeded')
      )
      if (res.data?.key) onAuthorized?.(res.data.key)
      onOpenChange(false)
    } finally {
      setIsCompleting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Codex authorization')}
      description={
        channelId
          ? t('Re-authorize this channel; the new key is saved automatically')
          : t('Authorize a new Codex account and fill in the key')
      }
      contentClassName='sm:max-w-lg'
      contentHeight='auto'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => void handleComplete()}
            disabled={isCompleting || !authorizeUrl}
          >
            {t('Complete authorization')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='space-y-2'>
          <div className='flex items-center gap-2'>
            <span className='bg-muted flex size-5 shrink-0 items-center justify-center rounded-full text-xs font-medium'>
              1
            </span>
            <span className='text-sm font-medium'>
              {t('Open the authorization page')}
            </span>
          </div>
          <Button
            variant='outline'
            size='sm'
            className='w-full'
            onClick={() => void handleStart()}
            disabled={isStarting}
          >
            <ExternalLink className='h-4 w-4' />
            {authorizeUrl
              ? t('Reopen authorization page')
              : t('Start authorization')}
          </Button>
          {authorizeUrl && (
            <div className='flex items-start gap-1'>
              <p className='text-muted-foreground min-w-0 flex-1 text-xs break-all'>
                {authorizeUrl}
              </p>
              <CopyButton value={authorizeUrl} className='h-5 w-5 shrink-0' />
            </div>
          )}
        </div>

        <div className='space-y-2'>
          <div className='flex items-center gap-2'>
            <span className='bg-muted flex size-5 shrink-0 items-center justify-center rounded-full text-xs font-medium'>
              2
            </span>
            <Label htmlFor='codex-oauth-input' className='text-sm font-medium'>
              {t('Paste the result back')}
            </Label>
          </div>
          <Textarea
            id='codex-oauth-input'
            rows={4}
            className='font-mono text-xs'
            placeholder='http://localhost:1455/auth/callback?code=...&state=...'
            value={input}
            onChange={(event) => setInput(event.target.value)}
            disabled={!authorizeUrl}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'After approving, the browser lands on an unreachable address — copy that full URL here. The link expires in 15 minutes.'
            )}
          </p>
        </div>
      </div>
    </Dialog>
  )
}
