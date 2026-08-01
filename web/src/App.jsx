import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  Album,
  AlertCircle,
  Archive,
  AudioLines,
  Check,
  ChevronDown,
  CircleStop,
  Clock3,
  Disc3,
  Download,
  ExternalLink,
  FileAudio,
  Film,
  FolderOpen,
  HardDrive,
  History,
  KeyRound,
  ListMusic,
  LoaderCircle,
  LogOut,
  Menu,
  Music2,
  RefreshCw,
  RotateCw,
  Save,
  Server,
  Settings,
  SlidersHorizontal,
  Trash2,
  UserRound,
  UsersRound,
  X,
  XCircle,
} from 'lucide-react'

const MODES = [
  { id: 'album', label: '专辑', Icon: Album },
  { id: 'song', label: '单曲', Icon: Music2 },
  { id: 'playlist', label: '播放列表', Icon: ListMusic },
  { id: 'atmos', label: '杜比全景声', Icon: AudioLines },
  { id: 'aac', label: 'AAC', Icon: FileAudio },
  { id: 'video', label: '音乐视频', Icon: Film },
  { id: 'artist', label: '艺术家', Icon: UsersRound },
]

const NAV_ITEMS = [
  { id: 'download', label: '下载', Icon: Download },
  { id: 'tasks', label: '任务记录', Icon: History },
  { id: 'settings', label: '设置', Icon: Settings },
  { id: 'system', label: '系统状态', Icon: Activity },
]

const stateLabels = {
  queued: '等待中',
  running: '下载中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
}

const modeLabels = Object.fromEntries(MODES.map((item) => [item.id, item.label]))

async function api(path, options = {}) {
  const response = await fetch(path, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  })
  if (!response.ok) {
    let detail = `请求失败 (${response.status})`
    try {
      const body = await response.json()
      detail = body.detail || body.error || detail
      if (Array.isArray(detail)) detail = detail[0]?.msg || '输入内容无效'
    } catch {
      // Preserve the status-based fallback.
    }
    const error = new Error(detail)
    error.status = response.status
    throw error
  }
  return response.json()
}

function formatBytes(bytes = 0) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** index).toFixed(index > 1 ? 1 : 0)} ${units[index]}`
}

function formatTime(timestamp) {
  if (!timestamp) return '--'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp * 1000))
}

function shortUrl(url = '') {
  try {
    const parsed = new URL(url)
    return decodeURIComponent(parsed.pathname.split('/').filter(Boolean).slice(-2).join(' / '))
  } catch {
    return url
  }
}

function StatusDot({ active, warning = false }) {
  return <span className={`status-dot ${active ? 'active' : warning ? 'warning' : ''}`} />
}

function Toast({ toast, onClose }) {
  if (!toast) return null
  const Icon = toast.type === 'error' ? XCircle : Check
  return (
    <div className={`toast ${toast.type || 'success'}`} role="status">
      <Icon size={17} />
      <span>{toast.message}</span>
      <button className="icon-btn compact" onClick={onClose} title="关闭">
        <X size={15} />
      </button>
    </div>
  )
}

function AuthScreen({ onAuthenticated }) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(event) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api('/api/auth/login', { method: 'POST', body: JSON.stringify({ password }) })
      onAuthenticated()
    } catch (requestError) {
      setError(requestError.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="auth-screen">
      <section className="auth-panel">
        <div className="brand-mark large"><Music2 size={26} strokeWidth={2.2} /></div>
        <p className="eyebrow">AM download</p>
        <h1>访问控制台</h1>
        <form onSubmit={submit}>
          <label htmlFor="app-password">访问密码</label>
          <div className="input-with-icon">
            <KeyRound size={17} />
            <input id="app-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoFocus />
          </div>
          {error && <p className="field-error">{error}</p>}
          <button className="primary-btn full" disabled={busy || !password}>
            {busy ? <LoaderCircle size={17} className="spin" /> : <ExternalLink size={17} />}
            进入
          </button>
        </form>
      </section>
    </main>
  )
}

function SignInDialog({ open, onClose, onDone }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [twoFactorCode, setTwoFactorCode] = useState('')
  const [step, setStep] = useState('credentials')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) {
      setPassword('')
      setTwoFactorCode('')
      setStep('credentials')
      setError('')
    }
  }, [open])

  if (!open) return null

  async function submit(event) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      if (step === 'two-factor') {
        await api('/api/wrapper/2fa', {
          method: 'POST',
          body: JSON.stringify({ code: twoFactorCode }),
        })
        setTwoFactorCode('')
        onDone()
        return
      }

      const result = await api('/api/wrapper/login', {
          method: 'POST',
          body: JSON.stringify({ username, password }),
        })
      setPassword('')
      if (result.requires_2fa) {
        setStep('two-factor')
        return
      }
      onDone()
    } catch (requestError) {
      if (step === 'two-factor') {
        setTwoFactorCode('')
        setStep('credentials')
        setError(`双重认证失败，请重新登录：${requestError.message}`)
      } else {
        setError(requestError.message)
      }
    } finally {
      setBusy(false)
    }
  }

  async function cancel() {
    if (step === 'two-factor') {
      setBusy(true)
      try {
        await api('/api/wrapper/login/cancel', { method: 'POST', body: '{}' })
      } catch {
        // The Wrapper also ends the pending login automatically after its timeout.
      } finally {
        setBusy(false)
      }
    }
    onClose()
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <section className="modal" role="dialog" aria-modal="true" aria-labelledby="signin-title">
        <header className="modal-header">
          <div>
            <span className="section-icon">{step === 'two-factor' ? <KeyRound size={18} /> : <UserRound size={18} />}</span>
            <h2 id="signin-title">{step === 'two-factor' ? '双重认证' : '登录 Apple Music'}</h2>
          </div>
          <button className="icon-btn" onClick={cancel} disabled={busy} title="关闭"><X size={18} /></button>
        </header>
        <form onSubmit={submit} className="modal-body">
          {step === 'credentials' ? (
            <>
              <div className="field-group">
                <label htmlFor="apple-id">Apple ID</label>
                <input id="apple-id" type="email" value={username} onChange={(event) => setUsername(event.target.value)} placeholder="name@example.com" autoComplete="username" required />
              </div>
              <div className="field-group">
                <label htmlFor="apple-password">密码</label>
                <input id="apple-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required />
              </div>
            </>
          ) : (
            <>
              <div className="inline-alert info"><KeyRound size={16} /><span>请输入受信任设备上显示的六位 Apple 验证码。</span></div>
              <div className="field-group">
                <label htmlFor="apple-two-factor">六位验证码</label>
                <input
                  id="apple-two-factor"
                  type="text"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={twoFactorCode}
                  onChange={(event) => setTwoFactorCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
                  autoFocus
                  required
                />
              </div>
            </>
          )}
          {error && <div className="inline-alert error"><AlertCircle size={16} /><span>{error}</span></div>}
          <footer className="modal-actions">
            <button type="button" className="secondary-btn" onClick={cancel} disabled={busy}>取消</button>
            <button className="primary-btn" disabled={busy || (step === 'credentials' ? !username || !password : twoFactorCode.length !== 6)}>
              {busy ? <LoaderCircle className="spin" size={17} /> : <KeyRound size={17} />}
              {busy ? '正在验证' : step === 'two-factor' ? '提交验证码' : '登录'}
            </button>
          </footer>
        </form>
      </section>
    </div>
  )
}

function TaskItem({ task, onCancel, expanded, onToggle }) {
  const StateIcon = task.state === 'completed' ? Check : task.state === 'failed' ? XCircle : task.state === 'running' ? LoaderCircle : task.state === 'cancelled' ? CircleStop : Clock3
  const canCancel = task.state === 'queued' || task.state === 'running'
  return (
    <article className={`task-item state-${task.state}`}>
      <div className="task-main">
        <span className="task-state-icon"><StateIcon size={18} className={task.state === 'running' ? 'spin' : ''} /></span>
        <div className="task-copy">
          <div className="task-title-row">
            <strong>{modeLabels[task.kind] || task.kind}</strong>
            <span className={`state-pill ${task.state}`}>{stateLabels[task.state]}</span>
          </div>
          <span className="task-url" title={task.url}>{shortUrl(task.url)}</span>
          <span className="task-message">{task.message}</span>
        </div>
        <div className="task-actions">
          <time>{formatTime(task.created_at)}</time>
          {canCancel && (
            <button className="icon-btn" onClick={() => onCancel(task.id)} title="取消任务"><CircleStop size={17} /></button>
          )}
          <button className={`icon-btn ${expanded ? 'open' : ''}`} onClick={onToggle} title="任务日志"><ChevronDown size={17} /></button>
        </div>
      </div>
      {(task.state === 'running' || task.state === 'queued') && (
        <div className="progress-track"><span style={{ width: `${Math.max(task.progress, task.state === 'running' ? 4 : 0)}%` }} /></div>
      )}
      {expanded && (
        <pre className="task-log">{task.logs?.length ? task.logs.join('\n') : '暂无日志'}</pre>
      )}
    </article>
  )
}

function DownloadPage({ status, settings, tasks, onCreate, onCancel, onSignIn }) {
  const [kind, setKind] = useState('album')
  const [url, setUrl] = useState('')
  const [advanced, setAdvanced] = useState(false)
  const [busy, setBusy] = useState(false)
  const [expanded, setExpanded] = useState(null)
  const recent = tasks.filter((task) => ['queued', 'running'].includes(task.state)).slice(0, 6)
  const wrapper = status?.wrapper || {}

  async function submit(event) {
    event.preventDefault()
    setBusy(true)
    try {
      await onCreate({
        kind,
        url,
        aac_type: settings?.aac_type || 'aac-lc',
        alac_max: settings?.alac_max || 192000,
        atmos_max: settings?.atmos_max || 2768,
        mv_audio_type: settings?.mv_audio_type || 'atmos',
        mv_max: settings?.mv_max || 2160,
      })
      setUrl('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="page-stack">
      {!wrapper.logged_in && (
        <section className="setup-band">
          <div className="setup-symbol"><KeyRound size={21} /></div>
          <div><strong>Apple Music 尚未登录</strong><span>Wrapper 已就绪，完成账户验证后即可创建下载任务。</span></div>
          <button className="primary-btn" onClick={onSignIn}><UserRound size={17} />登录</button>
        </section>
      )}

      <section className="workspace-section">
        <div className="section-heading">
          <div><p className="eyebrow">新建任务</p><h2>添加 Apple Music 下载</h2></div>
          <div className="service-state"><StatusDot active={wrapper.wrapper_running} warning={!wrapper.wrapper_running} />{wrapper.wrapper_running ? '服务可用' : '服务未就绪'}</div>
        </div>

        <form className="download-form" onSubmit={submit}>
          <div className="mode-grid" role="radiogroup" aria-label="下载类型">
            {MODES.map(({ id, label, Icon }) => (
              <button key={id} type="button" className={kind === id ? 'selected' : ''} onClick={() => setKind(id)} aria-pressed={kind === id}>
                <Icon size={18} /><span>{label}</span>
              </button>
            ))}
          </div>
          <div className="url-row">
            <div className="url-input"><Music2 size={18} /><input type="url" value={url} onChange={(event) => setUrl(event.target.value)} placeholder="https://music.apple.com/..." required /></div>
            <button className="primary-btn submit-download" disabled={busy || !url || !wrapper.logged_in || !wrapper.wrapper_running}>
              {busy ? <LoaderCircle className="spin" size={18} /> : <Download size={18} />}
              加入队列
            </button>
          </div>
          <button type="button" className="text-btn" onClick={() => setAdvanced((value) => !value)}>
            <SlidersHorizontal size={16} />当前参数
            <ChevronDown size={15} className={advanced ? 'rotate' : ''} />
          </button>
          {advanced && (
            <div className="parameter-strip">
              <span>ALAC <b>{Math.round((settings?.alac_max || 192000) / 1000)} kHz</b></span>
              <span>Atmos <b>{settings?.atmos_max || 2768} kbps</b></span>
              <span>AAC <b>{settings?.aac_type || 'aac-lc'}</b></span>
              <span>MV <b>{settings?.mv_max || 2160}p</b></span>
            </div>
          )}
        </form>
      </section>

      <section className="workspace-section task-section">
        <div className="section-heading compact-heading">
          <div><p className="eyebrow">队列</p><h2>正在处理</h2></div>
          <span className="count-label">{recent.length} 个任务</span>
        </div>
        <div className="task-list">
          {recent.length ? recent.map((task) => (
            <TaskItem key={task.id} task={task} onCancel={onCancel} expanded={expanded === task.id} onToggle={() => setExpanded(expanded === task.id ? null : task.id)} />
          )) : (
            <div className="empty-state"><Archive size={28} /><strong>队列为空</strong></div>
          )}
        </div>
      </section>
    </div>
  )
}

function TasksPage({ tasks, onCancel, onClear }) {
  const [filter, setFilter] = useState('all')
  const [expanded, setExpanded] = useState(null)
  const visible = filter === 'all' ? tasks : tasks.filter((task) => task.state === filter)
  return (
    <div className="page-stack">
      <section className="workspace-section full-section">
        <div className="section-heading">
          <div><p className="eyebrow">历史</p><h2>任务记录</h2></div>
          <button className="secondary-btn" onClick={onClear}><Trash2 size={16} />清理已结束</button>
        </div>
        <div className="filter-tabs">
          {[['all', '全部'], ['running', '下载中'], ['completed', '已完成'], ['failed', '失败']].map(([id, label]) => (
            <button key={id} className={filter === id ? 'active' : ''} onClick={() => setFilter(id)}>{label}</button>
          ))}
        </div>
        <div className="task-list history-list">
          {visible.length ? visible.map((task) => (
            <TaskItem key={task.id} task={task} onCancel={onCancel} expanded={expanded === task.id} onToggle={() => setExpanded(expanded === task.id ? null : task.id)} />
          )) : <div className="empty-state tall"><History size={30} /><strong>没有匹配的任务</strong></div>}
        </div>
      </section>
    </div>
  )
}

function Toggle({ checked, onChange, label }) {
  return (
    <label className="toggle-row">
      <span>{label}</span>
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <i aria-hidden="true" />
    </label>
  )
}

function SettingsPage({ initial, onSave }) {
  const [form, setForm] = useState(initial || {})
  const [busy, setBusy] = useState(false)
  useEffect(() => setForm(initial || {}), [initial])
  const set = (key, value) => setForm((current) => ({ ...current, [key]: value }))

  async function submit(event) {
    event.preventDefault()
    setBusy(true)
    try { await onSave(form) } finally { setBusy(false) }
  }

  return (
    <form className="settings-layout" onSubmit={submit}>
      <section className="settings-band">
        <div className="settings-label"><span className="section-icon"><UserRound size={18} /></span><div><h2>账户与区域</h2><p>Apple Music API</p></div></div>
        <div className="settings-fields two-column">
          <div className="field-group"><label>Storefront</label><input value={form.storefront || ''} maxLength={2} onChange={(event) => set('storefront', event.target.value.toLowerCase())} /></div>
          <div className="field-group"><label>歌词语言</label><input value={form.language || ''} placeholder="留空自动选择" onChange={(event) => set('language', event.target.value)} /></div>
          <div className="field-group span-two"><label>Media user token {form.media_user_token_set && <span className="saved-badge">已保存</span>}</label><input type="password" value={form.media_user_token || ''} placeholder={form.media_user_token_set ? '输入新值以替换' : '用于歌词、AAC-LC 与 MV'} onChange={(event) => set('media_user_token', event.target.value)} /></div>
        </div>
      </section>

      <section className="settings-band">
        <div className="settings-label"><span className="section-icon"><AudioLines size={18} /></span><div><h2>音频质量</h2><p>下载上限</p></div></div>
        <div className="settings-fields two-column">
          <div className="field-group"><label>ALAC 采样率</label><select value={form.alac_max || 192000} onChange={(event) => set('alac_max', Number(event.target.value))}><option value="192000">192 kHz</option><option value="96000">96 kHz</option><option value="48000">48 kHz</option><option value="44100">44.1 kHz</option></select></div>
          <div className="field-group"><label>Atmos 码率</label><select value={form.atmos_max || 2768} onChange={(event) => set('atmos_max', Number(event.target.value))}><option value="2768">2768 kbps</option><option value="2448">2448 kbps</option></select></div>
          <div className="field-group"><label>AAC 类型</label><select value={form.aac_type || 'aac-lc'} onChange={(event) => set('aac_type', event.target.value)}><option value="aac-lc">AAC-LC</option><option value="aac">AAC</option><option value="aac-binaural">AAC Binaural</option><option value="aac-downmix">AAC Downmix</option></select></div>
          <div className="field-group"><label>MV 分辨率</label><select value={form.mv_max || 2160} onChange={(event) => set('mv_max', Number(event.target.value))}><option value="2160">2160p</option><option value="1080">1080p</option><option value="720">720p</option><option value="480">480p</option></select></div>
        </div>
      </section>

      <section className="settings-band">
        <div className="settings-label"><span className="section-icon"><Disc3 size={18} /></span><div><h2>元数据与转换</h2><p>输出处理</p></div></div>
        <div className="settings-fields toggle-stack">
          <Toggle label="嵌入封面" checked={!!form.embed_cover} onChange={(value) => set('embed_cover', value)} />
          <Toggle label="嵌入歌词" checked={!!form.embed_lrc} onChange={(value) => set('embed_lrc', value)} />
          {form.embed_lrc && <div className="field-group inline-select"><label>嵌入歌词格式</label><select value={form.embed_lrc_type || 'word'} onChange={(event) => set('embed_lrc_type', event.target.value)}><option value="word">逐字</option><option value="line">逐行</option></select></div>}
          <Toggle label="保存独立歌词文件" checked={!!form.save_lrc_file} onChange={(value) => set('save_lrc_file', value)} />
          {form.save_lrc_file && <div className="field-group inline-select"><label>独立歌词格式</label><select value={form.save_lrc_file_type || 'line'} onChange={(event) => set('save_lrc_file_type', event.target.value)}><option value="word">逐字</option><option value="line">逐行</option></select></div>}
          <Toggle label="下载后转换" checked={!!form.convert_after_download} onChange={(value) => set('convert_after_download', value)} />
          {form.convert_after_download && <div className="field-group inline-select"><label>转换格式</label><select value={form.convert_format || 'flac'} onChange={(event) => set('convert_format', event.target.value)}><option value="flac">FLAC</option><option value="mp3">MP3</option><option value="opus">Opus</option><option value="wav">WAV</option><option value="copy">复制封装</option></select></div>}
        </div>
      </section>

      <section className="settings-band">
        <div className="settings-label"><span className="section-icon"><FolderOpen size={18} /></span><div><h2>命名格式</h2><p>文件与目录</p></div></div>
        <div className="settings-fields">
          <div className="field-group"><label>专辑目录</label><input value={form.album_folder_format || ''} onChange={(event) => set('album_folder_format', event.target.value)} /></div>
          <div className="field-group"><label>播放列表目录</label><input value={form.playlist_folder_format || ''} onChange={(event) => set('playlist_folder_format', event.target.value)} /></div>
          <div className="field-group"><label>歌曲文件</label><input value={form.song_file_format || ''} onChange={(event) => set('song_file_format', event.target.value)} /></div>
          <div className="field-group"><label>艺术家目录</label><input value={form.artist_folder_format || ''} onChange={(event) => set('artist_folder_format', event.target.value)} /></div>
        </div>
      </section>
      <div className="sticky-save"><button className="primary-btn" disabled={busy}><Save size={17} />{busy ? '正在保存' : '保存设置'}</button></div>
    </form>
  )
}

function SystemPage({ status, onRefresh, onRestart, onSignIn }) {
  const [logs, setLogs] = useState([])
  const [loadingLogs, setLoadingLogs] = useState(false)
  const wrapper = status?.wrapper || {}
  const storage = status?.storage || {}
  const usedPercent = storage.total ? Math.round((storage.used / storage.total) * 100) : 0

  async function loadLogs() {
    setLoadingLogs(true)
    try { setLogs((await api('/api/wrapper/logs')).lines || []) } finally { setLoadingLogs(false) }
  }

  return (
    <div className="page-stack">
      <section className="system-grid">
        <article className="metric-panel"><span className="metric-icon"><Server size={20} /></span><div><span>Wrapper</span><strong>{wrapper.wrapper_running ? '运行中' : '未运行'}</strong></div><StatusDot active={wrapper.wrapper_running} warning={!wrapper.wrapper_running} /></article>
        <article className="metric-panel"><span className="metric-icon"><UserRound size={20} /></span><div><span>Apple Music</span><strong>{wrapper.logged_in ? '已登录' : '未登录'}</strong></div><StatusDot active={wrapper.logged_in} /></article>
        <article className="metric-panel"><span className="metric-icon"><HardDrive size={20} /></span><div><span>下载存储</span><strong>{formatBytes(storage.free)} 可用</strong></div><span className="metric-small">{usedPercent}%</span></article>
        <article className="metric-panel"><span className="metric-icon"><Archive size={20} /></span><div><span>任务</span><strong>{status?.tasks?.completed || 0} 已完成</strong></div><span className="metric-small">{status?.tasks?.running || 0} 运行</span></article>
      </section>
      <section className="workspace-section system-details">
        <div className="section-heading"><div><p className="eyebrow">运行状态</p><h2>服务组件</h2></div><button className="icon-btn" onClick={onRefresh} title="刷新"><RefreshCw size={17} /></button></div>
        <div className="status-table">
          <div><span>Web 服务</span><b><StatusDot active />AM download {status?.version || 'v1.0.4'}</b></div>
          <div><span>Wrapper 管理器</span><b><StatusDot active={wrapper.manager === 'ready'} />{wrapper.manager === 'ready' ? '可用' : '不可用'}</b></div>
          <div><span>解密服务</span><b><StatusDot active={wrapper.wrapper_running} />{wrapper.wrapper_running ? '端口已监听' : '未启动'}</b></div>
          <div><span>FairPlay</span><b><StatusDot active={wrapper.fairplay_registered} warning={wrapper.logged_in && !wrapper.fairplay_registered} />{wrapper.fairplay_registered ? '已注册' : '未注册'}</b></div>
          <div><span>持久化目录</span><code>{storage.path || '/downloads'}</code></div>
        </div>
        <div className="system-actions">
          {!wrapper.logged_in && <button className="primary-btn" onClick={onSignIn}><UserRound size={16} />登录 Apple Music</button>}
          <button className="secondary-btn" onClick={onRestart} disabled={!wrapper.logged_in}><RotateCw size={16} />重启 Wrapper</button>
          <button className="secondary-btn" onClick={loadLogs}><FileAudio size={16} />{loadingLogs ? '读取中' : '查看日志'}</button>
        </div>
        {!!logs.length && <pre className="system-log">{logs.join('\n')}</pre>}
      </section>
    </div>
  )
}

export default function App() {
  const [auth, setAuth] = useState(null)
  const [page, setPage] = useState('download')
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [status, setStatus] = useState(null)
  const [settings, setSettings] = useState(null)
  const [tasks, setTasks] = useState([])
  const [signInOpen, setSignInOpen] = useState(false)
  const [toast, setToast] = useState(null)

  const notify = useCallback((message, type = 'success') => {
    setToast({ message, type })
    window.setTimeout(() => setToast(null), 4000)
  }, [])

  const refresh = useCallback(async (quiet = false) => {
    try {
      const [statusValue, taskValue] = await Promise.all([api('/api/status'), api('/api/tasks')])
      setStatus(statusValue)
      setTasks(taskValue.items || [])
      if (!settings) setSettings(await api('/api/settings'))
    } catch (error) {
      if (error.status === 401) setAuth({ enabled: true, authenticated: false })
      else if (!quiet) notify(error.message, 'error')
    }
  }, [notify, settings])

  useEffect(() => {
    api('/api/auth/status').then((value) => {
      setAuth(value)
      if (value.authenticated) refresh()
    }).catch((error) => notify(error.message, 'error'))
  }, [])

  useEffect(() => {
    if (!auth?.authenticated) return undefined
    const timer = window.setInterval(() => refresh(true), 2500)
    return () => window.clearInterval(timer)
  }, [auth?.authenticated, refresh])

  async function createTask(payload) {
    try {
      await api('/api/tasks', { method: 'POST', body: JSON.stringify(payload) })
      notify('任务已加入队列')
      await refresh(true)
    } catch (error) {
      notify(error.message, 'error')
      throw error
    }
  }

  async function cancelTask(id) {
    try {
      await api(`/api/tasks/${id}/cancel`, { method: 'POST' })
      notify('任务已取消')
      await refresh(true)
    } catch (error) { notify(error.message, 'error') }
  }

  async function clearTasks() {
    try {
      await api('/api/tasks', { method: 'DELETE' })
      notify('已清理结束的任务')
      await refresh(true)
    } catch (error) { notify(error.message, 'error') }
  }

  async function saveSettings(value) {
    try {
      const updated = await api('/api/settings', { method: 'PUT', body: JSON.stringify(value) })
      setSettings(updated)
      notify('设置已保存')
    } catch (error) {
      notify(error.message, 'error')
      throw error
    }
  }

  async function restartWrapper() {
    try {
      await api('/api/wrapper/restart', { method: 'POST' })
      notify('Wrapper 已重启')
      await refresh(true)
    } catch (error) { notify(error.message, 'error') }
  }

  async function logout() {
    await api('/api/auth/logout', { method: 'POST' })
    setAuth({ enabled: true, authenticated: false })
  }

  const pageTitle = useMemo(() => NAV_ITEMS.find((item) => item.id === page)?.label || '下载', [page])

  if (auth === null) return <div className="boot-screen"><LoaderCircle className="spin" size={28} /></div>
  if (auth.enabled && !auth.authenticated) return <AuthScreen onAuthenticated={() => { setAuth({ enabled: true, authenticated: true }); refresh() }} />

  return (
    <div className="app-shell">
      <aside className={sidebarOpen ? 'sidebar open' : 'sidebar'}>
        <div className="brand"><span className="brand-mark"><Music2 size={20} /></span><div><strong>AM download</strong><span>v1.0.4</span></div><button className="icon-btn sidebar-close" onClick={() => setSidebarOpen(false)}><X size={18} /></button></div>
        <nav>
          {NAV_ITEMS.map(({ id, label, Icon }) => <button key={id} className={page === id ? 'active' : ''} onClick={() => { setPage(id); setSidebarOpen(false) }}><Icon size={18} /><span>{label}</span></button>)}
        </nav>
        <div className="sidebar-footer">
          <div className="sidebar-status"><StatusDot active={status?.wrapper?.wrapper_running} warning={!status?.wrapper?.wrapper_running} /><span>{status?.wrapper?.wrapper_running ? '服务运行中' : '服务未就绪'}</span></div>
          {auth.enabled && <button onClick={logout}><LogOut size={17} /><span>退出控制台</span></button>}
        </div>
      </aside>
      {sidebarOpen && <button className="sidebar-scrim" onClick={() => setSidebarOpen(false)} aria-label="关闭导航" />}
      <main className="main-area">
        <header className="topbar"><button className="icon-btn menu-btn" onClick={() => setSidebarOpen(true)}><Menu size={20} /></button><div><h1>{pageTitle}</h1><span>{status?.wrapper?.logged_in ? 'Apple Music 已连接' : '等待账户连接'}</span></div><button className="icon-btn" onClick={() => refresh()} title="刷新"><RefreshCw size={18} /></button></header>
        <div className="page-content">
          {page === 'download' && <DownloadPage status={status} settings={settings} tasks={tasks} onCreate={createTask} onCancel={cancelTask} onSignIn={() => setSignInOpen(true)} />}
          {page === 'tasks' && <TasksPage tasks={tasks} onCancel={cancelTask} onClear={clearTasks} />}
          {page === 'settings' && <SettingsPage initial={settings} onSave={saveSettings} />}
          {page === 'system' && <SystemPage status={status} onRefresh={() => refresh()} onRestart={restartWrapper} onSignIn={() => setSignInOpen(true)} />}
        </div>
      </main>
      <SignInDialog open={signInOpen} onClose={() => setSignInOpen(false)} onDone={async () => { setSignInOpen(false); notify('Apple Music 登录成功'); await refresh(true) }} />
      <Toast toast={toast} onClose={() => setToast(null)} />
    </div>
  )
}
