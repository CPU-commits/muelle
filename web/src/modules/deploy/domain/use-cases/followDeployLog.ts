import { type DeployLogEvent, LOG_START, type LogOffset } from '../entities/deployLog'
import { LogStreamFailed } from '../errors/deployErrors'
import type { DeployDeps } from '../repositories/deployRepository'
import type { AppRef } from '../value-objects/appRef'
import type { DeployId } from '../value-objects/deployId'
import { type DeployStatus, isTerminalStatus } from '../value-objects/deployStatus'

/** Cada cuánto se pregunta por el log cuando no hay SSE. */
export const LOG_POLL_INTERVAL_MS = 1000

export interface FollowDeployLogInput {
	readonly app: AppRef
	readonly id: DeployId
	/**
	 * `true` si el deploy todavía puede cambiar (en cola o corriendo): se sigue por SSE.
	 * Uno terminado se lee entero por `?offset`, sin abrir un stream.
	 */
	readonly live: boolean
}

export type FollowDeployLog = (
	input: FollowDeployLogInput,
	signal: AbortSignal,
) => AsyncGenerator<DeployLogEvent, void, undefined>

/**
 * El log de un deploy hasta que termina (spec §7, "Log del deploy"): por SSE mientras
 * funcione; si el stream no abre o se corta, avisa con `polling` y sigue con
 * `GET …/log?offset=N` desde el último offset recibido, sin repetir líneas. Termina
 * cuando el deploy queda en un estado final y no queda log por leer, o con `signal`.
 */
export function makeFollowDeployLog({ deploys, wait }: DeployDeps): FollowDeployLog {
	return async function* followDeployLog(input, signal) {
		let offset: LogOffset = LOG_START
		let status: DeployStatus | null = null

		if (input.live) {
			try {
				for await (const event of deploys.streamLog(input.app, input.id, offset, signal)) {
					if (event.kind === 'chunk') offset = event.nextOffset
					if (event.kind === 'status') status = event.status
					yield event
					if (event.kind === 'status' && isTerminalStatus(event.status)) return
				}
			} catch (error) {
				if (signal.aborted) return
				if (!(error instanceof LogStreamFailed)) throw error
			}
			if (signal.aborted) return
			yield { kind: 'polling' }
		}

		while (!signal.aborted) {
			const page = await deploys.readLog(input.app, input.id, offset, signal)
			if (signal.aborted) return
			if (page.chunk) yield { kind: 'chunk', text: page.chunk, nextOffset: page.nextOffset }
			offset = page.nextOffset
			if (page.status !== status) {
				status = page.status
				yield { kind: 'status', status }
			}
			// Terminado y sin nada nuevo: el log está completo. Con chunk, puede haber más
			// páginas: se pide la siguiente enseguida.
			if (isTerminalStatus(page.status) && !page.chunk) return
			if (!page.chunk) await wait(LOG_POLL_INTERVAL_MS, signal)
		}
	}
}
