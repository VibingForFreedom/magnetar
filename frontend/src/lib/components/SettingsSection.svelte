<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		children: Snippet;
	}

	let { title, children }: Props = $props();
</script>

<div class="rounded-lg border border-border bg-bg-secondary p-5">
	<h3 class="mb-4 text-sm font-medium uppercase tracking-wider text-text-secondary">{title}</h3>
	{@render children()}
</div>
