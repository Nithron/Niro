import * as SwitchPrimitive from '@radix-ui/react-switch'
import { clsx } from 'clsx'
import type {
  AnchorHTMLAttributes,
  ComponentProps,
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import lockupDark from '../assets/niro-lockup-dark.svg'
import lockupLight from '../assets/niro-lockup-light.svg'

/** The Niro lockup, switching variant with the colour scheme. */
export function Logo({ className }: { className?: string }) {
  return (
    <span className={clsx('inline-block', className)}>
      <img src={lockupDark} alt="Niro" className="hidden h-full dark:block" />
      <img src={lockupLight} alt="Niro" className="h-full dark:hidden" />
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
}: ComponentProps<'button'> & { variant?: Variant }) {
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

// ---- form controls ------------------------------------------------------------

const control =
  'w-full rounded-lg border border-border bg-surface-2 px-3 py-2 text-sm text-fg placeholder:text-muted focus:border-brand focus:outline-none disabled:opacity-60'

export function Field({ label, help, htmlFor, children }: { label: string; help?: string; htmlFor?: string; children: ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <label htmlFor={htmlFor} className="text-sm font-medium">
        {label}
      </label>
      {children}
      {help && <p className="text-xs text-muted">{help}</p>}
    </div>
  )
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={clsx(control, props.className)} />
}

export function TextInput(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={clsx(control, props.className)} />
}

export function TextArea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea rows={4} {...props} className={clsx(control, 'font-mono', props.className)} />
}

/** A labelled switch row used in settings forms. */
export function SwitchRow({
  label,
  help,
  checked,
  onCheckedChange,
}: {
  label: string
  help?: string
  checked: boolean
  onCheckedChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-start justify-between gap-6">
      <div>
        <p className="text-sm font-medium">{label}</p>
        {help && <p className="mt-1 text-xs text-muted">{help}</p>}
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} label={label} />
    </div>
  )
}

const badgeTones = {
  neutral: 'bg-surface-2 text-muted',
  brand: 'bg-brand/15 text-brand',
  success: 'bg-success/15 text-success',
  warning: 'bg-warning/15 text-warning',
  danger: 'bg-danger/15 text-danger',
}

export function Badge({ tone = 'neutral', children }: { tone?: keyof typeof badgeTones; children: ReactNode }) {
  return <span className={clsx('inline-flex items-center rounded-md px-2 py-0.5 text-xs font-semibold', badgeTones[tone])}>{children}</span>
}

/** Inline error for a failed request; shows the API's message when it has one. */
export function RequestError({ error }: { error: unknown }) {
  const { t } = useTranslation()
  if (!error) return null
  let message = t('app.genericError')
  if (error instanceof ApiError) {
    if (error.code === 'bot_unavailable') message = t('errors.botUnavailable')
    else if (error.code === 'rate_limited') message = t('errors.rateLimited')
    else message = error.message || message
  }
  return (
    <p role="alert" className="text-sm text-danger">
      {message}
    </p>
  )
}
