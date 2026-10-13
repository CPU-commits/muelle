import { describe, expect, it } from 'vitest'
import {
	AmbiguousSessionError,
	BadOriginError,
	ForbiddenError,
	RateLimitedError,
	UnavailableError,
} from '@/shared/domain'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError } from '../errorMessage'

describe('describeError', () => {
	it('explica qué hacer con un origen o una sesión mal configurados', () => {
		expect(describeError(new BadOriginError())).toBe(m.common_bad_origin())
		expect(m.common_bad_origin()).toContain('MUELLE_PANEL_ORIGIN')
		expect(describeError(new AmbiguousSessionError())).toBe(m.common_ambiguous_session())
	})

	it('distingue una fase pendiente de un backend caído', () => {
		expect(describeError(new UnavailableError('x', { phase: 2 }))).toBe(
			m.common_coming_title({ phase: 2 }),
		)
		expect(describeError(new UnavailableError('caído'))).toBe(m.common_unavailable())
	})

	it('no le pide revisar la conexión a quien recibió un 5xx o un timeout', () => {
		expect(describeError(new UnavailableError('red', { reason: 'network' }))).toBe(
			m.common_unavailable(),
		)
		expect(describeError(new UnavailableError('500', { reason: 'server' }))).toBe(
			m.common_server_error(),
		)
		expect(m.common_server_error()).toContain('muelle respondió con un error')
		expect(describeError(new UnavailableError('lento', { reason: 'timeout' }))).toBe(
			m.common_timeout(),
		)
	})

	it('tiene un texto para permisos y límites, y uno genérico para el resto', () => {
		expect(describeError(new ForbiddenError('no'))).toBe(m.common_forbidden())
		expect(describeError(new RateLimitedError('espera'))).toBe(m.common_rate_limited())
		expect(describeError(new Error('raro'))).toBe(m.common_error_generic())
	})
})
