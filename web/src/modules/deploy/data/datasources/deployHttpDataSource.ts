import { apiUrl, http } from '@/shared/data/http/client'
import { parseOrThrow } from '@/shared/data/http/parse'
import type { AppRef, DeployId, LogOffset, LogTail } from '../../domain'
import {
	APP_LOG_END_EVENT,
	APP_LOG_EVENT,
	type AppStatusDto,
	appStatusDtoSchema,
	type CreateDeployPayload,
	DEPLOY_LOG_EVENT,
	DEPLOY_STATUS_EVENT,
	type DeployAcceptedDto,
	deployAcceptedDtoSchema,
	type DeployDto,
	deployDtoSchema,
	deployListDtoSchema,
	type LogPageDto,
	logPageDtoSchema,
} from '../dtos/deployDto'
import { eventStream, type StreamItem } from './eventStream'

function appPath(app: AppRef): string {
	const p = encodeURIComponent(app.project)
	const s = encodeURIComponent(app.stage)
	const a = encodeURIComponent(app.app)
	return `/projects/${p}/stages/${s}/apps/${a}`
}

function withSignal(signal?: AbortSignal) {
	return signal ? { signal } : {}
}

// Rutas de la Fase 2 (spec §10). Todas exigen sesión; las que el spec abre a tokens
// (estado, deploys, logs, rollback) también los aceptan, pero el panel usa la cookie.
export const deployHttpDataSource = {
	async status(app: AppRef, signal?: AbortSignal): Promise<AppStatusDto> {
		const payload = await http.get<unknown>(`${appPath(app)}/status`, withSignal(signal))
		return parseOrThrow(appStatusDtoSchema, payload, 'deploy.status')
	},

	async list(app: AppRef, signal?: AbortSignal): Promise<DeployDto[]> {
		const payload = await http.get<unknown>(`${appPath(app)}/deploys`, withSignal(signal))
		return parseOrThrow(deployListDtoSchema, payload, 'deploy.list')
	},

	async get(app: AppRef, id: DeployId, signal?: AbortSignal): Promise<DeployDto> {
		const payload = await http.get<unknown>(`${appPath(app)}/deploys/${id}`, withSignal(signal))
		return parseOrThrow(deployDtoSchema, payload, 'deploy.get')
	},

	async create(app: AppRef, body: CreateDeployPayload): Promise<DeployAcceptedDto> {
		const payload = await http.post<unknown>(`${appPath(app)}/deploys`, { body })
		return parseOrThrow(deployAcceptedDtoSchema, payload, 'deploy.create')
	},

	async rollback(app: AppRef): Promise<DeployAcceptedDto> {
		const payload = await http.post<unknown>(`${appPath(app)}/rollback`)
		return parseOrThrow(deployAcceptedDtoSchema, payload, 'deploy.rollback')
	},

	async readLog(
		app: AppRef,
		id: DeployId,
		offset: LogOffset,
		signal?: AbortSignal,
	): Promise<LogPageDto> {
		const payload = await http.get<unknown>(`${appPath(app)}/deploys/${id}/log`, {
			query: { offset },
			...withSignal(signal),
		})
		return parseOrThrow(logPageDtoSchema, payload, 'deploy.log')
	},

	/** SSE `…/deploys/{id}/log/stream`: eventos `log` y `status`. */
	streamLog(
		app: AppRef,
		id: DeployId,
		offset: LogOffset,
		signal: AbortSignal,
	): AsyncGenerator<StreamItem, void, undefined> {
		// `offset` va en la query para empezar desde ahí. El panel no deja que el EventSource
		// reconecte solo (un corte sigue por polling desde el último `id`): `Last-Event-ID` no
		// llega a mandarse.
		const url = apiUrl(`${appPath(app)}/deploys/${id}/log/stream`, offset ? { offset } : {})
		return eventStream(url, [DEPLOY_LOG_EVENT, DEPLOY_STATUS_EVENT], signal)
	},

	/** SSE `…/logs?service&(tail|since)&follow=1`: eventos `log` (una línea JSON) y `end`. */
	streamAppLogs(
		app: AppRef,
		service: string,
		tail: LogTail,
		since: Date | null,
		signal: AbortSignal,
	): AsyncGenerator<StreamItem, void, undefined> {
		// Al reconectar va `since` (la hora de la última línea) en vez de `tail`: todo lo
		// escrito desde ahí, hasta 5000. Un backend que todavía no lo soporta lo ignora y manda
		// las últimas 200 (su `tail` por defecto): el panel descarta lo repetido.
		const query = since
			? { service, since: since.toISOString(), follow: 1 }
			: { service, tail, follow: 1 }
		const url = apiUrl(`${appPath(app)}/logs`, query)
		return eventStream(url, [APP_LOG_EVENT, APP_LOG_END_EVENT], signal)
	},
}
