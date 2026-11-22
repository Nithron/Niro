import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { logout, type User } from '../api/client'
import { keys } from '../api/queries'
import { Button, Logo } from './ui'

export function Layout({ user, children }: { user: User; children: ReactNode }) {
  const { t } = useTranslation()
  const qc = useQueryClient()

  async function onLogout() {
    await logout()
    qc.clear()
    await qc.invalidateQueries({ queryKey: keys.me })
  }

  return (
    <div className="min-h-dvh">
      <header className="border-b border-border bg-surface/60 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-5xl items-center justify-between gap-4 px-4">
          <Link to="/" aria-label={t('nav.servers')}>
            <Logo className="h-7" />
          </Link>
          <div className="flex items-center gap-3">
            <img src={user.avatarUrl} alt="" className="size-8 rounded-full" />
            <span className="hidden text-sm sm:inline">{user.globalName ?? user.username}</span>
            <Button variant="ghost" onClick={onLogout}>
              {t('nav.logout')}
            </Button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-8">{children}</main>
    </div>
  )
}
