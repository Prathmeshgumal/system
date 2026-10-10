// Dark mode: "system" follows the OS setting, "light"/"dark" override it.
// The choice is saved in localStorage; index.html applies it before React
// loads, so the page never flashes white first.
export type Theme = 'system' | 'light' | 'dark'

const KEY = 'relay-theme'

export function getTheme(): Theme {
  try {
    const t = localStorage.getItem(KEY)
    return t === 'light' || t === 'dark' ? t : 'system'
  } catch {
    return 'system'
  }
}

export function setTheme(t: Theme) {
  try {
    if (t === 'system') localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, t)
  } catch {
    // private mode: the choice just isn't remembered
  }
  applyTheme(t)
}

export function isDark(t: Theme, systemDark: boolean): boolean {
  return t === 'dark' || (t === 'system' && systemDark)
}

export function applyTheme(t: Theme = getTheme()) {
  const systemDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false
  document.documentElement.classList.toggle('dark', isDark(t, systemDark))
}

/** Re-apply when the OS switches between light and dark (only matters for "system"). */
export function watchSystemTheme(): () => void {
  const mq = window.matchMedia?.('(prefers-color-scheme: dark)')
  if (!mq) return () => {}
  const onChange = () => applyTheme()
  mq.addEventListener('change', onChange)
  return () => mq.removeEventListener('change', onChange)
}

export const nextTheme: Record<Theme, Theme> = { system: 'light', light: 'dark', dark: 'system' }
