import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { authApi } from '../../api/client'
import { storeSession, useSession } from './session'

export function Login() {
  const session = useSession()
  const navigate = useNavigate()
  const location = useLocation()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const requestedDestination = typeof location.state === 'object' && location.state && 'from' in location.state && typeof location.state.from === 'string' ? location.state.from : ''
  const destination = requestedDestination.startsWith('/') && !requestedDestination.startsWith('//') ? requestedDestination : '/projects'
  const mutation = useMutation({
    mutationFn: authApi.login,
    onSuccess: (result) => {
      storeSession(result.access_token, result.expires_in)
      navigate(destination, { replace: true })
    },
  })
  if (session) return <Navigate to={destination} replace />
  const submit = (event: FormEvent) => {
    event.preventDefault()
    mutation.mutate({ username: username.trim(), password })
  }
  return <main className="login-page"><section className="login-card"><div className="login-brand"><span>O</span><strong>ORAG</strong></div><span className="eyebrow">Control plane</span><h1>登录 ORAG Console</h1><p>使用租户管理员账号管理项目、评测资产和机器凭证。</p><form onSubmit={submit}><label>用户名<input autoFocus autoComplete="username" required value={username} onChange={(event) => setUsername(event.target.value)} /></label><label>密码<input autoComplete="current-password" required type="password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>{mutation.isError ? <p className="login-error" role="alert">用户名或密码错误。</p> : null}<button className="primary-button" disabled={!username.trim() || !password || mutation.isPending}>{mutation.isPending ? '登录中…' : '登录'}</button></form><small>会话仅保存在当前浏览器标签页中，关闭标签页后需要重新登录。</small></section></main>
}
