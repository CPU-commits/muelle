import { type AppDomain, asHostname, type DomainChanges, type NewDomain } from '../../domain'
import type { CreateDomainPayload, DomainDto, UpdateDomainPayload } from '../dtos/domainDto'

export function domainFromDto(dto: DomainDto): AppDomain {
	const certificate =
		dto.cert_issuer && dto.cert_expires_at
			? { issuer: dto.cert_issuer, expiresAt: new Date(dto.cert_expires_at) }
			: null
	return {
		hostname: asHostname(dto.hostname),
		target: { service: dto.target_service, port: dto.target_port },
		proxied: dto.proxied,
		tlsMode: dto.tls_mode,
		zoneId: dto.cf_zone_id || null,
		recordId: dto.cf_record_id || null,
		status: dto.status,
		lastError: dto.last_error || null,
		errorKind: dto.last_error_code ?? null,
		certificate,
		records: dto.records.map((record) => ({ ...record })),
		createdAt: dto.created_at ? new Date(dto.created_at) : null,
		updatedAt: dto.updated_at ? new Date(dto.updated_at) : null,
	}
}

export function createDomainPayload(domain: NewDomain): CreateDomainPayload {
	return {
		hostname: domain.hostname,
		target_service: domain.target.service,
		target_port: domain.target.port,
		proxied: domain.proxied,
	}
}

export function updateDomainPayload(changes: DomainChanges): UpdateDomainPayload {
	return {
		...(changes.target
			? { target_service: changes.target.service, target_port: changes.target.port }
			: {}),
		...(changes.proxied !== undefined ? { proxied: changes.proxied } : {}),
	}
}
