import { useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { type ReactNode, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { logout, logoutEverywhere, type User } from '../api/client'
import { signedOut } from '../queryClient'
import { isNithronSite } from '../deployment'
import { Icon } from './Icon'
import { NiroMark } from './NiroMark'
import { GuildIcon } from './ui'

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

  const item = 'block w-full px-3 py-2 text-left text-sm hover:bg-hover focus-visible:bg-hover'
  return (
    <div className="min-h-dvh">
      {isNithronSite() && <FamilyBar />}
      <header className="border-b border-border">
        <div className="mx-auto flex h-16 max-w-5xl items-center justify-between gap-4 px-4">
          <Link to="/" aria-label={t('nav.servers')}>
            <ProductName />
          </Link>
          <details ref={menu} className="relative">
            <summary
              className="flex min-h-11 cursor-pointer list-none items-center gap-3 px-2 py-1 hover:bg-hover [&::-webkit-details-marker]:hidden"
              aria-label={t('nav.account')}
            >
              {/* Discord's default picture (no avatar of their own) becomes the member's initials, like a server without an icon. */}
              <GuildIcon name={user.globalName ?? user.username} url={user.avatarUrl?.includes('/embed/avatars/') ? undefined : user.avatarUrl} size={32} />
              <span className="hidden text-sm sm:inline">{user.globalName ?? user.username}</span>
              <Icon name="menu" />
            </summary>
            <div className="absolute right-0 z-20 mt-2 w-56 border border-edge bg-bg p-1">
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
      <footer className="mx-auto flex max-w-5xl gap-4 px-4 pb-8 text-help text-muted">
        <Link to="/terms" className="hover:text-fg">
          {t('legal.terms')}
        </Link>
        <Link to="/privacy" className="hover:text-fg">
          {t('legal.privacy')}
        </Link>
      </footer>
    </div>
  )
}

/** Nithron's family bar: the Nithron sites by name, Niro marked as the current one. Only on Nithron's own deployment. */
export function FamilyBar() {
  const { t } = useTranslation()
  // Brand names: the same in every language.
  const sites = [
    { name: 'Studio', href: 'https://nithron.com' },
    { name: 'Intelligence', href: 'https://ai.nithron.com' },
  ]
  return (
    <nav aria-label={t('nav.nithronSites')} className="border-b border-border">
      <div className="mx-auto flex h-[var(--family-bar)] max-w-5xl items-stretch gap-5 overflow-x-auto px-4 text-[length:var(--text-label)] whitespace-nowrap">
        <span className="flex items-center font-[560]">Nithron</span>
        <ul className="flex items-stretch gap-5">
          {sites.map((s) => (
            <li key={s.name} className="flex">
              <a href={s.href} className="flex items-center text-muted hover:text-fg">
                {s.name}
              </a>
            </li>
          ))}
          <li className="flex">
            <span aria-current="page" className="-mb-px flex items-center border-b-2 border-fg font-[560]">
              Niro
            </span>
          </li>
        </ul>
      </div>
    </nav>
  )
}

/** The product's name at the head of the page, in the pixel face: 24 px in the header, 48 px on sign-in. */
/**
 * The product's name in the pixel face, with the mark before it on Nithron's own deployment. The pixel face's cap is 7
 * of its 8 units and the mark's ink 14 of its 16, so a 32 px name pairs with a 32 px mark and a 48 px one with 48.
 */
export function ProductName({ large = false }: { large?: boolean }) {
  const size = large ? 48 : 32
  return (
    <span className="flex items-center gap-3">
      {/* One mark unit up (2 or 3 px), so the mark's ink and the name's capitals share a top and a bottom. */}
      {isNithronSite() && <NiroMark size={size} className={large ? 'relative -top-[3px]' : 'relative -top-0.5'} />}
      <span className={large ? 'pixel pixel-48 block' : 'pixel pixel-32 block'}>Niro</span>
    </span>
  )
}
