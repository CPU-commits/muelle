export type ColorsTheme = {
	background: {
		/** Fondo de la página y de los inputs. */
		page: string
		/** Tarjetas, filas y diálogos. */
		surface: string
		/** Pie de tarjeta, fila de stage, filas atenuadas. */
		sunken: string
		/** Fila con el puntero encima. */
		hover: string
		/** Hover de botón secundario o ghost, avatar, item activo de una subnav. */
		raised: string
		/** El 1 px entre filas de una lista. */
		divider: string
		/** Botón primario. */
		inverse: string
		inverseHover: string
		danger: string
		dangerHover: string
	}
	borders: {
		subtle: string
		control: string
		strong: string
		/** Borde de un control con foco. */
		focus: string
		/** Outline de `:focus-visible`. */
		outline: string
		danger: string
	}
	text: {
		primary: string
		secondary: string
		tertiary: string
		/** Deshabilitado e íconos: no cumple 4.5:1, no usarlo para texto que hay que leer. */
		disabled: string
		inverse: string
		strong: string
		/** Las barras del breadcrumb. */
		separator: string
		onDanger: string
	}
	status: {
		ok: string
		warn: string
		warnText: string
		warnBorder: string
		warnSurface: string
		error: string
		errorText: string
		errorBorder: string
		errorSurface: string
		errorDivider: string
		errorSunken: string
		idle: string
		okPulse: string
		okPulseEnd: string
	}
	overlay: string
	selection: string
	/** El QR va oscuro sobre blanco: los lectores no siempre leen uno invertido. */
	qr: { background: string; foreground: string }
}

// Escala en píxeles, con la clave igual al valor: `theme.space[12]` es 12px. El diseño
// usa pasos de 2 y 4, y nombrarlos (`md`, `lg`…) obligaba a recordar una tabla.
export type SpaceTheme = {
	0: string
	2: string
	4: string
	6: string
	8: string
	10: string
	12: string
	14: string
	16: string
	20: string
	24: string
	28: string
	32: string
	40: string
	48: string
	80: string
}

export type TypographyTheme = {
	fontFamily: { sans: string; mono: string }
	fontSize: {
		11: string
		12: string
		13: string
		14: string
		15: string
		16: string
		18: string
		20: string
		24: string
		28: string
	}
	fontWeight: { regular: string; medium: string; semibold: string }
	lineHeight: { tight: string; snug: string; normal: string; relaxed: string }
	letterSpacing: { tighter: string; tight: string; snug: string }
}

export type RadiusTheme = {
	/** Badges. */
	xs: string
	/** Íconos de proyecto. */
	sm: string
	/** Controles. */
	md: string
	/** Tarjetas y avisos. */
	lg: string
	/** Diálogos. */
	xl: string
	full: string
}

export type ControlTheme = {
	/** Acciones de fila. */
	xs: string
	/** Botones chicos. */
	sm: string
	/** Botones e inputs. */
	md: string
	/** Login. */
	lg: string
	/** Celdas del código de 6 dígitos. */
	xl: string
	/** Tabs de la nav. */
	tab: string
	badge: string
	badgeLg: string
}

export type ShadowsTheme = {
	focusRing: string
	modal: string
	menu: string
}

export type LayoutTheme = {
	maxWidth: string
	gutter: string
	authWidth: string
}

export type ZIndexTheme = {
	base: string
	sticky: string
	dropdown: string
	modal: string
	toast: string
}

export type Theme = {
	colors: ColorsTheme
	space: SpaceTheme
	typography: TypographyTheme
	radius: RadiusTheme
	control: ControlTheme
	shadows: ShadowsTheme
	layout: LayoutTheme
	zIndex: ZIndexTheme
	transition: { fast: string }
}
