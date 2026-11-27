import { useNavigate } from '@tanstack/react-router'
import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useDeleteGuildData } from '../api/queries'
import { Button, Card, Field, RequestError, TextInput } from './ui'

/**
 * The owner's "delete everything now" control. The server's name must be
 * typed to confirm, since this cannot be undone.
 */
export function DeleteServerData({ guildId, guildName }: { guildId: string; guildName: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const del = useDeleteGuildData(guildId)
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState('')
  const inputId = useId()
  const bodyId = useId()
  const trigger = useRef<HTMLButtonElement>(null)

  function cancel() {
    setOpen(false)
    setTyped('')
    // Return focus to the button that opened the form.
    requestAnimationFrame(() => trigger.current?.focus())
  }

  return (
    <Card className="mt-10">
      <h2 className="font-semibold">{t('deleteData.title')}</h2>
      <p id={bodyId} className="mt-1 max-w-prose text-sm text-muted">
        {t('deleteData.body')}
      </p>
      {!open ? (
        <Button ref={trigger} variant="line" className="mt-4" onClick={() => setOpen(true)}>
          {t('deleteData.start')}
        </Button>
      ) : (
        <form
          className="mt-4 grid max-w-md gap-3"
          onSubmit={(e) => {
            e.preventDefault()
            del.mutate(undefined, { onSuccess: () => void navigate({ to: '/' }) })
          }}
        >
          <Field label={t('deleteData.confirmLabel', { name: guildName })} htmlFor={inputId}>
            <TextInput
              id={inputId}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoComplete="off"
              autoFocus
            />
          </Field>
          <RequestError error={del.error} />
          <div className="flex gap-2">
            <Button
              type="submit"
              aria-describedby={bodyId}
              variant="line"
              disabled={typed.trim() !== guildName.trim() || del.isPending}
            >
              {del.isPending ? t('deleteData.deleting') : t('deleteData.confirm')}
            </Button>
            <Button type="button" variant="ghost" onClick={cancel} disabled={del.isPending}>
              {t('common.cancel')}
            </Button>
          </div>
        </form>
      )}
    </Card>
  )
}
