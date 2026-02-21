import { writable } from 'svelte/store';
import { connectSSE, type MetricsSnapshot } from '$lib/api';

export const metrics = writable<MetricsSnapshot | null>(null);
export const rateHistory = writable<number[]>([]);
export const metadataRateHistory = writable<number[]>([]);
export const matchRateHistory = writable<number[]>([]);

let eventSource: EventSource | null = null;

export function initMetricsStream() {
	if (eventSource) return;

	eventSource = connectSSE((snap) => {
		metrics.set(snap);
		rateHistory.update((h) => [...h.slice(-59), snap.discovery_rate]);
		metadataRateHistory.update((h) => [...h.slice(-59), snap.metadata_rate]);
		matchRateHistory.update((h) => [...h.slice(-59), snap.match_rate]);
	});
}

export function stopMetricsStream() {
	if (eventSource) {
		eventSource.close();
		eventSource = null;
	}
}
