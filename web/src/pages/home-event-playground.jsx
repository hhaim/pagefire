import { useEffect, useState } from 'preact/hooks'
import { apiFetch, apiGet } from '../api.js'

const newID = () => globalThis.crypto?.randomUUID?.() || `demo-${Date.now()}-${Math.random().toString(36).slice(2)}`
const initial = () => ({ event_id: newID(), event: 'info', incident_key: '', severity: 'low', summary: 'Demo event', details: '', repeat_interval_seconds: 300 })

function initialFromURL() {
  const params = new URLSearchParams(window.location.search)
  const incidentKey = params.get('incident_key')
  if (params.get('event') !== 'stop' || !incidentKey) return initial()
  return { ...initial(), event: 'stop', incident_key: incidentKey }
}

function jsonFor(form) {
  const body = { event_id: form.event_id, event: form.event }
  if (form.event !== 'info') body.incident_key = form.incident_key
  if (form.event !== 'stop') body.severity = form.severity
  if (form.summary) body.summary = form.summary
  if (form.details) body.details = form.details
  if (form.event === 'start' && form.severity === 'high') body.repeat_interval_seconds = Number(form.repeat_interval_seconds) || 300
  return JSON.stringify(body, null, 2)
}

export function HomeEventPlayground() {
  const [form, setForm] = useState(initialFromURL)
  const [json, setJSON] = useState(() => jsonFor(form))
  const [jsonError, setJSONError] = useState('')
  const [runs, setRuns] = useState([])
  const [alerts, setAlerts] = useState([])
  const [busy, setBusy] = useState(false)

  async function refreshAlerts() {
    const { data } = await apiGet('/alerts?source=home&limit=1000')
    if (data) setAlerts(data)
  }
  useEffect(() => { refreshAlerts() }, [])
  useEffect(() => {
    const alertID = new URLSearchParams(window.location.search).get('alert_id')
    if (!alertID) return
    apiGet(`/alerts/${encodeURIComponent(alertID)}`).then(({ data }) => {
      if (!data || data.source !== 'home' || data.status === 'resolved') return
      const next = { ...initial(), event: 'stop', incident_key: data.group_key, summary: data.summary, details: data.details }
      setForm(next)
      setJSON(jsonFor(next))
    })
  }, [])

  useEffect(() => {
    const timer = setInterval(async () => {
      const pending = runs.filter(r => r.result?.event_id && r.detail?.deliveries?.some(d => d.status === 'pending' || d.status === 'sending'))
      for (const run of pending) {
        const { data } = await apiGet(`/events/${encodeURIComponent(run.result.event_id)}`)
        if (data) setRuns(old => old.map(r => r.id === run.id ? { ...r, detail: data } : r))
      }
    }, 3000)
    return () => clearInterval(timer)
  }, [runs])

  function field(name, value) {
    const next = { ...form, [name]: value }
    setForm(next)
    setJSON(jsonFor(next))
    setJSONError('')
  }

  function editJSON(value) {
    setJSON(value)
    try {
      const parsed = JSON.parse(value)
      setForm(old => ({ ...old, ...parsed }))
      setJSONError('')
    } catch {
      setJSONError('JSON is not valid yet')
    }
  }

  async function send(fresh) {
    let body = json
    try {
      const parsed = JSON.parse(body)
      if (fresh) {
        parsed.event_id = newID()
        body = JSON.stringify(parsed, null, 2)
        setJSON(body)
        setForm(old => ({ ...old, event_id: parsed.event_id }))
      }
    } catch {
      setJSONError('Fix the JSON before sending')
      return
    }
    setBusy(true)
    const response = await apiFetch('/events', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body })
    const run = { id: newID(), at: new Date().toLocaleString(), request: body, httpStatus: response.status, result: response.data, error: response.error, detail: null }
    if (response.data?.event_id) {
      const detail = await apiGet(`/events/${encodeURIComponent(response.data.event_id)}`)
      run.detail = detail.data
    }
    setRuns(old => [run, ...old].slice(0, 20))
    await refreshAlerts()
    setBusy(false)
  }

  const active = alerts.filter(a => a.status !== 'resolved')

  return <div class="page">
    <div class="page-header"><h1>Event Playground</h1></div>
    <p class="text-muted">Build a request, inspect the exact JSON, and send it through the same event API used by external clients. Resending the same ID shows idempotency. <a href="/home-plugins">Configure notification plugins</a>. <a href="/event-ingestion">Create an ingestion key for external use</a>.</p>
    <div class="home-grid">
      <section class="home-card">
        <h2>Build event</h2>
        <label class="form-field"><span class="form-label">Event</span>
          <select class="form-control" value={form.event} onChange={e => field('event', e.target.value)}>
            <option value="info">Info</option><option value="start">Start</option><option value="stop">Stop</option>
          </select>
        </label>
        <label class="form-field"><span class="form-label">Client event ID</span>
          <input class="form-control" value={form.event_id} onInput={e => field('event_id', e.target.value)} />
        </label>
        {form.event !== 'info' && <label class="form-field"><span class="form-label">Incident key</span>
          <input class="form-control" value={form.incident_key} onInput={e => field('incident_key', e.target.value)} placeholder="boiler-room-water" />
        </label>}
        {form.event === 'stop' && active.length > 0 && <label class="form-field"><span class="form-label">Choose active alert</span>
          <select class="form-control" onChange={e => {
            const alert = active.find(a => a.id === e.target.value)
            if (!alert) return
            const next = { ...form, incident_key: alert.group_key, summary: alert.summary, details: alert.details }
            setForm(next)
            setJSON(jsonFor(next))
          }}>
            <option value="">Select an alert</option>
            {active.map(a => <option key={a.id} value={a.id}>{a.summary} ({a.group_key})</option>)}
          </select>
        </label>}
        {form.event !== 'stop' && <>
          <label class="form-field"><span class="form-label">Severity</span>
            <select class="form-control" value={form.severity} onChange={e => field('severity', e.target.value)}>
              <option value="low">Low</option><option value="mid">Mid</option><option value="high">High</option>
            </select>
          </label>
          <label class="form-field"><span class="form-label">Summary</span>
            <input class="form-control" value={form.summary} onInput={e => field('summary', e.target.value)} />
          </label>
          <label class="form-field"><span class="form-label">Details</span>
            <textarea class="form-control" rows="3" value={form.details} onInput={e => field('details', e.target.value)} />
          </label>
        </>}
        {form.event === 'start' && form.severity === 'high' && <label class="form-field"><span class="form-label">Repeat interval (seconds, min 300)</span>
          <input class="form-control" type="number" min="300" value={form.repeat_interval_seconds} onInput={e => field('repeat_interval_seconds', e.target.value)} />
        </label>}
      </section>
      <section class="home-card">
        <h2>Request JSON</h2>
        <textarea class="form-control home-json" spellcheck="false" value={json} onInput={e => editJSON(e.target.value)} />
        {jsonError && <p class="form-error">{jsonError}</p>}
        <div class="home-actions">
          <button class="btn btn-secondary" onClick={() => navigator.clipboard.writeText(json)}>Copy JSON</button>
          <button class="btn btn-primary" disabled={busy || !!jsonError} onClick={() => send(false)}>Resend same JSON</button>
          <button class="btn btn-success" disabled={busy || !!jsonError} onClick={() => send(true)}>Send as new event</button>
        </div>
        <p class="text-muted">A new start with the same active incident key is ignored. Send a stop, then start again to test a full cycle.</p>
      </section>
    </div>
    <section class="home-card home-history">
      <h2>Send history</h2>
      {runs.length === 0 && <p class="text-muted">No events sent in this browser session.</p>}
      {runs.map(run => <article class="home-run" key={run.id}>
        <div><strong>{run.at}</strong> · HTTP {run.httpStatus || 'network error'} · {run.result?.status || run.error}</div>
        <details><summary>Request and response</summary>
          <pre>{run.request}</pre><pre>{JSON.stringify(run.result || { error: run.error }, null, 2)}</pre>
        </details>
        {run.detail?.deliveries?.length > 0 && <div class="home-deliveries">
          {run.detail.deliveries.map(d => <span key={d.id}>{d.plugin_kind}: {d.status}{d.error ? ` (${d.error})` : ''}</span>)}
        </div>}
      </article>)}
    </section>

  </div>
}
