// Uploading a profile picture (Part 2.2, ADR 0004):
//  1. shrink it to 256×256 in a <canvas> (a 4 MB phone photo becomes ~20 KB)
//  2. ask the API for a presigned POST — permission for exactly one upload
//  3. POST the file straight to storage; the API never touches the bytes
//  4. tell the API it's there; it checks the file and switches the profile over
import { api } from '../lib/api'
import type { User } from '../lib/types'

export const AVATAR_SIZE = 256

type Start = { key: string; upload: { url: string; fields: Record<string, string> } }

/** Center-crops to a square and scales to size×size JPEG. */
export async function resizeToSquare(file: Blob, size = AVATAR_SIZE): Promise<Blob> {
  const img = await createImageBitmap(file)
  const side = Math.min(img.width, img.height)
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = size
  canvas.getContext('2d')!.drawImage(img, (img.width - side) / 2, (img.height - side) / 2, side, side, 0, 0, size, size)
  img.close()
  return new Promise((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('could not encode the picture'))), 'image/jpeg', 0.85),
  )
}

/** The form S3 expects: every policy field, then the file — which must come LAST. */
export function uploadForm(fields: Record<string, string>, file: Blob, contentType: string): FormData {
  const form = new FormData()
  for (const [k, v] of Object.entries(fields)) form.append(k, v)
  if (!('Content-Type' in fields)) form.append('Content-Type', contentType)
  form.append('file', file)
  return form
}

export async function uploadAvatar(file: File, resize = resizeToSquare): Promise<User> {
  const picture = await resize(file)
  const { key, upload } = await api<Start>('/api/me/avatar/upload', {
    method: 'POST',
    body: { content_type: 'image/jpeg' },
  })
  const res = await fetch(upload.url, { method: 'POST', body: uploadForm(upload.fields, picture, 'image/jpeg') })
  if (!res.ok) throw new Error(`Upload failed (${res.status})`)
  return api<User>('/api/me/avatar', { method: 'PUT', body: { key } })
}
