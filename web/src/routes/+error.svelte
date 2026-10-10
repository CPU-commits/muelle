<script lang="ts">
	import { resolve } from '$app/paths'
	import { page } from '$app/state'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { AuthLayout, EmptyState, LinkButton } from '@/shared/ui'
</script>

<AuthLayout appName="muelle" host={page.url.host} width="md">
	<EmptyState
		title={page.status === 404 ? m.not_found_title() : m.common_error_generic()}
		body={page.status === 404 ? m.not_found_body() : m.common_unavailable()}
	>
		{#snippet action()}
			<LinkButton href={resolve('/')} variant="secondary" size="sm">
				{m.not_found_home()}
			</LinkButton>
		{/snippet}
	</EmptyState>
</AuthLayout>
