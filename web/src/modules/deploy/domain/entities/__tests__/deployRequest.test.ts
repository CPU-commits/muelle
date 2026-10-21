import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import {
	buildDeployRequest,
	deployableServices,
	inferAppKind,
	overridableServices,
} from '../deployRequest'

describe('buildDeployRequest (spec §7, cuerpo de POST …/deploys)', () => {
	it('sin overrides es un redeploy', () => {
		expect(
			buildDeployRequest({ app: 'api', services: ['api'], overrides: { api: '  ' } }),
		).toEqual({ kind: 'redeploy' })
	})

	it('una app de formulario manda image', () => {
		expect(
			buildDeployRequest({
				app: 'api',
				services: ['api'],
				overrides: { api: ' ghcr.io/x/api:sha-1 ' },
			}),
		).toEqual({ kind: 'image', image: 'ghcr.io/x/api:sha-1' })
	})

	it('una app que nunca se desplegó se trata como de formulario', () => {
		expect(buildDeployRequest({ app: 'api', services: [], overrides: { api: 'x:1' } })).toEqual(
			{
				kind: 'image',
				image: 'x:1',
			},
		)
	})

	it('una compose manda images solo con los servicios que cambian', () => {
		expect(
			buildDeployRequest({
				app: 'agents',
				services: ['gateway', 'runner'],
				overrides: { gateway: 'ghcr.io/x/gateway:v2', runner: '' },
			}),
		).toEqual({ kind: 'images', images: { gateway: 'ghcr.io/x/gateway:v2' } })
	})

	it('el kind del catálogo gana sobre la deducción', () => {
		expect(
			buildDeployRequest({
				app: 'web',
				kind: 'compose',
				services: ['web'],
				overrides: { web: 'nginx:1.27' },
			}),
		).toEqual({ kind: 'images', images: { web: 'nginx:1.27' } })
	})

	it('una imagen inválida es un ValidationError con el servicio como campo', () => {
		try {
			buildDeployRequest({ app: 'api', services: ['api'], overrides: { api: 'no vale' } })
			expect.unreachable()
		} catch (error) {
			expect(error).toBeInstanceOf(ValidationError)
			expect((error as ValidationError).field).toBe('api')
		}
	})
})

describe('inferAppKind y overridableServices', () => {
	it('un solo servicio con el nombre de la app es de formulario', () => {
		expect(inferAppKind('api', ['api'])).toBe('service')
		expect(inferAppKind('agents', ['agents', 'runner'])).toBe('compose')
		expect(inferAppKind('web', ['nginx'])).toBe('compose')
	})

	it('el diálogo ofrece un campo por servicio, o uno solo en una de formulario', () => {
		expect(overridableServices('api', [])).toEqual(['api'])
		expect(overridableServices('agents', ['gateway', 'runner'])).toEqual(['gateway', 'runner'])
		expect(overridableServices('api', ['api'], 'service')).toEqual(['api'])
	})
})

describe('deployableServices', () => {
	it('los de la configuración (catálogo, §2.12): contra esos valida el backend `images`', () => {
		// Una compose que todavía no se desplegó: Swarm no tiene nada.
		expect(deployableServices(['web', 'worker'], [])).toEqual(['web', 'worker'])
		// Un servicio que se sacó del compose y Swarm todavía corre: no se ofrece.
		expect(deployableServices(['web'], ['web', 'old'])).toEqual(['web'])
	})

	it('sin la configuración (el catálogo que no cargó), los de Swarm', () => {
		expect(deployableServices(undefined, ['api'])).toEqual(['api'])
		// `[]`: el compose guardado no se pudo leer (§2.12).
		expect(deployableServices([], ['api'])).toEqual(['api'])
	})
})
