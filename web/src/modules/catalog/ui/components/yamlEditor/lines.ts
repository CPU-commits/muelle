/** Cuántas líneas tiene el texto, como las numera el editor. */
export function lineCount(text: string): number {
	let count = 1
	for (let index = 0; index < text.length; index += 1) {
		if (text.charCodeAt(index) === 10) count += 1
	}
	return count
}
