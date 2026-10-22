import {
	type AppDomain,
	asHostname,
	type DomainChanges,
	type NewDomain,
	type OriginCaState,
	type ProxyPorts,
	type ProxyStatus,
} from '../../domain'
import type {
	CreateDomainPayload,
	DomainDto,
	OriginCaDto,
	ProxyDto,
	UpdateDomainPayload,
} from '../dtos/domainDto'

function dateOrNull(text: string | null): Date | null {
	return text ? new Date(text) : null
}

export function originCaFromDto(dto: OriginCaDto | null): OriginCaState | null {
	if (!dto) return null
	return {
		status: dto.status,
		errorCode: dto.error_code,
		error: dto.error || null,
		retryAt: dateOrNull(dto.retry_at),
		expiresAt: dateOrNull(dto.expires_at),
	}
}

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
		certSource: dto.cert_source,
		originCa: originCaFromDto(dto.origin_ca),
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

export function proxyFromDto(dto: ProxyDto): ProxyStatus {
	return {
		status: dto.status,
		error: dto.error || null,
		syncedAt: dto.synced_at ? new Date(dto.synced_at) : null,
		caddy: {
			image: dto.caddy.image || null,
			running: dto.caddy.running,
			httpPort: dto.caddy.http_port ?? null,
			httpsPort: dto.caddy.https_port ?? null,
		},
		portsRollback: dto.ports_rollback
			? {
					attempted: proxyPortsFromDto(dto.ports_rollback.attempted),
					restored: proxyPortsFromDto(dto.ports_rollback.restored),
					error: dto.ports_rollback.error,
					at: new Date(dto.ports_rollback.at),
				}
			: null,
	}
}

function proxyPortsFromDto(dto: { http_port: number; https_port: number }): ProxyPorts {
	return { httpPort: dto.http_port, httpsPort: dto.https_port }
}
