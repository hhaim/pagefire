import { useEffect, useState } from 'preact/hooks'
import { apiGet, apiPut, apiDelete, apiPost } from '../api.js'
import { useAuth } from '../auth.jsx'

function PluginCard({ plugin, refresh, readOnly }) {
  const meta = { name: plugin.name, destination: plugin.destination_label, secret: plugin.secret_label }
  const [destination, setDestination] = useState(plugin.destination || '')
  const [secret, setSecret] = useState('')
  const [enabled, setEnabled] = useState(plugin.enabled)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')

  useEffect(() => {
    setDestination(plugin.destination || '')
    setEnabled(plugin.enabled)
  }, [plugin.destination, plugin.enabled])

  async function save() {
    setBusy(true)
    const { error } = await apiPut(`/home-plugins/${plugin.kind}`, { destination, secret, enabled })
    setBusy(false)
    setMessage(error || `${meta.name} settings saved`)
    if (!error) { setSecret(''); refresh() }
  }

  async function test() {
    setBusy(true)
    const { error } = await apiPost(`/home-plugins/${plugin.kind}/test`, {})
    setBusy(false)
    setMessage(error || `Test message sent to ${meta.name}`)
  }

  async function remove() {
    if (!window.confirm(`Remove ${meta.name} settings?`)) return
    setBusy(true)
    const { error } = await apiDelete(`/home-plugins/${plugin.kind}`)
    setBusy(false)
    setMessage(error || `${meta.name} settings removed`)
    if (!error) { setDestination(''); setSecret(''); setEnabled(false); refresh() }
  }

  return <details class="home-card home-plugin-accordion">
    <summary class="home-plugin-summary">
      <strong>{meta.name}</strong>
      <span class={plugin.enabled ? 'home-plugin-state enabled' : 'home-plugin-state'}>{plugin.configured ? (plugin.enabled ? 'Enabled' : 'Disabled') : 'Not configured'}</span>
    </summary>
    <div class="home-plugin-fields">
    {plugin.kind === 'telegram' && <p class="text-muted">Use a numeric chat ID and a dedicated bot. High alerts include an acknowledge button.</p>}
    <label class="form-field"><span class="form-label">{meta.destination} (global)</span>
      <input class="form-control" value={destination} onInput={e => setDestination(e.target.value)} disabled={readOnly} />
    </label>
    <label class="form-field"><span class="form-label">{meta.secret}</span>
      <input class="form-control" type="password" value={secret} onInput={e => setSecret(e.target.value)} placeholder={plugin.configured ? 'Leave blank to keep current token' : ''} autocomplete="off" disabled={readOnly} />
    </label>
    <label class="home-checkbox"><input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} disabled={readOnly} /> Enabled</label>
    {!readOnly && <div class="home-actions">
      <button class="btn btn-primary" disabled={busy} onClick={save}>Save</button>
      <button class="btn btn-secondary" disabled={busy || !plugin.configured} onClick={test}>Send test</button>
      {plugin.configured && <button class="btn btn-danger" disabled={busy} onClick={remove}>Remove</button>}
    </div>}
    {message && <p class="home-message">{message}</p>}
    </div>
  </details>
}

export function HomePlugins() {
  const { user } = useAuth()
  const [plugins, setPlugins] = useState(null)
  const [error, setError] = useState('')
  async function refresh() {
    const result = await apiGet('/home-plugins')
    setPlugins(result.data)
    setError(result.error || '')
  }
  useEffect(() => { refresh() }, [])

  return <div class="page">
    <div class="page-header"><h1>Notification Plugins</h1></div>
    <p class="text-muted">Each enabled plugin sends to one global destination for home events and weekly stats.</p>
    {user?.role !== 'admin' ? <p>Admin access is required to change plugin settings.</p> : null}
    {error && <p class="form-error">{error}</p>}
    <div class="home-plugin-list">{plugins?.map(p => <PluginCard key={p.kind} plugin={p} refresh={refresh} readOnly={user?.role !== 'admin'} />)}</div>
  </div>
}
