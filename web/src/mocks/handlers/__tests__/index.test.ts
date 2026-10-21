import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { server } from '@/mocks/server'
import { http } from '@/shared/data/http/client'
import { withDomainErrors } from '@/shared/data/http/domainErrors'
import { ApiError } from '@/shared/data/http/errors'
import { UnexpectedError } from '@/shared/domain'

// Como el backend: una ruta sin handler responde 404 `no_route`, y un repositorio la lee como
// un error inesperado (una ruta mal armada del panel).
//
// Las rutas son a propósito de ningún módulo: cuando un módulo de F1 mockea las suyas
// (deploys, status), una ruta real dejaría de ser "sin handler" y el test se rompería.
const NO_HANDLER = '/__sin_handler__'
const NESTED_NO_HANDLER = '/__sin_handler__/proyecto/stage/app/__tampoco__'

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterAll(() => server.close())

describe('handlers de MSW', () => {
	it('una ruta de la API sin handler responde 404 no_route', async () => {
		for (const path of [NO_HANDLER, NESTED_NO_HANDLER]) {
			const error = await http.get(path).catch((caught: unknown) => caught)

			expect(error).toBeInstanceOf(ApiError)
			expect((error as ApiError).status).toBe(404)
			expect((error as ApiError).code).toBe('no_route')
		}
	})

	it('también con otro método (un POST que nadie atiende)', async () => {
		const error = await http.post(NESTED_NO_HANDLER, { body: {} }).catch((caught) => caught)

		expect((error as ApiError).code).toBe('no_route')
	})

	it('un repositorio la lee como un error inesperado, no como un recurso que no existe', async () => {
		await expect(withDomainErrors(() => http.get(NO_HANDLER))).rejects.toBeInstanceOf(
			UnexpectedError,
		)
	})
})
