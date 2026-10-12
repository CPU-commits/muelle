import { describe, expect, it } from 'vitest'
import {
	InvalidAppSpec,
	InvalidCompose,
	InvalidDeployTimeout,
	InvalidSlug,
	makeCreateApp,
	newServiceSpec,
	type ProjectSlug,
	type StageSlug,
} from '../..'
import {
	fakeProject,
	fakeServiceApp,
	fakeStage,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

const where = { project: 'auditfy' as ProjectSlug, stage: 'staging' as StageSlug }

function repo() {
	return new InMemoryCatalogRepository({
		projects: [fakeProject('auditfy')],
		stages: [fakeStage('auditfy', 'staging')],
		apps: [fakeServiceApp('auditfy', 'staging', 'redis')],
	})
}

describe('createApp', () => {
	it('crea una app de formulario y una compose', async () => {
		const catalog = repo()
		const create = makeCreateApp({ catalog })

		await expect(
			create({ ...where, slug: 'API', kind: 'service', spec: newServiceSpec('nginx', 80) }),
		).resolves.toMatchObject({ slug: 'api', kind: 'service', deployTimeoutSeconds: 300 })
		await expect(
			create({
				...where,
				slug: 'agents',
				kind: 'compose',
				composeYaml: 'services:\n  a:\n    image: x\n',
				deployTimeoutSeconds: 600,
			}),
		).resolves.toMatchObject({ kind: 'compose', deployTimeoutSeconds: 600 })
	})

	it('valida slug, spec, compose y timeout antes de mandar', async () => {
		const catalog = repo()
		const create = makeCreateApp({ catalog })
		const spec = newServiceSpec('nginx', 80)

		await expect(
			create({ ...where, slug: '1x', kind: 'service', spec }),
		).rejects.toBeInstanceOf(InvalidSlug)
		await expect(
			create({ ...where, slug: 'x', kind: 'service', spec: { ...spec, port: 0 } }),
		).rejects.toBeInstanceOf(InvalidAppSpec)
		await expect(
			create({ ...where, slug: 'x', kind: 'compose', composeYaml: '  ' }),
		).rejects.toBeInstanceOf(InvalidCompose)
		await expect(
			create({ ...where, slug: 'x', kind: 'service', spec, deployTimeoutSeconds: 5 }),
		).rejects.toBeInstanceOf(InvalidDeployTimeout)
		expect(catalog.calls.createApp).toBe(0)
	})

	it('propaga el slug repetido en el stage', async () => {
		await expect(
			makeCreateApp({ catalog: repo() })({
				...where,
				slug: 'redis',
				kind: 'service',
				spec: newServiceSpec('redis', 6379),
			}),
		).rejects.toMatchObject({ name: 'SlugTaken', entity: 'app' })
	})
})
