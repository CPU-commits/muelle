import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import type { EnvField, EnvLimit, EnvRepository } from '../../domain'
import {
	EnvLimitExceeded,
	EnvOutOfScope,
	EnvOwnerNotFound,
	EnvRejected,
	utf8Length,
} from '../../domain'
import { envHttpDataSource } from '../datasources/envHttpDataSource'
import { envFromDto, patchPayload, replacePayload } from '../mappers/envMapper'

const cause = (error: ApiError) => ({ cause: error })

// `MaxBodyBytes` del Kit (src/cmd/http/utils/kit.go): un cuerpo más grande es un 400
// `invalid_json`. Se revisa antes de mandar, con el mismo JSON que manda el cliente HTTP.
export const MAX_BODY_BYTES = 1024 * 1024

function assertBodyFits(body: unknown, field: EnvField): void {
	if (utf8Length(JSON.stringify(body)) > MAX_BODY_BYTES) {
		throw new EnvLimitExceeded('request', field)
	}
}

// Un 422 `invalid` trae `"<campo>: <motivo>"`, con `vars`, `set` o `unset` como campo.
const FIELD_REASON = /^(vars|set|unset): (.+)$/s

// Los motivos de `entity.CheckCount`, `CheckTotals` y del tope de `unset` en PatchEnv: el
// panel los revisa antes de mandar, salvo cuando no puede (el tamaño de las secretas).
const LIMIT_REASONS: readonly { pattern: RegExp; limit: EnvLimit }[] = [
	{ pattern: /^máximo \d+ (variables|claves)$/, limit: 'count' },
	{ pattern: /suman más de 1 MiB/, limit: 'size' },
]

/**
 * Un 422 nunca llega a la pantalla con el texto del backend (en español, y sin traducir):
 * un límite conocido es `EnvLimitExceeded`; cualquier otro motivo, `EnvRejected`. Los dos
 * conservan el campo.
 */
function invalidEnv(error: ApiError): Error {
	const match = FIELD_REASON.exec(error.message)
	const field = match?.[1] as EnvField | undefined
	const reason = match?.[2] ?? error.message
	const known = LIMIT_REASONS.find(({ pattern }) => pattern.test(reason))
	if (known && field) return new EnvLimitExceeded(known.limit, field)
	return new EnvRejected(field, reason, cause(error))
}

// secrets no tiene mapper_errors.go propio: usa los comunes de cmd/http. Un 404 es que el
// proyecto, el stage o la app no existen; un 422 `invalid`, ver `invalidEnv`.
const ENV_ERRORS: DomainErrorMap = {
	codes: {
		invalid: invalidEnv,
		out_of_scope: (error) => new EnvOutOfScope(cause(error)),
	},
	notFound: (error) => new EnvOwnerNotFound(cause(error)),
}

export const httpEnvRepository: EnvRepository = {
	list: (owner, signal) =>
		withDomainErrors(
			async () => envFromDto(await envHttpDataSource.list(owner, signal)),
			ENV_ERRORS,
		),

	replace: (owner, replacement) =>
		withDomainErrors(async () => {
			const body = replacePayload(replacement)
			assertBodyFits(body, 'vars')
			await envHttpDataSource.replace(owner, body)
		}, ENV_ERRORS),

	patch: (owner, patch) =>
		withDomainErrors(async () => {
			const body = patchPayload(patch)
			assertBodyFits(body, patch.set.size > 0 ? 'set' : 'unset')
			await envHttpDataSource.patch(owner, body)
		}, ENV_ERRORS),
}
