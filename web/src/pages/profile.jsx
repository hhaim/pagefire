import { useState } from 'preact/hooks'
import { useAuth } from '../auth.jsx'
import { apiPut } from '../api.js'
import { TextInput } from '../components/form-field.jsx'
import { useToast } from '../components/toast.jsx'

export function Profile() {
  const { user } = useAuth()
  const toast = useToast()

  if (!user) return <div class="loading">Loading...</div>

  return (
    <div class="page">
      <div class="page-header">
        <h1>Profile &amp; Settings</h1>
      </div>

      <div class="detail-grid">
        <ProfileCard user={user} />
        <PasswordCard toast={toast} />
      </div>
    </div>
  )
}

// --- Profile Info ---
function ProfileCard({ user }) {
  return (
    <div class="detail-card">
      <h3>Account</h3>
      <div class="detail-row">
        <span class="detail-label">Name</span>
        <span>{user.name}</span>
      </div>
      <div class="detail-row">
        <span class="detail-label">Email</span>
        <span>{user.email}</span>
      </div>
      <div class="detail-row">
        <span class="detail-label">Role</span>
        <span class="source-tag">{user.role}</span>
      </div>
      <div class="detail-row">
        <span class="detail-label">Timezone</span>
        <span>{user.timezone || 'UTC'}</span>
      </div>
    </div>
  )
}

// --- Password Change ---
function PasswordCard({ toast }) {
  const [form, setForm] = useState({ current_password: '', new_password: '', confirm: '' })
  const [errors, setErrors] = useState({})
  const [saving, setSaving] = useState(false)

  const handleChange = async () => {
    const errs = {}
    if (!form.current_password) errs.current_password = 'Required'
    if (!form.new_password) errs.new_password = 'Required'
    else if (form.new_password.length < 8 || !/[A-Z]/.test(form.new_password) || !/[a-z]/.test(form.new_password) || !/[0-9]/.test(form.new_password)) errs.new_password = 'Min 8 chars, upper + lower + digit'
    if (form.new_password !== form.confirm) errs.confirm = 'Passwords do not match'
    setErrors(errs)
    if (Object.keys(errs).length > 0) return

    setSaving(true)
    const { error } = await apiPut('/auth/password', {
      current_password: form.current_password,
      new_password: form.new_password,
    })
    setSaving(false)
    if (error) {
      toast.error(error)
      return
    }
    toast.success('Password changed')
    setForm({ current_password: '', new_password: '', confirm: '' })
  }

  return (
    <div class="detail-card">
      <h3>Change Password</h3>
      <TextInput
        label="Current Password"
        type="password"
        value={form.current_password}
        onInput={(e) => setForm(prev => ({ ...prev, current_password: e.target.value }))}
        error={errors.current_password}
      />
      <TextInput
        label="New Password"
        type="password"
        value={form.new_password}
        onInput={(e) => setForm(prev => ({ ...prev, new_password: e.target.value }))}
        error={errors.new_password}
        placeholder="Min 8 chars, upper + lower + digit"
      />
      <TextInput
        label="Confirm New Password"
        type="password"
        value={form.confirm}
        onInput={(e) => setForm(prev => ({ ...prev, confirm: e.target.value }))}
        error={errors.confirm}
      />
      <div style="margin-top: 12px">
        <button class="btn btn-primary" onClick={handleChange} disabled={saving}>
          {saving ? 'Saving...' : 'Change Password'}
        </button>
      </div>
    </div>
  )
}
