import { type DomainTarget, formatTarget } from '../value-objects/domainTarget'
import type {
	DnsRecordType,
	DomainErrorKind,
	DomainStatus,
	TlsMode,
} from '../value-objects/domainStatus'
import type { Hostname } from '../value-objects/hostname'

// Un dominio de una app (spec §5.1 `domains`, §8 y §9). Crearlo, editarlo o borrarlo
// aplica al instante el DNS y la ruta de Caddy: no pasa por un deploy (§5.2 regla 4).

/** Un registro que el dominio necesita: el que muelle maneja, o el que hay que crear a mano. */
export interface DnsRecord {
	readonly type: DnsRecordType
	readonly name: string
	readonly content: string
}

export interface Certificate {
	readonly issuer: string
	readonly expiresAt: Date
}

export interface AppDomain {
	readonly hostname: Hostname
	readonly target: DomainTarget
	readonly proxied: boolean
	readonly tlsMode: TlsMode
	/** La zona de Cloudflare donde cae el hostname; null si no está en ninguna. */
	readonly zoneId: string | null
	/** El registro que creó o adoptó muelle; null si muelle no lo maneja. */
	readonly recordId: string | null
	readonly status: DomainStatus
	readonly lastError: string | null
	readonly errorKind: DomainErrorKind | null
	readonly certificate: Certificate | null
	readonly records: readonly DnsRecord[]
	readonly createdAt: Date | null
	/** El último cambio del dominio (estado, destino, Proxied…). */
	readonly updatedAt: Date | null
}

/** Lo que pide "Agregar". */
export interface NewDomain {
	readonly hostname: Hostname
	readonly target: DomainTarget
	readonly proxied: boolean
}

/** Lo que cambia "Editar" o el switch de Proxied. */
export interface DomainChanges {
	readonly target?: DomainTarget
	readonly proxied?: boolean
}

/**
 * Cómo se resuelve el DNS:
 * - `cloudflare`: muelle maneja el registro (lo creó o lo adoptó).
 * - `manual`: fuera de Cloudflare; el registro se crea a mano en otro proveedor.
 * - `unmanaged`: la zona está en Cloudflare pero muelle no maneja el registro (el que ya
 *   existía, o uno que falló al crearse).
 */
export type DnsMode = 'cloudflare' | 'manual' | 'unmanaged'

export function dnsMode(domain: AppDomain): DnsMode {
	if (domain.recordId !== null) return 'cloudflare'
	if (domain.tlsMode === 'http' && domain.zoneId === null) return 'manual'
	return 'unmanaged'
}

/** El estado que se muestra y la acción que va con él. */
export type DomainState =
	| 'active'
	/** Esperando que el registro manual resuelva: "Verificar". */
	| 'waiting_dns'
	/** Creando el registro o emitiendo el certificado: se refresca solo. */
	| 'provisioning'
	/** El registro ya existía en Cloudflare: "Adoptar". */
	| 'record_exists'
	/** Cloudflare o el certificado fallaron: "Reintentar". */
	| 'error'

export function domainState(domain: AppDomain): DomainState {
	if (domain.status === 'active') return 'active'
	if (domain.status === 'pending') {
		return dnsMode(domain) === 'manual' ? 'waiting_dns' : 'provisioning'
	}
	// Con `status: error` el backend siempre manda `last_error_code` (un CHECK de la tabla):
	// el caso se reconoce por el código, nunca por el texto del mensaje.
	return domain.errorKind === 'record_exists' && domain.recordId === null
		? 'record_exists'
		: 'error'
}

/**
 * Hay algo que se resuelve solo en el backend (muelle creando el registro o emitiendo el
 * certificado): la lista se refresca seguido hasta que termine.
 */
export function hasProvisioning(domains: readonly AppDomain[]): boolean {
	return domains.some((domain) => domainState(domain) === 'provisioning')
}

/**
 * Hay uno que espera un registro manual. muelle lo verifica cada minuto mientras siga
 * `pending` (contrato §3.1), así que también cambia solo, aunque más despacio.
 */
export function hasWaitingDns(domains: readonly AppDomain[]): boolean {
	return domains.some((domain) => domainState(domain) === 'waiting_dns')
}

/** Los que están en `error` (con o sin "Adoptar"), por hostname. */
export function failingDomains(domains: readonly AppDomain[]): AppDomain[] {
	return sortDomains(
		domains.filter((domain) => {
			const state = domainState(domain)
			return state === 'error' || state === 'record_exists'
		}),
	)
}

/** El switch de Proxied solo tiene sentido si muelle maneja el registro (§9). */
export function canToggleProxied(domain: AppDomain): boolean {
	return dnsMode(domain) === 'cloudflare'
}

/** Muelle verifica el certificado y pasa el dominio a `error` si vence en menos de esto. */
export const CERT_WARNING_DAYS = 14
const DAY_MS = 24 * 60 * 60 * 1000

export function certificateExpiresSoon(domain: AppDomain, nowMs: number = Date.now()): boolean {
	const certificate = domain.certificate
	if (!certificate) return false
	return certificate.expiresAt.getTime() - nowMs < CERT_WARNING_DAYS * DAY_MS
}

/** El dominio es un link que se puede abrir: activo, con certificado. */
export function isReachable(domain: AppDomain): boolean {
	return domain.status === 'active'
}

/** Los registros que hay que crear a mano (solo si muelle no los maneja). */
export function manualRecords(domain: AppDomain): readonly DnsRecord[] {
	return dnsMode(domain) === 'manual' ? domain.records : []
}

/** La IP a la que apunta (o apuntaría) el registro A. */
export function publicAddress(domain: AppDomain): string | null {
	return domain.records.find((record) => record.type === 'A')?.content ?? null
}

/**
 * Los destinos que ofrece el formulario: los que pasó quien llama (los del catálogo)
 * y los que ya usan los dominios de la app, sin repetir.
 */
export function suggestedTargets(
	domains: readonly AppDomain[],
	known: readonly string[] = [],
): string[] {
	const all = [...known, ...domains.map((domain) => formatTarget(domain.target))]
	return [...new Set(all)]
}

/** Por hostname, para que la lista no salte cuando cambia un estado. */
export function sortDomains(domains: readonly AppDomain[]): AppDomain[] {
	return [...domains].sort((a, b) => a.hostname.localeCompare(b.hostname))
}

export function replaceDomain(
	domains: readonly AppDomain[],
	updated: AppDomain,
): readonly AppDomain[] {
	return domains.map((domain) => (domain.hostname === updated.hostname ? updated : domain))
}
