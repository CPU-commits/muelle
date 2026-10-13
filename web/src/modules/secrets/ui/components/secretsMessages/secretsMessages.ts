import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ForbiddenError, UnavailableError, ValidationError } from '@/shared/domain'
import {
	type DotenvIssue,
	type DotenvSwallowed,
	type EnvKeyProblem,
	type EnvLimit,
	EnvLimitExceeded,
	EnvOwnerNotFound,
	type EnvOwnerKind,
	type EnvValueProblem,
	type ImportChangeReason,
	InvalidEnvKey,
	InvalidEnvValue,
	MAX_ENV_VARS,
} from '../../../domain'

// La pantalla elige el texto por la CLASE del error de dominio (y su `problem`), nunca
// por un status.

export function keyProblemMessage(problem: EnvKeyProblem): string {
	switch (problem) {
		case 'empty':
			return m.common_required()
		case 'format':
			return m.secrets_error_key_format()
		case 'too_long':
			return m.secrets_error_key_too_long()
		case 'reserved':
			return m.secrets_error_key_reserved()
	}
}

export function valueProblemMessage(problem: EnvValueProblem): string {
	switch (problem) {
		case 'too_large':
			return m.secrets_error_value_too_large()
		case 'nul':
			return m.secrets_error_value_nul()
		case 'invalid_utf8':
			return m.secrets_error_value_invalid_utf8()
	}
}

export function limitMessage(limit: EnvLimit): string {
	switch (limit) {
		case 'count':
			return m.secrets_error_limit_count({ max: MAX_ENV_VARS })
		case 'size':
			return m.secrets_error_limit_size()
		case 'request':
			return m.secrets_error_limit_request()
	}
}

export function notFoundMessage(kind: EnvOwnerKind): string {
	return kind === 'app' ? m.secrets_not_found_app() : m.secrets_not_found_stage()
}

/** El texto de un error de leer o guardar variables. */
export function secretsErrorMessage(error: unknown, kind: EnvOwnerKind): string {
	if (error instanceof InvalidEnvKey) return keyProblemMessage(error.problem)
	if (error instanceof InvalidEnvValue) return valueProblemMessage(error.problem)
	if (error instanceof EnvLimitExceeded) return limitMessage(error.limit)
	if (error instanceof EnvOwnerNotFound) return notFoundMessage(kind)
	if (error instanceof ForbiddenError) return m.secrets_error_out_of_scope()
	if (error instanceof UnavailableError) return m.common_unavailable()
	// Un 422 con una regla que el panel no conoce (EnvRejected) o cualquier otra validación:
	// un texto propio y traducido. El motivo del backend va en español: queda para logs.
	if (error instanceof ValidationError) return m.secrets_error_rejected()
	return m.common_error_generic()
}

/** "Línea 3: COMPOSE_X: el prefijo COMPOSE_ está reservado." */
/** "Línea 4: parece la variable B, pero quedó dentro del valor de PEM…" */
export function dotenvSwallowedMessage(swallowed: DotenvSwallowed): string {
	return m.secrets_import_swallowed({
		line: swallowed.line,
		looksLike: swallowed.looksLike,
		key: swallowed.key,
	})
}

export function dotenvIssueMessage(issue: DotenvIssue): string {
	const key = issue.key ?? ''
	const message = (() => {
		switch (issue.problem) {
			case 'missing_equals':
				return m.secrets_dotenv_missing_equals()
			case 'empty_key':
				return m.secrets_dotenv_empty_key()
			case 'invalid_key':
				return m.secrets_dotenv_invalid_key({ key })
			case 'key_too_long':
				return m.secrets_dotenv_key_too_long()
			case 'reserved_key':
				return m.secrets_dotenv_reserved_key({ key })
			case 'unterminated_quote':
				return m.secrets_dotenv_unterminated_quote()
			case 'trailing_text':
				return m.secrets_dotenv_trailing_text()
			case 'value_too_large':
				return m.secrets_dotenv_value_too_large({ key })
			case 'value_nul':
				return m.secrets_dotenv_value_nul({ key })
			case 'value_invalid_utf8':
				return m.secrets_dotenv_value_invalid_utf8({ key })
		}
	})()
	return m.secrets_import_line({ line: issue.line, message })
}

export function changeReasonLabel(reason: ImportChangeReason): string {
	switch (reason) {
		case 'value':
			return m.secrets_import_reason_value()
		case 'to_secret':
			return m.secrets_import_reason_to_secret()
		case 'to_plain':
			return m.secrets_import_reason_to_plain()
	}
}

/** La descripción del toast de un cambio guardado: cuándo se aplica. */
export function savedNote(kind: EnvOwnerKind): string {
	return kind === 'app' ? m.secrets_saved_app() : m.secrets_saved_stage()
}

/** Lo mismo para varias variables (importar un .env). */
export function importedNote(kind: EnvOwnerKind): string {
	return kind === 'app' ? m.secrets_imported_app() : m.secrets_imported_stage()
}

export function savedKeyNote(kind: EnvOwnerKind, key: string): string {
	return kind === 'app' ? m.secrets_saved_key_app({ key }) : m.secrets_saved_key_stage({ key })
}

export function deletedKeyNote(kind: EnvOwnerKind, key: string): string {
	return kind === 'app'
		? m.secrets_deleted_key_app({ key })
		: m.secrets_deleted_key_stage({ key })
}
