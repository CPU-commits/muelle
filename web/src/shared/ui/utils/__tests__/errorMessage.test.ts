import { describe, expect, it } from 'vitest'
import {
	AmbiguousSessionError,
	BadOriginError,
	ForbiddenError,
	PanelMovedError,
	RateLimitedError,
	UnavailableError,
} from '@/shared/domain'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError } from '../errorMessage'

describe('describeError', () => {
	it('explica qué hacer con un origen o una sesión mal configurados', () => {
		expect(describeError(new BadOriginError())).toBe(m.common_bad_origin())
		expect(describeError(new AmbiguousSessionError())).toBe(m.common_ambiguous_session())
	})

	it('con bad_origin dice por qué dirección entrar; MUELLE_PANEL_ORIGIN, solo detrás de otro proxy', () => {
		// El origen permitido sale de Ajustes (contrato §0.1): install ya no escribe
		// MUELLE_PANEL_ORIGIN, y pedir que se revise nombraría una variable que no está.
		for (const locale of ['es', 'en'] as const) {
			const text = m.common_bad_origin({}, { locale })
			expect(text).not.toMatch(/(revisa|check) MUELLE_PANEL_ORIGIN/)
			expect(text).toContain('https://')
			expect(text).toMatch(
				/otro proxy, esta dirección va en MUELLE_PANEL_ORIGIN|another proxy, this address goes in MUELLE_PANEL_ORIGIN/,
			)
		}
	})

	it('si el panel se mudó, dice a dónde seguir', () => {
		const error = new PanelMovedError('https://panel.tudominio.cl:8443')
		expect(error).toBeInstanceOf(BadOriginError)
		expect(describeError(error)).toBe(
			m.common_panel_moved({ origin: 'https://panel.tudominio.cl:8443' }),
		)
		expect(describeError(error)).toContain('https://panel.tudominio.cl:8443')
	})

	it('si el panel se mudó, también dice qué hacer si la nueva no responde: recargar esta', () => {
		// Solo Ajustes ve que muelle volvió a los puertos anteriores y suelta el freno: desde
		// otra pantalla, cada escritura seguiría diciendo "sigue allá" aunque allá no responda.
		// Recargar suelta el freno (es de la página), y esta dirección vuelve a aceptar cambios.
		for (const locale of ['es', 'en'] as const) {
			const text = m.common_panel_moved({ origin: 'https://x:8443' }, { locale })
			expect(text).toMatch(/recarga esta página|reload this page/)
			expect(text).toMatch(/puertos anteriores|previous ports/)
		}
	})

	it('un backend caído o sin red pide revisar la conexión', () => {
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
