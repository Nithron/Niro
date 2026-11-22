import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { GuildSummary } from '../api/client'
import { useGuilds, useInvite } from '../api/queries'
import { Card, ErrorState, GuildIcon, LinkButton, Spinner } from '../components/ui'

export function ServersPage() {
  const { t } = useTranslation()
  const guilds = useGuilds()

  return (
    <section>
      <h1 className="text-2xl font-semibold">{t('servers.title')}</h1>
      <p className="mt-1 text-muted">{t('servers.subtitle')}</p>
      <div className="mt-6">
        {guilds.isPending && <Spinner />}
        {guilds.isError && <ErrorState onRetry={() => void guilds.refetch()} />}
        {guilds.data?.length === 0 && (
          <Card>
            <p className="text-muted">{t('servers.empty')}</p>
          </Card>
        )}
        {guilds.data && guilds.data.length > 0 && (
          <>
            <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {guilds.data.map((g) => (
                <ServerCard key={g.id} guild={g} />
              ))}
            </ul>
            <p className="mt-6 text-sm text-muted">{t('servers.addedHint')}</p>
          </>
        )}
      </div>
    </section>
  )
}

function ServerCard({ guild }: { guild: GuildSummary }) {
  const { t } = useTranslation()
  return (
    <li>
      <Card className="flex h-full flex-col gap-4">
        <div className="flex items-center gap-3">
          <GuildIcon name={guild.name} url={guild.iconUrl} />
          <div className="min-w-0">
            <p className="truncate font-semibold">{guild.name}</p>
            {guild.owner && <p className="text-xs text-muted">{t('servers.owner')}</p>}
          </div>
        </div>
        <div className="mt-auto">
          {guild.botPresent ? (
            <Link
              to="/guilds/$guildId"
              params={{ guildId: guild.id }}
              className="inline-flex w-full items-center justify-center rounded-lg bg-gradient-to-r from-brand to-brand-2 px-4 py-2 text-sm font-semibold text-on-brand hover:opacity-90"
            >
              {t('servers.manage')}
            </Link>
          ) : (
            <InviteButton guildId={guild.id} />
          )}
        </div>
      </Card>
    </li>
  )
}

export function InviteButton({ guildId }: { guildId: string }) {
  const { t } = useTranslation()
  const invite = useInvite(guildId)
  return (
    <LinkButton
      variant="secondary"
      className="w-full"
      href={invite.data?.url}
      target="_blank"
      rel="noopener noreferrer"
      aria-disabled={!invite.data}
    >
      {t('servers.add')}
    </LinkButton>
  )
}
