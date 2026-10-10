import { create } from 'qrcode'

export interface QrPath {
	/** Lado del cuadrado en módulos, con el margen incluido. */
	readonly size: number
	/** Un único `<path>` con un cuadrito de 1×1 por módulo oscuro. */
	readonly d: string
}

// La zona quieta que piden los lectores de QR: 4 módulos de borde claro.
const QUIET_ZONE = 4

// La librería solo calcula la matriz; el SVG lo arma el componente, sin `{@html}`.
export function qrPath(text: string): QrPath {
	const { modules } = create(text, { errorCorrectionLevel: 'M' })
	const { size, data } = modules
	let d = ''
	for (let row = 0; row < size; row += 1) {
		for (let column = 0; column < size; column += 1) {
			if (data[row * size + column]) {
				d += `M${column + QUIET_ZONE} ${row + QUIET_ZONE}h1v1h-1z`
			}
		}
	}
	return { size: size + QUIET_ZONE * 2, d }
}
