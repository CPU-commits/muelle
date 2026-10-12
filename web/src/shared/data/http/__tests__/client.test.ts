import { delay, http as mock, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { apiUrl, configureHttp, http } from '../client'
import { ApiError, NetworkError, RequestTimeoutError } from '../errors'

const url = (path: string) => apiUrl(path)

const server = setupServer(
	mock.get(url('/ok'), () => HttpResponse.json({ hola: 'mundo' })),
	mock.post(url('/echo'), async ({ request }) =>
		HttpResponse.json({
			body: await request.json(),
			contentType: request.headers.get('content-type'),
		}),
	),
	mock.delete(url('/vacio'), () => new HttpResponse(null, { status: 204 })),
	mock.get(url('/sin-sesion'), () =>
		HttpResponse.json(
			{ error: { code: 'unauthenticated', message: 'inicia sesión' } },
			{ status: 401 },
		),
	),
	mock.get(url('/sin-2fa'), () =>
		HttpResponse.json(
			{ error: { code: 'totp_setup_required', message: 'configura 2FA' } },
			{ status: 403 },
		),
	),
	mock.get(url('/caido'), () => HttpResponse.error()),
	mock.get(url('/lento'), async () => {
		await delay(2_000)
		return HttpResponse.json({ tarde: true })
	}),
)

const onUnauthorized = vi.fn()
const onTotpSetupRequired = vi.fn()

beforeAll(() => {
	server.listen({ onUnhandledRequest: 'error' })
	configureHttp({ onUnauthorized, onTotpSetupRequired })
})
afterEach(() => {
	server.resetHandlers()
	vi.clearAllMocks()
})
afterAll(() => server.close())

describe('cliente HTTP', () => {
	it('arma la URL con /api/v1 y el query sin vacíos', () => {
		expect(apiUrl('/audit', { before: 10, limit: '', q: undefined })).toBe(
			'http://localhost/api/v1/audit?before=10',
		)
	})

	it('devuelve el JSON de una respuesta ok', async () => {
		await expect(http.get('/ok')).resolves.toEqual({ hola: 'mundo' })
	})

	it('manda el cuerpo como JSON', async () => {
		const response = await http.post<{ body: unknown; contentType: string }>('/echo', {
			body: { code: '123456' },
		})

		expect(response.body).toEqual({ code: '123456' })
		expect(response.contentType).toBe('application/json')
	})

	it('un 204 devuelve null', async () => {
		await expect(http.delete('/vacio')).resolves.toBeNull()
	})

	it('un 401 en una ruta con sesión tira ApiError y avisa para ir al login', async () => {
		const error = await http.get('/sin-sesion').catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(ApiError)
		expect((error as ApiError).code).toBe('unauthenticated')
		expect(onUnauthorized).toHaveBeenCalledOnce()
	})

	it('con redirectOn401: false, el 401 es de la pantalla y no redirige', async () => {
		await expect(http.get('/sin-sesion', { redirectOn401: false })).rejects.toBeInstanceOf(
			ApiError,
		)
		expect(onUnauthorized).not.toHaveBeenCalled()
	})

	it('un 403 totp_setup_required avisa para ir a configurar 2FA', async () => {
		await expect(http.get('/sin-2fa')).rejects.toBeInstanceOf(ApiError)
		expect(onTotpSetupRequired).toHaveBeenCalledOnce()
		expect(onUnauthorized).not.toHaveBeenCalled()
	})

	it('un fallo de red es un NetworkError', async () => {
		await expect(http.get('/caido')).rejects.toBeInstanceOf(NetworkError)
	})

	it('corta una petición que tarda más que el timeout', async () => {
		const error = await http.get('/lento', { timeoutMs: 50 }).catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(RequestTimeoutError)
		expect((error as RequestTimeoutError).timeoutMs).toBe(50)
	})

	it('una cancelación de quien llama sigue siendo un AbortError, no un timeout', async () => {
		const controller = new AbortController()
		const pending = http.get('/lento', { signal: controller.signal })
		controller.abort()

		const error = await pending.catch((caught: unknown) => caught)
		expect((error as DOMException).name).toBe('AbortError')
	})
})
