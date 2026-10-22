<script lang="ts">
	import { SettingsLayout } from '@/modules/admin'
	import type { LayoutProps } from './$types'

	let { children }: LayoutProps = $props()
</script>

<SettingsLayout>
	{@render children()}
</SettingsLayout>
