import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, type ModuleState } from '../api/client'
import { useGuild, useModules, useSetModule } from '../api/queries'
import { Card, ErrorState, GuildIcon, Spinner, Switch } from '../components/ui'
import i18n from '../i18n'
import { InviteButton } from './Servers'

export function GuildPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const guild = useGuild(guildId)
  const modules = useModules(guildId, guild.data?.botPresent === true)

  return (
    <section>
      <Link to="/" className="text-sm text-muted hover:text-fg">
        ← {t('guild.back')}
      </Link>

      {guild.isPending && <Spinner />}
      {guild.isError && (
        <div className="mt-6">
          <ErrorState
            message={guild.error instanceof ApiError && guild.error.status === 403 ? t('guild.forbidden') : undefined}
            onRetry={() => void guild.refetch()}
          />
        </div>
      )}

      {guild.data && (
        <>
          <div className="mt-4 flex items-center gap-4">
            <GuildIcon name={guild.data.name} url={guild.data.iconUrl} size={56} />
            <h1 className="text-2xl font-semibold">{guild.data.name}</h1>
          </div>

          {!guild.data.botPresent ? (
            <Card className="mt-8 max-w-md">
              <h2 className="font-semibold">{t('guild.notInServerTitle')}</h2>
              <p className="mt-1 text-muted">{t('guild.notInServerBody')}</p>
              <div className="mt-4">
                <InviteButton guildId={guildId} />
              </div>
            </Card>
          ) : (
            <div className="mt-8">
              <h2 className="text-lg font-semibold">{t('guild.modulesTitle')}</h2>
              <p className="mt-1 text-muted">{t('guild.modulesSubtitle')}</p>
              <div className="mt-4">
                {modules.isPending && <Spinner />}
                {modules.isError && <ErrorState onRetry={() => void modules.refetch()} />}
                {modules.data && (
                  <ul className="grid gap-3">
                    {modules.data.map((m) => (
                      <ModuleRow key={m.id} guildId={guildId} module={m} />
                    ))}
                  </ul>
                )}
              </div>
            </div>
          )}
        </>
      )}
    </section>
  )
}

/** Module names live in the catalog; unknown modules fall back to their ID. */
function moduleText(id: string, field: 'name' | 'description'): string {
  const key = `modules.${id}.${field}`
  return i18n.exists(key) ? i18n.t(key as never) : field === 'name' ? id : ''
}

function ModuleRow({ guildId, module }: { guildId: string; module: ModuleState }) {
  const { t } = useTranslation()
  const setModule = useSetModule(guildId)
  const [status, setStatus] = useState<'idle' | 'saved' | 'failed'>('idle')
  const name = moduleText(module.id, 'name')

  function onChange(enabled: boolean) {
    setStatus('idle')
    setModule.mutate(
      { moduleId: module.id, enabled },
      { onSuccess: () => setStatus('saved'), onError: () => setStatus('failed') },
    )
  }

  return (
    <li>
      <Card className="flex items-start justify-between gap-6">
        <div>
          <p className="font-semibold">{name}</p>
          <p className="mt-1 text-sm text-muted">{moduleText(module.id, 'description')}</p>
          {status === 'saved' && <p className="mt-2 text-xs text-success">{t('guild.saved')}</p>}
          {status === 'failed' && (
            <p role="alert" className="mt-2 text-xs text-danger">
              {t('guild.saveFailed')}
            </p>
          )}
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1">
          <Switch
            checked={module.enabled}
            onCheckedChange={onChange}
            disabled={module.alwaysOn || setModule.isPending}
            label={name}
          />
          {module.alwaysOn && <span className="text-xs text-muted">{t('guild.alwaysOn')}</span>}
        </div>
      </Card>
    </li>
  )
}
