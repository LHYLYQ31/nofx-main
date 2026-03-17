import React, { useState } from 'react'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import { t } from '../i18n/translations'
import { Header } from './Header'
import { ArrowLeft, KeyRound, Eye, EyeOff } from 'lucide-react'
import PasswordChecklist from 'react-password-checklist'
import { Input } from './ui/input'
import { toast } from 'sonner'

export function ResetPasswordPage() {
  const { language } = useLanguage()
  const { resetPassword, sendResetPasswordCode } = useAuth()
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [loading, setLoading] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [passwordValid, setPasswordValid] = useState(false)
  const [sendingCode, setSendingCode] = useState(false)
  const [countdown, setCountdown] = useState(0)

  const tr = (zh: string, en: string, id: string) =>
    language === 'zh' ? zh : language === 'id' ? id : en

  const startCountdown = () => {
    setCountdown(60)
    const timer = setInterval(() => {
      setCountdown((v) => {
        if (v <= 1) {
          clearInterval(timer)
          return 0
        }
        return v - 1
      })
    }, 1000)
  }

  const handleSendCode = async () => {
    if (!email.trim()) {
      setError(tr('请先输入邮箱', 'Please enter email first', 'Masukkan email terlebih dahulu'))
      return
    }
    setError('')
    setSendingCode(true)
    const result = await sendResetPasswordCode(email.trim())
    setSendingCode(false)

    if (!result.success) {
      const msg = result.message || tr('发送验证码失败', 'Failed to send verification code', 'Gagal mengirim kode verifikasi')
      setError(msg)
      toast.error(msg)
      return
    }

    toast.success(result.message || tr('验证码已发送', 'Verification code sent', 'Kode verifikasi telah dikirim'))
    startCountdown()
  }

  const handleResetPassword = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setSuccess(false)

    if (!code.trim()) {
      setError(tr('请输入验证码', 'Please enter verification code', 'Masukkan kode verifikasi'))
      return
    }

    if (newPassword !== confirmPassword) {
      setError(t('passwordMismatch', language))
      return
    }

    setLoading(true)

    const result = await resetPassword(email, code.trim(), newPassword)

    if (result.success) {
      setSuccess(true)
      toast.success(t('resetPasswordSuccess', language) || tr('重置成功', 'Reset successful', 'Reset berhasil'))
      setTimeout(() => {
        window.history.pushState({}, '', '/login')
        window.dispatchEvent(new PopStateEvent('popstate'))
      }, 3000)
    } else {
      const msg = result.message || t('resetPasswordFailed', language)
      setError(msg)
      toast.error(msg)
    }

    setLoading(false)
  }

  return (
    <div className="min-h-screen" style={{ background: '#0B0E11' }}>
      <Header simple />

      <div className="flex items-center justify-center" style={{ minHeight: 'calc(100vh - 80px)' }}>
        <div className="w-full max-w-md">
          <button
            onClick={() => {
              window.history.pushState({}, '', '/login')
              window.dispatchEvent(new PopStateEvent('popstate'))
            }}
            className="flex items-center gap-2 mb-6 text-sm hover:text-[#F0B90B] transition-colors"
            style={{ color: '#848E9C' }}
          >
            <ArrowLeft className="w-4 h-4" />
            {t('backToLogin', language)}
          </button>

          <div className="text-center mb-8">
            <div className="w-16 h-16 mx-auto mb-4 flex items-center justify-center rounded-full" style={{ background: 'rgba(240, 185, 11, 0.1)' }}>
              <KeyRound className="w-8 h-8" style={{ color: '#F0B90B' }} />
            </div>
            <h1 className="text-2xl font-bold" style={{ color: '#EAECEF' }}>
              {t('resetPasswordTitle', language)}
            </h1>
            <p className="text-sm mt-2" style={{ color: '#848E9C' }}>
              {tr('先获取邮箱验证码，验证通过后才能重置密码', 'Get verification code by email, then reset password', 'Dapatkan kode verifikasi email, lalu reset password')}
            </p>
          </div>

          <div className="rounded-lg p-6" style={{ background: '#1E2329', border: '1px solid #2B3139' }}>
            {success ? (
              <div className="text-center py-8">
                <div className="text-5xl mb-4">OK</div>
                <p className="text-lg font-semibold mb-2" style={{ color: '#EAECEF' }}>
                  {t('resetPasswordSuccess', language)}
                </p>
                <p className="text-sm" style={{ color: '#848E9C' }}>
                  {tr('3秒后将自动跳转到登录页面', 'Redirecting to login in 3 seconds...', 'Akan dialihkan ke login dalam 3 detik...')}
                </p>
              </div>
            ) : (
              <form onSubmit={handleResetPassword} className="space-y-4">
                <div>
                  <label className="block text-sm font-semibold mb-2" style={{ color: '#EAECEF' }}>
                    {t('email', language)}
                  </label>
                  <div className="flex gap-2">
                    <Input
                      type="email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder={t('emailPlaceholder', language)}
                      required
                    />
                    <button
                      type="button"
                      onClick={handleSendCode}
                      disabled={sendingCode || countdown > 0}
                      className="px-3 rounded text-xs font-semibold disabled:opacity-50"
                      style={{ background: '#F0B90B', color: '#000' }}
                    >
                      {countdown > 0
                        ? `${countdown}s`
                        : sendingCode
                          ? tr('发送中...', 'Sending...', 'Mengirim...')
                          : tr('发送验证码', 'Send Code', 'Kirim Kode')}
                    </button>
                  </div>
                </div>

                <div>
                  <label className="block text-sm font-semibold mb-2" style={{ color: '#EAECEF' }}>
                    {tr('验证码', 'Verification Code', 'Kode Verifikasi')}
                  </label>
                  <Input
                    type="text"
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    placeholder={tr('请输入6位验证码', 'Enter 6-digit code', 'Masukkan kode 6 digit')}
                    required
                  />
                </div>

                <div>
                  <label className="block text-sm font-semibold mb-2" style={{ color: '#EAECEF' }}>
                    {t('newPassword', language)}
                  </label>
                  <div className="relative">
                    <Input
                      type={showPassword ? 'text' : 'password'}
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      className="pr-10"
                      placeholder={t('newPasswordPlaceholder', language)}
                      required
                    />
                    <button
                      type="button"
                      onMouseDown={(e) => e.preventDefault()}
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute inset-y-0 right-2 w-8 h-10 flex items-center justify-center btn-icon"
                      style={{ color: 'var(--text-secondary)' }}
                    >
                      {showPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                    </button>
                  </div>
                </div>

                <div>
                  <label className="block text-sm font-semibold mb-2" style={{ color: '#EAECEF' }}>
                    {t('confirmPassword', language)}
                  </label>
                  <div className="relative">
                    <Input
                      type={showConfirmPassword ? 'text' : 'password'}
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      className="pr-10"
                      placeholder={t('confirmPasswordPlaceholder', language)}
                      required
                    />
                    <button
                      type="button"
                      onMouseDown={(e) => e.preventDefault()}
                      onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                      className="absolute inset-y-0 right-2 w-8 h-10 flex items-center justify-center btn-icon"
                      style={{ color: 'var(--text-secondary)' }}
                    >
                      {showConfirmPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                    </button>
                  </div>
                </div>

                <div className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>
                  <div className="mb-1" style={{ color: 'var(--brand-light-gray)' }}>
                    {t('passwordRequirements', language)}
                  </div>
                  <PasswordChecklist
                    rules={['minLength', 'capital', 'lowercase', 'number', 'specialChar', 'match']}
                    minLength={8}
                    value={newPassword}
                    valueAgain={confirmPassword}
                    messages={{
                      minLength: t('passwordRuleMinLength', language),
                      capital: t('passwordRuleUppercase', language),
                      lowercase: t('passwordRuleLowercase', language),
                      number: t('passwordRuleNumber', language),
                      specialChar: t('passwordRuleSpecial', language),
                      match: t('passwordRuleMatch', language),
                    }}
                    className="space-y-1"
                    onChange={(isValid) => setPasswordValid(isValid)}
                  />
                </div>

                {error && (
                  <div className="text-sm px-3 py-2 rounded" style={{ background: 'rgba(246, 70, 93, 0.1)', color: '#F6465D' }}>
                    {error}
                  </div>
                )}

                <button
                  type="submit"
                  disabled={loading || !passwordValid}
                  className="w-full px-4 py-2 rounded text-sm font-semibold transition-all hover:scale-105 disabled:opacity-50"
                  style={{ background: '#F0B90B', color: '#000' }}
                >
                  {loading ? t('loading', language) : t('resetPasswordButton', language)}
                </button>
              </form>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
