import { useEffect, useMemo, useState } from 'react'
import { Users, ShieldCheck, Save, RefreshCw } from 'lucide-react'
import type { Strategy } from '../types'
import { api } from '../lib/api'
import { notify } from '../lib/notify'

interface AdminUser {
  id: string
  email: string
  role: string
}

export function StrategyPermissionPage() {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [selectedUserID, setSelectedUserID] = useState<string>('')
  const [grantedStrategyIDs, setGrantedStrategyIDs] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [loadingUserPerm, setLoadingUserPerm] = useState(false)

  const selectedUser = useMemo(
    () => users.find((u) => u.id === selectedUserID) || null,
    [users, selectedUserID]
  )

  const loadBaseData = async () => {
    setLoading(true)
    try {
      const [userList, strategyList] = await Promise.all([
        api.getAdminUsers(),
        api.getStrategies(),
      ])
      const normalUsers = userList.filter((u) => u.role !== 'ADMIN')
      setUsers(normalUsers)
      setStrategies(strategyList)
      if (normalUsers.length > 0) {
        setSelectedUserID((prev) => prev || normalUsers[0].id)
      } else {
        setSelectedUserID('')
      }
    } catch (error) {
      notify.error(error instanceof Error ? error.message : 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }

  const loadUserPermissions = async (userID: string) => {
    if (!userID) {
      setGrantedStrategyIDs(new Set())
      return
    }
    setLoadingUserPerm(true)
    try {
      const strategyIDs = await api.getAdminUserStrategyIDs(userID)
      setGrantedStrategyIDs(new Set(strategyIDs))
    } catch (error) {
      notify.error(error instanceof Error ? error.message : 'Failed to load permissions')
    } finally {
      setLoadingUserPerm(false)
    }
  }

  useEffect(() => {
    loadBaseData()
  }, [])

  useEffect(() => {
    if (selectedUserID) {
      loadUserPermissions(selectedUserID)
    }
  }, [selectedUserID])

  const toggleStrategy = (strategyID: string) => {
    setGrantedStrategyIDs((prev) => {
      const next = new Set(prev)
      if (next.has(strategyID)) {
        next.delete(strategyID)
      } else {
        next.add(strategyID)
      }
      return next
    })
  }

  const selectAll = () => {
    setGrantedStrategyIDs(new Set(strategies.map((s) => s.id)))
  }

  const clearAll = () => {
    setGrantedStrategyIDs(new Set())
  }

  const savePermissions = async () => {
    if (!selectedUserID) return
    setSaving(true)
    try {
      await api.setAdminUserStrategyIDs(selectedUserID, Array.from(grantedStrategyIDs))
      notify.success('Strategy permissions updated')
    } catch (error) {
      notify.error(error instanceof Error ? error.message : 'Failed to save permissions')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="max-w-[1400px] mx-auto p-6 text-nofx-text-muted">Loading permissions...</div>
    )
  }

  return (
    <div className="max-w-[1400px] mx-auto p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-nofx-text flex items-center gap-2">
            <ShieldCheck className="w-6 h-6 text-nofx-gold" />
            Strategy Permissions
          </h1>
          <p className="text-sm text-nofx-text-muted mt-1">
            Grant multiple strategies to a specific user.
          </p>
        </div>
        <button
          onClick={loadBaseData}
          className="inline-flex items-center gap-2 px-3 py-2 rounded-lg bg-nofx-bg-lighter border border-nofx-gold/20 text-nofx-text hover:bg-white/5"
        >
          <RefreshCw className="w-4 h-4" />
          Refresh
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[320px_1fr] gap-4">
        <div className="rounded-lg border border-nofx-gold/20 bg-nofx-bg-lighter p-3">
          <div className="flex items-center gap-2 text-nofx-text mb-3">
            <Users className="w-4 h-4 text-nofx-gold" />
            <span className="text-sm font-semibold">Users</span>
          </div>
          <div className="space-y-2 max-h-[560px] overflow-y-auto">
            {users.length === 0 && (
              <div className="text-sm text-nofx-text-muted">No non-admin users found.</div>
            )}
            {users.map((user) => {
              const active = user.id === selectedUserID
              return (
                <button
                  key={user.id}
                  onClick={() => setSelectedUserID(user.id)}
                  className={`w-full text-left px-3 py-2 rounded-lg border transition-colors ${
                    active
                      ? 'bg-nofx-gold/10 border-nofx-gold/50 text-nofx-text'
                      : 'bg-nofx-bg border-nofx-gold/10 text-nofx-text-muted hover:text-nofx-text hover:border-nofx-gold/30'
                  }`}
                >
                  <div className="text-sm font-medium truncate">{user.email}</div>
                  <div className="text-xs opacity-70">{user.role}</div>
                </button>
              )
            })}
          </div>
        </div>

        <div className="rounded-lg border border-nofx-gold/20 bg-nofx-bg-lighter p-4">
          <div className="flex items-center justify-between gap-3 mb-3">
            <div>
              <div className="text-sm text-nofx-text-muted">Selected User</div>
              <div className="text-base font-semibold text-nofx-text">
                {selectedUser?.email || '-'}
              </div>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={selectAll}
                disabled={!selectedUserID || strategies.length === 0}
                className="px-3 py-1.5 rounded text-xs border border-nofx-gold/30 text-nofx-text disabled:opacity-40"
              >
                Select All
              </button>
              <button
                onClick={clearAll}
                disabled={!selectedUserID}
                className="px-3 py-1.5 rounded text-xs border border-nofx-gold/30 text-nofx-text disabled:opacity-40"
              >
                Clear
              </button>
              <button
                onClick={savePermissions}
                disabled={!selectedUserID || saving}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded text-xs font-semibold bg-nofx-gold text-black disabled:opacity-40"
              >
                <Save className="w-3.5 h-3.5" />
                {saving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>

          {loadingUserPerm ? (
            <div className="text-sm text-nofx-text-muted">Loading user permissions...</div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-2 max-h-[560px] overflow-y-auto pr-1">
              {strategies.map((strategy) => {
                const checked = grantedStrategyIDs.has(strategy.id)
                return (
                  <label
                    key={strategy.id}
                    className={`flex items-start gap-3 rounded-lg border p-3 cursor-pointer transition-colors ${
                      checked
                        ? 'border-nofx-gold/50 bg-nofx-gold/10'
                        : 'border-nofx-gold/15 bg-nofx-bg hover:border-nofx-gold/30'
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={() => toggleStrategy(strategy.id)}
                      className="mt-0.5"
                    />
                    <div className="min-w-0">
                      <div className="text-sm font-medium text-nofx-text truncate">{strategy.name}</div>
                      <div className="text-xs text-nofx-text-muted mt-1 line-clamp-2">
                        {strategy.description || 'No description'}
                      </div>
                    </div>
                  </label>
                )
              })}
              {strategies.length === 0 && (
                <div className="text-sm text-nofx-text-muted">No strategies available.</div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default StrategyPermissionPage
