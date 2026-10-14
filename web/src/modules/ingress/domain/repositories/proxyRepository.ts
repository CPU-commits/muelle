import type { ProxyStatus } from '../entities/proxyStatus'

// El PUERTO del estado del proxy (`GET /proxy`, Fase 3). Solo con sesión.
export interface ProxyRepository {
	status(signal?: AbortSignal): Promise<ProxyStatus>
}

export interface ProxyDeps {
	readonly proxy: ProxyRepository
}
