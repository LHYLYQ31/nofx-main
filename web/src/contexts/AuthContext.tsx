import React, { createContext, useContext, useState, useEffect } from 'react'
import { getSystemConfig } from '../lib/config'
import { reset401Flag, httpClient } from '../lib/httpClient'

interface User {
  id: string
  email: string
  role: 'ADMIN' | 'USER'
}

interface AuthContextType {
  user: User | null
  token: string | null
  login: (
    email: string,
    password: string
  ) => Promise<{
    success: boolean
    message?: string
  }>
  loginAdmin: (password: string) => Promise<{
    success: boolean
    message?: string
  }>
  register: (
    email: string,
    password: string,
    betaCode?: string
  ) => Promise<{ success: boolean; message?: string }>
  resetPassword: (
    email: string,
    newPassword: string
  ) => Promise<{ success: boolean; message?: string }>
  logout: () => void
  isLoading: boolean
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)

	const normalizeUser = (raw: any): User => ({
		id: raw?.id || '',
		email: raw?.email || '',
		role: raw?.role === 'ADMIN' ? 'ADMIN' : 'USER',
	})

	useEffect(() => {
    // Reset 401 flag on page load to allow fresh 401 handling
    reset401Flag()

    // 鍏堟鏌ユ槸鍚︿负绠＄悊鍛樻ā寮忥紙浣跨敤甯︾紦瀛樼殑绯荤粺閰嶇疆鑾峰彇锛?
    getSystemConfig()
      .then(() => {
        // 涓嶅啀鍦ㄧ鐞嗗憳妯″紡涓嬫ā鎷熺櫥褰曪紱缁熶竴妫€鏌ユ湰鍦板瓨鍌?
        const savedToken = localStorage.getItem('auth_token')
        const savedUser = localStorage.getItem('auth_user')
		if (savedToken && savedUser) {
			setToken(savedToken)
			setUser(normalizeUser(JSON.parse(savedUser)))
		}

        setIsLoading(false)
      })
      .catch((err) => {
        console.error('Failed to fetch system config:', err)
        // 鍙戠敓閿欒鏃讹紝缁х画妫€鏌ユ湰鍦板瓨鍌?
        const savedToken = localStorage.getItem('auth_token')
        const savedUser = localStorage.getItem('auth_user')

		if (savedToken && savedUser) {
			setToken(savedToken)
			setUser(normalizeUser(JSON.parse(savedUser)))
		}
        setIsLoading(false)
      })
  }, [])

  // Listen for unauthorized events from httpClient (401 responses)
  useEffect(() => {
    const handleUnauthorized = () => {
      console.log('Unauthorized event received - clearing auth state')
      // Clear auth state when 401 is detected
      setUser(null)
      setToken(null)
      // Note: localStorage cleanup is already done in httpClient
    }

    window.addEventListener('unauthorized', handleUnauthorized)

    return () => {
      window.removeEventListener('unauthorized', handleUnauthorized)
    }
  }, [])

  const login = async (email: string, password: string) => {
    try {
      const response = await fetch('/api/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ email, password }),
      })

      const data = await response.json()

      if (response.ok) {
        if (data.token) {
          // Reset 401 flag on successful login
          reset401Flag()

          const userInfo = { id: data.user_id, email: data.email, role: data.role || 'USER' }
          setToken(data.token)
          setUser(userInfo)
          localStorage.setItem('auth_token', data.token)
          localStorage.setItem('auth_user', JSON.stringify(userInfo))

          // Check and redirect to returnUrl if exists
          const returnUrl = sessionStorage.getItem('returnUrl')
          if (returnUrl) {
            sessionStorage.removeItem('returnUrl')
            window.history.pushState({}, '', returnUrl)
            window.dispatchEvent(new PopStateEvent('popstate'))
          } else {
            // 璺宠浆鍒伴厤缃〉闈?
            window.history.pushState({}, '', '/traders')
            window.dispatchEvent(new PopStateEvent('popstate'))
          }

          return { success: true, message: data.message }
        }

        // Unexpected success response
        return { success: false, message: data.message || '鐧诲綍鍝嶅簲寮傚父' }
      } else {
        return {
          success: false,
          message: data.error,
        }
      }
    } catch (error) {
      return { success: false, message: '鐧诲綍澶辫触锛岃閲嶈瘯' }
    }
  }

  const loginAdmin = async (password: string) => {
    try {
      const adminEmail = (import.meta.env.VITE_SUPER_ADMIN_EMAIL || 'admin@example.com')
        .trim()
        .toLowerCase()

      const response = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: adminEmail, password }),
      })
      const data = await response.json()
      if (response.ok) {
        if (data.role !== 'ADMIN') {
          return { success: false, message: 'Current account is not admin' }
        }

        // Reset 401 flag on successful login
        reset401Flag()

        const userInfo = {
          id: data.user_id || 'admin',
          email: data.email || adminEmail,
          role: 'ADMIN' as const,
        }
        setToken(data.token)
        setUser(userInfo)
        localStorage.setItem('auth_token', data.token)
        localStorage.setItem('auth_user', JSON.stringify(userInfo))

        // Check and redirect to returnUrl if exists
        const returnUrl = sessionStorage.getItem('returnUrl')
        if (returnUrl) {
          sessionStorage.removeItem('returnUrl')
          window.history.pushState({}, '', returnUrl)
          window.dispatchEvent(new PopStateEvent('popstate'))
        } else {
          // 璺宠浆鍒颁华琛ㄧ洏
          window.history.pushState({}, '', '/dashboard')
          window.dispatchEvent(new PopStateEvent('popstate'))
        }
        return { success: true }
      } else {
        return { success: false, message: data.error || 'Login failed' }
      }
    } catch (e) {
      return { success: false, message: 'Login failed, please retry' }
    }
  }

  const register = async (
    email: string,
    password: string,
    betaCode?: string
  ) => {
    const requestBody: {
      email: string
      password: string
      beta_code?: string
    } = { email, password }
    if (betaCode) {
      requestBody.beta_code = betaCode
    }

    try {
      const result = await httpClient.post<{
        token: string
        user_id: string
        email: string
        role: 'ADMIN' | 'USER'
        message: string
      }>('/api/register', requestBody)

      if (result.success && result.data) {
        // Reset 401 flag on successful login
        reset401Flag()

        const userInfo = { id: result.data.user_id, email: result.data.email, role: result.data.role || 'USER' }
        setToken(result.data.token)
        setUser(userInfo)
        localStorage.setItem('auth_token', result.data.token)
        localStorage.setItem('auth_user', JSON.stringify(userInfo))

        // Check and redirect to returnUrl if exists
        const returnUrl = sessionStorage.getItem('returnUrl')
        if (returnUrl) {
          sessionStorage.removeItem('returnUrl')
          window.history.pushState({}, '', returnUrl)
          window.dispatchEvent(new PopStateEvent('popstate'))
        } else {
          // 璺宠浆鍒伴厤缃〉闈?
          window.history.pushState({}, '', '/traders')
          window.dispatchEvent(new PopStateEvent('popstate'))
        }

        return {
          success: true,
          message: result.message || result.data.message,
        }
      }

      // Only business errors reach here (system/network errors were intercepted)
      return {
        success: false,
        message: result.message || 'Registration failed',
      }
    } catch (error) {
      console.error('Auth register error:', error);
      // Re-throw if it's a critical error, or return structured error
      // Since httpClient throws on 500, we should return a structured error response
      // to let the UI display it gracefully without crashing.
      return {
        success: false,
        message: error instanceof Error ? error.message : 'Detailed server error'
      }
    }
  }

  const resetPassword = async (email: string, newPassword: string) => {
    try {
      const response = await fetch('/api/reset-password', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          email,
          new_password: newPassword,
        }),
      })

      const data = await response.json()

      if (response.ok) {
        return { success: true, message: data.message }
      } else {
        return { success: false, message: data.error }
      }
    } catch (error) {
      return { success: false, message: '瀵嗙爜閲嶇疆澶辫触锛岃閲嶈瘯' }
    }
  }

  const logout = () => {
    const savedToken = localStorage.getItem('auth_token')
    if (savedToken) {
      fetch('/api/logout', {
        method: 'POST',
        headers: { Authorization: `Bearer ${savedToken}` },
      }).catch(() => {
        /* ignore network errors on logout */
      })
    }
    setUser(null)
    setToken(null)
    localStorage.removeItem('auth_token')
    localStorage.removeItem('auth_user')
  }

  return (
    <AuthContext.Provider
      value={{
        user,
        token,
        login,
        loginAdmin,
        register,
        resetPassword,
        logout,
        isLoading,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}
