import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const base = style({
	position: 'relative',
	display: 'inline-flex',
	alignItems: 'center',
	justifyContent: 'center',
	gap: theme.space[6],
	flexShrink: 0,
	border: '1px solid transparent',
	borderRadius: theme.radius.md,
	fontWeight: theme.typography.fontWeight.medium,
	lineHeight: 1,
	whiteSpace: 'nowrap',
	textDecoration: 'none',
	cursor: 'pointer',
	transition: `background-color ${theme.transition.fast}, border-color ${theme.transition.fast}, color ${theme.transition.fast}`,
	selectors: {
		'&:disabled, &[aria-disabled="true"]': {
			opacity: 0.5,
			cursor: 'not-allowed',
		},
	},
})

export const variants = styleVariants({
	primary: {
		backgroundColor: theme.colors.background.inverse,
		borderColor: theme.colors.background.inverse,
		color: theme.colors.text.inverse,
		selectors: {
			'&:hover:not(:disabled):not([aria-disabled="true"])': {
				backgroundColor: theme.colors.background.inverseHover,
				borderColor: theme.colors.background.inverseHover,
				color: theme.colors.text.inverse,
			},
		},
	},
	secondary: {
		backgroundColor: 'transparent',
		borderColor: theme.colors.borders.control,
		color: theme.colors.text.primary,
		selectors: {
			'&:hover:not(:disabled):not([aria-disabled="true"])': {
				backgroundColor: theme.colors.background.raised,
				color: theme.colors.text.primary,
			},
		},
	},
	ghost: {
		backgroundColor: 'transparent',
		color: theme.colors.text.secondary,
		fontWeight: theme.typography.fontWeight.regular,
		selectors: {
			'&:hover:not(:disabled):not([aria-disabled="true"])': {
				backgroundColor: theme.colors.background.raised,
				color: theme.colors.text.primary,
			},
		},
	},
	danger: {
		backgroundColor: theme.colors.background.danger,
		borderColor: theme.colors.borders.danger,
		color: theme.colors.text.onDanger,
		selectors: {
			'&:hover:not(:disabled):not([aria-disabled="true"])': {
				backgroundColor: theme.colors.background.dangerHover,
				borderColor: theme.colors.background.dangerHover,
				color: theme.colors.text.onDanger,
			},
		},
	},
})

export const sizes = styleVariants({
	xs: {
		height: theme.control.xs,
		paddingInline: theme.space[8],
		fontSize: theme.typography.fontSize[13],
	},
	sm: {
		height: theme.control.sm,
		paddingInline: theme.space[12],
		fontSize: theme.typography.fontSize[13],
	},
	md: {
		height: theme.control.md,
		paddingInline: theme.space[14],
		fontSize: theme.typography.fontSize[14],
	},
	lg: {
		height: theme.control.lg,
		paddingInline: theme.space[16],
		fontSize: theme.typography.fontSize[14],
	},
})

// Botón de solo ícono: cuadrado del alto de su tamaño. Necesita `aria-label`.
export const iconOnly = style({
	paddingInline: 0,
	aspectRatio: '1 / 1',
})

export const fullWidth = style({ width: '100%' })

export const content = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[6],
})

// Mientras carga, el contenido ocupa su lugar sin verse: el botón no cambia de ancho.
export const contentHidden = style({ visibility: 'hidden' })

export const spinnerSlot = style({
	position: 'absolute',
	inset: 0,
	display: 'grid',
	placeItems: 'center',
})

export type ButtonVariant = keyof typeof variants
export type ButtonSize = keyof typeof sizes
