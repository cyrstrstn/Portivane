import React, { useEffect, useMemo, useState } from 'react'
import {
  ArrowLeft, ArrowRight, ChevronRight, CircleAlert, Cloud,
  Copy, ExternalLink, Gauge, Globe2, LayoutDashboard, LoaderCircle, Moon, Plus,
  RefreshCw, Rocket, Search, Server, Settings, ShieldAlert, Square, Sun, TerminalSquare, Trash2,
  Unplug,
} from 'lucide-react'
import { api } from './api'
import type { AppState, Service } from './types'

const initialState: AppState = {
  account: { connected: false, cloudflaredInstalled: false }, services: [], logs: [],
}

type Screen = 'overview' | 'services' | 'deployments' | 'settings' | 'deployment'
type Theme = 'light' | 'dark'
type HostnameState = {
  status: 'idle' | 'checking' | 'available' | 'taken' | 'reuse' | 'error'
  hostname?: string
  message?: string
}

const delay = (milliseconds: number) => new Promise((resolve) => window.setTimeout(resolve, milliseconds))

function readRoute(): { screen: Screen; serviceId?: string } {
  const route = window.location.hash.replace(/^#\/?/, '')
  if (route.startsWith('deployments/')) {
    return { screen: 'deployment', serviceId: decodeURIComponent(route.slice('deployments/'.length)) }
  }
  if (route === 'services' || route === 'deployments' || route === 'settings') return { screen: route }
  return { screen: 'overview' }
}

function statusLabel(status: Service['status']) {
  if (status === 'available') return 'Ready'
  if (status === 'starting') return 'Connecting'
  if (status === 'live') return 'Live'
  if (status === 'error') return 'Failed'
  return 'Stopped'
}

function StatusBadge({ status }: { status: Service['status'] }) {
  return <span className={`status-badge ${status}`}><span aria-hidden="true" />{statusLabel(status)}</span>
}

function protocolName(service: Service) {
  return service.protocol.toUpperCase()
}

function originName(service: Service) {
  const host = service.originHost || 'localhost'
  // Normalize IPv6 loopback to localhost for readability
  const display = (host === '::1' || host === '0:0:0:0:0:0:0:1') ? 'localhost' : host
  return display.includes(':') ? `[${display}]:${service.port}` : `${display}:${service.port}`
}

function App() {
  const [authReady, setAuthReady] = useState(false)
  const [authConfigured, setAuthConfigured] = useState(false)
  const [authenticated, setAuthenticated] = useState(false)
  const [state, setState] = useState<AppState>(initialState)
  const [route, setRoute] = useState(readRoute)
  const [selectedId, setSelectedId] = useState('')
  const [detailTab, setDetailTab] = useState<'overview' | 'logs'>('overview')
  const [subdomain, setSubdomain] = useState('')
  const [hostnameState, setHostnameState] = useState<HostnameState>({ status: 'idle' })
  const [busy, setBusy] = useState<string | null>('loading')
  const [error, setError] = useState('')
  const [showAdd, setShowAdd] = useState(false)
  const [manualPort, setManualPort] = useState('')
  const [manualProtocol, setManualProtocol] = useState<Service['protocol']>('http')
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState<'web' | 'all'>('web')
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = window.localStorage.getItem('portivane-theme') ?? window.localStorage.getItem('wyrmhole-theme')
    if (saved === 'light' || saved === 'dark') return saved
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  })

  useEffect(() => {
    api.authStatus().then((result) => { setAuthConfigured(result.configured); if (!result.configured) setAuthenticated(true) }).catch(() => undefined).finally(() => setAuthReady(true))
  }, [])

  const activeCount = state.services.filter((service) => service.status === 'live' || service.status === 'starting').length
  const webCount = state.services.filter((service) => service.protocol !== 'tcp').length
  const deployments = state.services.filter((service) => service.hostname || service.tunnelId)
  const selected = useMemo(() => {
    const id = route.serviceId || selectedId
    return state.services.find((service) => service.id === id)
  }, [route.serviceId, selectedId, state.services])
  const selectedServiceId = selected?.id
  const selectedHostname = selected?.hostname

  const visibleServices = state.services.filter((service) => {
    if (scope === 'web' && service.protocol === 'tcp') return false
    const needle = query.trim().toLowerCase()
    if (!needle) return true
    return service.name.toLowerCase().includes(needle)
      || Boolean(service.process?.toLowerCase().includes(needle))
      || String(service.port).includes(needle)
  })

  function navigate(screen: Exclude<Screen, 'deployment'>, serviceId?: string) {
    window.location.hash = serviceId ? `${screen}/${encodeURIComponent(serviceId)}` : screen
  }

  function openDeployment(service: Service) {
    setSelectedId(service.id)
    setDetailTab('overview')
    window.location.hash = `deployments/${encodeURIComponent(service.id)}`
  }

  useEffect(() => {
    const onHashChange = () => setRoute(readRoute())
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    window.localStorage.setItem('portivane-theme', theme)
  }, [theme])

  useEffect(() => {
    api.state()
      .then(setState)
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setBusy(null))
  }, [])

  useEffect(() => {
    const timer = window.setInterval(() => api.refreshPorts().then(setState).catch(() => undefined), 8000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    const domain = state.account.domain
    if (selectedHostname && domain && selectedHostname.endsWith(`.${domain}`)) {
      setSubdomain(selectedHostname.slice(0, -(domain.length + 1)))
    } else {
      setSubdomain('')
    }
    setHostnameState({ status: 'idle' })
  }, [selectedServiceId, selectedHostname, state.account.domain])

  useEffect(() => {
    const value = subdomain.trim().toLowerCase()
    if (!selectedServiceId || !state.account.connected || !state.account.domain || !value) {
      setHostnameState({ status: 'idle' })
      return
    }
    setHostnameState({ status: 'checking' })
    let cancelled = false
    const timer = window.setTimeout(() => {
      api.checkHostname(value)
        .then((check) => {
          if (cancelled) return
          if (check.available) setHostnameState({ status: 'available', hostname: check.hostname })
          else if (selectedHostname === check.hostname) setHostnameState({ status: 'reuse', hostname: check.hostname })
          else setHostnameState({ status: 'taken', hostname: check.hostname, message: `${check.existingType ?? 'DNS'} record already exists` })
        })
        .catch((reason: Error) => { if (!cancelled) setHostnameState({ status: 'error', message: reason.message }) })
    }, 400)
    return () => { cancelled = true; window.clearTimeout(timer) }
  }, [subdomain, state.account.connected, state.account.domain, selectedServiceId, selectedHostname])

  async function run(key: string, action: () => Promise<AppState>) {
    setBusy(key)
    setError('')
    try {
      const next = await action()
      setState(next)
      return next
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Something went wrong.')
      return null
    } finally {
      setBusy(null)
    }
  }

  async function addService(event: React.FormEvent) {
    event.preventDefault()
    const port = Number(manualPort)
    if (!Number.isInteger(port) || port < 1 || port > 65535 || port === 4747) {
      setError('Enter a port between 1 and 65535. Port 4747 is reserved for Portivane.')
      return
    }
    const next = await run('add', () => api.saveService({ name: `Service on ${port}`, port, protocol: manualProtocol }))
    if (next) {
      const added = next.services.find((service) => service.port === port)
      if (added) setSelectedId(added.id)
      setManualPort('')
      setShowAdd(false)
    }
  }

  async function connectCloudflare() {
    setBusy('login')
    setError('')
    try {
      await api.login()
      for (let attempt = 0; attempt < 30; attempt += 1) {
        await delay(2000)
        const next = await api.state()
        setState(next)
        if (next.account.connected && next.account.domain) return
      }
      setError('Authorization is still waiting. Finish it in the Cloudflare browser tab, then refresh this page.')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Cloudflare authorization could not start.')
    } finally {
      setBusy(null)
    }
  }

  async function publishSelected() {
    if (!selected) return
    const next = await run(selected.id, () => api.start(selected.id, subdomain.trim()))
    if (next) setDetailTab('logs')
  }

  const navItems = [
    { id: 'overview' as const, label: 'Overview', icon: LayoutDashboard },
    { id: 'services' as const, label: 'Services', icon: Server },
    { id: 'deployments' as const, label: 'Deployments', icon: Rocket, count: activeCount },
    { id: 'settings' as const, label: 'Settings', icon: Settings },
  ]

  if (!authReady) return <LoadingScreen />
  if (!authenticated) return <AuthScreen setup={!authConfigured} onDone={() => { setAuthenticated(true); setBusy('loading'); api.state().then(setState).finally(() => setBusy(null)) }} />

  return (
    <div className="dashboard-shell">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <aside className="sidebar">
        <button className="brand" onClick={() => navigate('overview')} aria-label="Portivane overview">
          <span className="brand-mark"><img src="/portivane-logo.png" alt="" /></span>
          <span>Portivane</span>
        </button>
        <nav className="primary-nav" aria-label="Main navigation">
          {navItems.map((item) => {
            const active = route.screen === item.id || (item.id === 'deployments' && route.screen === 'deployment')
            const Icon = item.icon
            return <button key={item.id} className={active ? 'active' : ''} onClick={() => navigate(item.id)}><Icon size={16} /><span>{item.label}</span>{item.count ? <b>{item.count}</b> : null}</button>
          })}
        </nav>
        <button className="theme-toggle" onClick={() => setTheme(theme === 'light' ? 'dark' : 'light')} aria-label={`Switch to ${theme === 'light' ? 'dark' : 'light'} mode`} title={`Switch to ${theme === 'light' ? 'dark' : 'light'} mode`}>
          {theme === 'light' ? <Moon size={16} /> : <Sun size={16} />}<span>{theme === 'light' ? 'Dark mode' : 'Light mode'}</span>
        </button>
        <div className="sidebar-account">
          <span className={`connection-dot ${state.account.connected ? 'online' : ''}`} aria-hidden="true" />
          <span><strong>{state.account.domain ?? 'Cloudflare'}</strong><small>{state.account.connected ? 'Connected' : 'Not connected'}</small></span>
        </div>
      </aside>

      <main id="main-content" className="main-content">
        {!busy && !state.account.cloudflaredInstalled && (
          <div className="setup-banner">
            <span className="setup-banner-icon">⚡</span>
            <div><strong>cloudflared is not installed</strong><p>Portivane needs the Cloudflare tunnel daemon to create and run tunnels.</p></div>
            <button className="button primary" disabled={busy === 'install-cloudflared'} onClick={() => void run('install-cloudflared', api.installCloudflared)}>
              {busy === 'install-cloudflared' ? <><LoaderCircle size={15} className="spin" /> Installing…</> : 'Install cloudflared'}
            </button>
          </div>
        )}
        {!busy && state.account.cloudflaredInstalled && !state.account.connected && (
          <div className="setup-banner setup-banner--warn">
            <span className="setup-banner-icon">🔑</span>
            <div><strong>Connect your Cloudflare account</strong><p>Authorize Portivane to manage tunnels on your Cloudflare zone.</p></div>
            <button className="button primary" disabled={busy === 'login'} onClick={() => void connectCloudflare()}>
              {busy === 'login' ? <><LoaderCircle size={15} className="spin" /> Connecting…</> : 'Connect Cloudflare'}
            </button>
          </div>
        )}
        {busy === 'loading' ? <LoadingScreen /> : route.screen === 'overview' ? (
          <Overview state={state} deployments={deployments} activeCount={activeCount} webCount={webCount} onOpen={openDeployment} onNavigate={navigate} />
        ) : route.screen === 'services' ? (
          <ServicesScreen
            services={visibleServices} total={state.services.length} webCount={webCount} scope={scope} query={query}
            busy={busy} showAdd={showAdd} manualPort={manualPort} manualProtocol={manualProtocol}
            onScope={setScope} onQuery={setQuery} onRefresh={() => void run('refresh', api.refreshPorts)}
            onOpen={openDeployment} onShowAdd={setShowAdd} onPort={setManualPort} onProtocol={setManualProtocol} onAdd={addService}
          />
        ) : route.screen === 'deployments' ? (
          <DeploymentsScreen deployments={deployments} activeCount={activeCount} onOpen={openDeployment} onNew={() => navigate('services')} />
        ) : route.screen === 'settings' ? (
          <SettingsScreen state={state} busy={busy} theme={theme} onTheme={setTheme} onConnect={() => void connectCloudflare()} onInstallCloudflared={() => void run('install-cloudflared', api.installCloudflared)} />
        ) : selected ? (
          <DeploymentDetail
            service={selected} state={state} tab={detailTab} busy={busy} subdomain={subdomain}
            hostnameState={hostnameState} onBack={() => navigate('deployments')} onTab={setDetailTab}
            onSubdomain={setSubdomain} onPublish={() => void publishSelected()}
            onStop={() => void run(selected.id, () => api.stop(selected.id))}
            onDelete={(hostname) => void (async () => {
              const next = await run(`delete-${selected.id}`, () => api.deleteDeployment(selected.id, hostname))
              if (next) navigate('deployments')
            })()}
          />
        ) : (
          <NotFound onBack={() => navigate('deployments')} />
        )}
      </main>

      {error && <div className="error-toast" role="alert"><CircleAlert size={18} /><span>{error}</span><button onClick={() => setError('')}>Dismiss</button></div>}
    </div>
  )
}

function AuthScreen({ setup, onDone }: { setup: boolean; onDone: () => void }) {
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit(event: React.FormEvent) {
    event.preventDefault(); setError('')
    if (setup && password !== confirm) { setError('Passwords do not match.'); return }
    setBusy(true)
    try { if (setup) await api.authSetup(password); else await api.authLogin(password); onDone() }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Authentication failed.') }
    finally { setBusy(false) }
  }
  return <div className="auth-screen"><form className="auth-card" onSubmit={submit}><img src="/portivane-logo.png" alt="" /><h1>{setup ? 'Protect Portivane' : 'Welcome back'}</h1><p>{setup ? 'Set the password required to open this dashboard. You only do this once.' : 'Enter your Portivane password to continue.'}</p><label>Password<input type="password" minLength={8} autoFocus required value={password} onChange={(event) => setPassword(event.target.value)} /></label>{setup && <label>Confirm password<input type="password" minLength={8} required value={confirm} onChange={(event) => setConfirm(event.target.value)} /></label>}{error && <div className="auth-error">{error}</div>}<button className="button primary" disabled={busy}>{busy ? 'Checking…' : setup ? 'Set password' : 'Unlock Portivane'}</button></form></div>
}

function PageHeader({ title, description, action }: { title: string; description: string; action?: React.ReactNode }) {
  return <header className="page-header"><div><h1>{title}</h1><p>{description}</p></div>{action}</header>
}

function LoadingScreen() {
  return <div className="loading-screen"><LoaderCircle className="spin" /><span>Reading local services…</span></div>
}

function Overview({ state, deployments, activeCount, webCount, onOpen, onNavigate }: {
  state: AppState; deployments: Service[]; activeCount: number; webCount: number
  onOpen: (service: Service) => void; onNavigate: (screen: 'services' | 'deployments') => void
}) {
  const recent = [...deployments].sort((a, b) => Number(b.status === 'live') - Number(a.status === 'live')).slice(0, 5)
  return <div className="page overview-page">
    <PageHeader title="Overview" description="Your local services and public Cloudflare routes." action={<button className="button primary" onClick={() => onNavigate('services')}><Plus size={15} /> New deployment</button>} />
    <section className="overview-stats" aria-label="Portivane status">
      <div><span>Active tunnels</span><strong>{activeCount}</strong><small>{activeCount ? 'Serving traffic now' : 'No public services'}</small></div>
      <div><span>Web services</span><strong>{webCount}</strong><small>Detected on this device</small></div>
      <div><span>Cloudflare zone</span><strong className="domain-stat">{state.account.domain ?? 'Not connected'}</strong><small>{state.account.connected ? 'Authorization ready' : 'Connect in settings'}</small></div>
    </section>

    <section className="content-section">
      <div className="section-title"><div><h2>Recent deployments</h2><p>Named tunnels created from this device.</p></div><button className="text-action" onClick={() => onNavigate('deployments')}>View all <ArrowRight size={14} /></button></div>
      {recent.length ? <DeploymentTable services={recent} onOpen={onOpen} /> : <EmptyState icon={<Rocket />} title="No deployments yet" copy="Choose a detected service and assign its first public hostname." action={<button className="button" onClick={() => onNavigate('services')}>Browse services</button>} />}
    </section>

    <section className="system-strip">
      <span><Gauge size={16} /> Runtime</span>
      <strong>{state.account.cloudflaredInstalled ? `cloudflared ${state.account.cloudflaredVersion ?? 'installed'}` : 'cloudflared not detected'}</strong>
      <small>Management API · 127.0.0.1:4747</small>
    </section>
  </div>
}

function ServicesScreen(props: {
  services: Service[]; total: number; webCount: number; scope: 'web' | 'all'; query: string; busy: string | null
  showAdd: boolean; manualPort: string; manualProtocol: Service['protocol']
  onScope: (scope: 'web' | 'all') => void; onQuery: (query: string) => void; onRefresh: () => void
  onOpen: (service: Service) => void; onShowAdd: (show: boolean) => void; onPort: (port: string) => void
  onProtocol: (protocol: Service['protocol']) => void; onAdd: (event: React.FormEvent) => void
}) {
  return <div className="page">
    <PageHeader title="Services" description={`${props.webCount} web services detected across ${props.total} listening ports.`} action={<button className="button primary" onClick={() => props.onShowAdd(true)}><Plus size={15} /> Add service</button>} />
    <div className="toolbar">
      <label className="search-box"><Search size={15} /><input type="search" placeholder="Search by process or port…" value={props.query} onChange={(event) => props.onQuery(event.target.value)} /></label>
      <div className="segmented"><button className={props.scope === 'web' ? 'active' : ''} onClick={() => props.onScope('web')}>Web services</button><button className={props.scope === 'all' ? 'active' : ''} onClick={() => props.onScope('all')}>All ports</button></div>
      <button className="icon-button" aria-label="Refresh listening ports" title="Refresh ports" disabled={props.busy === 'refresh'} onClick={props.onRefresh}><RefreshCw size={15} className={props.busy === 'refresh' ? 'spin' : ''} /></button>
    </div>

    {props.showAdd && <form className="inline-create" onSubmit={props.onAdd}>
      <div><strong>Add a local service</strong><span>Use this when automatic detection misses a port.</span></div>
      <label><span>Port</span><input inputMode="numeric" placeholder="3000" value={props.manualPort} onChange={(event) => props.onPort(event.target.value)} autoFocus /></label>
      <label><span>Protocol</span><select value={props.manualProtocol} onChange={(event) => props.onProtocol(event.target.value as Service['protocol'])}><option value="http">HTTP</option><option value="https">HTTPS</option><option value="tcp">TCP</option></select></label>
      <button className="button primary" disabled={props.busy === 'add'}>{props.busy === 'add' ? <LoaderCircle size={15} className="spin" /> : 'Add service'}</button>
      <button type="button" className="button quiet" onClick={() => props.onShowAdd(false)}>Cancel</button>
    </form>}

    <section className="content-section service-directory">
      <div className="table-head service-grid"><span>Service</span><span>Origin</span><span>Protocol</span><span>Status</span><span /></div>
      {props.services.length ? <div className="service-table">{props.services.map((service) => <button key={service.id} className="service-entry service-grid" onClick={() => props.onOpen(service)}>
        <span className="service-identity"><i><Server size={16} /></i><span><strong>{service.name}</strong><small>{service.process || 'Manually configured'}</small></span></span>
        <code>{originName(service)}</code><span>{protocolName(service)}</span><StatusBadge status={service.status} /><ChevronRight size={15} />
      </button>)}</div> : <EmptyState icon={<Unplug />} title="No matching services" copy="Start a web server, change the filter, or add a custom port." />}
    </section>
  </div>
}

function DeploymentsScreen({ deployments, activeCount, onOpen, onNew }: { deployments: Service[]; activeCount: number; onOpen: (service: Service) => void; onNew: () => void }) {
  return <div className="page">
    <PageHeader title="Deployments" description={`${activeCount} active ${activeCount === 1 ? 'tunnel' : 'tunnels'} on this device.`} action={<button className="button primary" onClick={onNew}><Plus size={15} /> New deployment</button>} />
    <section className="content-section deployment-directory">
      <div className="section-title"><div><h2>All deployments</h2><p>Public routes and their current connector state.</p></div></div>
      {deployments.length ? <DeploymentTable services={deployments} onOpen={onOpen} /> : <EmptyState icon={<Rocket />} title="Nothing deployed" copy="Publish a local service to create its first deployment." action={<button className="button" onClick={onNew}>Choose a service</button>} />}
    </section>
  </div>
}

function DeploymentTable({ services, onOpen }: { services: Service[]; onOpen: (service: Service) => void }) {
  return <div className="deployment-table">
    <div className="table-head deployment-grid"><span>Deployment</span><span>Origin</span><span>Status</span><span /></div>
    {services.map((service) => <button key={service.id} className="deployment-row deployment-grid" onClick={() => onOpen(service)}>
      <span className="deployment-name"><i><Globe2 size={16} /></i><span><strong>{service.hostname || `${service.name}:${service.port}`}</strong><small>{service.name}</small></span></span>
      <code>{protocolName(service).toLowerCase()}://{originName(service)}</code><StatusBadge status={service.status} /><ChevronRight size={15} />
    </button>)}
  </div>
}

function DeploymentPreviewCard({ service, localUrl }: { service: Service; localUrl: string }) {
  const publicUrl = service.hostname ? `https://${service.hostname}` : null
  const isLive = service.status === 'live'
  // Always preview via local URL — avoids X-Frame-Options blocks on public URLs
  const previewUrl = localUrl
  // Show public URL in bar only when live, otherwise show local
  const displayUrl = isLive && publicUrl ? publicUrl : localUrl
  const [iframeError, setIframeError] = React.useState(false)
  const [iframeLoaded, setIframeLoaded] = React.useState(false)

  // Reset iframe state when the URL changes
  React.useEffect(() => {
    setIframeError(false)
    setIframeLoaded(false)
  }, [previewUrl])

  return (
    <div className="preview-card">
      {/* Left: browser preview */}
      <div className="preview-browser">
        <div className="preview-browser-chrome">
          <span className="preview-browser-dots">
            <i /><i /><i />
          </span>
          <div className="preview-browser-bar">
            <span>{displayUrl}</span>
          </div>
          <a
            className="preview-browser-visit"
            href={localUrl}
            target="_blank"
            rel="noreferrer"
            title="Open local service in new tab"
            aria-label="Open local service in new tab"
          >
            <ExternalLink size={12} />
          </a>
        </div>
        <div className="preview-viewport">
          {!iframeError ? (
            <>
              {!iframeLoaded && (
                <div className="preview-loading">
                  <LoaderCircle size={22} className="spin" />
                  <span>Loading preview…</span>
                </div>
              )}
              <iframe
                src={previewUrl}
                title={`Preview of ${service.name}`}
                sandbox="allow-scripts allow-same-origin allow-forms"
                onLoad={() => setIframeLoaded(true)}
                onError={() => setIframeError(true)}
                style={{ opacity: iframeLoaded ? 1 : 0 }}
              />
            </>
          ) : (
            <div className="preview-unavailable">
              <Globe2 size={32} />
              <span>Can't embed this service</span>
              <small>It may block embedding or require authentication — open it directly instead.</small>
              <a className="button primary" href={localUrl} target="_blank" rel="noreferrer">
                <ExternalLink size={13} /> Open {localUrl}
              </a>
            </div>
          )}
        </div>
      </div>

      {/* Right: deployment metadata */}
      <div className="preview-meta">
        {publicUrl && (
          <div className="preview-meta-row preview-meta-url">
            <span className="preview-meta-label">Deployment</span>
            <a href={publicUrl} target="_blank" rel="noreferrer" className="preview-meta-link">
              {service.hostname}
              <ExternalLink size={11} />
            </a>
          </div>
        )}

        {service.hostname && (
          <div className="preview-meta-row">
            <span className="preview-meta-label">Domain</span>
            <a href={publicUrl!} target="_blank" rel="noreferrer" className="preview-meta-domain">
              {service.hostname} <ExternalLink size={11} />
            </a>
          </div>
        )}

        <div className="preview-meta-row preview-meta-split">
          <div>
            <span className="preview-meta-label">Status</span>
            <div className="preview-meta-status">
              <span className={`preview-status-dot ${service.status}`} aria-hidden="true" />
              <strong>{statusLabel(service.status)}</strong>
            </div>
          </div>
          <div>
            <span className="preview-meta-label">Origin</span>
            <code className="preview-meta-code">{localUrl}</code>
          </div>
        </div>

        <div className="preview-meta-row">
          <span className="preview-meta-label">Source</span>
          <div className="preview-meta-source">
            <span className="preview-meta-tunnel-icon">
              <Server size={12} />
            </span>
            <span>
              <strong>{service.name}</strong>
              {service.tunnelId && (
                <code>&gt;_ {service.tunnelId.slice(0, 8)}</code>
              )}
            </span>
          </div>
        </div>

        <div className="preview-meta-row preview-meta-actions">
          <a className="button" href={localUrl} target="_blank" rel="noreferrer">
            <ExternalLink size={13} /> Local
          </a>
          {publicUrl && (
            <a className="button primary" href={publicUrl} target="_blank" rel="noreferrer">
              Visit <ExternalLink size={13} />
            </a>
          )}
        </div>

        {isLive && (
          <div className="preview-live-indicator">
            <span className="preview-live-pulse" aria-hidden="true" />
            Live · serving traffic
          </div>
        )}
      </div>
    </div>
  )
}

function DeploymentDetail(props: {
  service: Service; state: AppState; tab: 'overview' | 'logs'; busy: string | null; subdomain: string
  hostnameState: HostnameState; onBack: () => void; onTab: (tab: 'overview' | 'logs') => void
  onSubdomain: (value: string) => void; onPublish: () => void; onStop: () => void; onDelete: (hostname: string) => void
}) {
  const { service, state } = props
  const isRunning = service.status === 'live' || service.status === 'starting'
  const isDeploying = props.busy === service.id
  const hostname = props.hostnameState.hostname || service.hostname || (props.subdomain && state.account.domain ? `${props.subdomain}.${state.account.domain}` : '')
  const canPublish = Boolean(state.account.connected && state.account.cloudflaredInstalled && (props.hostnameState.status === 'available' || props.hostnameState.status === 'reuse'))
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteValue, setDeleteValue] = useState('')
  const logPanelRef = React.useRef<HTMLDivElement>(null)

  // Per-service scoped logs — never show stale logs from other deployments
  const serviceLogs = state.serviceLogs?.[service.id] ?? []

  // Auto-scroll log panel during deployment
  React.useEffect(() => {
    if (isDeploying || isRunning) {
      const el = logPanelRef.current
      if (el) el.scrollTop = el.scrollHeight
    }
  }, [serviceLogs.length, isDeploying, isRunning])

  // When deploying starts, switch to logs tab automatically
  React.useEffect(() => {
    if (isDeploying) props.onTab('logs')
  }, [isDeploying])

  async function copyLogs() {
    await navigator.clipboard.writeText(serviceLogs.join('\n'))
  }

  const rawHost = service.originHost || 'localhost'
  const displayHost = (rawHost === '::1' || rawHost === '0:0:0:0:0:0:0:1') ? 'localhost' : rawHost
  const localUrl = `${service.protocol === 'https' ? 'https' : 'http'}://${displayHost.includes(':') ? `[${displayHost}]` : displayHost}:${service.port}`

  return <div className="page deployment-detail">
    <button className="back-link" onClick={props.onBack}><ArrowLeft size={14} /> Deployments</button>
    <header className="deployment-header">
      <div className="deployment-title">
        <span className="deployment-icon"><Globe2 size={20} /></span>
        <div>
          <h1>{service.hostname || service.name}</h1>
          <p>{service.hostname ? service.name : `Local service on port ${service.port}`}</p>
        </div>
      </div>
      <div className="deployment-actions">
        <StatusBadge status={service.status} />
        <a className="button" href={localUrl} target="_blank" rel="noreferrer" title="Open local service"><ExternalLink size={14} /> Preview</a>
        {service.hostname && <a className="button" href={`https://${service.hostname}`} target="_blank" rel="noreferrer">Visit <ExternalLink size={14} /></a>}
        {isRunning ? <button className="button danger" disabled={props.busy === service.id} onClick={props.onStop}><Square size={13} /> Stop</button> : null}
      </div>
    </header>

    <nav className="detail-tabs" aria-label="Deployment sections">
      <button className={props.tab === 'overview' ? 'active' : ''} onClick={() => props.onTab('overview')}>Overview</button>
      <button className={props.tab === 'logs' ? 'active' : ''} onClick={() => props.onTab('logs')}>
        Deployment logs <span>{serviceLogs.length}</span>
      </button>
    </nav>

    {props.tab === 'logs' ? (
      <section className={`log-console${isDeploying || isRunning ? ' log-console--active' : ''}`}>
        <div className="console-bar">
          <span><TerminalSquare size={15} /> Runtime output {(isDeploying || isRunning) && <span className="console-live-badge">LIVE</span>}</span>
          <button disabled={!serviceLogs.length} onClick={() => void copyLogs()}><Copy size={14} /> Copy logs</button>
        </div>
        <div className="console-output" role="log" aria-live="polite" ref={logPanelRef}>
          {serviceLogs.length ? serviceLogs.slice(-200).map((line, index) => (
            <div key={`${index}-${line.slice(0, 20)}`}>
              <span>{String(index + 1).padStart(3, '0')}</span>
              <code>{line}</code>
            </div>
          )) : (
            <p>{isDeploying ? 'Starting deployment…' : 'No logs yet. Deploy this service to see output here.'}</p>
          )}
          {/* Status footer pinned at bottom of log stream */}
          {isDeploying && (
            <div className="log-status-row log-status-row--deploying">
              <LoaderCircle size={13} className="spin" /><span>Connecting to Cloudflare…</span>
            </div>
          )}
          {service.status === 'stopped' && serviceLogs.length > 0 && (
            <div className="log-status-row log-status-row--stopped">
              <Square size={12} /><span>Tunnel stopped</span>
            </div>
          )}
          {service.status === 'error' && serviceLogs.length > 0 && (
            <div className="log-status-row log-status-row--error">
              <CircleAlert size={13} /><span>Tunnel exited with an error</span>
            </div>
          )}
        </div>
      </section>
    ) : (
      <>
        <DeploymentPreviewCard service={service} localUrl={localUrl} />
        <div className="deployment-body">
          <section className="deployment-main">
            <div className="section-title"><div><h2>Public route</h2><p>Connect this local origin to your Cloudflare zone.</p></div></div>
            <div className={`route-diagram ${service.status}`}>
              <div><TerminalSquare size={18} /><span><small>Origin</small><strong>{originName(service)}</strong></span></div>
              <span className="route-rail"><i /></span>
              <div><Cloud size={18} /><span><small>Cloudflare edge</small><strong>{state.account.domain ?? 'Not connected'}</strong></span></div>
              <span className="route-rail"><i /></span>
              <div><Globe2 size={18} /><span><small>Public hostname</small><strong>{hostname || 'Not configured'}</strong></span></div>
            </div>
            <div className="publish-config">
              <label htmlFor="subdomain"><span>Subdomain</span><div className="domain-control"><input id="subdomain" value={props.subdomain} placeholder="photos" disabled={isRunning || !state.account.domain} onChange={(event) => props.onSubdomain(event.target.value)} /><b>.{state.account.domain ?? 'connect Cloudflare first'}</b></div></label>
              <p className={`hostname-message ${props.hostnameState.status}`}>{props.hostnameState.status === 'checking' ? 'Checking Cloudflare DNS…' : props.hostnameState.status === 'available' ? `${props.hostnameState.hostname} is available.` : props.hostnameState.status === 'reuse' ? 'This Portivane hostname is ready to reconnect.' : props.hostnameState.status === 'taken' ? `${props.hostnameState.hostname} is already in use.` : props.hostnameState.status === 'error' ? props.hostnameState.message : state.account.domain ? 'Enter the name that will appear before your domain.' : 'Connect Cloudflare in Settings first.'}</p>
              {!isRunning && (
                <button className="button primary deploy-button" disabled={!canPublish || isDeploying} onClick={props.onPublish}>
                  {isDeploying ? <><LoaderCircle size={15} className="spin" /> Deploying…</> : <><Rocket size={15} /> Deploy service</>}
                </button>
              )}
            </div>
          </section>
          <aside className="deployment-meta">
            <h2>Runtime</h2>
            <dl>
              <div><dt>Status</dt><dd><StatusBadge status={service.status} /></dd></div>
              <div><dt>Protocol</dt><dd>{protocolName(service)}</dd></div>
              <div><dt>Process</dt><dd>{service.process || 'Manual'}</dd></div>
              <div><dt>Local address</dt><dd><code>{originName(service)}</code></dd></div>
              <div><dt>Tunnel ID</dt><dd><code>{service.tunnelId ? `${service.tunnelId.slice(0, 8)}…` : 'Created on deploy'}</code></dd></div>
            </dl>
            <a className="button preview-local-btn" href={localUrl} target="_blank" rel="noreferrer"><ExternalLink size={13} /> Preview local</a>
            <button className="text-action logs-link" onClick={() => props.onTab('logs')}>Open deployment logs <ArrowRight size={14} /></button>
          </aside>
        </div>
        {service.hostname && service.tunnelId && <section className="danger-zone">
          <div><ShieldAlert size={18} /><span><strong>Delete deployment</strong><p>Remove <b>{service.hostname}</b>, its DNS record, tunnel, and deployment record. The listening port will remain under Services.</p></span></div>
          {!confirmDelete ? <button className="button danger" onClick={() => setConfirmDelete(true)}><Trash2 size={14} /> Delete deployment</button> : <div className="delete-confirmation">
            <label htmlFor="delete-confirmation">Type <strong>{service.hostname}</strong> to confirm</label>
            <div><input id="delete-confirmation" value={deleteValue} onChange={(event) => setDeleteValue(event.target.value)} autoComplete="off" spellCheck={false} /><button className="button danger-solid" disabled={deleteValue !== service.hostname || props.busy === `delete-${service.id}`} onClick={() => props.onDelete(deleteValue)}>{props.busy === `delete-${service.id}` ? <LoaderCircle size={15} className="spin" /> : <Trash2 size={14} />} Delete forever</button><button className="button quiet" onClick={() => { setConfirmDelete(false); setDeleteValue('') }}>Cancel</button></div>
          </div>}
        </section>}
      </>
    )}
  </div>
}

function SettingsScreen({ state, busy, theme, onTheme, onConnect, onInstallCloudflared }: { state: AppState; busy: string | null; theme: Theme; onTheme: (theme: Theme) => void; onConnect: () => void; onInstallCloudflared: () => void }) {
  return <div className="page settings-page">
    <PageHeader title="Settings" description="Cloudflare authorization and local runtime information." />
    <section className="settings-section">
      <div><h2>Cloudflare account</h2><p>Portivane uses the official cloudflared authorization flow.</p></div>
      <div className="settings-value"><span className={`connection-dot ${state.account.connected ? 'online' : ''}`} /><span><strong>{state.account.domain ?? 'No domain connected'}</strong><small>{state.account.connected ? 'Authorized zone' : 'Required before deploying'}</small></span>{!state.account.connected && <button className="button primary" disabled={busy === 'login'} onClick={onConnect}>{busy === 'login' ? <LoaderCircle size={15} className="spin" /> : 'Connect Cloudflare'}</button>}</div>
    </section>
    <section className="settings-section">
      <div><h2>Connector</h2><p>The official Cloudflare Tunnel daemon. Portivane can install it automatically beside itself.</p></div>
      <div className="settings-value">
        <span className={`connection-dot ${state.account.cloudflaredInstalled ? 'online' : ''}`} />
        <span><strong>{state.account.cloudflaredInstalled ? 'cloudflared installed' : 'cloudflared not found'}</strong><small>{state.account.cloudflaredInstalled ? (state.account.cloudflaredVersion ?? 'Ready') : 'Required to create and run tunnels'}</small></span>
        {!state.account.cloudflaredInstalled && <button className="button primary" disabled={busy === 'install-cloudflared'} onClick={onInstallCloudflared}>{busy === 'install-cloudflared' ? <><LoaderCircle size={15} className="spin" /> Installing…</> : 'Install cloudflared'}</button>}
      </div>
    </section>
    <section className="settings-section">
      <div><h2>Appearance</h2><p>Choose the dashboard theme used on this device.</p></div>
      <div className="theme-choices" role="group" aria-label="Appearance">
        <button className={theme === 'light' ? 'active' : ''} onClick={() => onTheme('light')}><Sun size={17} /><span><strong>Light</strong><small>Bright workspace</small></span></button>
        <button className={theme === 'dark' ? 'active' : ''} onClick={() => onTheme('dark')}><Moon size={17} /><span><strong>Dark</strong><small>Low-light console</small></span></button>
      </div>
    </section>
    <section className="settings-section">
      <div><h2>Local management</h2><p>The dashboard is accessible only from this device.</p></div>
      <div className="settings-value"><span><strong>127.0.0.1:4747</strong><small>Loopback interface</small></span></div>
    </section>
  </div>
}

function EmptyState({ icon, title, copy, action }: { icon: React.ReactNode; title: string; copy: string; action?: React.ReactNode }) {
  return <div className="empty-state"><span>{icon}</span><strong>{title}</strong><p>{copy}</p>{action}</div>
}

function NotFound({ onBack }: { onBack: () => void }) {
  return <div className="loading-screen"><CircleAlert /><strong>Deployment not found</strong><button className="button" onClick={onBack}>Back to deployments</button></div>
}

export default App
