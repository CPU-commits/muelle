import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import { DockerUnavailable, type ProxyRepository } from '../../domain'
import { domainHttpDataSource } from '../datasources/domainHttpDataSource'
import { proxyFromDto } from '../mappers/domainMapper'

// `GET /proxy` es de la Fase 3: sin ella, `no_route` → `UnavailableError` con `phase: 3`
// (la pantalla no muestra nada). Con Docker caído, `503 docker_unavailable`.
const PROXY_ERRORS: DomainErrorMap = {
	phase: 3,
	codes: {
		docker_unavailable: (error: ApiError) => new DockerUnavailable({ cause: error }),
	},
}

export const httpProxyRepository: ProxyRepository = {
	status: (signal) =>
		withDomainErrors(
			async () => proxyFromDto(await domainHttpDataSource.proxy(signal)),
			PROXY_ERRORS,
		),
}
