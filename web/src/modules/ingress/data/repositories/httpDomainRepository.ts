import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import {
	CloudflareUnavailable,
	DockerUnavailable,
	DomainNotFound,
	type DomainRepository,
	HostnameTaken,
	NothingToAdopt,
} from '../../domain'
import { domainHttpDataSource } from '../datasources/domainHttpDataSource'
import { domainFromDto, createDomainPayload, updateDomainPayload } from '../mappers/domainMapper'

const cause = (error: ApiError) => ({ cause: error })

// El 422 `invalid` del backend nombra los campos del cuerpo; en el panel son dos.
const FIELDS = {
	hostname: 'hostname',
	target_service: 'target',
	target_port: 'target',
	proxied: 'proxied',
} as const

// `502 cloudflare_unavailable` y `503 docker_unavailable` (contrato §0.2), con su texto.
const UPSTREAM = {
	cloudflare_unavailable: (error: ApiError) => new CloudflareUnavailable(cause(error)),
	docker_unavailable: (error: ApiError) => new DockerUnavailable(cause(error)),
}

// Todo es de la Fase 3: contra un backend sin ella, la ruta no existe (`no_route` /
// `method_not_allowed`) y es `UnavailableError` con `phase: 3`.
const LIST_ERRORS: DomainErrorMap = { phase: 3, fields: FIELDS, codes: UPSTREAM }

// Al crear, `409 conflict` es el hostname tomado (es único en todo muelle).
const CREATE_ERRORS: DomainErrorMap = {
	phase: 3,
	fields: FIELDS,
	codes: { ...UPSTREAM, conflict: (error) => new HostnameTaken(cause(error)) },
}

// Las rutas de UN dominio: `not_found` es ese dominio (no la ruta, que es `no_route`).
const ONE_DOMAIN_ERRORS: DomainErrorMap = {
	phase: 3,
	fields: FIELDS,
	codes: {
		...UPSTREAM,
		not_found: (error) => new DomainNotFound(cause(error)),
		nothing_to_adopt: (error) => new NothingToAdopt(cause(error)),
	},
}

export const httpDomainRepository: DomainRepository = {
	list: (app, signal) =>
		withDomainErrors(
			async () => (await domainHttpDataSource.list(app, signal)).map(domainFromDto),
			LIST_ERRORS,
		),

	create: (app, domain) =>
		withDomainErrors(
			async () =>
				domainFromDto(await domainHttpDataSource.create(app, createDomainPayload(domain))),
			CREATE_ERRORS,
		),

	update: (app, hostname, changes) =>
		withDomainErrors(
			async () =>
				domainFromDto(
					await domainHttpDataSource.update(app, hostname, updateDomainPayload(changes)),
				),
			ONE_DOMAIN_ERRORS,
		),

	remove: (app, hostname) =>
		withDomainErrors(() => domainHttpDataSource.remove(app, hostname), ONE_DOMAIN_ERRORS),

	adopt: (app, hostname) =>
		withDomainErrors(
			async () => domainFromDto(await domainHttpDataSource.adopt(app, hostname)),
			ONE_DOMAIN_ERRORS,
		),

	retry: (app, hostname) =>
		withDomainErrors(
			async () => domainFromDto(await domainHttpDataSource.retry(app, hostname)),
			ONE_DOMAIN_ERRORS,
		),
}
