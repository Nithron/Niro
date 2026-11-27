import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { GuildSummary } from '../api/client'
import { useGuilds, useInvite } from '../api/queries'
import { buttonClasses, ErrorState, GuildIcon, LinkButton, Spinner } from '../components/ui'

export function ServersPage() {
  const { t } = useTranslation()
  const guilds = useGuilds()

  return (
    <section>
      <h1 className="text-screen">{t('servers.title')}</h1>
      <p className="mt-1 text-muted">{t('servers.subtitle')}</p>
      <div className="mt-6">
        {guilds.isPending && <Spinner />}
        {guilds.isError && <ErrorState onRetry={() => void guilds.refetch()} />}
        {guilds.data?.length === 0 && <p className="border-y border-border py-6 text-muted">{t('servers.empty')}</p>}
        {guilds.data && guilds.data.length > 0 && (
          <>
            <ul className="divide-y divide-border border-y border-border">
              {guilds.data.map((g) => (
                <ServerRow key={g.id} guild={g} />
              ))}
            </ul>
            <p className="mt-6 text-sm text-muted">{t('servers.addedHint')}</p>
          </>
        )}
      </div>
    </section>
  )
}

/** One server per row: its icon (data), its name, a word for Niro's state, the action at the end. */
function ServerRow({ guild }: { guild: GuildSummary }) {
  const { t } = useTranslation()
  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 py-3">
      <GuildIcon name={guild.name} url={guild.iconUrl} size={32} />
      <div className="min-w-0 flex-1">
        <p className="truncate font-[560]">{guild.name}</p>
        <p className="text-help text-muted">
          {guild.botPresent ? t('servers.present') : t('servers.notAdded')}
          {guild.owner && <> · {t('servers.owner')}</>}
        </p>
      </div>
      {guild.botPresent ? (
        <Link to="/guilds/$guildId" params={{ guildId: guild.id }} className={buttonClasses('line', 'compact')}>
          {t('servers.manage')}
        </Link>
      ) : (
        <InviteButton guildId={guild.id} compact />
      )}
    </li>
  )
}

export function InviteButton({ guildId, compact = false }: { guildId: string; compact?: boolean }) {
  const { t } = useTranslation()
  const invite = useInvite(guildId)
  return (
    <LinkButton
      variant="line"
      size={compact ? 'compact' : 'default'}
      href={invite.data?.url}
      target="_blank"
      rel="noopener noreferrer"
      aria-disabled={!invite.data}
    >
      {t('servers.add')}
    </LinkButton>
  )
}
