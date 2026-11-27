import { Link, Outlet } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, type ModuleState } from '../api/client'
import { useGuild, useModules, useSetModule } from '../api/queries'
import { DeleteServerData } from '../components/DeleteServerData'
import { Icon } from '../components/Icon'
import { ErrorState, GuildIcon, Spinner, Status, Switch } from '../components/ui'
import i18n from '../i18n'
import { InviteButton } from './Servers'

const tabs = [
  { to: '/guilds/$guildId', key: 'tabs.modules', exact: true },
  { to: '/guilds/$guildId/moderation', key: 'tabs.moderation', exact: false },
  { to: '/guilds/$guildId/cases', key: 'tabs.cases', exact: false },
  { to: '/guilds/$guildId/automod', key: 'tabs.automod', exact: false },
  { to: '/guilds/$guildId/guardian', key: 'tabs.guardian', exact: true },
  { to: '/guilds/$guildId/guardian/detections', key: 'tabs.detections', exact: false },
  { to: '/guilds/$guildId/guardian/audit', key: 'tabs.audit', exact: false },
  { to: '/guilds/$guildId/logging', key: 'tabs.logging', exact: false },
  { to: '/guilds/$guildId/roles', key: 'tabs.roles', exact: false },
  { to: '/guilds/$guildId/welcome', key: 'tabs.welcome', exact: false },
  { to: '/guilds/$guildId/tickets', key: 'tabs.tickets', exact: false },
] as const

/** Server header, tabs, and the active tab (or an invite if Niro isn't there). */
export function GuildLayout({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const guild = useGuild(guildId)

  return (
    <section>
      <Link to="/" className="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
        <Icon name="back" />
        {t('guild.back')}
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
            <GuildIcon name={guild.data.name} url={guild.data.iconUrl} size={32} />
            <h1 className="text-screen">{guild.data.name}</h1>
          </div>

          {!guild.data.botPresent ? (
            <>
            <div className="mt-8 border-y border-border py-6">
              <h2 className="text-panel">{t('guild.notInServerTitle')}</h2>
              <p className="mt-1 text-muted">{t('guild.notInServerBody')}</p>
              <div className="mt-4">
                <InviteButton guildId={guildId} />
              </div>
            </div>
            {guild.data.owner && <DeleteServerData guildId={guildId} guildName={guild.data.name} />}
            </>
          ) : (
            <>
              <nav className="mt-6 flex gap-1 overflow-x-auto border-b border-border" aria-label={guild.data.name}>
                {tabs.map((tab) => (
                  <Link
                    key={tab.to}
                    to={tab.to}
                    params={{ guildId }}
                    activeOptions={{ exact: tab.exact }}
                    className="group -mb-px border-b-2 border-transparent px-4 py-2 text-sm whitespace-nowrap text-muted hover:text-fg data-[status=active]:border-fg data-[status=active]:text-fg"
                  >
                    {/* A hidden copy at the heavier weight holds the width, so the row does not shift when the current tab changes. */}
                    <span className="inline-grid justify-items-center">
                      <span aria-hidden className="invisible col-start-1 row-start-1 font-[560]">
                        {t(tab.key)}
                      </span>
                      <span className="col-start-1 row-start-1 group-data-[status=active]:font-[560]">{t(tab.key)}</span>
                    </span>
                  </Link>
                ))}
              </nav>
              <div className="mt-6">
                <Outlet />
              </div>
            </>
          )}
        </>
      )}
    </section>
  )
}

export function ModulesPage({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const modules = useModules(guildId, true)
  const guild = useGuild(guildId)
  return (
    <div>
      <h2 className="text-panel">{t('guild.modulesTitle')}</h2>
      <p className="mt-1 text-muted">{t('guild.modulesSubtitle')}</p>
      <div className="mt-4">
        {modules.isPending && <Spinner />}
        {modules.isError && <ErrorState onRetry={() => void modules.refetch()} />}
        {modules.data && (
          <>
          <p className="mb-4 flex items-baseline gap-3">
              <span className="pixel pixel-32">{modules.data.filter((m) => m.enabled).length}</span>
              <span className="text-muted">{t('guild.modulesOn', { total: modules.data.length })}</span>
            </p>
          <ul className="divide-y divide-border border-y border-border">
            {modules.data.map((m) => (
              <ModuleRow key={m.id} guildId={guildId} module={m} />
            ))}
          </ul>
          </>
        )}
      </div>
      {guild.data?.owner && <DeleteServerData guildId={guildId} guildName={guild.data.name} />}
    </div>
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
    <li className="flex items-start justify-between gap-6 py-4">
        <div>
          <p className="font-[560]">{name}</p>
          <p className="mt-1 text-sm text-muted">{moduleText(module.id, 'description')}</p>
          {status === 'saved' && (
            <p className="mt-2 text-xs">
              <Status state="ok">{t('guild.saved')}</Status>
            </p>
          )}
          {status === 'failed' && (
            <p role="alert" className="mt-2 text-xs">
              <Status state="failed">{t('guild.saveFailed')}</Status>
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
          {module.alwaysOn && <span className="text-help text-muted">{t('guild.alwaysOn')}</span>}
        </div>
    </li>
  )
}
