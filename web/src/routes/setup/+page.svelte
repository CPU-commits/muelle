<script lang="ts">
	import { SetupScreen } from '@/modules/auth'
	import type { PageProps } from './$types'

	let { data }: PageProps = $props()
</script>

<SetupScreen token={data.token} />
