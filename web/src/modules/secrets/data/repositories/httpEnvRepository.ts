import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import type { EnvField, EnvRepository } from '../../domain'
import { EnvLimitExceeded, EnvOutOfScope, EnvOwnerNotFound, utf8Length } from '../../domain'
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

// secrets no tiene mapper_errors.go propio: usa los comunes de cmd/http. Un 404 es que el
// proyecto, el stage o la app no existen; un 422 `invalid` trae `vars`, `set` o `unset`
// como campo (`"set: clave \"X\" inválida: …"`).
const ENV_ERRORS: DomainErrorMap = {
	codes: {
		out_of_scope: (error) => new EnvOutOfScope(cause(error)),
	},
	notFound: (error) => new EnvOwnerNotFound(cause(error)),
	fields: { vars: 'vars', set: 'set', unset: 'unset' },
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
