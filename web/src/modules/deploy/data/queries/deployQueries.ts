import { type QueryClient, type QueryKey, queryOptions } from '@tanstack/svelte-query'
import { deployUseCases } from '../../container'
import {
	type AppRef,
	type CreateDeployInput,
	type Deploy,
	type DeployAccepted,
	type DeployId,
	type FollowDeployLogInput,
	hasActiveDeploy,
	latestDeploy,
	type StreamAppLogsInput,
} from '../../domain'

export const deployKeys = {
	all: ['deploy'] as QueryKey,
	app: (app: AppRef): QueryKey => ['deploy', app.project, app.stage, app.app],
	status: (app: AppRef): QueryKey => [...deployKeys.app(app), 'status'],
	list: (app: AppRef): QueryKey => [...deployKeys.app(app), 'list'],
	one: (app: AppRef, id: DeployId): QueryKey => [...deployKeys.app(app), 'one', id],
}

/** Cada cuánto se refresca el estado en el Resumen (con la pestaña visible). */
export const STATUS_REFRESH_MS = 5_000

/** Cada cuánto se refresca el historial mientras hay un deploy en cola o corriendo. */
export const ACTIVE_DEPLOY_REFRESH_MS = 3_000

export interface QueryToggles {
	/** Cada cuánto refrescar (ms), o `false` para nunca. Sin él, no se refresca solo. */
	readonly refreshEvery?: number | false
	/** `false` para no pedir nada (una fila de Proyectos fuera de la vista, un id vacío). */
	readonly enabled?: boolean
	/**
	 * Sin reintentos: un dato secundario (las columnas de una fila de Proyectos) que, si
	 * falla, dice "—" enseguida en vez de esperar dos reintentos.
	 */
	readonly noRetry?: boolean
}

function toggles({ refreshEvery, enabled, noRetry }: QueryToggles) {
	return {
		...(refreshEvery !== undefined ? { refetchInterval: refreshEvery } : {}),
		...(enabled !== undefined ? { enabled } : {}),
		...(noRetry ? { retry: false } : {}),
		// TanStack no refresca con la pestaña oculta (refetchIntervalInBackground: false).
		refetchIntervalInBackground: false,
	}
}

/** El estado de la app en Swarm. Lo usa el Resumen, Logs (servicios) y Proyectos (F2). */
export function appStatusQuery(app: AppRef, options: QueryToggles = {}) {
	return queryOptions({
		queryKey: deployKeys.status(app),
		queryFn: ({ signal }) => deployUseCases.getAppStatus(app, signal),
		staleTime: STATUS_REFRESH_MS,
		...toggles(options),
	})
}

/** El historial, del más nuevo al más viejo. */
export function deploysQuery(app: AppRef, options: QueryToggles = {}) {
	return queryOptions({
		queryKey: deployKeys.list(app),
		queryFn: ({ signal }) => deployUseCases.listDeploys(app, signal),
		staleTime: 10_000,
		...toggles(options),
	})
}

/** El historial para la pantalla Deploys: se refresca solo mientras hay uno en curso. */
export function liveDeploysQuery(app: AppRef) {
	return queryOptions({
		...deploysQuery(app),
		refetchInterval: (query) =>
			hasActiveDeploy(query.state.data ?? []) ? ACTIVE_DEPLOY_REFRESH_MS : false,
	})
}

/** Un deploy por id (el `?deploy=` que ya no está en el historial, o que no existe). */
export function deployQuery(app: AppRef, id: DeployId | null, options: QueryToggles = {}) {
	return queryOptions({
		queryKey: deployKeys.one(app, id ?? (0 as DeployId)),
		queryFn: ({ signal }) => deployUseCases.getDeploy(app, id ?? (0 as DeployId), signal),
		staleTime: 10_000,
		...toggles({ ...options, enabled: (options.enabled ?? true) && id !== null }),
	})
}

/**
 * El último deploy (de cualquier estado), o null si nunca hubo. Comparte el cache con
 * `deploysQuery`: es la misma petición con un `select`.
 */
export function lastDeployQuery(app: AppRef, options: QueryToggles = {}) {
	return queryOptions({
		...deploysQuery(app, options),
		select: (deploys: readonly Deploy[]): Deploy | null => latestDeploy(deploys),
	})
}

/** Lo que cambia cuando se encola un deploy: el historial y, después, el estado. */
function invalidateApp(client: QueryClient, app: AppRef): Promise<void> {
	return client.invalidateQueries({ queryKey: deployKeys.app(app) })
}

export function createDeployMutation(client: QueryClient) {
	return {
		mutationFn: (input: CreateDeployInput): Promise<DeployAccepted> =>
			deployUseCases.createDeploy(input),
		onSuccess: (_accepted: DeployAccepted, input: CreateDeployInput) =>
			invalidateApp(client, input.app),
	}
}

export function rollbackMutation(client: QueryClient) {
	return {
		mutationFn: (app: AppRef): Promise<DeployAccepted> => deployUseCases.rollback(app),
		onSuccess: (_accepted: DeployAccepted, app: AppRef) => invalidateApp(client, app),
	}
}

/** Un deploy cambió de estado (lo dijo el stream de su log): el historial ya no vale. */
export function refreshDeploys(client: QueryClient, app: AppRef): Promise<void> {
	return client.invalidateQueries({ queryKey: deployKeys.app(app) })
}

// Los streams no son queries (no hay nada que cachear: llegan, se muestran y se van),
// pero entran por acá igual, para que un viewModel nunca hable con el container.

/** El log de un deploy hasta que termina: SSE con fallback a `?offset`. */
export function followDeployLog(input: FollowDeployLogInput, signal: AbortSignal) {
	return deployUseCases.followDeployLog(input, signal)
}

/** Los logs en vivo de un servicio. */
export function streamAppLogs(input: StreamAppLogsInput, signal: AbortSignal) {
	return deployUseCases.streamAppLogs(input, signal)
}
