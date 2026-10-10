import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import {
	type AppLogEvent,
	asLogOffset,
	type DeployLogEvent,
	DeployNotFound,
	type DeployRepository,
	isDeployStatus,
	LogStreamFailed,
	NoPreviousDeploy,
} from '../../domain'
import { deployHttpDataSource } from '../datasources/deployHttpDataSource'
import { EventStreamError, type StreamMessage } from '../datasources/eventStream'
import {
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

// Todo es de la Fase 2: contra un backend sin ella, la ruta no existe (`no_route` /
// `method_not_allowed`) y es `UnavailableError` con `phase: 2`: la pantalla muestra
// <ComingSoon>. Un `not_found` es otra cosa: el recurso no existe.
const DEPLOY_ERRORS: DomainErrorMap = { phase: 2 }

// Las rutas de UN deploy (`…/deploys/{id}`, su log): `not_found` es ese deploy. Va en
// `codes` para que gane siempre; la ruta inexistente (`no_route`) sigue siendo la fase.
const ONE_DEPLOY_ERRORS: DomainErrorMap = {
	phase: 2,
	codes: { not_found: (error) => new DeployNotFound(cause(error)) },
}

// Un 409 en el rollback es "no hay un succeeded anterior" (spec §5.2 regla 6). Se acepta
// el código propio que se supone en el README y el `conflict` genérico del Kit.
const ROLLBACK_ERRORS: DomainErrorMap = {
	phase: 2,
	codes: {
		no_previous_deploy: (error) => new NoPreviousDeploy(cause(error)),
		conflict: (error) => new NoPreviousDeploy(cause(error)),
	},
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

	create: (app, request) =>
		withDomainErrors(
			async () =>
				deployAcceptedFromDto(
					await deployHttpDataSource.create(app, createDeployPayload(request)),
				),
			DEPLOY_ERRORS,
		),

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
				if (event) yield event
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
				signal,
			)) {
				if (item.kind === 'open') {
					opened = true
					yield { kind: 'open' }
					continue
				}
				const event = appLogEvent(item)
				if (event) yield event
			}
		} catch (error) {
			throw streamFailure(error, opened)
		}
	},
}
