<script lang="ts">
	import { TotpSetupScreen } from '@/modules/auth'
	import type { PageProps } from './$types'

	let { data }: PageProps = $props()
</script>

<TotpSetupScreen me={data.me} />
