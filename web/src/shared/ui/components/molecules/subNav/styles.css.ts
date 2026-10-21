import { createContainer, globalStyle, style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// La nav vertical de Ajustes y de Configuración (`m-sub` en el diseño).
export const nav = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
	minWidth: 0,
})

export const link = style({
	display: 'flex',
	alignItems: 'center',
	height: theme.control.sm,
	paddingInline: theme.space[10],
	borderRadius: theme.radius.md,
	color: theme.colors.text.secondary,
	transition: `color ${theme.transition.fast}, background-color ${theme.transition.fast}`,
	selectors: {
		'&:hover': { color: theme.colors.text.primary },
	},
})

export const linkActive = style({
	backgroundColor: theme.colors.background.raised,
	color: theme.colors.text.primary,
	fontWeight: theme.typography.fontWeight.medium,
})

// El layout de dos columnas que acompaña a la subnav: 180 px de nav y el resto de
// contenido; en un teléfono, una debajo de la otra.
const NAV_BASIS = 180
const CONTENT_BASIS = 600
// El gap entre columnas (`theme.space[40]`) en número: la condición de un @container no
// acepta var(). Si cambia uno, cambia el otro (lo atrapa SubNavLayout.svelte.test.ts).
const COLUMN_GAP = 40

// El layout es el contenedor de su propia query: las columnas van una al lado de la otra
// justo desde 180 + 40 + 600 = 820 px de ancho del layout, lo mismo que decide el
// flex-wrap, sin depender del ancho de la pantalla ni del gutter del AppShell.
const layoutContainer = createContainer()
const sideBySide = `${layoutContainer} (min-width: ${NAV_BASIS + COLUMN_GAP + CONTENT_BASIS}px)`

export const layout = style({
	containerName: layoutContainer,
	containerType: 'inline-size',
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'flex-start',
	gap: `${theme.space[24]} ${theme.space[40]}`,
})

// Al costado del contenido, la nav acompaña el scroll. El header del AppShell no es sticky
// (se va con la página), así que arriba no hay nada que esquivar: solo el aire de
// `stickyTop`, entre el borde de la pantalla y los links. Si la nav no entra en la pantalla,
// scrollea adentro en vez de quedar cortada. Arriba del contenido (teléfono) se queda quieta:
// sticky, taparía lo que viene abajo.
const stickyTop = theme.space[24]
// Con overflow, la nav recortaría el anillo de foco global de sus links (2 px de outline a
// 2 px del borde: 4 px por fuera). El padding le hace lugar y el margen negativo lo
// compensa: los links y el contenido quedan donde estaban. Pegada arriba, la caja empieza
// ese padding antes, así los links quedan a `stickyTop`.
const ringRoom = theme.space[4]
const asideTop = `calc(${stickyTop} - ${ringRoom})`

export const aside = style({
	flex: `1 1 ${NAV_BASIS}px`,
	minWidth: 0,
	'@container': {
		[sideBySide]: {
			position: 'sticky',
			top: asideTop,
			maxHeight: `calc(100dvh - ${asideTop} * 2)`,
			overflowY: 'auto',
			flexBasis: `calc(${NAV_BASIS}px + ${ringRoom} * 2)`,
			padding: ringRoom,
			margin: `calc(${ringRoom} * -1)`,
		},
	},
})

export const content = style({
	flex: `4 1 ${CONTENT_BASIS}px`,
	minWidth: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[20],
})

// Los destinos de los links de la nav (las secciones, por su id): al saltar a una, la
// tarjeta queda a la altura de los links de la nav pegada arriba, no contra el borde. Lo
// demás con id que haya adentro (un campo, un título) solo gana ese aire si algo lo trae a
// la vista.
globalStyle(`${content} [id]`, {
	scrollMarginTop: stickyTop,
})
