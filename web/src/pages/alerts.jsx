import { useCallback, useEffect, useState } from 'preact/hooks'
import { apiGet } from '../api.js'
import { TimeAgo } from '../components/time-ago.jsx'

const WINDOWS = [['1h', '1 hour'], ['1d', '1 day'], ['1w', '1 week'], ['1m', '1 month']]
const PAGE_SIZE = 100

function eventTime(seconds) {
  return new Date(seconds * 1000).toLocaleString()
}

export function Alerts() {
  const [windowSize, setWindowSize] = useState('1d')
  const [type, setType] = useState('')
  const [eventClass, setEventClass] = useState('')
  const [source, setSource] = useState(() => new URLSearchParams(window.location.search).get('source') || '')
  const [client, setClient] = useState('')
  const [open, setOpen] = useState(() => new URLSearchParams(window.location.search).has('status') ? 'true' : '')
  const [regex, setRegex] = useState('')
  const [appliedRegex, setAppliedRegex] = useState('')
  const [page, setPage] = useState(0)
  const [data, setData] = useState(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState(null)
  const [eventDetail, setEventDetail] = useState(null)

  useEffect(() => {
    const timer = setTimeout(() => setAppliedRegex(regex), 300)
    return () => clearTimeout(timer)
  }, [regex])
  useEffect(() => { setPage(0) }, [windowSize, type, eventClass, source, client, open, appliedRegex])

  const fetchEvents = useCallback(async () => {
    setLoading(true)
    const params = new URLSearchParams({ window: windowSize, limit: String(PAGE_SIZE), offset: String(page * PAGE_SIZE) })
    if (type) params.set('type', type)
    if (eventClass) params.set('class', eventClass)
    if (source) params.set('source', source)
    if (client) params.set('client', client)
    if (open) params.set('open', open)
    if (appliedRegex) params.set('message_regex', appliedRegex)
    const response = await apiGet('/alert-events?' + params)
    setData(response.data)
    setError(response.error || '')
    setLoading(false)
  }, [windowSize, type, eventClass, source, client, open, appliedRegex, page])
  useEffect(() => { fetchEvents() }, [fetchEvents])

  useEffect(() => {
    setEventDetail(null)
    if (selected?.origin !== 'home' || !selected.event_id) return
    apiGet('/events/' + encodeURIComponent(selected.event_id)).then(({ data }) => setEventDetail(data))
  }, [selected])

  const events = data?.events || []
  const buckets = data?.buckets || []
  const maxBucket = Math.max(1, ...buckets.map(b => b.info + b.warning + b.error))
  const scale = 140 / maxBucket
  const typeCounts = Object.entries(data?.type_counts || {}).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
  const totals = buckets.reduce((acc, b) => {
    acc.info += b.info
    acc.warning += b.warning
    acc.error += b.error
    return acc
  }, { info: 0, warning: 0, error: 0 })

  function clearFilters() {
    setType('')
    setEventClass('')
    setSource('')
    setClient('')
    setOpen('')
    setRegex('')
  }

  return <div class="page">
    <div class="page-header"><h1>Alerts & Events</h1></div>
    <div class="stat-cards">
      <div class="stat-card stat-card-blue"><div class="stat-value">{data?.total ?? '—'}</div><div class="stat-label">Events in window</div></div>
      <div class="stat-card stat-card-red"><div class="stat-value">{data?.open_alerts ?? '—'}</div><div class="stat-label">Open alerts</div></div>
      <div class="stat-card stat-card-red"><div class="stat-value">{totals.error}</div><div class="stat-label">Critical events</div></div>
      <div class="stat-card stat-card-yellow"><div class="stat-value">{totals.warning}</div><div class="stat-label">Warnings</div></div>
    </div>
    <section class="detail-card event-filters">
      <div class="event-filter-grid">
        <label>Window
          <select class="form-control" value={windowSize} onChange={e => setWindowSize(e.target.value)}>
            {WINDOWS.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <label>Type
          <select class="form-control" value={type} onChange={e => setType(e.target.value)}>
            <option value="">All types</option>
            {(data?.facets?.types || []).map(value => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label>Class
          <select class="form-control" value={eventClass} onChange={e => setEventClass(e.target.value)}>
            <option value="">All classes</option>
            <option value="info">Info</option>
            <option value="warning">Warning</option>
            <option value="error">Error</option>
          </select>
        </label>
        <label>Source
          <select class="form-control" value={source} onChange={e => setSource(e.target.value)}>
            <option value="">All sources</option>
            {(data?.facets?.sources || []).map(value => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label>Client / service
          <select class="form-control" value={client} onChange={e => setClient(e.target.value)}>
            <option value="">All clients</option>
            {(data?.facets?.clients || []).map(value => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label>Open
          <select class="form-control" value={open} onChange={e => setOpen(e.target.value)}>
            <option value="">All</option>
            <option value="true">Yes</option>
            <option value="false">No</option>
          </select>
        </label>
        <label class="event-regex">Message regex
          <input class="form-control" value={regex} onInput={e => setRegex(e.target.value)} placeholder="water|door|offline" />
        </label>
      </div>
      <div class="home-actions">
        <button class="btn btn-secondary" onClick={clearFilters}>Clear filters</button>
        <button class="btn btn-primary" onClick={fetchEvents}>Refresh</button>
      </div>
      {error && <p class="form-error">{error}</p>}
    </section>

    <section class="detail-card">
      <div class="card-header-row"><h3>Events over time</h3><span class="text-muted">Stacked by class · local time</span></div>
      <div class="event-legend"><span class="event-info">Info {totals.info}</span><span class="event-warning">Warning {totals.warning}</span><span class="event-error">Error {totals.error}</span></div>
      <div class="event-chart" role="img" aria-label="Stacked event counts over time">
        {buckets.map(bucket => <div class="event-chart-bar" key={bucket.time} title={eventTime(bucket.time) + ': ' + (bucket.info + bucket.warning + bucket.error) + ' events'}>
          <span class="event-info-bg" style={{ height: bucket.info * scale + 'px' }} />
          <span class="event-warning-bg" style={{ height: bucket.warning * scale + 'px' }} />
          <span class="event-error-bg" style={{ height: bucket.error * scale + 'px' }} />
        </div>)}
      </div>
      {buckets.length > 0 && <div class="event-chart-axis"><span>{eventTime(buckets[0].time)}</span><span>{eventTime(buckets[buckets.length - 1].time)}</span></div>}
      <div class="event-type-counts">
        <button class={type === '' ? 'event-type-chip active' : 'event-type-chip'} onClick={() => setType('')}>All {typeCounts.reduce((sum, [, count]) => sum + count, 0)}</button>
        {typeCounts.map(([name, count]) => <button key={name} class={type === name ? 'event-type-chip active' : 'event-type-chip'} onClick={() => setType(type === name ? '' : name)}>{name} {count}</button>)}
      </div>
    </section>

    <section class="detail-card">
      <div class="card-header-row"><h3>Events</h3><span class="text-muted">{data?.total || 0} matching · {events.length} on this page · double-click an open Home Event to prepare its stop</span></div>
      {loading ? <div class="loading">Loading...</div> : events.length === 0 ? <p class="text-muted">No events match these filters.</p> : <>
        <div class="event-table-wrap"><table class="data-table">
          <thead><tr><th>Time</th><th>Class</th><th>Type</th><th>Message</th><th>Source</th><th>Client / service</th><th>Open</th></tr></thead>
          <tbody>{events.map(event => <tr key={event.origin + event.id} class="clickable-row" onClick={() => setSelected(event)} onDblClick={() => {
            if (event.origin === 'home' && event.open && event.alert_id) window.location.href = `/home-events?event=stop&alert_id=${encodeURIComponent(event.alert_id)}`
          }}>
            <td title={eventTime(event.created_at)}><TimeAgo time={event.created_at} /></td>
            <td><span class={'event-class event-' + event.class}>{event.class}</span></td>
            <td>{event.type}</td>
            <td class="summary-cell"><strong>{event.summary || event.type}</strong>{event.details && <div class="text-muted event-preview">{event.details}</div>}</td>
            <td>{event.source}</td><td>{event.client || '—'}</td><td>{event.open ? 'Yes' : 'No'}</td>
          </tr>)}</tbody>
        </table></div>
        <div class="pagination">
          <button class="btn btn-secondary btn-sm" disabled={page === 0} onClick={() => setPage(page - 1)}>Previous</button>
          <span class="pagination-info">Page {page + 1}</span>
          <button class="btn btn-secondary btn-sm" disabled={(page + 1) * PAGE_SIZE >= data.total} onClick={() => setPage(page + 1)}>Next</button>
        </div>
      </>}
    </section>

    {selected && <section class="detail-card event-detail">
      <div class="card-header-row"><h3>Event detail</h3><button class="btn btn-secondary btn-sm" onClick={() => setSelected(null)}>Close</button></div>
      <p><strong>{selected.summary || selected.type}</strong> · {eventTime(selected.created_at)}</p>
      {selected.details && <pre>{selected.details}</pre>}
      {selected.origin === 'home' && selected.open && selected.alert_id && <a href={`/home-events?event=stop&alert_id=${encodeURIComponent(selected.alert_id)}`}>Prepare stop event</a>}
      {selected.origin === 'service' && <a href={'/alerts/' + selected.alert_id}>Open service alert</a>}
      <details><summary>Event JSON and deliveries</summary><pre>{JSON.stringify(eventDetail || selected, null, 2)}</pre></details>
    </section>}
  </div>
}
