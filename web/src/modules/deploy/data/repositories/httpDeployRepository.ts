import type { ApiError } from '@/shared/data/http/errors'
import {
	type DomainErrorMap,
	toDomainError,
	withDomainErrors,
} from '@/shared/data/http/domainErrors'
import { UnauthorizedError, ValidationError } from '@/shared/domain'
import {
	type AppLogEvent,
	type AppRef,
	AppRemoving,
	asLogOffset,
	type DeployLogEvent,
	DeployNotFound,
	type DeployRepository,
	DockerUnavailable,
	ImageRejected,
	type ImageRejectionReason,
	isDeployStatus,
	isTerminalStatus,
	LogStreamFailed,
	NoPreviousDeploy,
} from '../../domain'
import { deployHttpDataSource } from '../datasources/deployHttpDataSource'
import { EventStreamError, type StreamMessage } from '../datasources/eventStream'
import {
	APP_LOG_END_EVENT,
	APP_LOG_EVENT,
	appLogLineDtoSchema,
	DEPLOY_LOG_EVENT,
	DEPLOY_STATUS_EVENT,
} from '../dtos/deployDto'
import {
	appLogLineFromDto,
	appStatusFromDto,
	createDeployPayload,
	deployAcceptedFromDto,
	deployFromDto,
	logPageFromDto,
} from '../mappers/deployMapper'

const cause = (error: ApiError) => ({ cause: error })

// `503 docker_unavailable` (contrato §0.2): Docker no responde, con su propio texto.
const docker = { docker_unavailable: (error: ApiError) => new DockerUnavailable(cause(error)) }

const DEPLOY_ERRORS: DomainErrorMap = { codes: docker }

// Las rutas de UN deploy (`…/deploys/{id}`, su log): `not_found` es ese deploy.
const ONE_DEPLOY_ERRORS: DomainErrorMap = {
	codes: { ...docker, not_found: (error) => new DeployNotFound(cause(error)) },
}

// Encolar (un deploy o un rollback): `409 conflict` es una app que se está borrando
// (contrato §1.1 y §2.2), y dura a lo más 2 min.
const removing = { conflict: (error: ApiError) => new AppRemoving(cause(error)) }

const CREATE_ERRORS: DomainErrorMap = { codes: { ...docker, ...removing } }

// `409 no_previous_deploy`: no hay un succeeded anterior al actual (contrato §2.7).
const ROLLBACK_ERRORS: DomainErrorMap = {
	codes: {
		...docker,
		...removing,
		no_previous_deploy: (error) => new NoPreviousDeploy(cause(error)),
	},
}

// Los motivos del `422` de una imagen (contrato §2.2). El backend no les da código propio:
// se reconocen por el texto, y lo que no se reconoce es "ref inválida".
const IMAGE_REASONS: readonly [RegExp, ImageRejectionReason][] = [
	[/no los dos/i, 'both'],
	[/compose|por servicio/i, 'compose'],
	[/servicio no existe|no existe en la app/i, 'unknown_service'],
]

/** Un 422 en `image` o `images.<servicio>` sale como `ImageRejected`, con el motivo. */
function imageRejection(error: unknown): unknown {
	if (!(error instanceof ValidationError) || !error.field) return error
	const service = error.field === 'image' ? null : error.field.replace(/^images\./, '')
	if (service !== null && !error.field.startsWith('images.')) return error
	const reason = IMAGE_REASONS.find(([pattern]) => pattern.test(error.message))?.[1] ?? 'invalid'
	return new ImageRejected(error.message, service, reason, { cause: error })
}

/** Si la sesión se cayó: el 401 sale como `UnauthorizedError` (y el cliente ya mandó al login). */
async function probeSession(app: AppRef): Promise<void> {
	try {
		await deployHttpDataSource.status(app)
	} catch (error) {
		const translated = toDomainError(error, DEPLOY_ERRORS)
		if (translated instanceof UnauthorizedError) throw translated
	}
}

/** Un stream que no abre, se corta o manda algo que no se entiende: falla como stream. */
function streamFailure(error: unknown, opened: boolean): unknown {
	if (error instanceof EventStreamError)
		return new LogStreamFailed(error.opened, { cause: error })
	if (error instanceof DOMException && error.name === 'AbortError') return error
	if (error instanceof LogStreamFailed) return error
	return new LogStreamFailed(opened, { cause: error })
}

function deployLogEvent(item: StreamMessage): DeployLogEvent | null {
	if (item.type === DEPLOY_LOG_EVENT) {
		const next = Number(item.lastEventId)
		if (item.lastEventId === '' || !Number.isSafeInteger(next) || next < 0) {
			throw new Error(`id de evento inválido: "${item.lastEventId}"`)
		}
		return { kind: 'chunk', text: `${item.data}\n`, nextOffset: asLogOffset(next) }
	}
	if (item.type === DEPLOY_STATUS_EVENT) {
		const status = item.data.trim()
		if (!isDeployStatus(status)) throw new Error(`estado inválido: "${status}"`)
		return { kind: 'status', status }
	}
	return null
}

function appLogEvent(item: StreamMessage): AppLogEvent | null {
	if (item.type === APP_LOG_END_EVENT) {
		return { kind: 'end', reason: item.data.trim() === 'timeout' ? 'timeout' : 'eof' }
	}
	if (item.type !== APP_LOG_EVENT) return null
	let payload: unknown = null
	try {
		payload = JSON.parse(item.data)
	} catch {
		payload = null
	}
	const parsed = appLogLineDtoSchema.safeParse(payload)
	// Una línea que no es el JSON acordado se muestra igual, como texto: una línea rara
	// no corta los logs de nadie.
	const dto = parsed.success ? parsed.data : { line: item.data, stream: 'stdout' as const }
	return { kind: 'line', line: appLogLineFromDto(dto) }
}

export const httpDeployRepository: DeployRepository = {
	status: (app, signal) =>
		withDomainErrors(
			async () => appStatusFromDto(await deployHttpDataSource.status(app, signal)),
			DEPLOY_ERRORS,
		),

	list: (app, signal) =>
		withDomainErrors(
			async () => (await deployHttpDataSource.list(app, signal)).map(deployFromDto),
			DEPLOY_ERRORS,
		),

	get: (app, id, signal) =>
		withDomainErrors(
			async () => deployFromDto(await deployHttpDataSource.get(app, id, signal)),
			ONE_DEPLOY_ERRORS,
		),

	create: async (app, request) => {
		try {
			return await withDomainErrors(
				async () =>
					deployAcceptedFromDto(
						await deployHttpDataSource.create(app, createDeployPayload(request)),
					),
				CREATE_ERRORS,
			)
		} catch (error) {
			throw imageRejection(error)
		}
	},

	rollback: (app) =>
		withDomainErrors(
			async () => deployAcceptedFromDto(await deployHttpDataSource.rollback(app)),
			ROLLBACK_ERRORS,
		),

	readLog: (app, id, offset, signal) =>
		withDomainErrors(
			async () => logPageFromDto(await deployHttpDataSource.readLog(app, id, offset, signal)),
			ONE_DEPLOY_ERRORS,
		),

	async *streamLog(app, id, offset, signal) {
		let opened = false
		try {
			for await (const item of deployHttpDataSource.streamLog(app, id, offset, signal)) {
				if (item.kind === 'open') {
					opened = true
					continue
				}
				const event = deployLogEvent(item)
				if (!event) continue
				yield event
				// Después del status final el servidor cierra: no es un corte.
				if (event.kind === 'status' && isTerminalStatus(event.status)) return
			}
		} catch (error) {
			throw streamFailure(error, opened)
		}
	},

	async *streamAppLogs(app, query, signal) {
		let opened = false
		try {
			for await (const item of deployHttpDataSource.streamAppLogs(
				app,
				query.service,
				query.tail,
				query.since ?? null,
				signal,
			)) {
				if (item.kind === 'open') {
					opened = true
					yield { kind: 'open' }
					continue
				}
				const event = appLogEvent(item)
				if (!event) continue
				yield event
				// Después de `end` el servidor cierra: no es un corte, no hay más que leer.
				if (event.kind === 'end') return
			}
		} catch (error) {
			const failure = streamFailure(error, opened)
			// Un EventSource no deja ver el status: si no abrió, se pregunta por la sesión. Un
			// 401 ahí lo maneja el cliente HTTP (manda al login) y no se reintenta más.
			if (failure instanceof LogStreamFailed && !failure.opened && !signal.aborted) {
				await probeSession(app)
			}
			throw failure
		}
	},
}
