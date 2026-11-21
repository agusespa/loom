<script lang="ts">
	import type { HabitWeek, Status } from './api.ts';
	import { fmt } from './dates.ts';
	import StatusIcon from './StatusIcon.svelte';

	let {
		habits,
		today,
		isCurrent,
		onedit
	}: {
		habits: HabitWeek[];
		today: string;
		isCurrent: boolean;
		/** Saves a status (null clears it); resolves to an error message, or null when saved. */
		onedit: (habit: string, date: string, status: Status | null) => Promise<string | null>;
	} = $props();

	const DAY_LETTERS = ['M', 'T', 'W', 'T', 'F', 'S', 'S'];
	const LABEL: Record<Status, string> = { done: 'Done', missed: 'Missed', planned: 'Planned' };

	let selected = $state<{ habit: string; date: string } | null>(null);
	let saving = $state(false);
	let error = $state<string | null>(null);

	const dates = $derived(habits[0]?.days.map((d) => d.date) ?? []);

	// A different week clears the selection; reloading the same week (after a save) keeps it.
	const weekStart = $derived(dates[0]);
	$effect(() => {
		void weekStart;
		selected = null;
	});

	const sel = $derived.by(() => {
		if (!selected) return null;
		const h = habits.find((h) => h.key === selected!.habit);
		const d = h?.days.find((d) => d.date === selected!.date);
		return h && d ? { habit: h, day: d } : null;
	});

	/** What the selected day can be set to (plus its current status): only plans for days ahead, and plannable habits only. */
	const choices = $derived.by((): Status[] => {
		if (!sel) return [];
		const future = sel.day.date > today;
		const all: Status[] = sel.habit.plannable
			? future
				? ['planned']
				: ['done', 'missed', 'planned']
			: future
				? []
				: ['done'];
		return sel.day.status && !all.includes(sel.day.status) ? [...all, sel.day.status] : all;
	});

	/** Each habit wears one of the icon's thread colors, by position. */
	const thread = (i: number) => `var(--thread-${i % 5})`;

	let wrap = $state<HTMLDivElement>();
	let menuPos = $state({ left: 0, top: 0 });
	const MENU_W = 168;

	function open(e: MouseEvent, habit: string, date: string) {
		error = null;
		if (selected?.habit === habit && selected.date === date) {
			selected = null;
			return;
		}
		// Anchor the menu under the tapped cell, kept inside the grid.
		const cell = (e.currentTarget as HTMLElement).getBoundingClientRect();
		const box = wrap!.getBoundingClientRect();
		const center = cell.left + cell.width / 2 - box.left;
		menuPos = {
			left: Math.max(4, Math.min(center - MENU_W / 2, box.width - MENU_W - 4)),
			top: cell.bottom - box.top + 6
		};
		selected = { habit, date };
	}

	function close() {
		selected = null;
		error = null;
	}

	async function set(status: Status | null) {
		if (!sel || saving) return;
		if (status === sel.day.status) return close();
		saving = true;
		error = await onedit(sel.habit.key, sel.day.date, status);
		saving = false;
		if (!error) close();
	}

	function onWindowPointer(e: PointerEvent) {
		const t = e.target as Element;
		if (selected && !t.closest('.menu') && !t.closest('.d')) close();
	}
</script>

<svelte:window onpointerdown={onWindowPointer} onkeydown={(e) => e.key === 'Escape' && close()} />

<div class="grid-wrap" bind:this={wrap}>
	<div class="grid" role="table" aria-label="Habits this week">
		<div class="row head" role="row">
			<span></span>
			{#each dates as date, i (date)}
				<span class:today-l={date === today} role="columnheader">{DAY_LETTERS[i]}</span>
			{/each}
		</div>
		{#each habits as h, i (h.key)}
			<div class="row" role="row" style:--hc={thread(i)}>
				<span class="name" role="rowheader">{h.name}</span>
				{#each h.days as d (d.date)}
					<button
						class="d {d.status ?? 'empty'}"
						class:future={d.date > today}
						class:today={isCurrent && d.date === today}
						class:sel={selected?.habit === h.key && selected.date === d.date}
						role="cell"
						aria-label="{h.name}, {fmt(d.date, { weekday: 'long' })}: {d.status
							? LABEL[d.status]
							: 'nothing'}"
						onclick={(e) => open(e, h.key, d.date)}
					>
						{#if d.status}<StatusIcon status={d.status} />{/if}
					</button>
				{/each}
			</div>
		{/each}
	</div>

	{#if sel}
		<div
			class="menu"
			role="menu"
			style:--hc={thread(habits.indexOf(sel.habit))}
			aria-label="Set {sel.habit.name}"
			style:left="{menuPos.left}px"
			style:top="{menuPos.top}px"
			style:width="{MENU_W}px"
		>
			<div class="menu-head">
				{fmt(sel.day.date, { weekday: 'short', day: 'numeric', month: 'short' })} · {sel.habit.name}
			</div>
			{#if choices.length}
				{#each choices as s (s)}
					<button
						class="menu-item"
						role="menuitemradio"
						aria-checked={sel.day.status === s}
						disabled={saving}
						onclick={() => set(s)}
					>
						<span class="swatch {s}"><StatusIcon status={s} size={11} /></span>{LABEL[s]}
						{#if sel.day.status === s}<span class="check">✓</span>{/if}
					</button>
				{/each}
				<button
					class="menu-item"
					role="menuitemradio"
					aria-checked={!sel.day.status}
					disabled={saving}
					onclick={() => set(null)}
				>
					<span class="swatch empty"></span>Nothing
					{#if !sel.day.status}<span class="check">✓</span>{/if}
				</button>
			{:else}
				<div class="menu-note">Nothing to log yet</div>
			{/if}
			{#if error}<div class="menu-note err">{error}</div>{/if}
		</div>
	{/if}
</div>
