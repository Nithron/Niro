import * as SwitchPrimitive from '@radix-ui/react-switch'
import { clsx } from 'clsx'
import type { AnchorHTMLAttributes, ButtonHTMLAttributes, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import lockupDark from '../assets/nithro-lockup-dark.svg'
import lockupLight from '../assets/nithro-lockup-light.svg'

/** The Nithro lockup, switching variant with the colour scheme. */
export function Logo({ className }: { className?: string }) {
  return (
    <span className={clsx('inline-block', className)}>
      <img src={lockupDark} alt="Nithro" className="hidden h-full dark:block" />
      <img src={lockupLight} alt="Nithro" className="h-full dark:hidden" />
    </span>
  )
}

const buttonBase =
  'inline-flex items-center justify-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold transition-colors disabled:opacity-50 disabled:pointer-events-none'
const variants = {
  primary: 'bg-gradient-to-r from-brand to-brand-2 text-on-brand hover:opacity-90',
  secondary: 'border border-border bg-surface-2 text-fg hover:bg-surface',
  ghost: 'text-muted hover:text-fg',
}
type Variant = keyof typeof variants

export function Button({
  variant = 'primary',
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return <button className={clsx(buttonBase, variants[variant], className)} {...props} />
}

export function LinkButton({
  variant = 'primary',
  className,
  ...props
}: AnchorHTMLAttributes<HTMLAnchorElement> & { variant?: Variant }) {
  return <a className={clsx(buttonBase, variants[variant], className)} {...props} />
}

export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={clsx('rounded-xl border border-border bg-surface p-5', className)}>{children}</div>
}

export function Switch({
  checked,
  onCheckedChange,
  disabled,
  label,
}: {
  checked: boolean
  onCheckedChange: (v: boolean) => void
  disabled?: boolean
  label: string
}) {
  return (
    <SwitchPrimitive.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={label}
      className="relative h-6 w-11 shrink-0 rounded-full bg-border transition-colors data-[state=checked]:bg-gradient-to-r data-[state=checked]:from-brand data-[state=checked]:to-brand-2 disabled:cursor-not-allowed disabled:opacity-60"
    >
      <SwitchPrimitive.Thumb className="block size-5 translate-x-0.5 rounded-full bg-white shadow transition-transform data-[state=checked]:translate-x-[22px]" />
    </SwitchPrimitive.Root>
  )
}

/** Server icon, or its initials when it has none. */
export function GuildIcon({ name, url, size = 48 }: { name: string; url?: string; size?: number }) {
  const style = { width: size, height: size }
  if (url) return <img src={url} alt="" style={style} className="rounded-xl" />
  const initials = name
    .split(/\s+/)
    .map((w) => w[0])
    .join('')
    .slice(0, 3)
  return (
    <span
      style={style}
      aria-hidden
      className="inline-flex items-center justify-center rounded-xl bg-surface-2 text-sm font-semibold text-muted"
    >
      {initials}
    </span>
  )
}

export function Spinner() {
  const { t } = useTranslation()
  return (
    <div role="status" className="flex items-center justify-center gap-3 py-16 text-muted">
      <span className="size-5 animate-spin rounded-full border-2 border-border border-t-brand" />
      {t('app.loading')}
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message?: string; onRetry?: () => void }) {
  const { t } = useTranslation()
  return (
    <Card className="text-center">
      <p className="text-danger">{message ?? t('app.genericError')}</p>
      {onRetry && (
        <Button variant="secondary" className="mt-4" onClick={onRetry}>
          {t('app.retry')}
        </Button>
      )}
    </Card>
  )
}
