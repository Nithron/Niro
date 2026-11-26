import { useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { type ReactNode, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { logout, logoutEverywhere, type User } from '../api/client'
import { signedOut } from '../queryClient'
import { Logo } from './ui'

export function Layout({ user, children }: { user: User; children: ReactNode }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const menu = useRef<HTMLDetailsElement>(null)

  // Close the account menu on a click outside it or on Escape.
  useEffect(() => {
    const close = (e: Event) => {
      const el = menu.current
      if (!el?.open) return
      if (e instanceof KeyboardEvent ? e.key === 'Escape' : !el.contains(e.target as Node)) el.open = false
    }
    document.addEventListener('click', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('click', close)
      document.removeEventListener('keydown', close)
    }
  }, [])

  async function signOut(everywhere: boolean) {
    await (everywhere ? logoutEverywhere() : logout())
    // Refetching the user returns 401, so the root route shows the login screen.
    await navigate({ to: '/' })
    await signedOut(qc)
  }

  const item = 'block w-full rounded-md px-3 py-2 text-left text-sm hover:bg-border/40 focus-visible:bg-border/40'
  return (
    <div className="min-h-dvh">
      <header className="border-b border-border bg-surface/60 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-5xl items-center justify-between gap-4 px-4">
          <Link to="/" aria-label={t('nav.servers')}>
            <Logo className="h-7" />
          </Link>
          <details ref={menu} className="relative">
            <summary
              className="flex cursor-pointer list-none items-center gap-3 rounded-md px-2 py-1 hover:bg-border/40 [&::-webkit-details-marker]:hidden"
              aria-label={t('nav.account')}
            >
              <img src={user.avatarUrl} alt="" className="size-8 rounded-full" />
              <span className="hidden text-sm sm:inline">{user.globalName ?? user.username}</span>
            </summary>
            <div className="absolute right-0 z-20 mt-2 w-56 rounded-lg border border-border bg-surface p-1 shadow-lg">
              <button type="button" className={item} onClick={() => void signOut(false)}>
                {t('nav.logout')}
              </button>
              <button type="button" className={item} onClick={() => void signOut(true)}>
                {t('nav.logoutEverywhere')}
              </button>
            </div>
          </details>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-8">{children}</main>
    </div>
  )
}
