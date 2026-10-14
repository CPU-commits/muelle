import { http } from '@/shared/data/http/client'
import { parseOrThrow } from '@/shared/data/http/parse'
import type { AppRef } from '../../domain'
import {
	type CreateDomainPayload,
	type DomainDto,
	domainDtoSchema,
	domainListDtoSchema,
	type ProxyDto,
	proxyDtoSchema,
	type UpdateDomainPayload,
} from '../dtos/domainDto'

function domainsPath(app: AppRef): string {
	const p = encodeURIComponent(app.project)
	const s = encodeURIComponent(app.stage)
	const a = encodeURIComponent(app.app)
	return `/projects/${p}/stages/${s}/apps/${a}/domains`
}

function domainPath(app: AppRef, hostname: string): string {
	return `${domainsPath(app)}/${encodeURIComponent(hostname)}`
}

// Rutas de la Fase 3 (spec §10). Todas exigen sesión (un token de API no maneja dominios).
export const domainHttpDataSource = {
	async list(app: AppRef, signal?: AbortSignal): Promise<DomainDto[]> {
		const payload = await http.get<unknown>(domainsPath(app), signal ? { signal } : {})
		return parseOrThrow(domainListDtoSchema, payload, 'ingress.list')
	},

	async create(app: AppRef, body: CreateDomainPayload): Promise<DomainDto> {
		const payload = await http.post<unknown>(domainsPath(app), { body })
		return parseOrThrow(domainDtoSchema, payload, 'ingress.create')
	},

	async update(app: AppRef, hostname: string, body: UpdateDomainPayload): Promise<DomainDto> {
		const payload = await http.patch<unknown>(domainPath(app, hostname), { body })
		return parseOrThrow(domainDtoSchema, payload, 'ingress.update')
	},

	async remove(app: AppRef, hostname: string): Promise<void> {
		await http.delete<unknown>(domainPath(app, hostname))
	},

	async adopt(app: AppRef, hostname: string): Promise<DomainDto> {
		const payload = await http.post<unknown>(`${domainPath(app, hostname)}/adopt`)
		return parseOrThrow(domainDtoSchema, payload, 'ingress.adopt')
	},

	async retry(app: AppRef, hostname: string): Promise<DomainDto> {
		const payload = await http.post<unknown>(`${domainPath(app, hostname)}/retry`)
		return parseOrThrow(domainDtoSchema, payload, 'ingress.retry')
	},

	/** `GET /proxy`: el estado de muelle-caddy (solo con sesión). */
	async proxy(signal?: AbortSignal): Promise<ProxyDto> {
		const payload = await http.get<unknown>('/proxy', signal ? { signal } : {})
		return parseOrThrow(proxyDtoSchema, payload, 'ingress.proxy')
	},
}
