import { describe, expect, it } from 'vitest'
import {
	appContents,
	appRefFrom,
	appStackName,
	appSummaryOf,
	deployStateOf,
	projectInitial,
	sameAppRef,
	serviceTargets,
	stageNetwork,
} from '../..'
import {
	fakeComposeApp,
	fakeServiceApp,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

const deployed = new Date('2026-10-01T12:00:00.000Z')

describe('app', () => {
	it('arma los nombres de Swarm: red del stage y stack de la app', () => {
		const ref = appRefFrom('auditfy', 'staging', 'api')
		expect(appStackName(ref)).toBe('auditfy_staging_api')
		expect(stageNetwork({ project: 'auditfy', slug: 'staging' })).toBe('auditfy_staging')
		expect(projectInitial({ slug: 'aumi-agents' as never })).toBe('a')
		expect(sameAppRef(ref, appRefFrom('auditfy', 'staging', 'api'))).toBe(true)
		expect(sameAppRef(ref, appRefFrom('auditfy', 'production', 'api'))).toBe(false)
	})

	it('el estado del deploy sale de lastDeployedAt y pendingChanges', () => {
		expect(deployStateOf({ lastDeployedAt: null, pendingChanges: false })).toBe(
			'never_deployed',
		)
		expect(deployStateOf({ lastDeployedAt: deployed, pendingChanges: true })).toBe(
			'pending_changes',
		)
		expect(deployStateOf({ lastDeployedAt: deployed, pendingChanges: false })).toBe(
			'up_to_date',
		)
	})

	it('lo que corre: la imagen o los servicios del compose', () => {
		expect(appContents(fakeServiceApp('p', 's', 'api'))).toEqual({
			kind: 'image',
			image: 'nginx:1.27-alpine',
		})
		const compose = fakeComposeApp(
			'p',
			's',
			'agents',
			'services:\n  gateway:\n    image: a\n  runner:\n    image: b\n',
		)
		expect(appContents(compose)).toEqual({ kind: 'services', services: ['gateway', 'runner'] })
	})

	it('los destinos de un dominio: un servicio:puerto por cada puerto que declara', () => {
		const app = {
			...fakeComposeApp('auditfy', 'staging', 'front'),
			services: [
				{ name: 'api', ports: [3000] },
				{ name: 'web', ports: [] },
				{ name: 'worker', ports: [80, 8080] },
			],
		}
		expect(serviceTargets(app)).toEqual(['api:3000', 'worker:80', 'worker:8080'])
		// Sin la Fase 2 el backend no los manda: no hay sugerencias del catálogo.
		expect(serviceTargets({ ...app, services: null })).toEqual([])
	})

	it('el resumen junta todo lo del encabezado', () => {
		const app = fakeServiceApp('auditfy', 'staging', 'api', {
			lastDeployedAt: deployed,
			pendingChanges: true,
		})
		expect(appSummaryOf(app)).toEqual({
			ref: appRefFrom('auditfy', 'staging', 'api'),
			name: 'api',
			kind: 'service',
			contents: { kind: 'image', image: 'nginx:1.27-alpine' },
			deployState: 'pending_changes',
			pendingChanges: true,
			lastDeployedAt: deployed,
			stackName: 'auditfy_staging_api',
			stageNetwork: 'auditfy_staging',
		})
	})
})
