import { useEffect, useMemo, useState } from 'react'
import { Users, ShieldCheck, Save, RefreshCw } from 'lucide-react'
import { useLanguage } from '../contexts/LanguageContext'
import type { Strategy } from '../types'
import { api } from '../lib/api'
import { notify } from '../lib/notify'
import { httpClient } from '../lib/httpClient'

interface AdminUser {
  id: string
  email: string
  role: 'ADMIN' | 'USER' | string
}

type UserRole = 'ADMIN' | 'USER'

type Copy = {
  title: string
  subtitle: string
  refresh: string
  users: string
  searchPlaceholder: string
  searching: string
  search: string
  inputHint: string
  noUsers: string
  selectedUser: string
  currentRole: string
  setUser: string
  setAdmin: string
  selectAll: string
  clear: string
  save: string
  saving: string
  loading: string
  loadingPermissions: string
  noStrategies: string
  noDescription: string
  errKeyword: string
  errLoadStrategies: string
  errSearchUsers: string
  errLoadPerms: string
  okSavePerms: string
  errSavePerms: string
  okRoleUpdated: string
  errRoleUpdated: string
}

const COPY: Record<'en' | 'zh' | 'id', Copy> = {
  en: {
    title: 'Strategy Permissions & Admin Roles',
    subtitle: 'Search users, then grant strategies. Only super admin can change admin role.',
    refresh: 'Refresh',
    users: 'Users',
    searchPlaceholder: 'Search by email or user id',
    searching: 'Searching...',
    search: 'Search',
    inputHint: 'Input keyword and click Search.',
    noUsers: 'No matching users found.',
    selectedUser: 'Selected User',
    currentRole: 'Current Role',
    setUser: 'Set USER',
    setAdmin: 'Set ADMIN',
    selectAll: 'Select All',
    clear: 'Clear',
    save: 'Save',
    saving: 'Saving...',
    loading: 'Loading permissions...',
    loadingPermissions: 'Loading user permissions...',
    noStrategies: 'No strategies available.',
    noDescription: 'No description',
    errKeyword: 'Please input keyword to search users',
    errLoadStrategies: 'Failed to load strategies',
    errSearchUsers: 'Failed to search users',
    errLoadPerms: 'Failed to load permissions',
    okSavePerms: 'Strategy permissions updated',
    errSavePerms: 'Failed to save permissions',
    okRoleUpdated: 'Role updated to',
    errRoleUpdated: 'Failed to update role',
  },
  zh: {
    title: '\u7b56\u7565\u6388\u6743\u4e0e\u7ba1\u7406\u5458\u6743\u9650',
    subtitle: '\u5148\u641c\u7d22\u7528\u6237\uff0c\u518d\u914d\u7f6e\u7b56\u7565\u3002\u4ec5\u8d85\u7ea7\u7ba1\u7406\u5458\u53ef\u4fee\u6539\u7ba1\u7406\u5458\u89d2\u8272\u3002',
    refresh: '\u5237\u65b0',
    users: '\u7528\u6237',
    searchPlaceholder: '\u6309\u90ae\u7bb1\u6216\u7528\u6237 ID \u641c\u7d22',
    searching: '\u641c\u7d22\u4e2d...',
    search: '\u641c\u7d22',
    inputHint: '\u8bf7\u5148\u8f93\u5165\u5173\u952e\u8bcd\uff0c\u518d\u70b9\u51fb\u641c\u7d22\u3002',
    noUsers: '\u672a\u627e\u5230\u5339\u914d\u7528\u6237\u3002',
    selectedUser: '\u5f53\u524d\u9009\u62e9\u7528\u6237',
    currentRole: '\u5f53\u524d\u89d2\u8272',
    setUser: '\u8bbe\u4e3a\u666e\u901a\u7528\u6237',
    setAdmin: '\u8bbe\u4e3a\u7ba1\u7406\u5458',
    selectAll: '\u5168\u9009',
    clear: '\u6e05\u7a7a',
    save: '\u4fdd\u5b58',
    saving: '\u4fdd\u5b58\u4e2d...',
    loading: '\u6b63\u5728\u52a0\u8f7d\u6743\u9650\u9875\u9762...',
    loadingPermissions: '\u6b63\u5728\u52a0\u8f7d\u7528\u6237\u6743\u9650...',
    noStrategies: '\u6682\u65e0\u53ef\u7528\u7b56\u7565\u3002',
    noDescription: '\u65e0\u63cf\u8ff0',
    errKeyword: '\u8bf7\u8f93\u5165\u641c\u7d22\u5173\u952e\u8bcd',
    errLoadStrategies: '\u52a0\u8f7d\u7b56\u7565\u5931\u8d25',
    errSearchUsers: '\u641c\u7d22\u7528\u6237\u5931\u8d25',
    errLoadPerms: '\u52a0\u8f7d\u6743\u9650\u5931\u8d25',
    okSavePerms: '\u7b56\u7565\u6743\u9650\u5df2\u66f4\u65b0',
    errSavePerms: '\u4fdd\u5b58\u6743\u9650\u5931\u8d25',
    okRoleUpdated: '\u89d2\u8272\u5df2\u66f4\u65b0\u4e3a',
    errRoleUpdated: '\u66f4\u65b0\u89d2\u8272\u5931\u8d25',
  },  id: {
    title: 'Izin Strategi & Peran Admin',
    subtitle: 'Cari pengguna dulu lalu atur strategi. Hanya super admin yang boleh ubah peran admin.',
    refresh: 'Muat Ulang',
    users: 'Pengguna',
    searchPlaceholder: 'Cari dengan email atau ID pengguna',
    searching: 'Mencari...',
    search: 'Cari',
    inputHint: 'Masukkan kata kunci lalu klik Cari.',
    noUsers: 'Tidak ada pengguna yang cocok.',
    selectedUser: 'Pengguna Terpilih',
    currentRole: 'Peran Saat Ini',
    setUser: 'Jadikan USER',
    setAdmin: 'Jadikan ADMIN',
    selectAll: 'Pilih Semua',
    clear: 'Kosongkan',
    save: 'Simpan',
    saving: 'Menyimpan...',
    loading: 'Memuat halaman izin...',
    loadingPermissions: 'Memuat izin pengguna...',
    noStrategies: 'Tidak ada strategi tersedia.',
    noDescription: 'Tanpa deskripsi',
    errKeyword: 'Masukkan kata kunci untuk mencari pengguna',
    errLoadStrategies: 'Gagal memuat strategi',
    errSearchUsers: 'Gagal mencari pengguna',
    errLoadPerms: 'Gagal memuat izin',
    okSavePerms: 'Izin strategi berhasil diperbarui',
    errSavePerms: 'Gagal menyimpan izin',
    okRoleUpdated: 'Peran berhasil diubah ke',
    errRoleUpdated: 'Gagal mengubah peran',
  },
}

const decodeUnicodeText = (text: string): string => {
  let value = String(text ?? '')
  for (let i = 0; i < 3; i += 1) {
    const decoded = value.replace(/\\+u([0-9a-fA-F]{4})/g, (_, hex) => String.fromCharCode(parseInt(hex, 16)))
    if (decoded === value) break
    value = decoded
  }
  return value
}

const decodeCopy = (copy: Copy): Copy =>
  Object.fromEntries(Object.entries(copy).map(([key, value]) => [key, decodeUnicodeText(value)])) as Copy

const getErrorMessage = (error: unknown, fallback: string): string => {
  const raw = error instanceof Error ? error.message : fallback
  const normalized = decodeUnicodeText(raw || fallback)
  return normalized || fallback
}

export function StrategyPermissionPage() {
  const { language } = useLanguage()
  const i18n = useMemo(() => decodeCopy(COPY[language]), [language])
  const [users, setUsers] = useState<AdminUser[]>([])
  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [selectedUserID, setSelectedUserID] = useState<string>('')
  const [grantedStrategyIDs, setGrantedStrategyIDs] = useState<Set<string>>(new Set())
  const [keyword, setKeyword] = useState('')
  const [hasSearched, setHasSearched] = useState(false)
  const [loading, setLoading] = useState(true)
  const [searchingUsers, setSearchingUsers] = useState(false)
  const [saving, setSaving] = useState(false)
  const [updatingRole, setUpdatingRole] = useState(false)
  const [loadingUserPerm, setLoadingUserPerm] = useState(false)
  const [canManageRoles, setCanManageRoles] = useState(false)

  const selectedUser = useMemo(
    () => users.find((u) => u.id === selectedUserID) || null,
    [users, selectedUserID]
  )

  const loadStrategies = async () => {
    setLoading(true)
    try {
      const strategyList = await api.getStrategies()
      setStrategies(strategyList)
    } catch (error) {
      notify.error(getErrorMessage(error, i18n.errLoadStrategies))
    } finally {
      setLoading(false)
    }
  }

  const searchUsers = async (keywordOverride?: string) => {
    const q = decodeUnicodeText(keywordOverride ?? keyword).trim()
    if (!q) {
      notify.error(decodeUnicodeText(i18n.errKeyword))
      return
    }
    setSearchingUsers(true)
    setHasSearched(true)
    try {
      const query = q ? `?q=${encodeURIComponent(q)}` : ''
      const result = await httpClient.get<{
        users: AdminUser[]
        can_manage_roles?: boolean
      }>(`/api/admin/users${query}`)
      if (!result.success) {
        throw new Error(decodeUnicodeText(result.message || i18n.errSearchUsers))
      }

      const superAdminCandidates = [
        (import.meta.env.VITE_SUPER_ADMIN_EMAIL || '').trim().toLowerCase(),
        'admin@example.com',
      ].filter(Boolean)
      const userList = (result.data?.users || []).filter((u) => !superAdminCandidates.includes((u.email || '').toLowerCase()))
      setCanManageRoles(Boolean(result.data?.can_manage_roles))
      setUsers(userList)
      if (userList.length > 0) {
        setSelectedUserID((prev) => (prev && userList.some((u) => u.id === prev) ? prev : userList[0].id))
      } else {
        setSelectedUserID('')
        setGrantedStrategyIDs(new Set())
      }
    } catch (error) {
      notify.error(getErrorMessage(error, i18n.errSearchUsers))
    } finally {
      setSearchingUsers(false)
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
      notify.error(getErrorMessage(error, i18n.errLoadPerms))
    } finally {
      setLoadingUserPerm(false)
    }
  }

  useEffect(() => {
    void loadStrategies()
  }, [])

  useEffect(() => {
    if (selectedUserID) {
      void loadUserPermissions(selectedUserID)
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
      notify.success(decodeUnicodeText(i18n.okSavePerms))
    } catch (error) {
      notify.error(getErrorMessage(error, i18n.errSavePerms))
    } finally {
      setSaving(false)
    }
  }

  const setUserRole = async (role: UserRole) => {
    if (!selectedUserID || !canManageRoles) return
    setUpdatingRole(true)
    try {
      const roleResult = await httpClient.put(`/api/admin/users/${selectedUserID}/role`, { role })
      if (!roleResult.success) {
        throw new Error(decodeUnicodeText(roleResult.message || i18n.errRoleUpdated))
      }
      notify.success(`${i18n.okRoleUpdated} ${role}`)
      await searchUsers()
    } catch (error) {
      notify.error(getErrorMessage(error, i18n.errRoleUpdated))
    } finally {
      setUpdatingRole(false)
    }
  }

  const onRefresh = async () => {
    await loadStrategies()
    if (hasSearched) {
      await searchUsers()
    }
  }

  if (loading) {
    return <div className="max-w-[1400px] mx-auto p-6 text-nofx-text-muted">{i18n.loading}</div>
  }

  return (
    <div className="max-w-[1400px] mx-auto p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-nofx-text flex items-center gap-2">
            <ShieldCheck className="w-6 h-6 text-nofx-gold" />
            {i18n.title}
          </h1>
          <p className="text-sm text-nofx-text-muted mt-1">{i18n.subtitle}</p>
        </div>
        <button
          onClick={onRefresh}
          className="inline-flex items-center gap-2 px-3 py-2 rounded-lg bg-nofx-bg-lighter border border-nofx-gold/20 text-nofx-text hover:bg-white/5"
        >
          <RefreshCw className="w-4 h-4" />
          {i18n.refresh}
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[320px_1fr] gap-4">
        <div className="rounded-lg border border-nofx-gold/20 bg-nofx-bg-lighter p-3">
          <div className="flex items-center gap-2 text-nofx-text mb-3">
            <Users className="w-4 h-4 text-nofx-gold" />
            <span className="text-sm font-semibold">{i18n.users}</span>
          </div>

          <div className="flex items-center gap-2 mb-3">
            <input
              value={keyword}
              onChange={(e) => setKeyword(decodeUnicodeText(e.target.value))}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  void searchUsers()
                }
              }}
              placeholder={i18n.searchPlaceholder}
              className="w-full px-3 py-2 rounded bg-nofx-bg border border-nofx-gold/20 text-sm text-nofx-text"
            />
            <button
              onClick={() => void searchUsers()}
              disabled={searchingUsers}
              className="px-3 py-2 rounded text-xs font-semibold bg-nofx-gold text-black disabled:opacity-40"
            >
              {searchingUsers ? i18n.searching : i18n.search}
            </button>
          </div>

          <div className="space-y-2 max-h-[520px] overflow-y-auto">
            {!hasSearched && <div className="text-sm text-nofx-text-muted">{i18n.inputHint}</div>}
            {hasSearched && users.length === 0 && <div className="text-sm text-nofx-text-muted">{i18n.noUsers}</div>}
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
              <div className="text-sm text-nofx-text-muted">{i18n.selectedUser}</div>
              <div className="text-base font-semibold text-nofx-text">{selectedUser?.email || '-'}</div>
              <div className="text-xs text-nofx-text-muted mt-1">
                {i18n.currentRole}: {selectedUser?.role || '-'}
              </div>
            </div>
            <div className="flex items-center gap-2">
              {canManageRoles && (
                <>
                  <button
                    onClick={() => void setUserRole('USER')}
                    disabled={!selectedUserID || updatingRole || selectedUser?.role === 'USER'}
                    className="px-3 py-1.5 rounded text-xs border border-nofx-gold/30 text-nofx-text disabled:opacity-40"
                  >
                    {i18n.setUser}
                  </button>
                  <button
                    onClick={() => void setUserRole('ADMIN')}
                    disabled={!selectedUserID || updatingRole || selectedUser?.role === 'ADMIN'}
                    className="px-3 py-1.5 rounded text-xs border border-nofx-gold/30 text-nofx-text disabled:opacity-40"
                  >
                    {i18n.setAdmin}
                  </button>
                </>
              )}
              <button
                onClick={selectAll}
                disabled={!selectedUserID || strategies.length === 0}
                className="px-3 py-1.5 rounded text-xs border border-nofx-gold/30 text-nofx-text disabled:opacity-40"
              >
                {i18n.selectAll}
              </button>
              <button
                onClick={clearAll}
                disabled={!selectedUserID}
                className="px-3 py-1.5 rounded text-xs border border-nofx-gold/30 text-nofx-text disabled:opacity-40"
              >
                {i18n.clear}
              </button>
              <button
                onClick={() => void savePermissions()}
                disabled={!selectedUserID || saving}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded text-xs font-semibold bg-nofx-gold text-black disabled:opacity-40"
              >
                <Save className="w-3.5 h-3.5" />
                {saving ? i18n.saving : i18n.save}
              </button>
            </div>
          </div>

          {loadingUserPerm ? (
            <div className="text-sm text-nofx-text-muted">{i18n.loadingPermissions}</div>
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
                      <div className="text-sm font-medium text-nofx-text truncate">{decodeUnicodeText(strategy.name || '')}</div>
                      <div className="text-xs text-nofx-text-muted mt-1 line-clamp-2">
                        {decodeUnicodeText(strategy.description || i18n.noDescription)}
                      </div>
                    </div>
                  </label>
                )
              })}
              {strategies.length === 0 && <div className="text-sm text-nofx-text-muted">{i18n.noStrategies}</div>}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default StrategyPermissionPage
