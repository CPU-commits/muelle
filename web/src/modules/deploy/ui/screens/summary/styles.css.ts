import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const page = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
	minWidth: 0,
})

// "Servicio · stack auditfy_staging_api · red auditfy_staging"
export const detailsLine = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: `${theme.space[4]} ${theme.space[16]}`,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const detail = style({
	display: 'inline-flex',
	gap: theme.space[6],
	minWidth: 0,
})

export const badgeLink = style({
	borderRadius: theme.radius.full,
	textDecoration: 'none',
})

export const actionRow = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: theme.space[8],
})

export const live = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
})

// Las métricas del servicio: una grilla que envuelve, con divisores de 1 px. Cada celda
// dibuja su línea derecha e inferior y la grilla se corre 1 px para esconder las del
// borde (la tarjeta corta lo que sobra): así un hueco al final queda del color de la
// tarjeta y no como un bloque gris.
export const metrics = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(auto-fill, minmax(min(240px, 100%), 1fr))',
	marginRight: '-1px',
	marginBottom: '-1px',
})

export const metric = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[6],
	minWidth: 0,
	padding: `${theme.space[16]} ${theme.space[20]}`,
	boxShadow: `inset -1px -1px 0 ${theme.colors.borders.subtle}`,
})

export const term = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const bigValue = style({
	fontSize: theme.typography.fontSize[20],
	fontWeight: theme.typography.fontWeight.medium,
	letterSpacing: theme.typography.letterSpacing.tight,
	fontVariantNumeric: 'tabular-nums',
})

export const unit = style({
	fontSize: theme.typography.fontSize[14],
	fontWeight: theme.typography.fontWeight.regular,
	letterSpacing: 'normal',
	color: theme.colors.text.tertiary,
})

// El diseño usa #6E6E77, que sobre la tarjeta no llega a 4.5:1: va el terciario.
export const ofTotal = style({
	color: theme.colors.text.tertiary,
})

export const inlineValue = style({
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[8],
	minWidth: 0,
	fontSize: theme.typography.fontSize[14],
})

export const deployLink = style({
	fontFamily: theme.typography.fontFamily.mono,
	textDecoration: 'none',
	selectors: {
		'&:hover': { textDecoration: 'underline', textUnderlineOffset: '3px' },
	},
})

export const muted = style({
	color: theme.colors.text.tertiary,
})

export const digestRow = style({
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[6],
	minWidth: 0,
	margin: `-${theme.space[4]} 0`,
})

export const taskId = style({
	color: theme.colors.text.tertiary,
})

export const taskState = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	whiteSpace: 'nowrap',
})

export const taskError = style({
	color: theme.colors.status.errorText,
})

export const empty = style({
	padding: `${theme.space[16]} ${theme.space[20]}`,
	color: theme.colors.text.tertiary,
})
