import { ValidationError } from '@/shared/domain'
import { type AppLogEvent, LOG_TAILS, type LogTail } from '../entities/appLogs'
import type { DeployDeps } from '../repositories/deployRepository'
import type { AppRef } from '../value-objects/appRef'

export interface StreamAppLogsInput {
	readonly app: AppRef
	readonly service: string
	/** Cuántas líneas viejas traer antes de seguir. */
	readonly tail: LogTail
	/** Al reconectar: seguir desde la hora de la última línea recibida (en vez de `tail`). */
	readonly since?: Date | null
}

export type StreamAppLogs = (
	input: StreamAppLogsInput,
	signal: AbortSignal,
) => AsyncGenerator<AppLogEvent, void, undefined>

function isLogTail(value: number): value is LogTail {
	return value === 0 || (LOG_TAILS as readonly number[]).includes(value)
}

/** Los logs en vivo de un servicio, hasta `signal`. `LogStreamFailed` si se corta. */
export function makeStreamAppLogs({ deploys }: Pick<DeployDeps, 'deploys'>): StreamAppLogs {
	return async function* streamAppLogs(input, signal) {
		const service = input.service.trim()
		if (!service) throw new ValidationError('elige un servicio', 'service')
		if (!isLogTail(input.tail)) throw new ValidationError('cantidad de líneas inválida', 'tail')
		const since = input.since ?? null
		if (since && Number.isNaN(since.getTime())) {
			throw new ValidationError('fecha inválida', 'since')
		}
		yield* deploys.streamAppLogs(
			input.app,
			{ service, tail: input.tail, ...(since ? { since } : {}) },
			signal,
		)
	}
}
