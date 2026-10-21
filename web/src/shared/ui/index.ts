// Superficie pública del design system. Las pantallas importan de acá, nunca de
// `components/<nivel>/<pieza>/...`. El catálogo con las props está en web/CLAUDE.md.

// Átomos
export { default as Avatar } from './components/atoms/avatar/Avatar.svelte'
export type { AvatarSize } from './components/atoms/avatar/styles.css'
export { default as Badge } from './components/atoms/badge/Badge.svelte'
export type { BadgeShape, BadgeSize, BadgeTone } from './components/atoms/badge/styles.css'
export { default as Button } from './components/atoms/button/Button.svelte'
export { default as LinkButton } from './components/atoms/button/LinkButton.svelte'
export type { ButtonSize, ButtonVariant } from './components/atoms/button/styles.css'
export { default as Checkbox } from './components/atoms/checkbox/Checkbox.svelte'
export type { ControlSize } from './components/atoms/control.css'
export { default as Icon } from './components/atoms/icon/Icon.svelte'
export type { IconName } from './components/atoms/icon/icons'
export { default as Input } from './components/atoms/input/Input.svelte'
export { default as Logo } from './components/atoms/logo/Logo.svelte'
export { default as Mono } from './components/atoms/mono/Mono.svelte'
export type { MonoSize, MonoTone } from './components/atoms/mono/styles.css'
export { default as Select } from './components/atoms/select/Select.svelte'
export {
	default as ShadowHost,
	type ShadowMount,
} from './components/atoms/shadowHost/ShadowHost.svelte'
export { default as Spinner } from './components/atoms/spinner/Spinner.svelte'
export type { SpinnerSize } from './components/atoms/spinner/styles.css'
export { default as StatusDot } from './components/atoms/statusDot/StatusDot.svelte'
export type { StatusDotSize, StatusTone } from './components/atoms/statusDot/styles.css'
export { default as Switch } from './components/atoms/switch/Switch.svelte'
export { default as Textarea } from './components/atoms/textarea/Textarea.svelte'
export { default as TextLink } from './components/atoms/textLink/TextLink.svelte'

// Moléculas
export { default as Banner } from './components/molecules/banner/Banner.svelte'
export type { BannerTone } from './components/molecules/banner/styles.css'
export { default as Breadcrumb } from './components/molecules/breadcrumb/Breadcrumb.svelte'
export type {
	BreadcrumbItem,
	BreadcrumbLink,
	BreadcrumbMenu,
} from './components/molecules/breadcrumb/types'
export { default as Card } from './components/molecules/card/Card.svelte'
export type { CardTone } from './components/molecules/card/styles.css'
export { default as CodeInput } from './components/molecules/codeInput/CodeInput.svelte'
export { default as ComingSoon } from './components/molecules/comingSoon/ComingSoon.svelte'
export { default as ConfirmDialog } from './components/molecules/confirmDialog/ConfirmDialog.svelte'
export { default as CopyButton } from './components/molecules/copyButton/CopyButton.svelte'
export { default as EmptyState } from './components/molecules/emptyState/EmptyState.svelte'
export { default as ErrorState } from './components/molecules/errorState/ErrorState.svelte'
export { default as Field } from './components/molecules/field/Field.svelte'
export type { FieldA11y } from './components/molecules/field/types'
export { default as List } from './components/molecules/list/List.svelte'
export { default as ListRow } from './components/molecules/list/ListRow.svelte'
export type { ListRowHeight } from './components/molecules/list/styles.css'
export { default as LoadingState } from './components/molecules/loadingState/LoadingState.svelte'
export { default as LoadMore } from './components/molecules/loadMore/LoadMore.svelte'
export { default as LocaleSwitcher } from './components/molecules/localeSwitcher/LocaleSwitcher.svelte'
export { default as Menu } from './components/molecules/menu/Menu.svelte'
export type { MenuEntry } from './components/molecules/menu/types'
export { default as Modal } from './components/molecules/modal/Modal.svelte'
export type { ModalSize } from './components/molecules/modal/styles.css'
export { default as PageHeader } from './components/molecules/pageHeader/PageHeader.svelte'
export { default as Pagination } from './components/molecules/pagination/Pagination.svelte'
export { default as RadioGroup } from './components/molecules/radioGroup/RadioGroup.svelte'
export { default as SearchInput } from './components/molecules/searchInput/SearchInput.svelte'
export { default as SegmentedControl } from './components/molecules/segmentedControl/SegmentedControl.svelte'
export type { ChoiceOption } from './components/molecules/segmentedControl/types'
export { default as SubNav } from './components/molecules/subNav/SubNav.svelte'
export { default as SubNavLayout } from './components/molecules/subNav/SubNavLayout.svelte'
export {
	SectionSpy,
	type SectionSpyOptions,
} from './components/molecules/subNav/sectionSpy.svelte.ts'
export { cellActions, cellEnd, cellMono, rowMuted } from './components/molecules/table/styles.css'
export { default as Table } from './components/molecules/table/Table.svelte'
export { default as Tabs } from './components/molecules/tabs/Tabs.svelte'
export type { NavItem } from './components/molecules/tabs/types'
export { default as Toaster } from './components/molecules/toaster/Toaster.svelte'

// Organismos
export { default as AppShell } from './components/organisms/appShell/AppShell.svelte'
export { default as ShellHeader } from './components/organisms/appShell/ShellHeader.svelte'
export { default as AuthLayout } from './components/organisms/authLayout/AuthLayout.svelte'
export type { AuthWidth } from './components/organisms/authLayout/styles.css'

// Clases sueltas y helpers de la vista
export { srOnly, textLink, truncate } from './themes/utilities.css'
export { describeError } from './utils/errorMessage'
export { firstError } from './utils/forms'
export { formatDate, formatDateTime, formatNumber, formatRelative } from './utils/format'
export { type QueryValue, withQuery } from './utils/url'
