import { describe, expect, it } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { serverFieldMessage } from '../catalogMessages'

// Los motivos son los textos exactos del backend (src/internal/catalog/domain/vo/appspec.go
// y src/shared/domain/validate/validate.go): el 422 llega como "<campo>: <motivo>".
describe('serverFieldMessage', () => {
	it('healthcheck.path: el texto del formato solo si el motivo es el formato', () => {
		expect(
			serverFieldMessage(
				'healthcheck.path',
				'debe empezar con / y usar solo letras, números y ._~/?=%+-',
			),
		).toBe(m.catalog_spec_health_path())
		expect(serverFieldMessage('healthcheck.path', 'solo aplica a type http o bun-http')).toBe(
			m.catalog_error_healthcheck_shape(),
		)
	})

	it('healthcheck.command: "es obligatorio" es del comando; "solo aplica" es del tipo', () => {
		expect(
			serverFieldMessage(
				'healthcheck.command',
				'es obligatorio con type command (máximo 64 argumentos)',
			),
		).toBe(m.catalog_spec_health_command())
		expect(serverFieldMessage('healthcheck.command', 'solo aplica a type command')).toBe(
			m.catalog_error_healthcheck_shape(),
		)
	})

	it('traduce los otros campos por su motivo, y deja pasar lo que no conoce', () => {
		expect(serverFieldMessage('slug', '"docker" está reservado')).toBe(
			m.catalog_error_slug_reserved({ slug: 'docker' }),
		)
		expect(
			serverFieldMessage('healthcheck.type', 'debe ser http, bun-http, command o none'),
		).toBe(m.catalog_error_healthcheck_shape())
		expect(
			serverFieldMessage('spec', 'JSON inválido: el campo "port" tiene un valor incorrecto'),
		).toBe(m.catalog_error_spec_shape())
		expect(serverFieldMessage('otro', 'un motivo nuevo')).toBe('un motivo nuevo')
	})
})
