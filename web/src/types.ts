export type Service = {
  id: string
  name: string
  port: number
  protocol: 'http' | 'https' | 'tcp'
  process?: string
  originHost?: string
  hostname?: string
  status: 'available' | 'starting' | 'live' | 'stopped' | 'error'
  tunnelId?: string
}

export type Account = {
  connected: boolean
  name?: string
  email?: string
  domain?: string
  cloudflaredInstalled: boolean
  cloudflaredVersion?: string
}

export type AppState = { account: Account; services: Service[]; logs: string[]; serviceLogs?: Record<string, string[]> }

export type HostnameCheck = {
  hostname: string
  available: boolean
  existingType?: string
}
