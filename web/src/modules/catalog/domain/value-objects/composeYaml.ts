import { type ComposeProblem, InvalidCompose } from '../errors/catalogErrors'

/** `entity.MaxComposeBytes`: el backend cuenta bytes, no caracteres. */
export const COMPOSE_MAX_BYTES = 256 * 1024

const encoder = new TextEncoder()

export function composeByteLength(yaml: string): number {
	return encoder.encode(yaml).length
}

// En la Fase 1 el backend solo exige que el compose no esté vacío y pese hasta 256 KiB;
// la Fase 2 lo valida entero con compose-go antes de desplegar.
export function composeProblem(yaml: string): ComposeProblem | null {
	if (yaml.trim().length === 0) return 'empty'
	return composeByteLength(yaml) > COMPOSE_MAX_BYTES ? 'too_large' : null
}

export function parseComposeYaml(yaml: string): string {
	const problem = composeProblem(yaml)
	if (problem) throw new InvalidCompose(problem)
	return yaml
}

// --- Resumen de servicios ---------------------------------------------------------------

export type ComposeHealthcheck = 'defined' | 'disabled' | 'missing'

export interface ComposeServiceSummary {
	readonly name: string
	readonly image: string | null
	/** `deploy.replicas`; `null` si no lo define (Swarm usa 1). */
	readonly replicas: number | null
	readonly healthcheck: ComposeHealthcheck
	/** Los volúmenes nombrados que monta (no los bind mounts). */
	readonly volumes: readonly string[]
}

interface Line {
	readonly indent: number
	readonly text: string
}

// Un `#` que empieza un comentario: al principio o después de un espacio, y fuera de
// comillas.
function withoutComment(text: string): string {
	let quote: string | null = null
	for (let index = 0; index < text.length; index += 1) {
		const char = text.charAt(index)
		if (quote) {
			if (char === quote) quote = null
		} else if (char === '"' || char === "'") {
			quote = char
		} else if (char === '#' && (index === 0 || /\s/.test(text.charAt(index - 1)))) {
			return text.slice(0, index).trimEnd()
		}
	}
	return text
}

function toLines(yaml: string): Line[] {
	return yaml.split(/\r?\n/).flatMap((raw) => {
		const text = withoutComment(raw.trim())
		if (text === '' || text === '---') return []
		return [{ indent: raw.length - raw.trimStart().length, text }]
	})
}

const KEY = /^("[^"]*"|'[^']*'|[^\s:'"][^:]*?):(?:\s+(.*))?$/

function unquote(value: string): string {
	const trimmed = value.trim()
	if (/^".*"$|^'.*'$/.test(trimmed)) return trimmed.slice(1, -1)
	return trimmed
}

function keyOf(line: Line): { key: string; value: string } | null {
	const match = KEY.exec(line.text)
	if (!match) return null
	return { key: unquote(match[1] ?? ''), value: (match[2] ?? '').trim() }
}

/** Las líneas que cuelgan de `lines[start]` (más indentadas que ella). */
function blockAfter(lines: readonly Line[], start: number): Line[] {
	const parent = lines[start]
	if (!parent) return []
	const block: Line[] = []
	for (const line of lines.slice(start + 1)) {
		if (line.indent <= parent.indent) break
		block.push(line)
	}
	return block
}

function volumeName(entry: string): string | null {
	const source = unquote(entry).split(':')[0] ?? ''
	if (source === '' || /^[/.~$]/.test(source)) return null
	return source
}

function namedVolumes(value: string, block: readonly Line[]): string[] {
	const names: string[] = []
	if (value.startsWith('[')) {
		for (const item of value.replace(/^\[|\]$/g, '').split(',')) {
			const name = volumeName(item)
			if (name) names.push(name)
		}
		return names
	}
	for (const line of block) {
		const short = /^-\s+(.+)$/.exec(line.text)
		const entry = short?.[1] ?? ''
		if (short && !KEY.test(entry)) {
			const name = volumeName(entry)
			if (name) names.push(name)
			continue
		}
		// Sintaxis larga: `- type: volume` / `source: datos`.
		const source = /^(?:-\s+)?source:\s*(.+)$/.exec(line.text)
		if (source) {
			const name = volumeName(source[1] ?? '')
			if (name) names.push(name)
		}
	}
	return names
}

function summarizeService(
	name: string,
	value: string,
	body: readonly Line[],
): ComposeServiceSummary {
	let image: string | null = null
	let replicas: number | null = null
	let healthcheck: ComposeHealthcheck = 'missing'
	let volumes: string[] = []

	const propIndent = body[0]?.indent
	body.forEach((line, index) => {
		if (line.indent !== propIndent) return
		const pair = keyOf(line)
		if (!pair) return
		const nested = blockAfter(body, index)
		const nestedText = [pair.value, ...nested.map((item) => item.text)].join('\n')
		switch (pair.key) {
			case 'image':
				image = unquote(pair.value) || null
				break
			case 'deploy': {
				const match = /(?:^|[\s{,])replicas:\s*(\d+)/m.exec(nestedText)
				if (match) replicas = Number(match[1])
				break
			}
			case 'healthcheck':
				healthcheck =
					/disable:\s*true/.test(nestedText) || /test:\s*\[?\s*["']?NONE/.test(nestedText)
						? 'disabled'
						: 'defined'
				break
			case 'volumes':
				volumes = namedVolumes(pair.value, nested)
				break
		}
	})

	// `servicio: { image: x }` en una línea: lo mínimo, la imagen.
	if (body.length === 0 && value.startsWith('{')) {
		image = /image:\s*([^,}]+)/.exec(value)?.[1]?.trim() ?? null
	}
	return { name, image, replicas, healthcheck, volumes }
}

/**
 * Los servicios de un compose, para mostrarlos (Proyectos y Configuración). Es una
 * lectura aproximada del YAML en estilo bloque, el de casi todos los compose: lo que no
 * entiende lo deja afuera. La validación de verdad es la de compose-go en la Fase 2.
 */
export function summarizeCompose(yaml: string): ComposeServiceSummary[] {
	const lines = toLines(yaml)
	const start = lines.findIndex((line) => line.indent === 0 && /^services:\s*$/.test(line.text))
	if (start === -1) return []

	const block = blockAfter(lines, start)
	const serviceIndent = block[0]?.indent
	const services: ComposeServiceSummary[] = []
	block.forEach((line, index) => {
		if (line.indent !== serviceIndent) return
		const pair = keyOf(line)
		if (!pair) return
		services.push(summarizeService(pair.key, pair.value, blockAfter(block, index)))
	})
	return services
}
