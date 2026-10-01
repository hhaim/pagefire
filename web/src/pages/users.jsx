import { useState } from 'preact/hooks'
import { useApi } from '../hooks.js'
import { useAuth } from '../auth.jsx'
import { apiPost, apiPut, apiDelete } from '../api.js'
import { EmptyState } from '../components/empty-state.jsx'
import { StatusBadge } from '../components/status-badge.jsx'
import { Modal } from '../components/modal.jsx'
import { TextInput, SelectInput } from '../components/form-field.jsx'
import { ConfirmDialog } from '../components/confirm-dialog.jsx'
import { useToast } from '../components/toast.jsx'

const TIMEZONES = [
  'UTC', 'America/New_York', 'America/Chicago', 'America/Denver',
  'America/Los_Angeles', 'America/Sao_Paulo', 'Europe/London',
  'Europe/Berlin', 'Europe/Moscow', 'Asia/Kolkata', 'Asia/Shanghai',
  'Asia/Tokyo', 'Australia/Sydney', 'Pacific/Auckland',
].map(tz => ({ value: tz, label: tz }))

const emptyForm = { username: '', password: '', timezone: 'UTC' }

export function Users() {
  const { data: users, loading, refetch } = useApi('/users')
  const { user: currentUser } = useAuth()
  const toast = useToast()

  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState(null)
  const [form, setForm] = useState(emptyForm)
  const [errors, setErrors] = useState({})
  const [saving, setSaving] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState(null)
  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setErrors({})
    setModalOpen(true)
  }

  const openEdit = (user) => {
    setEditing(user)
    setForm({ username: user.email, password: '', timezone: user.timezone || 'UTC' })
    setErrors({})
    setModalOpen(true)
  }

  const isAdmin = currentUser?.role === 'admin'

  const validate = () => {
    const errs = {}
    if (!form.username.trim()) errs.username = 'Username is required'
    if (!editing && !form.password) errs.password = 'Password is required'
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  const handleSave = async () => {
    if (!validate()) return
    setSaving(true)
    const username = form.username.trim()
    const payload = { name: editing?.name || username, email: username, timezone: editing ? form.timezone : 'UTC' }
    if (!editing) {
      payload.role = 'admin'
      payload.password = form.password
    }
    const { data, error } = editing
      ? await apiPut(`/users/${editing.id}`, payload)
      : await apiPost('/users', payload)
    setSaving(false)
    if (error) {
      toast.error(error)
      return
    }
    toast.success(editing ? 'User updated' : 'User created')
    setModalOpen(false)
    refetch()
  }

  const handleDelete = async () => {
    const { error } = await apiDelete(`/users/${deleteTarget.id}`)
    if (error) {
      toast.error(error)
    } else {
      toast.success('User deleted')
      refetch()
    }
    setDeleteTarget(null)
  }

  const setField = (field) => (e) => {
    setForm(prev => ({ ...prev, [field]: e.target.value }))
    if (errors[field]) setErrors(prev => ({ ...prev, [field]: null }))
  }

  return (
    <div class="page">
      <div class="page-header">
        <h1>Users</h1>
        {isAdmin && (
          <div class="actions">
            <button class="btn btn-primary" onClick={openCreate}>Add User</button>
          </div>
        )}
      </div>

      {loading ? (
        <div class="loading">Loading...</div>
      ) : !users || users.length === 0 ? (
        <EmptyState message="No users" />
      ) : (
        <table class="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Username</th>
              <th>Role</th>
              <th>Timezone</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {users.map(u => (
              <tr key={u.id}>
                <td class="bold">{u.name}</td>
                <td>{u.email}</td>
                <td><StatusBadge status={u.role} /></td>
                <td class="text-muted">{u.timezone || '—'}</td>
                {isAdmin && (
                  <td class="row-actions">
                    <button class="btn-icon" onClick={() => openEdit(u)} title="Edit">&#9998;</button>
                    {currentUser?.id !== u.id && (
                      <button class="btn-icon btn-icon-danger" onClick={() => setDeleteTarget(u)} title="Delete">&times;</button>
                    )}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title={editing ? 'Edit User' : 'Add User'}>
        {
          <div>
            <TextInput label="Username" value={form.username} onInput={setField('username')} error={errors.username} placeholder="admin" />
            {!editing && <TextInput label="Password" value={form.password} onInput={setField('password')} error={errors.password} type="password" autoComplete="new-password" />}
            {editing && <SelectInput label="Timezone" value={form.timezone} onChange={setField('timezone')} options={TIMEZONES} />}
            <div class="form-actions">
              <button class="btn btn-secondary" onClick={() => setModalOpen(false)}>Cancel</button>
              <button class="btn btn-primary" onClick={handleSave} disabled={saving}>
                {saving ? 'Saving...' : editing ? 'Save Changes' : 'Create Admin'}
              </button>
            </div>
          </div>
        }
      </Modal>

      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete User"
        message={`Are you sure you want to delete "${deleteTarget?.name}"? This action cannot be undone.`}
      />
    </div>
  )
}
