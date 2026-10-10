import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider } from './AuthProvider'
import { LoginPage } from './LoginPage'
import { SignupPage } from './SignupPage'

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function renderAt(path: string) {
  const router = createMemoryRouter(
    [
      { path: '/login', element: <LoginPage /> },
      { path: '/signup', element: <SignupPage /> },
      { path: '/', element: <p>Chats home</p> },
    ],
    { initialEntries: [path] },
  )
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </QueryClientProvider>,
  )
}

const user = { id: 1, username: 'alice', display_name: 'Alice', bio: '', email: 'a@x.io', created_at: '' }

describe('auth pages', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  it('shows the server error for wrong credentials', async () => {
    fetchMock
      .mockResolvedValueOnce(json(401, { error: { code: 'unauthorized', message: 'not logged in' } })) // startup refresh
      .mockResolvedValueOnce(
        json(401, { error: { code: 'invalid_credentials', message: 'invalid email or password' } }),
      )
    renderAt('/login')

    await userEvent.type(await screen.findByLabelText('Email'), 'alice@x.io')
    await userEvent.type(screen.getByLabelText('Password'), 'wrong-password')
    await userEvent.click(screen.getByRole('button', { name: 'Log in' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid email or password')
  })

  it('logs in and leaves the login page', async () => {
    fetchMock
      .mockResolvedValueOnce(json(401, { error: { code: 'unauthorized', message: '' } }))
      .mockResolvedValueOnce(json(200, { access_token: 't', expires_at: '', user }))
    renderAt('/login')

    await userEvent.type(await screen.findByLabelText('Email'), 'a@x.io')
    await userEvent.type(screen.getByLabelText('Password'), 'password123')
    await userEvent.click(screen.getByRole('button', { name: 'Log in' }))

    expect(await screen.findByText('Chats home')).toBeInTheDocument()
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ email: 'a@x.io', password: 'password123' })
  })

  it('an existing session skips the login page', async () => {
    fetchMock.mockResolvedValueOnce(json(200, { access_token: 't', expires_at: '', user }))
    renderAt('/login')
    expect(await screen.findByText('Chats home')).toBeInTheDocument()
  })

  it('signup shows each field error next to its field', async () => {
    fetchMock
      .mockResolvedValueOnce(json(401, { error: { code: 'unauthorized', message: '' } }))
      .mockResolvedValueOnce(
        json(409, {
          error: { code: 'username_taken', message: 'taken', fields: { username: 'already taken' } },
        }),
      )
    renderAt('/signup')
    await userEvent.type(await screen.findByLabelText('Username'), 'alice')
    await userEvent.click(screen.getByRole('button', { name: 'Sign up' }))

    expect(await screen.findByText('already taken')).toBeInTheDocument()
    expect(screen.getByLabelText('Username')).toHaveAttribute('aria-invalid', 'true')
  })
})
