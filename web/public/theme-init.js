// Apply the saved theme before anything paints (see src/lib/theme.ts).
// A separate file rather than an inline <script>, so the production
// Content-Security-Policy can forbid inline scripts entirely.
try {
  var t = localStorage.getItem('relay-theme')
  var dark = t === 'dark' || (t !== 'light' && matchMedia('(prefers-color-scheme: dark)').matches)
  if (dark) document.documentElement.classList.add('dark')
} catch (e) {}
