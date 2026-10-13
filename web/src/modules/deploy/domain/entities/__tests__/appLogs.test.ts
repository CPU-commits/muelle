import { describe, expect, it } from 'vitest'
import {
	advanceResumePoint,
	type AppLogLine,
	appendCapped,
	isRepeatedAfterResume,
	lineKey,
	logTasks,
	matchesFilter,
	parseAppLogText,
	reconnectDelayMs,
	stripAnsi,
} from '../appLogs'

const ESC = String.fromCharCode(27)
const BEL = String.fromCharCode(7)

describe('stripAnsi', () => {
	it('saca colores, movimientos de cursor y links OSC 8', () => {
		expect(stripAnsi(`${ESC}[31mERROR${ESC}[0m boom`)).toBe('ERROR boom')
		expect(stripAnsi(`${ESC}[1;32mok${ESC}[39;49m · ${ESC}[2K${ESC}[1Glisto`)).toBe(
			'ok · listo',
		)
		expect(stripAnsi(`ver ${ESC}]8;;https://x.cl${BEL}x.cl${ESC}]8;;${BEL} ahora`)).toBe(
			'ver x.cl ahora',
		)
	})

	it('deja igual un texto sin escapes', () => {
		expect(stripAnsi('GET /health 200 · 1 ms')).toBe('GET /health 200 · 1 ms')
	})
})

describe('parseAppLogText', () => {
	// El mensaje no se recorta nunca: el nivel solo da color a la línea.
	it.each([
		['WARN consulta lenta · 812 ms', 'warn'],
		['ERROR redis: ECONNRESET', 'error'],
		['INFO: listo', 'info'],
		['WARNING: disco al 91 %', 'warn'],
		['FATAL sin base de datos', 'error'],
		['2026/10/10 14:00:00 ERROR boom', 'error'],
		['14:00:00.123 [api] WARN lento', 'warn'],
		['[warn] token por vencer', 'warn'],
		['[ERROR] redis: ECONNRESET', 'error'],
		['[Info] arrancando', 'info'],
		['time=2026 level=error msg="no conecta"', 'error'],
		['level=WARN msg=lento', 'warn'],
		['{"level":"warning","msg":"token expirado"}', 'warn'],
		['{"severity":"FATAL","message":"sin db"}', 'error'],
		['{"lvl":"debug","msg":"x"}', 'info'],
	] as const)('%j → %s, sin tocar el texto', (raw, level) => {
		expect(parseAppLogText(raw)).toEqual({ level, text: raw })
	})

	// Palabras de verdad que no son un nivel: no cambian el color.
	it.each([
		'error: relation "findings_v2" does not exist',
		'Error al conectar con redis',
		'warning de deprecación en la línea 4',
		'GET /v1/ERROR-codes 200 · 3 ms',
		'resultado: ERRORES=0',
		'{"msg":"sin nivel"}',
		'{no es json',
	])('%j es info', (raw) => {
		expect(parseAppLogText(raw)).toEqual({ level: 'info', text: raw })
	})

	it('los escapes ANSI se sacan antes de leer el nivel', () => {
		expect(parseAppLogText(`${ESC}[31mERROR${ESC}[0m boom`)).toEqual({
			level: 'error',
			text: 'ERROR boom',
		})
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

describe('reconectar', () => {
	it('reconnectDelayMs espera 1, 2, 4… hasta 30 s', () => {
		const delays = Array.from({ length: 8 }, (_, attempt) => reconnectDelayMs(attempt))
		expect(delays).toEqual([1000, 2000, 4000, 8000, 16_000, 30_000, 30_000, 30_000])
		expect(reconnectDelayMs(1000)).toBe(30_000)
		expect(reconnectDelayMs(-1)).toBe(1000)
	})

	const at = (iso: string, text: string, task = 'api.1'): AppLogLine => ({
		time: new Date(iso),
		task,
		stream: 'stdout',
		text,
		level: 'info',
	})

	it('advanceResumePoint sigue la hora más nueva y junta las líneas de ese instante', () => {
		let point = advanceResumePoint(null, line('sin hora'))
		expect(point).toBeNull()

		point = advanceResumePoint(point, at('2026-10-10T14:31:51.517Z', 'a'))
		point = advanceResumePoint(point, at('2026-10-10T14:31:51.517Z', 'b', 'api.2'))
		// Una línea de otra tarea que llega tarde con una hora anterior no lo mueve.
		point = advanceResumePoint(point, at('2026-10-10T14:31:51.000Z', 'tarde'))
		expect(point?.since.toISOString()).toBe('2026-10-10T14:31:51.517Z')
		expect(point?.seen.size).toBe(2)

		point = advanceResumePoint(point, at('2026-10-10T14:31:52.000Z', 'c'))
		expect(point?.since.toISOString()).toBe('2026-10-10T14:31:52.000Z')
		expect([...(point?.seen ?? [])]).toEqual([lineKey(at('2026-10-10T14:31:52.000Z', 'c'))])
	})

	it('isRepeatedAfterResume descarta lo de antes de since y lo ya visto en ese instante', () => {
		const point = advanceResumePoint(null, at('2026-10-10T14:31:51.517Z', 'ya estaba'))
		if (!point) throw new Error('sin punto')

		expect(isRepeatedAfterResume(at('2026-10-10T14:31:50.000Z', 'vieja'), point)).toBe(true)
		expect(isRepeatedAfterResume(at('2026-10-10T14:31:51.517Z', 'ya estaba'), point)).toBe(true)
		// Mismo instante pero otra línea (u otra tarea): es nueva.
		expect(isRepeatedAfterResume(at('2026-10-10T14:31:51.517Z', 'otra'), point)).toBe(false)
		expect(
			isRepeatedAfterResume(at('2026-10-10T14:31:51.517Z', 'ya estaba', 'api.2'), point),
		).toBe(false)
		expect(isRepeatedAfterResume(at('2026-10-10T14:31:52.000Z', 'nueva'), point)).toBe(false)
		// Sin hora no se puede saber: se muestra.
		expect(isRepeatedAfterResume(line('sin hora'), point)).toBe(false)
	})
})
