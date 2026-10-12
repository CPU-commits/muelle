import { describe, expect, it } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ForbiddenError, UnavailableError, ValidationError } from '@/shared/domain'
import {
	EmptyEnvPatch,
	EnvLimitExceeded,
	EnvOutOfScope,
	EnvOwnerNotFound,
	InvalidEnvKey,
	MAX_ENV_VARS,
} from '../../../../domain'
import { dotenvIssueMessage, secretsErrorMessage } from '../secretsMessages'

describe('secretsErrorMessage', () => {
	it('elige el texto por la clase del error', () => {
		expect(secretsErrorMessage(new InvalidEnvKey('reserved', 'COMPOSE_X', 'set'), 'app')).toBe(
			m.secrets_error_key_reserved(),
		)
		expect(secretsErrorMessage(new EnvLimitExceeded('count', 'set'), 'app')).toBe(
			m.secrets_error_limit_count({ max: MAX_ENV_VARS }),
		)
		expect(secretsErrorMessage(new EnvOwnerNotFound(), 'app')).toBe(m.secrets_not_found_app())
		expect(secretsErrorMessage(new EnvOwnerNotFound(), 'stage')).toBe(
			m.secrets_not_found_stage(),
		)
		expect(secretsErrorMessage(new EnvOutOfScope(), 'stage')).toBe(
			m.secrets_error_out_of_scope(),
		)
		expect(secretsErrorMessage(new ForbiddenError('x'), 'stage')).toBe(
			m.secrets_error_out_of_scope(),
		)
		expect(secretsErrorMessage(new UnavailableError('x'), 'app')).toBe(m.common_unavailable())
	})

	it('un 422 del backend muestra su motivo; un error de programación, el genérico', () => {
		expect(secretsErrorMessage(new ValidationError('máximo 500 claves', 'unset'), 'app')).toBe(
			'máximo 500 claves',
		)
		expect(secretsErrorMessage(new EmptyEnvPatch(), 'app')).toBe(m.common_error_generic())
		expect(secretsErrorMessage(new Error('?'), 'app')).toBe(m.common_error_generic())
	})
})

describe('dotenvIssueMessage', () => {
	it('dice la línea y el problema, con la clave si la hay', () => {
		expect(dotenvIssueMessage({ line: 3, problem: 'reserved_key', key: 'COMPOSE_X' })).toBe(
			m.secrets_import_line({
				line: 3,
				message: m.secrets_dotenv_reserved_key({ key: 'COMPOSE_X' }),
			}),
		)
		expect(dotenvIssueMessage({ line: 1, problem: 'unterminated_quote', key: 'A' })).toBe(
			m.secrets_import_line({ line: 1, message: m.secrets_dotenv_unterminated_quote() }),
		)
	})
})
