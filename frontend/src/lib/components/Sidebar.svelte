<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { LayoutDashboard, Database, Target, Settings, Magnet } from 'lucide-svelte';

	const links = [
		{ href: '/' as const, label: 'Dashboard', icon: LayoutDashboard },
		{ href: '/torrents' as const, label: 'Torrents', icon: Database },
		{ href: '/matcher' as const, label: 'Matcher', icon: Target },
		{ href: '/settings' as const, label: 'Settings', icon: Settings }
	];
</script>

<aside class="fixed left-0 top-0 z-10 flex h-screen w-56 flex-col border-r border-border bg-bg-secondary">
	<div class="flex items-center gap-2 px-5 py-5">
		<Magnet class="h-5 w-5 text-accent-blue" />
		<span class="text-base font-semibold tracking-tight text-text-primary">Magnetar</span>
	</div>

	<nav class="flex-1 px-3 py-2">
		{#each links as link (link.href)}
			{@const active = page.url.pathname === link.href}
			<a
				href={resolve(link.href)}
				class="mb-1 flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors {active
					? 'bg-bg-tertiary text-text-primary'
					: 'text-text-secondary hover:bg-bg-tertiary/50 hover:text-text-primary'}"
			>
				<link.icon class="h-4 w-4" />
				{link.label}
			</a>
		{/each}
	</nav>

	<div class="border-t border-border px-5 py-3">
		<span class="text-xs text-text-secondary">v0.1.0-dev</span>
	</div>
</aside>
