import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const shell = style({
	minHeight: '100vh',
	display: 'flex',
	flexDirection: 'column',
	backgroundColor: theme.colors.background.page,
})

// El contenedor del diseño: 1248 px de ancho máximo y gutter de 16 a 24 px.
const container = {
	width: '100%',
	maxWidth: theme.layout.maxWidth,
	marginInline: 'auto',
	paddingInline: theme.layout.gutter,
} as const

export const header = style({
	borderBottom: `1px solid ${theme.colors.borders.subtle}`,
	backgroundColor: theme.colors.background.page,
})

export const headerInner = style({
	...container,
	paddingTop: theme.space[10],
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[4],
})

export const topRow = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
	minHeight: theme.control.lg,
})

export const brand = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[10],
	height: theme.control.md,
	padding: `0 ${theme.space[8]} 0 ${theme.space[6]}`,
	marginLeft: `calc(${theme.space[6]} * -1)`,
	borderRadius: theme.radius.md,
	color: theme.colors.text.primary,
	selectors: {
		'&:hover': { backgroundColor: theme.colors.background.raised },
	},
})

export const brandName = style({
	fontSize: theme.typography.fontSize[15],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.tight,
})

export const accountName = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
})

export const accountUser = style({
	fontWeight: theme.typography.fontWeight.medium,
})

export const accountHint = style({
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

export const main = style({
	...container,
	flex: 1,
	paddingTop: theme.space[32],
	paddingBottom: theme.space[80],
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[28],
	minWidth: 0,
	selectors: {
		'&:focus, &:focus-visible': { outline: 'none' },
	},
})

// "Saltar al contenido": invisible hasta que el teclado le da foco.
export const skipLink = style({
	position: 'absolute',
	top: theme.space[8],
	left: theme.space[8],
	zIndex: theme.zIndex.toast,
	padding: `${theme.space[8]} ${theme.space[12]}`,
	borderRadius: theme.radius.md,
	backgroundColor: theme.colors.background.inverse,
	color: theme.colors.text.inverse,
	fontWeight: theme.typography.fontWeight.medium,
	transform: 'translateY(-200%)',
	selectors: {
		'&:focus-visible': { transform: 'none' },
	},
})
