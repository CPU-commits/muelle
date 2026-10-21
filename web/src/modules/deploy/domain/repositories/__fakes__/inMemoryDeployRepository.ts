import { NotFoundError, ValidationError } from '@/shared/domain'
import type { AppLogEvent, AppLogsQuery } from '../../entities/appLogs'
import type { AppStatus } from '../../entities/appStatus'
import { type Deploy, type DeployAccepted, rollbackTarget } from '../../entities/deploy'
import {
	asLogOffset,
	type DeployLogEvent,
	type LogOffset,
	type LogPage,
} from '../../entities/deployLog'
import type { DeployRequest } from '../../entities/deployRequest'
import { LogStreamFailed, NoPreviousDeploy } from '../../errors/deployErrors'
import type { AppRef } from '../../value-objects/appRef'
import { asDeployId, type DeployId } from '../../value-objects/deployId'
import type { DeployStatus } from '../../value-objects/deployStatus'
import type { DeployRepository } from '../deployRepository'

/** Un guion de stream: los eventos y, opcionalmente, cortarlo después de N. */
export interface StreamScript<T> {
	readonly events: readonly T[]
	/** Tras emitir esta cantidad, rechaza con `LogStreamFailed` (0 = no abre). */
	readonly failAfter?: number
}

/** Lo que ve cada `readLog`, en orden: la llamada N usa la foto N (la última se repite). */
export interface LogSnapshot {
	readonly text: string
	readonly status: DeployStatus
}

// El fake del puerto para los tests de dominio: todo en memoria y configurable desde el
// test, sin red ni MSW. Registra lo que se le pidió para poder verificarlo.
export class InMemoryDeployRepository implements DeployRepository {
	appStatus: AppStatus = { services: [] }
	deploys: Deploy[] = []
	/** Fotos del log por id de deploy. */
	logs = new Map<number, LogSnapshot[]>()
	/** Cuántos caracteres devuelve cada página de `readLog` (sin límite por defecto). */
	pageSize = Number.POSITIVE_INFINITY
	logStream: StreamScript<DeployLogEvent> = { events: [] }
	appLogStream: StreamScript<AppLogEvent> = { events: [] }

	readonly created: { app: AppRef; request: DeployRequest }[] = []
	readonly readLogOffsets: LogOffset[] = []
	readonly streamOffsets: LogOffset[] = []
	readonly appLogQueries: AppLogsQuery[] = []
	rollbacks = 0
	#reads = new Map<number, number>()

	async status(): Promise<AppStatus> {
		return this.appStatus
	}

	async list(): Promise<readonly Deploy[]> {
		return this.deploys
	}

	async get(_app: AppRef, id: DeployId): Promise<Deploy> {
		const deploy = this.deploys.find((candidate) => candidate.id === id)
		if (!deploy) throw new NotFoundError(`no existe el deploy #${id}`)
		return deploy
	}

	async create(app: AppRef, request: DeployRequest): Promise<DeployAccepted> {
		this.created.push({ app, request })
		const id = Math.max(0, ...this.deploys.map((deploy) => deploy.id)) + 1
		return { id: asDeployId(id) }
	}

	async rollback(): Promise<DeployAccepted> {
		this.rollbacks += 1
		const target = rollbackTarget(this.deploys)
		if (!target) throw new NoPreviousDeploy()
		const id = Math.max(0, ...this.deploys.map((deploy) => deploy.id)) + 1
		return { id: asDeployId(id) }
	}

	async readLog(_app: AppRef, id: DeployId, offset: LogOffset): Promise<LogPage> {
		this.readLogOffsets.push(offset)
		const snapshots = this.logs.get(id)
		if (!snapshots || snapshots.length === 0)
			throw new NotFoundError(`no existe el deploy #${id}`)
		const call = this.#reads.get(id) ?? 0
		this.#reads.set(id, call + 1)
		const snapshot = snapshots[Math.min(call, snapshots.length - 1)] as LogSnapshot
		// Como el backend: un offset más allá del final es `422 offset`.
		if (offset > snapshot.text.length)
			throw new ValidationError('pasa el largo del log', 'offset')
		const end = Math.min(snapshot.text.length, offset + this.pageSize)
		const chunk = snapshot.text.slice(offset, end)
		return {
			chunk,
			nextOffset: asLogOffset(offset + chunk.length),
			status: snapshot.status,
			supersededBy: null,
		}
	}

	async *streamLog(
		_app: AppRef,
		_id: DeployId,
		offset: LogOffset,
		signal: AbortSignal,
	): AsyncGenerator<DeployLogEvent> {
		this.streamOffsets.push(offset)
		yield* play(this.logStream, signal)
	}

	async *streamAppLogs(
		_app: AppRef,
		query: AppLogsQuery,
		signal: AbortSignal,
	): AsyncGenerator<AppLogEvent> {
		this.appLogQueries.push(query)
		yield* play(this.appLogStream, signal)
	}
}

async function* play<T>(script: StreamScript<T>, signal: AbortSignal): AsyncGenerator<T> {
	let emitted = 0
	for (const event of script.events) {
		if (signal.aborted) return
		if (script.failAfter !== undefined && emitted >= script.failAfter) {
			throw new LogStreamFailed(emitted > 0)
		}
		yield event
		emitted += 1
	}
	if (script.failAfter !== undefined && emitted >= script.failAfter && !signal.aborted) {
		throw new LogStreamFailed(emitted > 0)
	}
}
