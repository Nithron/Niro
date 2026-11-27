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
import { Icon } from './Icon'

// The parts of the dashboard in the Nithron identity (DESIGN-APPS.md in nithron-site): one ground parted by
// hairlines, square panels, controls with 8 px corners, ink for the first action, no hue in the interface,
// and a status as a mark and a word. Colours come only from the names in styles.css.

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

/**
 * A panel of a form: a hairline above, its title, an optional line of help in the fine size, then one column
 * of fields. Panels follow one another down the page, parted by their hairlines.
 */
export function Panel({ title, help, className, children }: { title: string; help?: string; className?: string; children: ReactNode }) {
  return (
    <section className={clsx('grid gap-5 border-t border-border py-6', className)}>
      <div>
        <h3 className="text-panel">{title}</h3>
        {help && <p className="mt-1 max-w-[var(--measure-fine)] text-help text-muted">{help}</p>}
      </div>
      {children}
    </section>
  )
}

/** A panel: square, on the page's ground, parted by a hairline. No fill, no shadow. */
export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={clsx('border border-border p-5', className)}>{children}</div>
}

// ---- status ------------------------------------------------------------------

export type State = 'ok' | 'warning' | 'failed'

/**
 * The mark of a state: the pixel set's state icon at 16 px (ok the filled square, warning the open square,
 * failed the cross), in the state's line colour. The word beside it carries the meaning, so the colour never
 * stands alone.
 */
export function Mark({ state }: { state: State }) {
  const color = { ok: 'text-ok', warning: 'text-warning', failed: 'text-failed' }[state]
  return <Icon name={state} className={color} />
}

const stateText = { ok: 'text-ok-text', warning: 'text-warning-text', failed: 'text-failed-text' }

/** A state as a mark and a word (or a sentence). */
export function Status({ state, className, children }: { state: State; className?: string; children: ReactNode }) {
  return (
    <span className={clsx('inline-flex items-center gap-2', stateText[state], className)}>
      <Mark state={state} />
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
      <span className="mt-0.5">{state ? <Mark state={state} /> : <Icon name="notice" className="text-muted" />}</span>
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

/** Loading: words in the pixel face with the block cursor, where the data will stand. */
export function Spinner() {
  const { t } = useTranslation()
  return (
    <div role="status" className="pixel flex items-center py-16 text-muted">
      {t('app.loading')}
      <span aria-hidden className="cursor ml-1 inline-block h-4 w-2 bg-fg" />
    </div>
  )
}

/** A lamp for something live, such as raid mode running: the set's running mark, blinking, with its word beside it. */
export function Lamp({ state }: { state?: State }) {
  const color = state ? { ok: 'text-ok', warning: 'text-warning', failed: 'text-failed' }[state] : 'text-fg'
  return <Icon name="running" className={clsx('lamp', color)} />
}

/** An empty screen or list: its words in the pixel face, and its one action when it has one. */
export function Empty({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="grid justify-items-start gap-4 border-y border-border py-8">
      <p className="pixel text-muted">{children}</p>
      {action}
    </div>
  )
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
      {over ? (
        <span className="inline-flex items-center gap-2 text-failed-text">
          <Mark state="failed" />
          <span className="pixel">{children}</span>
        </span>
      ) : (
        <span className="pixel text-muted">{children}</span>
      )}
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
