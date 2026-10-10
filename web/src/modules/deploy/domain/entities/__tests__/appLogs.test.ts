import { describe, expect, it } from 'vitest'
import { type AppLogLine, appendCapped, logTasks, matchesFilter, parseAppLogText } from '../appLogs'

describe('parseAppLogText', () => {
	it('lee el nivel de un prefijo y lo saca del mensaje', () => {
		expect(parseAppLogText('WARN consulta lenta · 812 ms')).toEqual({
			level: 'warn',
			text: 'consulta lenta · 812 ms',
		})
		expect(parseAppLogText('[error] redis: ECONNRESET')).toEqual({
			level: 'error',
			text: 'redis: ECONNRESET',
		})
		expect(parseAppLogText('INFO: listo')).toEqual({ level: 'info', text: 'listo' })
	})

	it('lee JSON con level y msg', () => {
		expect(parseAppLogText('{"level":"warning","msg":"token expirado"}')).toEqual({
			level: 'warn',
			text: 'token expirado',
		})
		expect(parseAppLogText('{"severity":"FATAL","message":"sin db"}')).toEqual({
			level: 'error',
			text: 'sin db',
		})
	})

	it('un JSON sin mensaje se muestra entero', () => {
		const raw = '{"level":"info","path":"/health"}'
		expect(parseAppLogText(raw)).toEqual({ level: 'info', text: raw })
	})

	it('logfmt: el nivel sale de level= y el mensaje queda igual', () => {
		const raw = 'time=2026 level=error msg="no conecta"'
		expect(parseAppLogText(raw)).toEqual({ level: 'error', text: raw })
	})

	it('sin pistas es info y el texto no se toca', () => {
		expect(parseAppLogText('GET /health 200 · 1 ms')).toEqual({
			level: 'info',
			text: 'GET /health 200 · 1 ms',
		})
		expect(parseAppLogText('{no es json')).toEqual({ level: 'info', text: '{no es json' })
	})
})

describe('appendCapped', () => {
	it('suma y se queda con las últimas', () => {
		expect(appendCapped([1, 2, 3], [4, 5], 4)).toEqual([2, 3, 4, 5])
		expect(appendCapped([1, 2], [3], 5)).toEqual([1, 2, 3])
	})

	it('sin nuevas igual respeta el tope', () => {
		expect(appendCapped([1, 2, 3], [], 2)).toEqual([2, 3])
	})
})

function line(text: string, task: string | null = 'api.1'): AppLogLine {
	return { time: null, task, stream: 'stdout', text, level: 'info' }
}

describe('filtros', () => {
	it('matchesFilter busca sin mayúsculas y por tarea', () => {
		const sample = line('GET /v1/Audits 200')
		expect(matchesFilter(sample, { query: 'audits', task: '' })).toBe(true)
		expect(matchesFilter(sample, { query: '  ', task: 'api.1' })).toBe(true)
		expect(matchesFilter(sample, { query: '', task: 'api.2' })).toBe(false)
		expect(matchesFilter(sample, { query: 'health', task: '' })).toBe(false)
	})

	it('logTasks ordena las tareas con números naturales', () => {
		const lines = [line('a', 'api.10'), line('b', 'api.2'), line('c', null), line('d', 'api.2')]
		expect(logTasks(lines)).toEqual(['api.2', 'api.10'])
	})
})
