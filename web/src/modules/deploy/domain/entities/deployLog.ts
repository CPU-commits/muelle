import type { Brand } from '@/shared/domain'
import type { DeployStatus } from '../value-objects/deployStatus'
import { stripAnsi } from './appLogs'
import type { DeployId } from '../value-objects/deployId'

// El log de un deploy (spec §7): texto, una línea por evento, cada una con su timestamp.
// Los valores de variables `secret` ya llegan reemplazados por `***`.

/**
 * Hasta dónde se leyó el log. Es un número opaco del backend (bytes del log guardado): el
 * panel nunca lo calcula, solo devuelve el `next_offset` (o el `id` de SSE) que recibió.
 */
export type LogOffset = Brand<number, 'LogOffset'>

export const LOG_START = 0 as LogOffset

export function asLogOffset(raw: number): LogOffset {
	return raw as LogOffset
}

/** Una página de `GET …/deploys/{id}/log?offset=N`. */
export interface LogPage {
	/** Siempre líneas completas, hasta 64 KiB. */
	readonly chunk: string
	readonly nextOffset: LogOffset
	readonly status: DeployStatus
	/** En un `superseded`: el deploy que lo reemplazó. */
	readonly supersededBy: DeployId | null
}

/** Lo que llega mientras se sigue un log, por SSE o por polling. */
export type DeployLogEvent =
	| { readonly kind: 'chunk'; readonly text: string; readonly nextOffset: LogOffset }
	| { readonly kind: 'status'; readonly status: DeployStatus }
	/** SSE falló o se cortó: lo que sigue llega por polling desde el último offset. */
	| { readonly kind: 'polling' }
	/**
	 * El backend no reconoció el offset (`422 offset`): el log se vuelve a leer desde el
	 * principio, y lo que se tenía se descarta.
	 */
	| { readonly kind: 'reset' }

/** Las líneas que se guardan en memoria, en el log de un deploy y en los logs en vivo. */
export const LOG_LINE_CAP = 5000

export type LogTone = 'normal' | 'command' | 'ok' | 'warn' | 'error'

export interface LogLine {
	/** Número de línea desde el principio del log (la key de la lista). */
	readonly n: number
	readonly time: Date | null
	readonly text: string
	readonly tone: LogTone
	/** La línea tal cual llegó (para copiar). */
	readonly raw: string
}

export interface LogBuffer {
	readonly lines: readonly LogLine[]
	/** El final de un chunk que todavía no terminó en `\n`. */
	readonly partial: string
	/** Cuántas líneas llegaron en total (aunque las primeras se hayan descartado). */
	readonly count: number
}

export const EMPTY_LOG: LogBuffer = { lines: [], partial: '', count: 0 }

const TIMESTAMP = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))\s+(.*)$/s

const TONES: readonly [RegExp, LogTone][] = [
	[/^\$\s/, 'command'],
	[/\b(deploy fallido|non-zero exit|fatal)\b|^error\b/i, 'error'],
	[/\b(unhealthy|rollback_started|rollback_completed|paused|timeout esperando)\b/i, 'warn'],
	[/^advertencia\b/i, 'warn'],
	[/\b(deploy exitoso|UpdateStatus: completed)\b/i, 'ok'],
]

/** El color de una línea según lo que dice (comandos, fallas, avisos, éxito). */
export function logLineTone(text: string): LogTone {
	for (const [pattern, tone] of TONES) if (pattern.test(text)) return tone
	return 'normal'
}

export function parseLogLine(input: string, n: number): LogLine {
	// El stderr de `docker stack deploy` puede traer colores de terminal.
	const raw = stripAnsi(input)
	const match = TIMESTAMP.exec(raw)
	const time = match?.[1] ? new Date(match[1]) : null
	const valid = time !== null && !Number.isNaN(time.getTime())
	const text = valid && match ? (match[2] ?? '') : raw
	return { n, time: valid ? time : null, text, tone: logLineTone(text), raw }
}

/**
 * Suma un pedazo del log. Los chunks del polling pueden cortar una línea por la mitad:
 * lo que queda después del último `\n` espera al siguiente. Se guardan las últimas
 * `cap` líneas.
 */
export function appendLogChunk(buffer: LogBuffer, chunk: string, cap = LOG_LINE_CAP): LogBuffer {
	if (chunk.length === 0) return buffer
	const pieces = (buffer.partial + chunk).split('\n')
	const partial = pieces.pop() ?? ''
	const parsed = pieces.map((raw, index) =>
		parseLogLine(raw.replace(/\r$/, ''), buffer.count + index + 1),
	)
	const lines = parsed.length === 0 ? buffer.lines : [...buffer.lines, ...parsed]
	return {
		lines: lines.length > cap ? lines.slice(lines.length - cap) : lines,
		partial,
		count: buffer.count + parsed.length,
	}
}

/** Cierra el log: una última línea sin `\n` también cuenta. */
export function flushLog(buffer: LogBuffer, cap = LOG_LINE_CAP): LogBuffer {
	return buffer.partial ? appendLogChunk(buffer, '\n', cap) : buffer
}

/** El texto entero, para copiar. */
export function logText(buffer: LogBuffer): string {
	const lines = buffer.lines.map((line) => line.raw)
	if (buffer.partial) lines.push(buffer.partial)
	return lines.join('\n')
}

export interface LogSegment {
	readonly text: string
	/** `***`: un valor secreto que el backend enmascaró. */
	readonly masked: boolean
}

export const MASK = '***'

/** Separa los `***` para que la vista los marque como secreto oculto. */
export function maskedSegments(text: string): LogSegment[] {
	if (!text.includes(MASK)) return [{ text, masked: false }]
	const segments: LogSegment[] = []
	const parts = text.split(MASK)
	parts.forEach((part, index) => {
		if (part) segments.push({ text: part, masked: false })
		if (index < parts.length - 1) segments.push({ text: MASK, masked: true })
	})
	return segments
}
