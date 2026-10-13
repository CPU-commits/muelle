import { describe, expect, it } from 'vitest'
import { DEFAULT_HEALTHCHECK_TIMINGS, type ServiceApp, SPEC_MESSAGES_ES } from '../../../../domain'
import { fakeServiceApp } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import {
	checkSection,
	healthSection,
	imageSection,
	resourcesSection,
	runSection,
	type SectionMessages,
	sectionChanges,
	valueAt,
	visibleFieldOf,
	volumesSection,
} from '../sections'

const messages: SectionMessages = {
	...SPEC_MESSAGES_ES,
	unclosedQuote: () => 'comilla sin cerrar',
	deployTimeout: () => 'timeout fuera de rango',
}

const app: ServiceApp = fakeServiceApp('auditfy', 'staging', 'api', {
	spec: {
		image: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
		port: 3000,
		replicas: 2,
		command: ['node', 'dist/main.js'],
		healthcheck: { type: 'http', path: '/health', ...DEFAULT_HEALTHCHECK_TIMINGS },
		volumes: [{ name: 'uploads', mount: '/app/uploads' }],
		resources: { memoryMb: 512, cpus: null },
	},
})

describe('las tarjetas de Configuración', () => {
	it('los valores del formulario salen del spec guardado, como texto', () => {
		expect(imageSection.values(app)).toEqual({
			image: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
		})
		expect(runSection.values(app)).toEqual({
			port: '3000',
			replicas: '2',
			command: 'node dist/main.js',
		})
		expect(healthSection.values(app)).toMatchObject({
			type: 'http',
			path: '/health',
			intervalSeconds: '10',
		})
		expect(resourcesSection.values(app)).toEqual({
			memoryMb: '512',
			cpus: '',
			deployTimeoutSeconds: '300',
		})
	})

	it('sin cambios no hay nada que mandar', () => {
		const unchanged = [
			sectionChanges(imageSection, imageSection.values(app), app, messages),
			sectionChanges(runSection, runSection.values(app), app, messages),
			sectionChanges(healthSection, healthSection.values(app), app, messages),
			sectionChanges(volumesSection, volumesSection.values(app), app, messages),
			sectionChanges(resourcesSection, resourcesSection.values(app), app, messages),
		]
		expect(unchanged).toEqual([null, null, null, null, null])
	})

	it('guardar una tarjeta manda el spec completo con su parte cambiada', () => {
		const changes = sectionChanges(
			runSection,
			{ port: '8080', replicas: '3', command: "sh -c 'node x.js'" },
			app,
			messages,
		)
		expect(changes).toEqual({
			spec: { ...app.spec, port: 8080, replicas: 3, command: ['sh', '-c', 'node x.js'] },
		})
	})

	it('el timeout va aparte y no manda el spec si los límites no cambiaron', () => {
		expect(
			sectionChanges(
				resourcesSection,
				{ memoryMb: '512', cpus: '', deployTimeoutSeconds: '600' },
				app,
				messages,
			),
		).toEqual({ deployTimeoutSeconds: 600 })
		expect(
			sectionChanges(
				resourcesSection,
				{ memoryMb: '', cpus: '', deployTimeoutSeconds: '300' },
				app,
				messages,
			),
		).toEqual({ spec: { ...app.spec, resources: null } })
	})

	it('pasar a "Ninguno" deja el healthcheck sin path ni tiempos', () => {
		const values = { ...healthSection.values(app), type: 'none' as const }
		expect(sectionChanges(healthSection, values, app, messages)).toEqual({
			spec: { ...app.spec, healthcheck: { type: 'none' } },
		})
	})

	it('los errores quedan en los campos de la tarjeta, con nombres del formulario', () => {
		const health = checkSection(
			healthSection,
			{ ...healthSection.values(app), path: 'sin-barra', intervalSeconds: '' },
			app,
			messages,
		)
		expect(health.fields).toEqual({
			path: SPEC_MESSAGES_ES.healthcheckPath(),
			intervalSeconds: SPEC_MESSAGES_ES.interval(),
		})
		expect(health.elsewhere).toEqual([])

		const volumes = checkSection(
			volumesSection,
			{
				volumes: [
					{ name: 'uploads', mount: '/app/uploads/' },
					{ name: 'uploads', mount: '/data' },
				],
			},
			app,
			messages,
		)
		expect(volumes.fields).toEqual({
			'volumes[0].mount': SPEC_MESSAGES_ES.volumeMount(),
			'volumes[1].name': SPEC_MESSAGES_ES.volumeNameRepeated('uploads'),
		})
	})

	it('una comilla sin cerrar y un timeout fuera de rango son errores de su campo', () => {
		expect(
			checkSection(
				runSection,
				{ port: '3000', replicas: '2', command: "sh -c 'x" },
				app,
				messages,
			).fields,
		).toEqual({ command: 'comilla sin cerrar' })
		expect(
			checkSection(
				healthSection,
				{ ...healthSection.values(app), type: 'command', command: '' },
				app,
				messages,
			).fields,
		).toEqual({ command: SPEC_MESSAGES_ES.healthcheckCommand() })
		expect(
			checkSection(
				resourcesSection,
				{ memoryMb: '8', cpus: '0.5', deployTimeoutSeconds: '10' },
				app,
				messages,
			).fields,
		).toEqual({
			memoryMb: SPEC_MESSAGES_ES.memory(),
			deployTimeoutSeconds: 'timeout fuera de rango',
		})
	})

	it('un problema del spec guardado en otra tarjeta no se pierde', () => {
		const broken: ServiceApp = { ...app, spec: { ...app.spec, port: 0 } }

		const check = checkSection(imageSection, imageSection.values(broken), broken, messages)

		expect(check.fields).toEqual({})
		expect(check.elsewhere).toEqual([{ field: 'port', message: SPEC_MESSAGES_ES.port() }])
	})

	it('cada tarjeta reconoce sus campos del backend', () => {
		expect(healthSection.fieldOf('healthcheck.startPeriodSeconds')).toBe('startPeriodSeconds')
		expect(healthSection.fieldOf('port')).toBeNull()
		expect(volumesSection.fieldOf('volumes[2].name')).toBe('volumes[2].name')
		expect(resourcesSection.fieldOf('deployTimeoutSeconds')).toBe('deployTimeoutSeconds')
		expect(runSection.fieldOf('spec')).toBeNull()
	})

	it('un campo que no está a la vista no recibe el error: va arriba de la tarjeta', () => {
		const http = healthSection.values(app)
		expect(visibleFieldOf(healthSection, 'healthcheck.path', http)).toBe('path')
		expect(visibleFieldOf(healthSection, 'healthcheck.command', http)).toBeNull()
		expect(visibleFieldOf(healthSection, 'healthcheck.type', http)).toBe('type')
		const none = { ...http, type: 'none' as const }
		expect(visibleFieldOf(healthSection, 'healthcheck.intervalSeconds', none)).toBeNull()
		const volumes = volumesSection.values(app)
		expect(visibleFieldOf(volumesSection, 'volumes[0].mount', volumes)).toBe('volumes[0].mount')
		expect(visibleFieldOf(volumesSection, 'volumes[3].mount', volumes)).toBeNull()
		expect(visibleFieldOf(volumesSection, 'volumes', volumes)).toBe('volumes')
		expect(visibleFieldOf(imageSection, 'port', imageSection.values(app))).toBeNull()
	})

	it('valueAt lee un campo por su nombre', () => {
		expect(valueAt({ volumes: [{ name: 'a' }, { name: 'b' }] }, 'volumes[1].name')).toBe('b')
		expect(valueAt({ image: 'x' }, 'image')).toBe('x')
		expect(valueAt({}, 'volumes[0].name')).toBeUndefined()
	})
})
