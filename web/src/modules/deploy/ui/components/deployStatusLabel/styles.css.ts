import { style, styleVariants } from '@vanilla-extract/css'
import { spin } from '@/shared/ui/themes/animations.css'
import { theme } from '@/shared/ui/themes/theme.css'

export const root = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[6],
	fontSize: theme.typography.fontSize[13],
	whiteSpace: 'nowrap',
})

// La pastilla del encabezado del log ("● Fallido").
export const pill = style({
	height: '22px',
	padding: `0 ${theme.space[8]}`,
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.full,
	fontSize: theme.typography.fontSize[12],
})

export const textTones = styleVariants({
	queued: { color: theme.colors.text.secondary },
	running: { color: theme.colors.text.secondary },
	succeeded: { color: theme.colors.text.secondary },
	failed: { color: theme.colors.status.errorText },
	superseded: { color: theme.colors.text.tertiary },
})

// "En curso": un anillo que gira en vez de un punto. El tema no tiene el azul del
// diseño; el ámbar de "en proceso" más el giro lo distinguen de los estados finales.
export const running = style({
	display: 'inline-block',
	flexShrink: 0,
	width: '8px',
	height: '8px',
	borderRadius: theme.radius.full,
	border: `1.5px solid ${theme.colors.status.warn}`,
	borderTopColor: 'transparent',
	animation: `${spin} 0.9s linear infinite`,
	'@media': {
		'(prefers-reduced-motion: reduce)': {
			animation: 'none',
			borderTopColor: theme.colors.status.warn,
		},
	},
})

export const runningSmall = style({ width: '6px', height: '6px', borderWidth: '1.5px' })
