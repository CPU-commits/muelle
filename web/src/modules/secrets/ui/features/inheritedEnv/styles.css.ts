import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const section = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	minWidth: 0,
})

export const header = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
})

export const heading = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'baseline',
	gap: `${theme.space[4]} ${theme.space[10]}`,
	minWidth: 0,
})

export const title = style({
	fontSize: theme.typography.fontSize[16],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const meta = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const keyCell = style({
	flex: '1 1 220px',
	minWidth: 0,
	overflowWrap: 'anywhere',
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

// La reemplaza la app: la clave se apaga (el texto sigue diciendo por qué).
export const keyReplaced = style({
	color: theme.colors.text.tertiary,
})

export const valueCell = style({
	flex: '2 1 280px',
	minWidth: 0,
	display: 'flex',
	alignItems: 'center',
})

export const noteCell = style({
	flex: '0 0 170px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const empty = style({
	padding: `${theme.space[14]} ${theme.space[20]}`,
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.sunken,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
