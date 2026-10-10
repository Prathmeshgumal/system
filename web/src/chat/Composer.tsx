import { useState, type KeyboardEvent } from 'react'
import type { Message } from '../lib/types'
import { primaryButton } from '../ui/buttons'

const MAX = 4000

type Props = {
  onSend: (body: string) => void
  /** Called on every keystroke; the caller throttles it into typing events. */
  onTyping?: () => void
  replyTo?: Message | null
  onCancelReply?: () => void
}

/** Enter sends, Shift+Enter adds a new line. */
export function Composer({ onSend, onTyping, replyTo, onCancelReply }: Props) {
  const [text, setText] = useState('')
  const canSend = text.trim().length > 0 && text.length <= MAX

  function send() {
    if (!canSend) return
    onSend(text)
    setText('')
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Escape' && replyTo) onCancelReply?.()
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      send()
    }
  }

  return (
    <div className="border-t border-zinc-200 p-3 dark:border-zinc-800">
      {replyTo && (
        <div className="mb-2 flex items-start gap-2 rounded-lg border-l-4 border-sky-500 bg-zinc-50 px-3 py-1.5 text-sm dark:bg-zinc-900">
          <div className="min-w-0 flex-1">
            <span className="block text-xs font-semibold text-sky-700 dark:text-sky-400">
              Replying to {replyTo.sender?.display_name ?? 'Deleted account'}
            </span>
            <span className="block truncate text-zinc-600 dark:text-zinc-400">{replyTo.body}</span>
          </div>
          <button onClick={onCancelReply} aria-label="Cancel reply" className="text-zinc-500">
            ✕
          </button>
        </div>
      )}
      <div className="flex items-end gap-2">
        <textarea
          value={text}
          onChange={(e) => {
            setText(e.target.value)
            if (e.target.value) onTyping?.()
          }}
          onKeyDown={onKeyDown}
          rows={1}
          placeholder="Message"
          aria-label="Message"
          className="max-h-40 min-h-10 flex-1 resize-none rounded-xl border border-zinc-300 bg-white px-3 py-2 outline-none focus:border-sky-500 dark:border-zinc-700 dark:bg-zinc-900"
        />
        <button onClick={send} disabled={!canSend} className={primaryButton}>
          Send
        </button>
      </div>
    </div>
  )
}
