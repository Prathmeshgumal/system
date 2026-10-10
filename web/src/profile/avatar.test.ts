import { afterEach, describe, expect, it, vi } from 'vitest'
import { uploadAvatar, uploadForm } from './avatar'

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

describe('avatar upload', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('puts the policy fields first and the file last', () => {
    const form = uploadForm({ key: 'avatars/1/a.jpg', policy: 'p', 'x-amz-signature': 's' }, new Blob(['x']), 'image/jpeg')
    expect([...form.keys()]).toEqual(['key', 'policy', 'x-amz-signature', 'Content-Type', 'file'])
  })

  it('does not send Content-Type twice when the policy already has it', () => {
    const form = uploadForm({ 'Content-Type': 'image/jpeg' }, new Blob(['x']), 'image/jpeg')
    expect(form.getAll('Content-Type')).toHaveLength(1)
  })

  it('shrinks, gets a policy, posts to storage, then confirms the key', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    fetchMock
      .mockResolvedValueOnce(json(200, { key: 'avatars/1/k.jpg', upload: { url: '/storage/b/', fields: { key: 'avatars/1/k.jpg' } } }))
      .mockResolvedValueOnce(new Response(null, { status: 204 })) // storage
      .mockResolvedValueOnce(json(200, { id: 1, avatar_url: '/media/avatars/1/k.jpg' }))

    const resize = vi.fn(async () => new Blob(['small'], { type: 'image/jpeg' }))
    const user = await uploadAvatar(new File(['big photo'], 'me.png'), resize)

    expect(resize).toHaveBeenCalledOnce()
    expect(fetchMock.mock.calls.map((c) => [c[0], c[1].method])).toEqual([
      ['/api/me/avatar/upload', 'POST'],
      ['/storage/b/', 'POST'],
      ['/api/me/avatar', 'PUT'],
    ])
    expect(fetchMock.mock.calls[1][1].body).toBeInstanceOf(FormData)
    expect(JSON.parse(fetchMock.mock.calls[2][1].body)).toEqual({ key: 'avatars/1/k.jpg' })
    expect(user.avatar_url).toBe('/media/avatars/1/k.jpg')
  })

  it('a storage rejection stops before confirming', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    fetchMock
      .mockResolvedValueOnce(json(200, { key: 'k', upload: { url: '/s/', fields: {} } }))
      .mockResolvedValueOnce(new Response('<Error>EntityTooLarge</Error>', { status: 400 }))
    await expect(uploadAvatar(new File(['x'], 'x'), async (b) => b)).rejects.toThrow('Upload failed (400)')
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
