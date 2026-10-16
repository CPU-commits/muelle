import { yamlLanguage } from '@codemirror/lang-yaml'

/**
 * Las líneas (desde 1) donde el parser de YAML de CodeMirror encontró un error de
 * sintaxis: indentación rota, un `[` sin cerrar, una clave fuera de lugar. Es la misma
 * gramática que resalta el editor; las reglas de compose (puertos, build, redes) las valida
 * el backend al guardar (contrato §2.12).
 */
export function yamlSyntaxErrorLines(text: string): number[] {
	const tree = yamlLanguage.parser.parse(text)
	const offsets: number[] = []
	tree.iterate({
		enter(node) {
			if (node.type.isError) offsets.push(node.from)
		},
	})
	if (offsets.length === 0) return []

	const lineStarts = [0]
	for (let index = 0; index < text.length; index += 1) {
		if (text.charCodeAt(index) === 10) lineStarts.push(index + 1)
	}
	const lineOf = (offset: number) => {
		let line = 0
		while (line + 1 < lineStarts.length && (lineStarts[line + 1] ?? Infinity) <= offset) {
			line += 1
		}
		return line + 1
	}
	return [...new Set(offsets.map(lineOf))].sort((a, b) => a - b)
}
