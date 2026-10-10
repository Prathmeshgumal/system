// Browser notifications. Permission must be requested from a click (browsers
// ignore or block prompts that appear on their own).
export function notificationsSupported(): boolean {
  return typeof window !== 'undefined' && 'Notification' in window
}

export function notificationPermission(): NotificationPermission | 'unsupported' {
  return notificationsSupported() ? Notification.permission : 'unsupported'
}

export async function requestNotifications(): Promise<NotificationPermission | 'unsupported'> {
  if (!notificationsSupported()) return 'unsupported'
  return Notification.requestPermission()
}

/** Shows a notification only when the tab is in the background and we're allowed to. */
export function showNotification(title: string, body: string, tag: string, onClick: () => void) {
  if (!notificationsSupported() || Notification.permission !== 'granted' || !document.hidden) return
  // tag: a newer notification for the same chat replaces the older one.
  const n = new Notification(title, { body, tag })
  n.onclick = () => {
    window.focus()
    onClick()
    n.close()
  }
}
