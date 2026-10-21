import { describe, expect, it } from 'vitest'
import { InvalidSlug, makeCreateStage, ProjectNotFound, type ProjectSlug } from '../..'
import {
	fakeProject,
	fakeStage,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

const auditfy = 'auditfy' as ProjectSlug

describe('createStage', () => {
	it('crea el stage con el slug normalizado', async () => {
		const catalog = new InMemoryCatalogRepository({ projects: [fakeProject('auditfy')] })

		await expect(
			makeCreateStage({ catalog })({ project: auditfy, slug: 'Staging' }),
		).resolves.toMatchObject({ project: 'auditfy', slug: 'staging' })
	})

	it('un slug de más de 12 no llega al backend', async () => {
		const catalog = new InMemoryCatalogRepository({ projects: [fakeProject('auditfy')] })

		await expect(
			makeCreateStage({ catalog })({ project: auditfy, slug: 'a-very-long-stage' }),
		).rejects.toMatchObject({ name: 'InvalidSlug', max: 12 })
		await expect(
			makeCreateStage({ catalog })({ project: auditfy, slug: '' }),
		).rejects.toBeInstanceOf(InvalidSlug)
		expect(catalog.calls.createStage).toBe(0)
	})

	it('propaga el slug repetido y el proyecto que no existe', async () => {
		const catalog = new InMemoryCatalogRepository({
			projects: [fakeProject('auditfy')],
			stages: [fakeStage('auditfy', 'staging')],
		})
		const create = makeCreateStage({ catalog })

		await expect(create({ project: auditfy, slug: 'staging' })).rejects.toMatchObject({
			name: 'SlugTaken',
			entity: 'stage',
		})
		await expect(
			create({ project: 'otro' as ProjectSlug, slug: 'staging' }),
		).rejects.toBeInstanceOf(ProjectNotFound)
	})
})
