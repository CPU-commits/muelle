import type { ProxyStatus } from '../../entities/proxyStatus'
import type { ProxyRepository } from '../proxyRepository'

/** El proxy como lo ve el backend cuando todo anda. */
export function makeProxyStatus(overrides: Partial<ProxyStatus> = {}): ProxyStatus {
	return {
		status: 'synced',
		error: null,
		syncedAt: new Date('2026-10-10T14:20:41Z'),
		caddy: {
			image: 'ghcr.io/cpu-commits/muelle-caddy:v0.1.0',
			running: true,
			httpPort: 80,
			httpsPort: 443,
		},
		portsRollback: null,
		panelCertificate: {
			hostname: 'panel.tudominio.cl',
			certSource: 'letsencrypt',
			originCa: null,
		},
		...overrides,
	}
}

// El fake del puerto para los tests de dominio.
export class InMemoryProxyRepository implements ProxyRepository {
	current: ProxyStatus = makeProxyStatus()
	failure: Error | null = null

	async status(): Promise<ProxyStatus> {
		if (this.failure) throw this.failure
		return this.current
	}
}
