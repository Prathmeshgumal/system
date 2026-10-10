import { createBrowserRouter, Navigate } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { LoginPage } from './auth/LoginPage'
import { RequireAuth } from './auth/RequireAuth'
import { SignupPage } from './auth/SignupPage'
import { ChatLayout, NoChatSelected } from './chat/ChatLayout'
import { ChatView } from './chat/ChatView'
import { ProfilePage } from './profile/ProfilePage'
import { RealtimeProvider } from './realtime/RealtimeProvider'
import { StatusPage } from './status/StatusPage'

// Logged-in pages share one WebSocket (RealtimeProvider) and the chat sidebar.
function Shell() {
  return (
    <RealtimeProvider>
      <ChatLayout />
    </RealtimeProvider>
  )
}

const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  { path: '/signup', element: <SignupPage /> },
  { path: '/status', element: <StatusPage /> },
  {
    element: <RequireAuth />,
    children: [
      {
        element: <Shell />,
        children: [
          { index: true, element: <NoChatSelected /> },
          { path: 'chats/:chatId', element: <ChatView /> },
          { path: 'profile', element: <ProfilePage /> },
        ],
      },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
])

export default function App() {
  return <RouterProvider router={router} />
}
