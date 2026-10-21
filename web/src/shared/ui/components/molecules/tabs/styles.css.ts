import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// La nav subrayada del header: tabs de 44 px, el activo con 2 px claros abajo. El
// margen negativo de abajo apoya el subrayado sobre el borde del header.
export const nav = style({
	display: 'flex',
	gap: theme.space[4],
	overflowX: 'auto',
	marginInline: `calc(${theme.space[8]} * -1)`,
	marginBottom: '-1px',
	scrollbarWidth: 'none',
})

export const tab = style({
	flex: 'none',
	display: 'flex',
	alignItems: 'center',
	height: theme.control.tab,
	paddingInline: theme.space[8],
	borderBottom: '2px solid transparent',
	color: theme.colors.text.secondary,
	transition: `color ${theme.transition.fast}, border-color ${theme.transition.fast}`,
	selectors: {
		'&:hover': { color: theme.colors.text.primary },
		'&:focus-visible': { outlineOffset: '-2px', borderRadius: theme.radius.md },
	},
})

export const tabActive = style({
	borderBottomColor: theme.colors.borders.outline,
	color: theme.colors.text.primary,
})
