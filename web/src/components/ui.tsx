import * as SwitchPrimitive from '@radix-ui/react-switch'
import { clsx } from 'clsx'
import {
  type AnchorHTMLAttributes,
  type ComponentProps,
  type InputHTMLAttributes,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'

// The parts of the dashboard in the Nithron identity (DESIGN-APPS.md in nithron-site): one ground parted by
// hairlines, square panels, controls with 8 px corners, ink for the first action, no hue in the interface,
// and a status as a mark and a word. Colours come only from the names in styles.css.

/** The Niro lockup, the <N> mark and the name, drawn in the text colour. */
export function Logo({ className }: { className?: string }) {
  return (
    <svg role="img" aria-label="Niro" viewBox="0 0 972 216" className={clsx('inline-block w-auto', className)}>
      <g fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round">
        <svg x="0" y="0" width="400" height="216" viewBox="56 148 400 216">
          <g strokeWidth="44">
            <path d="M144 180 L88 256 L144 332" />
            <path d="M368 180 L424 256 L368 332" />
            <path d="M214 336 V176 L298 336 V176" />
          </g>
        </svg>
        <svg x="460" y="8" width="512" height="200" viewBox="0 0 512 200">
          <g strokeWidth="40">
            <path d="M20 160 V40 L100 160 V40" />
            <path d="M172 160 V40" />
            <path d="M244 160 V40 H284 A30 30 0 0 1 284 100 H244 M282 100 L329 160" />
            <rect x="401" y="40" width="90" height="120" rx="45" />
          </g>
        </svg>
      </g>
    </svg>
  )
}

// ---- buttons ------------------------------------------------------------------

const buttonBase =
  'inline-flex items-center justify-center gap-2 rounded-ctl text-sm font-[560] transition-colors duration-150 disabled:pointer-events-none disabled:opacity-50'
const sizes = {
  /** 44 px, a form's action. */
  default: 'min-h-11 px-4 py-2',
  /** 32 px, inside a row or a toolbar. */
  compact: 'min-h-8 px-3 py-1',
}
const variants = {
  /** The first action of a form or a screen: an ink fill. One per form. */
  primary: 'bg-fg text-bg hover:bg-fg/85',
  /** Every other action, destructive ones too: a line, never a fill. */
  line: 'border border-edge text-fg hover:bg-hover',
  /** A quiet action in words. */
  ghost: 'text-muted hover:bg-hover hover:text-fg',
}
type Variant = keyof typeof variants
type Size = keyof typeof sizes

export function Button({
  variant = 'primary',
  size = 'default',
  className,
  ...props
}: ComponentProps<'button'> & { variant?: Variant; size?: Size }) {
  return <button className={clsx(buttonBase, sizes[size], variants[variant], className)} {...props} />
}

export function LinkButton({
  variant = 'primary',
  size = 'default',
  className,
  ...props
}: AnchorHTMLAttributes<HTMLAnchorElement> & { variant?: Variant; size?: Size }) {
  return <a className={clsx(buttonBase, sizes[size], variants[variant], className)} {...props} />
}

/** The classes of a button, for a router link that has to look like one. */
export function buttonClasses(variant: Variant = 'primary', size: Size = 'default') {
  return clsx(buttonBase, sizes[size], variants[variant])
}

// ---- surfaces -----------------------------------------------------------------

/** A panel: square, on the page's ground, parted by a hairline. No fill, no shadow. */
export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={clsx('border border-border p-5', className)}>{children}</div>
}

// ---- status ------------------------------------------------------------------

export type State = 'ok' | 'warning' | 'failed'

/**
 * The mark of a state, a pixel shape on a 5 by 5 grid shown at 10 px: a filled square for ok, an open
 * square for a warning, a cross for a failure. Its colour is the state's line colour; the word beside it
 * carries the meaning, so the colour never stands alone.
 */
export function Mark({ state }: { state: State }) {
  const color = { ok: 'text-ok', warning: 'text-warning', failed: 'text-failed' }[state]
  return (
    <svg aria-hidden viewBox="0 0 5 5" width="10" height="10" shapeRendering="crispEdges" className={clsx('shrink-0', color)} fill="currentColor">
      {state === 'ok' && <rect width="5" height="5" />}
      {state === 'warning' && <path fillRule="evenodd" d="M0 0H5V5H0ZM1 1V4H4V1Z" />}
      {state === 'failed' && <path d="M0 0H1V1H0ZM1 1H2V2H1ZM2 2H3V3H2ZM3 3H4V4H3ZM4 4H5V5H4ZM4 0H5V1H4ZM3 1H4V2H3ZM1 3H2V4H1ZM0 4H1V5H0Z" />}
    </svg>
  )
}

const stateText = { ok: 'text-ok-text', warning: 'text-warning-text', failed: 'text-failed-text' }

/** A state as a mark and a word (or a sentence). */
export function Status({ state, className, children }: { state: State; className?: string; children: ReactNode }) {
  return (
    <span className={clsx('inline-flex items-baseline gap-2', stateText[state], className)}>
      <span className="self-center">
        <Mark state={state} />
      </span>
      <span>{children}</span>
    </span>
  )
}

const badgeText = { neutral: 'text-muted', ok: 'text-ok-text', warning: 'text-warning-text', failed: 'text-failed-text' }

/**
 * A short status beside a thing: a mark and a word. "neutral" is a word with no mark and no colour, for
 * anything that is not a state (a case's action, a rule's trigger, a closed ticket).
 */
export function Badge({ tone = 'neutral', children }: { tone?: State | 'neutral'; children: ReactNode }) {
  return (
    <span className={clsx('inline-flex items-center gap-1.5 text-xs font-[560] whitespace-nowrap', badgeText[tone])}>
      {tone !== 'neutral' && <Mark state={tone} />}
      {children}
    </span>
  )
}

/**
 * A notice in a hairline frame. A notice that is no state (a hint, a module that is off) has no hue: ink
 * words and a mark of its own. A notice about a state takes that state's mark, its words stay ink.
 */
export function Notice({ state, className, children }: { state?: State; className?: string; children: ReactNode }) {
  return (
    <div className={clsx('flex items-start gap-3 border border-border p-3 text-sm text-fg', className)}>
      <span aria-hidden className="mt-0.5 inline-flex h-4 w-4 shrink-0 items-center justify-center font-mono text-muted">
        {state ? <Mark state={state} /> : 'i'}
      </span>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

// ---- controls -----------------------------------------------------------------

/**
 * A switch: a 1 px edge track; on is an ink fill with the knob at the far end. Its label says what is on.
 */
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
      className="relative inline-flex h-6 w-11 shrink-0 items-center border border-edge bg-bg transition-colors duration-150 data-[state=checked]:border-fg data-[state=checked]:bg-fg disabled:cursor-not-allowed disabled:opacity-50"
    >
      <SwitchPrimitive.Thumb className="block size-4 translate-x-[3px] bg-edge transition-transform duration-150 data-[state=checked]:translate-x-[23px] data-[state=checked]:bg-bg" />
    </SwitchPrimitive.Root>
  )
}

/** Server icon (data from Discord), small and square, or its initials when it has none. */
export function GuildIcon({ name, url, size = 48 }: { name: string; url?: string; size?: number }) {
  const style = { width: size, height: size }
  if (url) return <img src={url} alt="" style={style} />
  const initials = name
    .split(/\s+/)
    .map((w) => w[0])
    .join('')
    .slice(0, 3)
  return (
    <span
      style={style}
      aria-hidden
      className="inline-flex items-center justify-center border border-border font-mono text-sm text-muted"
    >
      {initials}
    </span>
  )
}

/** Loading: words in the pixel display face (mono until it arrives) with the block cursor, where the data will stand. */
export function Spinner() {
  const { t } = useTranslation()
  return (
    <div role="status" className="pixel-display flex items-center py-16 text-sm text-muted">
      {t('app.loading')}
      <span aria-hidden className="cursor ml-1 inline-block h-4 w-2 bg-fg" />
    </div>
  )
}

/** A lamp for something live, such as raid mode running: an 8 px square that blinks, with its word beside it. */
export function Lamp({ state }: { state?: State }) {
  const color = state ? { ok: 'bg-ok', warning: 'bg-warning', failed: 'bg-failed' }[state] : 'bg-fg'
  return <span aria-hidden className={clsx('lamp inline-block size-2 shrink-0', color)} />
}

/**
 * A meter: a row of square blocks on whole pixels that fill one by one, never a smooth bar, with the number
 * beside it in the mono face. Over the limit every block is filled and the number takes the failure's mark.
 */
export function BlockMeter({ value, max, label, children, blocks = 10 }: { value: number; max: number; label: string; children: ReactNode; blocks?: number }) {
  const over = value > max
  const filled = over ? blocks : Math.min(blocks, Math.ceil((Math.max(0, value) / Math.max(1, max)) * blocks))
  return (
    <span className="inline-flex items-center gap-2">
      <span
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={max}
        aria-valuenow={value}
        className="pixelated inline-flex gap-0.5"
      >
        {Array.from({ length: blocks }, (_, i) => (
          <span
            key={i}
            className={clsx('inline-block size-2 border', i < filled ? (over ? 'border-failed bg-failed' : 'border-fg bg-fg') : 'border-edge')}
          />
        ))}
      </span>
      {over ? <Status state="failed">{children}</Status> : <span className="font-mono text-help text-muted">{children}</span>}
    </span>
  )
}

/** A failure at the place it happened: a cross, the words, what to do. */
export function ErrorState({ message, onRetry }: { message?: string; onRetry?: () => void }) {
  const { t } = useTranslation()
  return (
    <div role="alert" className="flex flex-wrap items-center gap-4 border-y border-border py-4">
      <Status state="failed">{message ?? t('app.genericError')}</Status>
      {onRetry && (
        <Button variant="line" size="compact" onClick={onRetry}>
          {t('app.retry')}
        </Button>
      )}
    </div>
  )
}

// ---- form controls ------------------------------------------------------------

const control =
  'w-full rounded-ctl border border-edge bg-bg px-3 text-sm text-fg placeholder:text-dim aria-[invalid=true]:border-failed disabled:opacity-50'

export function Field({ label, help, htmlFor, children }: { label: string; help?: string; htmlFor?: string; children: ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <label htmlFor={htmlFor} className="text-sm font-[560]">
        {label}
      </label>
      {children}
      {help && <p className="text-help text-muted">{help}</p>}
    </div>
  )
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={clsx(control, 'h-11', props.className)} />
}

export function TextInput(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={clsx(control, 'h-11', props.className)} />
}

export function TextArea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea rows={4} {...props} className={clsx(control, 'min-h-11 py-2 font-mono', props.className)} />
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
        <p className="text-sm font-[560]">{label}</p>
        {help && <p className="mt-1 text-help text-muted">{help}</p>}
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} label={label} />
    </div>
  )
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
    <p role="alert" className="text-sm">
      <Status state="failed">{message}</Status>
    </p>
  )
}

// ---- confirm dialog ------------------------------------------------------------

export type ConfirmOptions = {
  /** The subject, first and short ("Start raid mode"). Defaults to the action's words. */
  title?: string
  /** The sentence that says the consequence. */
  message: string
  /** The words of the confirming button, saying what is destroyed or started ("Delete case #12"). */
  action: string
}

type Pending = ConfirmOptions & { resolve: (ok: boolean) => void }

/**
 * Niro's own confirm dialog, in place of the browser's. Returns a function that opens the dialog and
 * resolves true or false, and the element to render while it is open:
 *
 *   const [confirm, confirmDialog] = useConfirm()
 *   if (await confirm({ message, action })) remove()
 *   ...
 *   {confirmDialog}
 */
export function useConfirm() {
  const [pending, setPending] = useState<Pending | null>(null)
  const confirm = useCallback(
    (opts: ConfirmOptions) => new Promise<boolean>((resolve) => setPending({ ...opts, resolve })),
    [],
  )
  const dialog = pending ? (
    <ConfirmDialog
      title={pending.title ?? pending.action}
      message={pending.message}
      action={pending.action}
      onClose={(ok) => {
        pending.resolve(ok)
        setPending(null)
      }}
    />
  ) : null
  return [confirm, dialog] as const
}

function ConfirmDialog({ title, message, action, onClose }: Required<Omit<ConfirmOptions, 'title'>> & { title: string; onClose: (ok: boolean) => void }) {
  const { t } = useTranslation()
  const panel = useRef<HTMLDivElement>(null)
  const cancel = useRef<HTMLButtonElement>(null)
  const titleId = useId()
  const messageId = useId()

  useEffect(() => {
    // Focus the safe choice; give focus back to what opened the dialog when it closes.
    const opener = document.activeElement as HTMLElement | null
    cancel.current?.focus()
    return () => opener?.focus?.()
  }, [])

  function onKeyDown(e: ReactKeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose(false)
      return
    }
    if (e.key !== 'Tab') return
    // Keep focus inside the dialog.
    const items = panel.current?.querySelectorAll<HTMLElement>('button:not([disabled])')
    if (!items || items.length === 0) return
    const first = items[0]!
    const last = items[items.length - 1]!
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/70 p-4" onMouseDown={(e) => e.target === e.currentTarget && onClose(false)}>
      <div
        ref={panel}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={messageId}
        onKeyDown={onKeyDown}
        className="w-full max-w-md border border-edge bg-bg p-6"
      >
        <h2 id={titleId} className="text-panel">
          {title}
        </h2>
        <p id={messageId} className="mt-2 text-sm text-muted">
          {message}
        </p>
        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <Button ref={cancel} variant="ghost" onClick={() => onClose(false)}>
            {t('common.cancel')}
          </Button>
          <Button variant="line" onClick={() => onClose(true)}>
            {action}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
