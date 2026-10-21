import type { AppDomain } from '../../entities/appDomain'
import type { AppRef } from '../../value-objects/appRef'
import { asHostname } from '../../value-objects/hostname'

export const APP: AppRef = { project: 'auditfy', stage: 'staging', app: 'api' }

/** Un dominio activo manejado por muelle en Cloudflare; `overrides` para el resto. */
export function makeDomain(hostname: string, overrides: Partial<AppDomain> = {}): AppDomain {
	return {
		hostname: asHostname(hostname),
		target: { service: 'api', port: 3000 },
		proxied: true,
		tlsMode: 'cloudflare',
		zoneId: 'zona-tudominio',
		recordId: 'registro-1',
		status: 'active',
		lastError: null,
		errorKind: null,
		certificate: { issuer: "Let's Encrypt", expiresAt: new Date('2027-01-08T00:00:00Z') },
		records: [{ type: 'A', name: hostname, content: '203.0.113.24' }],
		createdAt: new Date('2026-10-01T12:00:00Z'),
		updatedAt: new Date('2026-10-01T12:00:00Z'),
		...overrides,
	}
}

/** El del diseño: el registro ya existía y muelle no lo creó. */
export function recordExists(hostname: string): AppDomain {
	return makeDomain(hostname, {
		recordId: null,
		status: 'error',
		errorKind: 'record_exists',
		lastError: 'el registro ya existe',
		certificate: null,
	})
}

/** El del diseño: fuera de Cloudflare, esperando que el registro manual resuelva. */
export function manualPending(hostname: string): AppDomain {
	return makeDomain(hostname, {
		tlsMode: 'http',
		zoneId: null,
		recordId: null,
		proxied: false,
		status: 'pending',
		certificate: null,
	})
}
