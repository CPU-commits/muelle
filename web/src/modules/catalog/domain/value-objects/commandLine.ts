// `command` y `healthcheck.command` son listas de argumentos (el backend no las pasa por
// un shell). El formulario las muestra como una sola línea con las comillas de siempre:
// `node dist/x.js --name 'con espacios'` ⇄ ["node", "dist/x.js", "--name", "con espacios"].
// Las comillas solo agrupan: no hay variables, globs ni operadores.

const PLAIN_ARG = /^[^\s'"\\]+$/

/** Una lista de argumentos como línea editable. `parseCommandLine` la devuelve igual. */
export function formatCommandLine(args: readonly string[]): string {
	return args
		.map((arg) => {
			if (PLAIN_ARG.test(arg)) return arg
			if (!arg.includes("'")) return `'${arg}'`
			return `"${arg.replace(/(["\\])/g, '\\$1')}"`
		})
		.join(' ')
}

/**
 * Separa la línea en argumentos. Las comillas simples son literales; en las dobles, `\`
 * escapa `"` y `\`; afuera, `\` escapa el carácter que sigue. `null` si una comilla
 * queda sin cerrar.
 */
export function parseCommandLine(line: string): string[] | null {
	const args: string[] = []
	let current = ''
	let inArg = false
	let quote: "'" | '"' | null = null

	for (let index = 0; index < line.length; index += 1) {
		const char = line.charAt(index)
		if (quote === "'") {
			if (char === "'") quote = null
			else current += char
			continue
		}
		if (quote === '"') {
			const next = line.charAt(index + 1)
			if (char === '\\' && (next === '"' || next === '\\')) {
				current += next
				index += 1
			} else if (char === '"') {
				quote = null
			} else {
				current += char
			}
			continue
		}
		if (/\s/.test(char)) {
			if (inArg) args.push(current)
			current = ''
			inArg = false
			continue
		}
		inArg = true
		if (char === "'" || char === '"') {
			quote = char
		} else if (char === '\\' && index + 1 < line.length) {
			current += line.charAt(index + 1)
			index += 1
		} else {
			current += char
		}
	}

	if (quote) return null
	if (inArg) args.push(current)
	return args
}
