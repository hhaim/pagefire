import { useRouter } from 'preact-router'
import { useEffect, useState } from 'preact/hooks'
import { useAuth } from '../auth.jsx'

const NAV_ITEMS = [
  { path: '/alerts', label: 'Alerts', icon: '⚡' },
  { path: '/home-events', label: 'Event Playground', icon: '🧪' },
  { path: '/event-ingestion', label: 'Event Ingestion', icon: '🔑' },
  { path: '/home-plugins', label: 'Notification Plugins', icon: '🔌' },
  { path: '/users', label: 'Users', icon: '👤' },
]

export function Layout({ children }) {
  const { user, logout } = useAuth()
  const [routerState] = useRouter()
  const currentPath = routerState.url || '/'
	const [sidebarOpen, setSidebarOpen] = useState(() => window.matchMedia('(min-width: 701px)').matches)

	useEffect(() => {
		const media = window.matchMedia('(min-width: 701px)')
		const update = event => setSidebarOpen(event.matches)
		media.addEventListener('change', update)
		return () => media.removeEventListener('change', update)
	}, [])

  return (
    <div class={`layout${sidebarOpen ? ' sidebar-open' : ''}`}>
      <button class="sidebar-toggle" type="button" aria-label={sidebarOpen ? 'Close navigation' : 'Open navigation'} aria-expanded={sidebarOpen} onClick={() => setSidebarOpen(open => !open)}>{sidebarOpen ? '×' : '☰'}</button>
      {sidebarOpen && <button class="sidebar-backdrop" aria-label="Close navigation" onClick={() => setSidebarOpen(false)} />}
      <aside class="sidebar">
        <div class="sidebar-header">
          <a href="/" class="sidebar-logo">PageFire</a>
        </div>
        <nav class="sidebar-nav">
          {NAV_ITEMS.map(item => (
            <a
              key={item.path}
              href={item.path}
              onClick={() => { if (window.matchMedia('(max-width: 700px)').matches) setSidebarOpen(false) }}
              class={`nav-item ${(currentPath === '/' && item.path === '/alerts' || (item.exact ? currentPath === item.path : currentPath.startsWith(item.path))) ? 'active' : ''}`}
            >
              <span class="nav-icon">{item.icon}</span>
              {item.label}
            </a>
          ))}
        </nav>
        <div class="sidebar-footer">
          {user && <a href="/profile" class="sidebar-user">{user.name}</a>}
          <button class="logout-button" onClick={logout}>
            Sign out
          </button>
        </div>
      </aside>
      <main class="content">
        {children}
      </main>
    </div>
  )
}
