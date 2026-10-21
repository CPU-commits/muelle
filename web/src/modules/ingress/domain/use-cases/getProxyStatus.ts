import type { ProxyStatus } from '../entities/proxyStatus'
import type { ProxyDeps } from '../repositories/proxyRepository'

export type GetProxyStatus = (signal?: AbortSignal) => Promise<ProxyStatus>

/** El estado del proxy (`GET /proxy`). Con Docker caído, `DockerUnavailable`. */
export function makeGetProxyStatus({ proxy }: ProxyDeps): GetProxyStatus {
	return async (signal) => proxy.status(signal)
}
