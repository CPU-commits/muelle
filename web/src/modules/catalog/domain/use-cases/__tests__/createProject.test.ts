import { describe, expect, it } from 'vitest'
import { InvalidProjectName, InvalidSlug, makeCreateProject, SlugTaken } from '../..'
import {
	fakeProject,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

describe('createProject', () => {
	it('normaliza el slug y el nombre antes de mandarlos', async () => {
		const catalog = new InMemoryCatalogRepository()

		const project = await makeCreateProject({ catalog })({
			slug: ' Auditfy',
			name: ' Auditfy ',
		})

		expect(project).toMatchObject({ slug: 'auditfy', name: 'Auditfy' })
		expect(catalog.projects).toHaveLength(1)
	})

	it('no llama al backend con un slug o un nombre inválidos', async () => {
		const catalog = new InMemoryCatalogRepository()
		const create = makeCreateProject({ catalog })

		await expect(create({ slug: 'muelle', name: 'Muelle' })).rejects.toBeInstanceOf(InvalidSlug)
		await expect(create({ slug: 'ok', name: '   ' })).rejects.toBeInstanceOf(InvalidProjectName)
		expect(catalog.calls.createProject).toBe(0)
	})

	it('propaga el slug tomado', async () => {
		const catalog = new InMemoryCatalogRepository({ projects: [fakeProject('auditfy')] })

		await expect(
			makeCreateProject({ catalog })({ slug: 'auditfy', name: 'Otro' }),
		).rejects.toMatchObject({ name: 'SlugTaken', entity: 'project' })
		await expect(
			makeCreateProject({ catalog })({ slug: 'auditfy', name: 'Otro' }),
		).rejects.toBeInstanceOf(SlugTaken)
	})
})
