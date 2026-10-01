import { useEffect, useState } from 'preact/hooks'
import { apiDelete, apiGet, apiPost } from '../api.js'
import { useAuth } from '../auth.jsx'
import { TimeAgo } from '../components/time-ago.jsx'

export function EventIngestion() {
  const { user } = useAuth()
  const [info, setInfo] = useState(null)
  const [token, setToken] = useState('')
  const [existingKey, setExistingKey] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function refresh() {
    const result = await apiGet('/event-ingestion-key')
    setInfo(result.data)
    setError(result.error || '')
  }
  useEffect(() => { refresh() }, [])

  async function rotate() {
    setBusy(true)
    const result = await apiPost('/event-ingestion-key', {})
    setBusy(false)
    setError(result.error || '')
    if (result.data?.token) {
      setToken(result.data.token)
      await refresh()
    }
  }

  async function rememberExistingKey() {
    setBusy(true)
    const result = await apiPost('/event-ingestion-key/secret', { token: existingKey.trim() })
    setBusy(false)
    if (result.error) {
      setError(result.error)
      return
    }
    setExistingKey('')
    setError('')
    await revealKey()
    await refresh()
  }

  async function revealKey() {
    setBusy(true)
    const result = await apiGet('/event-ingestion-key/secret')
    setBusy(false)
    if (result.error) {
      setError(result.error)
      return
    }
    setToken(result.data.token)
    setError('')
  }

  async function revoke() {
    setBusy(true)
    const result = await apiDelete('/event-ingestion-key')
    setBusy(false)
    setError(result.error || '')
    if (!result.error) {
      setToken('')
      setExistingKey('')
      await refresh()
    }
  }

  const key = token || 'YOUR_INGESTION_KEY'
  const endpoint = window.location.origin + '/api/v1/events'
  const start = [
    "curl -X POST '" + endpoint + "' \\",
    "  -H 'Authorization: Bearer " + key + "' \\",
    "  -H 'Content-Type: application/json' \\",
    "  -d '{\"event_id\":\"demo-start-001\",\"event\":\"start\",\"incident_key\":\"demo-water\",\"severity\":\"high\",\"summary\":\"Water leak\",\"details\":\"Boiler room sensor\"}'",
  ].join('\n')
  const stop = [
    "curl -X POST '" + endpoint + "' \\",
    "  -H 'Authorization: Bearer " + key + "' \\",
    "  -H 'Content-Type: application/json' \\",
    "  -d '{\"event_id\":\"demo-stop-001\",\"event\":\"stop\",\"incident_key\":\"demo-water\"}'",
  ].join('\n')
  const infoEvent = [
    "curl -X POST '" + endpoint + "' \\",
    "  -H 'Authorization: Bearer " + key + "' \\",
    "  -H 'Content-Type: application/json' \\",
    "  -d '{\"event_id\":\"demo-info-001\",\"event\":\"info\",\"incident_key\":\"demo-water\",\"severity\":\"high\",\"summary\":\"Water sensor update\",\"details\":\"Reading 99\"}'",
  ].join('\n')

  return <div class="page">
    <div class="page-header"><h1>Event Ingestion</h1></div>
    <p class="text-muted">One key sends start, stop, and info events through the same API as the playground. It has no automatic expiration and can only submit events.</p>
    <section class="home-card">
      <h2>Ingestion key</h2>
      {info?.configured ? <p>Active key: <code>{info.prefix}…</code> · created <TimeAgo time={info.created_at} /></p> : <p class="text-muted">No ingestion key yet.</p>}
      {token && <div><p>Active key</p><pre>{token}</pre><button class="btn btn-secondary" onClick={() => navigator.clipboard.writeText(token)}>Copy key</button><button class="btn btn-secondary" onClick={() => setToken('')}>Hide key</button></div>}
      {user?.role === 'admin' ? <div class="home-actions">
        <button class="btn btn-primary" disabled={busy} onClick={rotate}>{info?.configured ? 'Rotate key' : 'Create key'}</button>
        {info?.configured && <button class="btn btn-danger" disabled={busy} onClick={revoke}>Revoke key</button>}
      </div> : <p class="text-muted">An administrator creates or rotates the key.</p>}
      {user?.role === 'admin' && info?.configured && !info.recoverable && <div class="home-actions">
        <label class="key-import-field">Enter your current key once to enable reveal and copy in PageFire<input class="form-control" type="password" value={existingKey} onInput={event => setExistingKey(event.currentTarget.value)} autoComplete="off" /></label>
        <button class="btn btn-secondary" disabled={busy || !existingKey.trim()} onClick={rememberExistingKey}>Save current key</button>
      </div>}
      {user?.role === 'admin' && info?.configured && info.recoverable && !token && <button class="btn btn-secondary" disabled={busy} onClick={revealKey}>Show active key</button>}
      {info?.configured && <p class="text-muted">Rotating replaces the current key immediately. Existing senders must use the new value.</p>}
      {error && <p class="form-error">{error}</p>}
    </section>
    <section class="home-card home-history">
      <h2>Send a high alert</h2>
      <pre>{start}</pre>
      <h2>Stop the same alert</h2>
      <pre>{stop}</pre>
      <h2>Send a high info event</h2>
      <pre>{infoEvent}</pre>
      <p class="text-muted">Each event needs a new <code>event_id</code> and an <code>incident_key</code>. Start and stop use the same incident key. <a href="/home-events">Try it in the playground</a>.</p>
    </section>
  </div>
}
