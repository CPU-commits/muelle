<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { sizes, spinner, type SpinnerSize } from './styles.css'

	type Props = {
		size?: SpinnerSize
		label?: string
		/** Solo visual: el texto de "cargando" lo dice otro (LoadingState, el botón ocupado). */
		decorative?: boolean
	}

	let { size = 'md', label, decorative = false }: Props = $props()
</script>

{#if decorative}
	<span aria-hidden="true" class={[spinner, sizes[size]]}></span>
{:else}
	<span role="status" aria-label={label ?? m.common_loading()} class={[spinner, sizes[size]]}
	></span>
{/if}
