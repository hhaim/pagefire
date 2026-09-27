import Router from 'preact-router'
import { AuthProvider, LoginGate } from './auth.jsx'
import { ToastProvider } from './components/toast.jsx'
import { Layout } from './components/layout.jsx'
import { Alerts } from './pages/alerts.jsx'
import { Users } from './pages/users.jsx'
import { Profile } from './pages/profile.jsx'
import { InviteAccept } from './pages/invite.jsx'
import { HomeEventPlayground } from './pages/home-event-playground.jsx'
import { HomePlugins } from './pages/home-plugins.jsx'
import { EventIngestion } from './pages/event-ingestion.jsx'

export function App() {
  // Invite page is public (no auth required)
  const path = typeof window !== 'undefined' ? window.location.pathname : '/'
  if (path.startsWith('/invite/')) {
    const token = path.replace('/invite/', '')
    return <InviteAccept token={token} />
  }

  return (
    <AuthProvider>
      <LoginGate>
        <ToastProvider>
        <Layout>
          <Router>
            <Alerts path="/" />
            <Alerts path="/alerts" />
            <Users path="/users" />
            <Profile path="/profile" />
            <HomeEventPlayground path="/home-events" />
            <HomePlugins path="/home-plugins" />
            <EventIngestion path="/event-ingestion" />
          </Router>
        </Layout>
        </ToastProvider>
      </LoginGate>
    </AuthProvider>
  )
}
