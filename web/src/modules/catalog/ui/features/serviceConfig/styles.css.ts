import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Las tarjetas de App·Configuración: controles de hasta 560 px de ancho, como el diseño.

export const page = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[28],
})

export const sections = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[20],
	minWidth: 0,
})

export const form = style({ minWidth: 0 })

export const narrow = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
	maxWidth: '560px',
	minWidth: 0,
})

export const grid2 = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(auto-fit, minmax(min(160px, 100%), 1fr))',
	gap: theme.space[16],
	maxWidth: '560px',
})

export const grid4 = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(auto-fit, minmax(min(120px, 100%), 1fr))',
	gap: theme.space[16],
	maxWidth: '560px',
})

export const grid3 = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(auto-fit, minmax(min(150px, 100%), 1fr))',
	gap: theme.space[16],
	maxWidth: '560px',
})

export const fieldError = style({
	margin: 0,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.status.errorText,
})

export const imageField = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
	maxWidth: '560px',
})

// La vista previa del healthcheck (`<pre>` del diseño).
export const preview = style({
	margin: 0,
	maxWidth: '100%',
	boxSizing: 'border-box',
	padding: `${theme.space[12]} ${theme.space[14]}`,
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.md,
	backgroundColor: theme.colors.background.page,
	color: theme.colors.text.secondary,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[12],
	lineHeight: theme.typography.lineHeight.relaxed,
	whiteSpace: 'pre-wrap',
	wordBreak: 'break-all',
})

// Volúmenes: dos columnas (nombre 1, ruta 2) y el botón de quitar.
export const volumeList = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
	maxWidth: '560px',
	margin: 0,
	padding: 0,
	listStyle: 'none',
})

export const volumeHeader = style({
	display: 'flex',
	gap: theme.space[8],
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
	color: theme.colors.text.secondary,
})

export const volumeNameColumn = style({ flex: '1 1 0', minWidth: 0 })
export const volumeMountColumn = style({ flex: '2 1 0', minWidth: 0 })
export const volumeActionColumn = style({ flex: `0 0 ${theme.control.md}` })

export const volumeRow = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[4],
})

export const volumeInputs = style({
	display: 'flex',
	gap: theme.space[8],
	alignItems: 'center',
})

export const empty = style({
	margin: 0,
	color: theme.colors.text.tertiary,
	fontSize: theme.typography.fontSize[13],
})

export const addButton = style({ alignSelf: 'flex-start' })
