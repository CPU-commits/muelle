import { style } from '@vanilla-extract/css'
import { menuIn } from '@/shared/ui/themes/animations.css'
import { theme } from '@/shared/ui/themes/theme.css'

export const root = style({
	position: 'relative',
	display: 'inline-flex',
})

// El botón de "Opciones" de una fila: los tres puntos, ghost, 32×32.
export const triggerGhost = style({
	display: 'grid',
	placeItems: 'center',
	width: theme.control.sm,
	height: theme.control.sm,
	borderRadius: theme.radius.md,
	color: theme.colors.text.tertiary,
	transition: `background-color ${theme.transition.fast}, color ${theme.transition.fast}`,
	selectors: {
		'&:hover, &[aria-expanded="true"]': {
			backgroundColor: theme.colors.background.raised,
			color: theme.colors.text.primary,
		},
	},
})

// El de la cuenta: el avatar ya trae su círculo, el botón solo lo envuelve.
export const triggerBare = style({
	display: 'inline-flex',
	borderRadius: theme.radius.full,
})

// En la capa superior (`popover`): se borran los estilos del UA para popovers (inset 0,
// margin auto, borde) y la posición la escribe `placeBelow`.
export const panel = style({
	position: 'fixed',
	inset: 'auto',
	margin: 0,
	minWidth: '220px',
	maxWidth: 'min(320px, calc(100vw - 16px))',
	maxHeight: 'calc(100vh - 16px)',
	overflowY: 'auto',
	padding: theme.space[4],
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
	color: theme.colors.text.primary,
	boxShadow: theme.shadows.menu,
	animation: `${menuIn} 0.12s ease-out`,
	'@media': {
		'(prefers-reduced-motion: reduce)': { animation: 'none' },
	},
})

export const header = style({
	padding: `${theme.space[8]} ${theme.space[10]} ${theme.space[10]}`,
	marginBottom: theme.space[4],
	borderBottom: `1px solid ${theme.colors.borders.subtle}`,
})

export const list = style({
	listStyle: 'none',
	margin: 0,
	padding: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
	outline: 'none',
})

export const item = style({
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[8],
	width: '100%',
	minHeight: theme.control.sm,
	paddingInline: theme.space[10],
	borderRadius: theme.radius.md,
	color: theme.colors.text.primary,
	fontSize: theme.typography.fontSize[14],
	textAlign: 'left',
	whiteSpace: 'nowrap',
	cursor: 'pointer',
	selectors: {
		'&:hover, &:focus-visible': {
			backgroundColor: theme.colors.background.raised,
			outline: 'none',
		},
		'&:disabled': { opacity: 0.5, cursor: 'not-allowed' },
	},
})

export const itemDanger = style({
	color: theme.colors.status.errorText,
})

// El ícono de un item. Si algún item del menú tiene uno, los que no tienen dejan el mismo
// hueco vacío: así todos los textos empiezan en la misma x.
export const itemIcon = style({
	display: 'flex',
	flex: 'none',
	width: '16px',
	color: theme.colors.text.tertiary,
})

export const separator = style({
	height: '1px',
	marginBlock: theme.space[4],
	backgroundColor: theme.colors.borders.subtle,
})
