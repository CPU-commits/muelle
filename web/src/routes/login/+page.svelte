<script lang="ts">
	import { LoginScreen } from '@/modules/auth'
	import type { PageProps } from './$types'

	let { data }: PageProps = $props()
</script>

<LoginScreen initialStep={data.step} />
