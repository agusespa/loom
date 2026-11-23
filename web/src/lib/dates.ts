/** Dates travel as YYYY-MM-DD strings; they are handled as UTC midnights. */

export function parseDate(s: string): Date {
	const [y, m, d] = s.split('-').map(Number);
	return new Date(Date.UTC(y, m - 1, d));
}

export function fmt(s: string, opts: Intl.DateTimeFormatOptions): string {
	return parseDate(s).toLocaleDateString('en-GB', { timeZone: 'UTC', ...opts });
}

export function addDays(s: string, n: number): string {
	const d = parseDate(s);
	d.setUTCDate(d.getUTCDate() + n);
	return d.toISOString().slice(0, 10);
}

/** The ISO week number: the week holding this week's Thursday. */
export function isoWeek(s: string): number {
	const thu = parseDate(s);
	thu.setUTCDate(thu.getUTCDate() + 3 - ((thu.getUTCDay() + 6) % 7));
	const jan1 = Date.UTC(thu.getUTCFullYear(), 0, 1);
	return 1 + Math.floor((thu.getTime() - jan1) / 864e5 / 7);
}

export function pct(x: number | null | undefined): string {
	return x === null || x === undefined ? '–' : Math.round(x * 100) + '%';
}

/** Change in whole percentage points from prev to a; null when there's nothing to compare. */
export function delta(a: number | null, prev: number | null): number | null {
	return a === null || prev === null ? null : Math.round(a * 100) - Math.round(prev * 100);
}

export function ago(ms: number, now = Date.now()): string {
	const min = Math.round((now - ms) / 60000);
	if (min < 1) return 'just now';
	if (min < 60) return `${min} min ago`;
	const h = Math.round(min / 60);
	if (h < 48) return `${h} h ago`;
	return `${Math.round(h / 24)} days ago`;
}
