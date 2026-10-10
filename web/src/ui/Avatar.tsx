import { initials } from '../lib/format'

const colors = ['bg-rose-500', 'bg-amber-500', 'bg-emerald-500', 'bg-sky-500', 'bg-violet-500', 'bg-pink-500', 'bg-teal-500']

export function Avatar({ name, id, size = 'md' }: { name: string; id: number; size?: 'sm' | 'md' }) {
  const dim = size === 'sm' ? 'h-8 w-8 text-xs' : 'h-10 w-10 text-sm'
  return (
    <div
      aria-hidden
      className={`${dim} ${colors[Math.abs(id) % colors.length]} flex shrink-0 items-center justify-center rounded-full font-semibold text-white`}
    >
      {initials(name)}
    </div>
  )
}
