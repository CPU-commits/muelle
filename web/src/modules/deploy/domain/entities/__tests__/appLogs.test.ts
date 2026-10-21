import { describe, expect, it } from 'vitest'
import {
	type AppLogLine,
	appendCapped,
	lineKey,
	logTasks,
	makeResumeFilter,
	matchesFilter,
	parseAppLogText,
	reconnectDelayMs,
	ResumeTracker,
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

	it('ResumeTracker sigue la hora más nueva y cuenta las líneas de ese milisegundo', () => {
		const tracker = new ResumeTracker()
		tracker.note(line('sin hora'))
		expect(tracker.point()).toBeNull()

		tracker.note(at('2026-10-10T14:31:51.517Z', 'a'))
		tracker.note(at('2026-10-10T14:31:51.517Z', 'a'))
		tracker.note(at('2026-10-10T14:31:51.517Z', 'b', 'api.2'))
		// Una línea de otra tarea que llega tarde con una hora anterior no lo mueve.
		tracker.note(at('2026-10-10T14:31:51.000Z', 'tarde'))
		let point = tracker.point()
		expect(point?.since.toISOString()).toBe('2026-10-10T14:31:51.517Z')
		expect(point?.seen.get(lineKey(at('2026-10-10T14:31:51.517Z', 'a')))).toBe(2)
		expect(point?.seen.size).toBe(2)

		// La hora avanza: lo del milisegundo anterior se descarta.
		tracker.note(at('2026-10-10T14:31:52.000Z', 'c'))
		point = tracker.point()
		expect(point?.since.toISOString()).toBe('2026-10-10T14:31:52.000Z')
		expect([...(point?.seen.keys() ?? [])]).toEqual([
			lineKey(at('2026-10-10T14:31:52.000Z', 'c')),
		])

		tracker.clear()
		expect(tracker.point()).toBeNull()
	})

	it('makeResumeFilter descarta lo de antes de since y las copias ya vistas de ese ms', () => {
		const tracker = new ResumeTracker()
		tracker.note(at('2026-10-10T14:31:51.517Z', 'ya estaba'))
		tracker.note(at('2026-10-10T14:31:51.517Z', 'dos veces'))
		tracker.note(at('2026-10-10T14:31:51.517Z', 'dos veces'))
		const point = tracker.point()
		if (!point) throw new Error('sin punto')
		const isRepeat = makeResumeFilter(point)

		// Lo de antes: un backend que todavía no soporta `since` manda las últimas igual.
		expect(isRepeat(at('2026-10-10T14:31:50.000Z', 'vieja'))).toBe(true)
		expect(isRepeat(at('2026-10-10T14:31:51.517Z', 'ya estaba'))).toBe(true)
		// Una tercera copia de "dos veces" en ese ms es nueva: se tenían dos.
		expect(isRepeat(at('2026-10-10T14:31:51.517Z', 'dos veces'))).toBe(true)
		expect(isRepeat(at('2026-10-10T14:31:51.517Z', 'dos veces'))).toBe(true)
		expect(isRepeat(at('2026-10-10T14:31:51.517Z', 'dos veces'))).toBe(false)
		// Mismo instante pero otra línea (u otra tarea): es nueva.
		expect(isRepeat(at('2026-10-10T14:31:51.517Z', 'otra'))).toBe(false)
		expect(isRepeat(at('2026-10-10T14:31:51.517Z', 'ya estaba', 'api.2'))).toBe(false)
		expect(isRepeat(at('2026-10-10T14:31:52.000Z', 'nueva'))).toBe(false)
		// Sin hora no se puede saber: se muestra.
		expect(isRepeat(line('sin hora'))).toBe(false)
	})

	it('10 000 líneas en el mismo milisegundo no copian nada por línea', () => {
		const tracker = new ResumeTracker()
		const lines = Array.from({ length: 10_000 }, (_, index) =>
			at('2026-10-10T14:31:51.517Z', `línea ${index}`),
		)
		const started = performance.now()
		for (const item of lines) tracker.note(item)
		const point = tracker.point()
		if (!point) throw new Error('sin punto')
		const isRepeat = makeResumeFilter(point)
		const repeated = lines.filter((item) => isRepeat(item)).length
		const elapsed = performance.now() - started

		expect(point.seen.size).toBe(10_000)
		expect(repeated).toBe(10_000)
		expect(isRepeat(at('2026-10-10T14:31:51.517Z', 'otra más'))).toBe(false)
		// Copiar el conjunto en cada línea (lo de antes) eran ~50 millones de inserciones.
		expect(elapsed).toBeLessThan(250)
	})
})
