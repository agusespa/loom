<script lang="ts">
	import type { Day, HabitWeek, Status } from './api.ts';
	import { fmt } from './dates.ts';

	let { habits, today, isCurrent }: { habits: HabitWeek[]; today: string; isCurrent: boolean } = $props();

	const DAY_LETTERS = ['M', 'T', 'W', 'T', 'F', 'S', 'S'];
	const LABEL: Record<Status, string> = { done: 'Done', missed: 'Missed', planned: 'Planned' };
	const GLYPH: Record<Status, string> = { done: '✓', missed: '×', planned: '' };

	let selected = $state<string | null>(null);

	const dates = $derived(habits[0]?.days.map((d) => d.date) ?? []);

	// A different week clears the selection.
	$effect(() => {
		void dates[0];
		selected = null;
	});

	const detail = $derived.by(() => {
		if (!selected) return null;
		for (const h of habits) {
			const d = h.days.find((d) => `${h.key}/${d.date}` === selected);
			if (d) return describe(h, d);
		}
		return null;
	});

	function describe(h: HabitWeek, d: Day): string {
		const parts = [
			fmt(d.date, { weekday: 'short', day: 'numeric', month: 'short' }),
			h.name,
			d.status ? LABEL[d.status] : 'Nothing logged'
		];
		if (d.note) parts.push(d.note);
		if (d.value !== null) parts.push(h.key === 'meditation' ? `${Math.round(d.value)} min` : String(d.value));
		return parts.join(' · ');
	}
</script>

<div class="grid" role="table" aria-label="Habits this week">
	<div class="row head" role="row">
		<span></span>
		{#each dates as date, i (date)}
			<span class:today-l={date === today} role="columnheader">{DAY_LETTERS[i]}</span>
		{/each}
	</div>
	{#each habits as h (h.key)}
		<div class="row" role="row">
			<span class="name" role="rowheader">{h.name}</span>
			{#each h.days as d (d.date)}
				{@const key = `${h.key}/${d.date}`}
				<button
					class="d {d.status ?? 'empty'}"
					class:future={d.date > today}
					class:today={isCurrent && d.date === today}
					class:sel={selected === key}
					role="cell"
					aria-label="{h.name}, {fmt(d.date, { weekday: 'long' })}: {d.status ? LABEL[d.status] : 'nothing'}"
					onclick={() => (selected = selected === key ? null : key)}
				>
					{d.status ? GLYPH[d.status] : ''}
				</button>
			{/each}
		</div>
	{/each}
</div>
<div class="detail muted small" aria-live="polite">{detail ?? ' '}</div>
