import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import { DockerUnavailable, type ProxyRepository } from '../../domain'
import { domainHttpDataSource } from '../datasources/domainHttpDataSource'
import { proxyFromDto } from '../mappers/domainMapper'

// `GET /proxy` (contrato §3.3). Con Docker caído, `503 docker_unavailable`.
const PROXY_ERRORS: DomainErrorMap = {
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
