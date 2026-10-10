import { describe, expect, it } from 'vitest'
import { isDark, nextTheme } from './theme'

describe('theme', () => {
  it('system follows the OS; light and dark override it', () => {
    expect(isDark('system', true)).toBe(true)
    expect(isDark('system', false)).toBe(false)
    expect(isDark('light', true)).toBe(false)
    expect(isDark('dark', false)).toBe(true)
  })
  it('the toggle cycles system → light → dark → system', () => {
    expect([nextTheme.system, nextTheme.light, nextTheme.dark]).toEqual(['light', 'dark', 'system'])
  })
})
