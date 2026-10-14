import { describe, expect, it } from 'vitest'
import { AppNotFound, appRefFrom, makeDeleteApp } from '../..'
import {
	fakeServiceApp,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

describe('deleteApp', () => {
	it('borra la app; otra vez es AppNotFound', async () => {
		const catalog = new InMemoryCatalogRepository({
			apps: [fakeServiceApp('auditfy', 'staging', 'api')],
		})
		const deleteApp = makeDeleteApp({ catalog })
		const ref = appRefFrom('auditfy', 'staging', 'api')

		await deleteApp(ref)

		expect(catalog.apps).toEqual([])
		await expect(deleteApp(ref)).rejects.toBeInstanceOf(AppNotFound)
	})
})
