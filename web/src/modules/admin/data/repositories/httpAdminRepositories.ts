import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import type { AuditRepository, RegistryRepository, SettingsRepository } from '../../domain'
import {
	CloudflareUnavailable,
	PortsDockerUnavailable,
	RegistryExists,
	RegistryNotFound,
	TotpRequiredFirst,
	WouldLockOut,
} from '../../domain'
import { adminHttpDataSource } from '../datasources/adminHttpDataSource'
import {
	auditEventFromDto,
	newRegistryPayload,
	registryCredentialsPayload,
	registryFromDto,
	settingsFromDto,
	settingsPatchPayload,
} from '../mappers/adminMapper'

const cause = (error: ApiError) => ({ cause: error })

// `totp_required_first` (mapper_errors.go), `would_lock_out`, `cloudflare_unavailable` (al
// verificar el token o el permiso de los certificados de Cloudflare) y `docker_unavailable`
// (contrato §4.1: el único `PATCH` que toca Docker es el de los puertos, que los prueba antes
// de guardar), y los 422 de los campos de la Fase 3 y de Origin CA.
const SETTINGS_ERRORS: DomainErrorMap = {
	codes: {
		totp_required_first: (error) => new TotpRequiredFirst(cause(error)),
		would_lock_out: (error) => new WouldLockOut(cause(error)),
		cloudflare_unavailable: (error) => new CloudflareUnavailable(cause(error)),
		docker_unavailable: (error) => new PortsDockerUnavailable(cause(error)),
	},
	fields: {
		public_ip: 'ipv4',
		public_ipv6: 'ipv6',
		acme_email: 'acmeEmail',
		proxy_http_port: 'httpPort',
		proxy_https_port: 'httpsPort',
		panel_allowed_cidrs: 'panelAllowedCidrs',
		cloudflare_token: 'cloudflareToken',
		cloudflare_origin_ca: 'cloudflareOriginCa',
	},
}

// `/registries` (contrato §2.10): un 404 `not_found` es el registry que ya no está.
const REGISTRY_ERRORS: DomainErrorMap = {
	codes: { conflict: (error) => new RegistryExists(cause(error)) },
	notFound: (error) => new RegistryNotFound(cause(error)),
	fields: { host: 'host', username: 'username', password: 'password' },
}

export const httpSettingsRepository: SettingsRepository = {
	get: (signal) =>
		withDomainErrors(
			async () => settingsFromDto(await adminHttpDataSource.getSettings(signal)),
			SETTINGS_ERRORS,
		),

	patch: (patch) =>
		withDomainErrors(
			async () =>
				settingsFromDto(
					await adminHttpDataSource.patchSettings(settingsPatchPayload(patch)),
				),
			SETTINGS_ERRORS,
		),
}

export const httpAuditRepository: AuditRepository = {
	list: (query, signal) =>
		withDomainErrors(async () =>
			(await adminHttpDataSource.listAudit({ ...query }, signal)).map(auditEventFromDto),
		),
}

export const httpRegistryRepository: RegistryRepository = {
	list: (signal) =>
		withDomainErrors(
			async () => (await adminHttpDataSource.listRegistries(signal)).map(registryFromDto),
			REGISTRY_ERRORS,
		),

	create: (registry) =>
		withDomainErrors(
			async () =>
				registryFromDto(
					await adminHttpDataSource.createRegistry(newRegistryPayload(registry)),
				),
			REGISTRY_ERRORS,
		),

	update: (host, credentials) =>
		withDomainErrors(
			async () =>
				registryFromDto(
					await adminHttpDataSource.updateRegistry(
						host,
						registryCredentialsPayload(credentials),
					),
				),
			REGISTRY_ERRORS,
		),

	remove: (host) =>
		withDomainErrors(() => adminHttpDataSource.deleteRegistry(host), REGISTRY_ERRORS),
}
