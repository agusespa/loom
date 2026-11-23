import { describe, expect, it } from 'vitest';
import { addDays, ago, delta, isoWeek, pct } from './dates.ts';

describe('isoWeek', () => {
	it('numbers weeks by their Thursday', () => {
		expect(isoWeek('2026-11-16')).toBe(47);
		// 29 Dec 2025 belongs to week 1 of 2026.
		expect(isoWeek('2025-12-29')).toBe(1);
		// 28 Dec 2026 is in week 53 of 2026.
		expect(isoWeek('2026-12-28')).toBe(53);
	});
});

describe('addDays', () => {
	it('crosses months, years and DST without drifting', () => {
		expect(addDays('2026-10-19', 7)).toBe('2026-10-26');
		expect(addDays('2026-12-28', 7)).toBe('2027-01-04');
		expect(addDays('2026-03-02', -7)).toBe('2026-02-23');
	});
});

describe('pct', () => {
	it('rounds and shows a dash for no value', () => {
		expect(pct(0.666)).toBe('67%');
		expect(pct(null)).toBe('–');
	});
});

describe('ago', () => {
	const now = Date.UTC(2026, 10, 21, 12);
	it('picks a unit', () => {
		expect(ago(now - 20_000, now)).toBe('just now');
		expect(ago(now - 5 * 60_000, now)).toBe('5 min ago');
		expect(ago(now - 3 * 3600_000, now)).toBe('3 h ago');
		expect(ago(now - 3 * 86400_000, now)).toBe('3 days ago');
	});
});

describe('delta', () => {
	it('compares rounded percentages', () => {
		expect(delta(0.784, 0.721)).toBe(6);
		expect(delta(0.5, 0.5)).toBe(0);
		expect(delta(0.5, null)).toBeNull();
		expect(delta(null, 0.5)).toBeNull();
	});
});
