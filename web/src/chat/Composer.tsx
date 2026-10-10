import { useState, type KeyboardEvent } from 'react'
import { primaryButton } from '../ui/buttons'

const MAX = 4000

/** Enter sends, Shift+Enter adds a new line. */
export function Composer({ onSend }: { onSend: (body: string) => void }) {
  const [text, setText] = useState('')
  const canSend = text.trim().length > 0 && text.length <= MAX

  function send() {
    if (!canSend) return
    onSend(text)
    setText('')
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      send()
    }
  }

  return (
    <div className="flex items-end gap-2 border-t border-zinc-200 p-3 dark:border-zinc-800">
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
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
  )
}
