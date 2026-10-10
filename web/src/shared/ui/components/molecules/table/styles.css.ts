import { globalStyle, style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const wrapper = style({
	width: '100%',
	overflowX: 'auto',
})

// Con borde propio (tabla suelta); dentro de una <Card flush> va sin él.
export const framed = style({
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
})

export const table = style({
	width: '100%',
	borderCollapse: 'collapse',
	fontSize: theme.typography.fontSize[13],
})

// La pantalla escribe `<thead><tr><th>` planos adentro de <Table>: el estilo se engancha
// por descendencia en vez de envolver cada celda en un componente.
globalStyle(`${table} th`, {
	textAlign: 'left',
	fontWeight: theme.typography.fontWeight.regular,
	color: theme.colors.text.tertiary,
	padding: `${theme.space[10]} ${theme.space[12]}`,
	borderBottom: `1px solid ${theme.colors.borders.subtle}`,
	whiteSpace: 'nowrap',
})

globalStyle(`${table} td`, {
	padding: theme.space[12],
	borderBottom: `1px solid ${theme.colors.background.divider}`,
	verticalAlign: 'middle',
})

globalStyle(`${table} th:first-child, ${table} td:first-child`, {
	paddingLeft: theme.space[20],
})

globalStyle(`${table} th:last-child, ${table} td:last-child`, {
	paddingRight: theme.space[20],
})

globalStyle(`${table} tbody tr:last-child td`, {
	borderBottom: 'none',
})

globalStyle(`${table} tbody tr`, {
	transition: `background-color ${theme.transition.fast}`,
})

globalStyle(`${table} tbody tr:hover`, {
	backgroundColor: theme.colors.background.hover,
})

export const cellActions = style({
	display: 'flex',
	gap: theme.space[4],
	justifyContent: 'flex-end',
})

export const cellEnd = style({})

globalStyle(`${table} th${cellEnd}, ${table} td${cellEnd}`, { textAlign: 'right' })

export const cellMono = style({
	fontFamily: theme.typography.fontFamily.mono,
})

// Una fila histórica (tarea apagada): todo el texto en gris.
export const rowMuted = style({
	color: theme.colors.text.tertiary,
})
