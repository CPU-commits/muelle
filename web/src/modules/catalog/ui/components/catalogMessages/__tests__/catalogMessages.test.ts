import { describe, expect, it } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ConflictError } from '@/shared/domain'
import { AppDeployStopping, AppHasDomains, StageNetworkConflict } from '../../../../domain'
import { catalogErrorMessage, serverFieldMessage } from '../catalogMessages'

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

	// Al guardar, catalog valida el compose con las reglas de §6.3 del diseño (contrato §2.12).
	it('compose_yaml: vacío y tamaño con su texto; un servicio repetido en el stage, traducido', () => {
		expect(serverFieldMessage('composeYaml', 'es obligatorio')).toBe(
			m.catalog_error_compose_empty(),
		)
		expect(serverFieldMessage('composeYaml', 'no puede superar 256 KiB')).toBe(
			m.catalog_error_compose_too_large(),
		)
		expect(
			serverFieldMessage(
				'composeYaml',
				'el servicio "web" ya existe en la app "front" del stage: los servicios de un stage comparten la red',
			),
		).toBe(m.catalog_error_compose_service_taken({ service: 'web', app: 'front' }))
	})

	it('compose_yaml: el resto de las reglas, con el motivo del backend (no "es obligatorio")', () => {
		const reason =
			'services.web.ports: publicar puertos no está soportado en la v1: los dominios van por Caddy'
		// Empieza con la ruta de la clave: queda tal cual (las claves de YAML distinguen
		// mayúsculas, y `Services.web.ports` no existe).
		expect(serverFieldMessage('composeYaml', reason)).toBe(reason)
		expect(serverFieldMessage('composeYaml', 'volumes.data.driver: no está soportado')).toBe(
			'volumes.data.driver: no está soportado',
		)
		expect(
			serverFieldMessage('composeYaml', 'el compose debe tener al menos un servicio'),
		).toBe('El compose debe tener al menos un servicio')
	})

	it('slug: el de un servicio de una app compose del stage, traducido', () => {
		expect(
			serverFieldMessage(
				'slug',
				'"web" ya es un servicio de la app compose "front" del stage',
			),
		).toBe(m.catalog_error_slug_is_service({ slug: 'web', app: 'front' }))
	})
})

describe('catalogErrorMessage', () => {
	it('borrar una app: con dominios, o con un deploy que no se detiene', () => {
		expect(catalogErrorMessage(new AppHasDomains())).toBe(m.catalog_error_app_has_domains())
		expect(catalogErrorMessage(new AppDeployStopping())).toBe(
			m.catalog_error_app_deploy_stopping(),
		)
	})

	it('crear un stage: una red de Docker con su nombre que no es overlay attachable', () => {
		const error = new StageNetworkConflict('auditfy_qa')
		expect(catalogErrorMessage(error)).toBe(
			m.catalog_error_stage_network_conflict({ network: 'auditfy_qa' }),
		)
		expect(error).toBeInstanceOf(ConflictError)
		// Nombra la red (la que hay que borrar o renombrar en el servidor) y por qué no sirve:
		// que la creara muelle o no da igual, una overlay attachable se usa (contrato §3.4).
		for (const locale of ['es', 'en'] as const) {
			const text = m.catalog_error_stage_network_conflict(
				{ network: 'auditfy_qa' },
				{ locale },
			)
			expect(text).toContain('auditfy_qa')
			expect(text).toMatch(/overlay attachable|attachable overlay/)
		}
	})
})
