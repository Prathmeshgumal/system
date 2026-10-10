import { initials } from '../lib/format'

const colors = ['bg-rose-500', 'bg-amber-500', 'bg-emerald-500', 'bg-sky-500', 'bg-violet-500', 'bg-pink-500', 'bg-teal-500']
const sizes = { sm: 'h-8 w-8 text-xs', md: 'h-10 w-10 text-sm', lg: 'h-24 w-24 text-2xl' }

/** A profile picture, or coloured initials when there isn't one. */
export function Avatar({
  name,
  id,
  url,
  size = 'md',
}: {
  name: string
  id: number
  url?: string | null
  size?: keyof typeof sizes
}) {
  if (url) {
    return <img src={url} alt="" className={`${sizes[size]} shrink-0 rounded-full object-cover`} />
  }
  return (
    <div
      aria-hidden
      className={`${sizes[size]} ${colors[Math.abs(id) % colors.length]} flex shrink-0 items-center justify-center rounded-full font-semibold text-white`}
    >
      {initials(name)}
    </div>
  )
}
