import type { AppLogEvent, AppLogsQuery } from '../entities/appLogs'
import type { AppStatus } from '../entities/appStatus'
import type { Deploy, DeployAccepted } from '../entities/deploy'
import type { DeployLogEvent, LogOffset, LogPage } from '../entities/deployLog'
import type { DeployRequest } from '../entities/deployRequest'
import type { AppRef } from '../value-objects/appRef'
import type { DeployId } from '../value-objects/deployId'

// El PUERTO del módulo: estado, deploys y logs de una app (Fase 2, spec §7 y §10). Lo
// implementa `data/repositories/httpDeployRepository`; los tests usan el fake en memoria.
// Contra un backend sin la Fase 2, todo rechaza con `UnavailableError` (`phase: 2`).
export interface DeployRepository {
	status(app: AppRef, signal?: AbortSignal): Promise<AppStatus>
	/** El historial, del más nuevo al más viejo (los últimos 20). */
	list(app: AppRef, signal?: AbortSignal): Promise<readonly Deploy[]>
	get(app: AppRef, id: DeployId, signal?: AbortSignal): Promise<Deploy>
	create(app: AppRef, request: DeployRequest): Promise<DeployAccepted>
	/** `NoPreviousDeploy` si no hay a dónde volver. */
	rollback(app: AppRef): Promise<DeployAccepted>
	/** Una página del log desde `offset` (el polling de la API). */
	readLog(app: AppRef, id: DeployId, offset: LogOffset, signal?: AbortSignal): Promise<LogPage>
	/**
	 * El log por SSE desde `offset`: chunks y cambios de estado. Termina cuando el deploy
	 * termina (o con `signal`); si no abre o se corta, rechaza con `LogStreamFailed`.
	 * Nunca emite `polling`: eso lo decide el caso de uso.
	 */
	streamLog(
		app: AppRef,
		id: DeployId,
		offset: LogOffset,
		signal: AbortSignal,
	): AsyncIterable<DeployLogEvent>
	/** Los logs en vivo de un servicio (SSE). `LogStreamFailed` si no abre o se corta. */
	streamAppLogs(app: AppRef, query: AppLogsQuery, signal: AbortSignal): AsyncIterable<AppLogEvent>
}

/** Espera `ms`, o menos si `signal` aborta. Se inyecta para que los tests no esperen. */
export type Wait = (ms: number, signal: AbortSignal) => Promise<void>

export interface DeployDeps {
	readonly deploys: DeployRepository
	readonly wait: Wait
}

export const realWait: Wait = (ms, signal) =>
	new Promise<void>((resolve) => {
		if (signal.aborted) return resolve()
		const done = () => {
			clearTimeout(timer)
			signal.removeEventListener('abort', done)
			resolve()
		}
		const timer = setTimeout(done, ms)
		signal.addEventListener('abort', done)
	})
