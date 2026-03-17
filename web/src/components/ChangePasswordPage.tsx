import React, { useState } from 'react'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import { Header } from './Header'
import { ArrowLeft, Eye, EyeOff, KeyRound } from 'lucide-react'
import PasswordChecklist from 'react-password-checklist'
import { Input } from './ui/input'
import { toast } from 'sonner'

export function ChangePasswordPage() {
  const { language } = useLanguage()
  const { user, changePassword } = useAuth()

  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [showOld, setShowOld] = useState(false)
  const [showNew, setShowNew] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)
  const [passwordValid, setPasswordValid] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const tr = (zh: string, en: string, id: string) => (language === 'zh' ? zh : language === 'id' ? id : en)

  const goLogin = () => {
    window.history.pushState({}, '', '/login')
    window.dispatchEvent(new PopStateEvent('popstate'))
  }

  const goDashboard = () => {
    window.history.pushState({}, '', '/dashboard')
    window.dispatchEvent(new PopStateEvent('popstate'))
  }

  if (!user) {
    return (
      <div className="min-h-screen" style={{ background: '#0B0E11' }}>
        <Header simple />
        <div className="flex items-center justify-center" style={{ minHeight: 'calc(100vh - 80px)' }}>
          <div className="text-center">
            <p style={{ color: '#EAECEF' }}>{tr('请先登录后再修改密码', 'Please login first to change password', 'Silakan login terlebih dahulu untuk mengubah password')}</p>
            <button onClick={goLogin} className="mt-4 px-4 py-2 rounded" style={{ background: '#F0B90B', color: '#000' }}>
              {tr('去登录', 'Go Login', 'Ke Login')}
            </button>
          </div>
        </div>
      </div>
    )
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setSuccess('')

    if (newPassword !== confirmPassword) {
      setError(tr('两次输入的新密码不一致', 'New passwords do not match', 'Password baru tidak cocok'))
      return
    }
    if (oldPassword === newPassword) {
      setError(tr('新密码不能与旧密码相同', 'New password must be different from old password', 'Password baru harus berbeda dari password lama'))
      return
    }

    setLoading(true)
    const result = await changePassword(oldPassword, newPassword)
    setLoading(false)

    if (!result.success) {
      const msg = result.message || tr('修改密码失败', 'Failed to change password', 'Gagal mengubah password')
      setError(msg)
      toast.error(msg)
      return
    }

    const okMsg = result.message || tr('密码修改成功', 'Password changed successfully', 'Password berhasil diubah')
    setSuccess(okMsg)
    toast.success(okMsg)
    setOldPassword('')
    setNewPassword('')
    setConfirmPassword('')
  }

  return (
    <div className="min-h-screen" style={{ background: '#0B0E11' }}>
      <Header simple />
      <div className="flex items-center justify-center" style={{ minHeight: 'calc(100vh - 80px)' }}>
        <div className="w-full max-w-md">
          <button
            onClick={goDashboard}
            className="flex items-center gap-2 mb-6 text-sm hover:text-[#F0B90B] transition-colors"
            style={{ color: '#848E9C' }}
          >
            <ArrowLeft className="w-4 h-4" />
            {tr('返回', 'Back', 'Kembali')}
          </button>

          <div className="text-center mb-8">
            <div className="w-16 h-16 mx-auto mb-4 flex items-center justify-center rounded-full" style={{ background: 'rgba(240, 185, 11, 0.1)' }}>
              <KeyRound className="w-8 h-8" style={{ color: '#F0B90B' }} />
            </div>
            <h1 className="text-2xl font-bold" style={{ color: '#EAECEF' }}>
              {tr('修改密码', 'Change Password', 'Ubah Password')}
            </h1>
          </div>

          <div className="rounded-lg p-6" style={{ background: '#1E2329', border: '1px solid #2B3139' }}>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-semibold mb-2" style={{ color: '#EAECEF' }}>
                  {tr('当前密码', 'Current Password', 'Password Saat Ini')}
                </label>
                <div className="relative">
                  <Input
                    type={showOld ? 'text' : 'password'}
                    value={oldPassword}
                    onChange={(e) => setOldPassword(e.target.value)}
                    className="pr-10"
                    required
                  />
                  <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => setShowOld(!showOld)} className="absolute inset-y-0 right-2 w-8 h-10 flex items-center justify-center btn-icon" style={{ color: 'var(--text-secondary)' }}>
                    {showOld ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-sm font-semibold mb-2" style={{ color: '#EAECEF' }}>
                  {tr('新密码', 'New Password', 'Password Baru')}
                </label>
                <div className="relative">
                  <Input
                    type={showNew ? 'text' : 'password'}
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    className="pr-10"
                    required
                  />
                  <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => setShowNew(!showNew)} className="absolute inset-y-0 right-2 w-8 h-10 flex items-center justify-center btn-icon" style={{ color: 'var(--text-secondary)' }}>
                    {showNew ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-sm font-semibold mb-2" style={{ color: '#EAECEF' }}>
                  {tr('确认新密码', 'Confirm New Password', 'Konfirmasi Password Baru')}
                </label>
                <div className="relative">
                  <Input
                    type={showConfirm ? 'text' : 'password'}
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    className="pr-10"
                    required
                  />
                  <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => setShowConfirm(!showConfirm)} className="absolute inset-y-0 right-2 w-8 h-10 flex items-center justify-center btn-icon" style={{ color: 'var(--text-secondary)' }}>
                    {showConfirm ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                  </button>
                </div>
              </div>

              <div className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>
                <PasswordChecklist
                  rules={['minLength', 'capital', 'lowercase', 'number', 'specialChar', 'match']}
                  minLength={8}
                  value={newPassword}
                  valueAgain={confirmPassword}
                  messages={{
                    minLength: tr('至少 8 位', 'At least 8 characters', 'Minimal 8 karakter'),
                    capital: tr('包含大写字母', 'Contains uppercase letter', 'Mengandung huruf besar'),
                    lowercase: tr('包含小写字母', 'Contains lowercase letter', 'Mengandung huruf kecil'),
                    number: tr('包含数字', 'Contains number', 'Mengandung angka'),
                    specialChar: tr('包含特殊字符', 'Contains special character', 'Mengandung karakter khusus'),
                    match: tr('两次输入一致', 'Passwords match', 'Password cocok'),
                  }}
                  className="space-y-1"
                  onChange={(isValid) => setPasswordValid(isValid)}
                />
              </div>

              {error && <div className="text-sm px-3 py-2 rounded" style={{ background: 'rgba(246, 70, 93, 0.1)', color: '#F6465D' }}>{error}</div>}
              {success && <div className="text-sm px-3 py-2 rounded" style={{ background: 'rgba(14, 203, 129, 0.1)', color: '#0ECB81' }}>{success}</div>}

              <button
                type="submit"
                disabled={loading || !passwordValid}
                className="w-full px-4 py-2 rounded text-sm font-semibold transition-all hover:scale-105 disabled:opacity-50"
                style={{ background: '#F0B90B', color: '#000' }}
              >
                {loading ? tr('提交中...', 'Submitting...', 'Mengirim...') : tr('确认修改', 'Change Password', 'Ubah Password')}
              </button>
            </form>
          </div>
        </div>
      </div>
    </div>
  )
}
