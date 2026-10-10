<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { sizes, spinner, type SpinnerSize } from './styles.css'

	let { size = 'md', label }: { size?: SpinnerSize; label?: string } = $props()
</script>

<span role="status" aria-label={label ?? m.common_loading()} class={[spinner, sizes[size]]}></span>
