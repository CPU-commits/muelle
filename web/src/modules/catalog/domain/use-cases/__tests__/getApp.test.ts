import { describe, expect, it } from 'vitest'
import { AppNotFound, appRefFrom, makeGetApp } from '../..'
import {
	fakeServiceApp,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

describe('getApp', () => {
	it('devuelve la app o AppNotFound', async () => {
		const api = fakeServiceApp('auditfy', 'staging', 'api')
		const getApp = makeGetApp({ catalog: new InMemoryCatalogRepository({ apps: [api] }) })

		await expect(getApp(appRefFrom('auditfy', 'staging', 'api'))).resolves.toBe(api)
		await expect(getApp(appRefFrom('auditfy', 'staging', 'otra'))).rejects.toBeInstanceOf(
			AppNotFound,
		)
	})
})
