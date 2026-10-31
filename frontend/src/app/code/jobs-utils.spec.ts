import { canCancel, canRetry, formatDuration, isActive, jobProgress, jobsQuery, prettyValue } from './jobs-utils';

describe('jobs helpers', () => {
  it('progress: numbers, percentages, fractions, text', () => {
    expect(jobProgress(45)).toEqual({ pct: 45, text: '45%' });
    expect(jobProgress('72%')).toEqual({ pct: 72, text: '72%' });
    expect(jobProgress('150')).toEqual({ pct: 100, text: '100%' });
    expect(jobProgress('3/10')).toEqual({ pct: 30, text: '3/10' });
    expect(jobProgress('3/0')).toEqual({ pct: null, text: '3/0' });
    expect(jobProgress('sending mails')).toEqual({ pct: null, text: 'sending mails' });
    expect(jobProgress(null)).toEqual({ pct: null, text: '' });
    expect(jobProgress('')).toEqual({ pct: null, text: '' });
  });
  it('actions per status', () => {
    expect(['pending', 'running', 'done', 'failed', 'cancelled'].map(s => [canCancel({ status: s }), canRetry({ status: s }), isActive({ status: s })])).toEqual([
      [true, false, true], [true, false, true], [false, false, false], [false, true, false], [false, true, false],
    ]);
  });
  it('query parameters', () => {
    expect(jobsQuery('', ' ', 0)).toEqual({ status: undefined, name: undefined, page: 1, perPage: 50 });
    expect(jobsQuery('failed', ' sendMail ', 3, 20)).toEqual({ status: 'failed', name: 'sendMail', page: 3, perPage: 20 });
  });
  it('durations', () => {
    expect(formatDuration('2026-10-24T10:00:00Z', '2026-10-24T10:00:00.850Z')).toBe('850 ms');
    expect(formatDuration('2026-10-24T10:00:00Z', '2026-10-24T10:00:04.2Z')).toBe('4.2 s');
    expect(formatDuration('2026-10-24T10:00:00Z', '2026-10-24T10:03:05Z')).toBe('3 m 05 s');
    expect(formatDuration('2026-10-24T10:00:00Z', '2026-10-24T12:10:00Z')).toBe('2 h 10 m');
    expect(formatDuration(null, '2026-10-24T12:10:00Z')).toBe('');
    expect(formatDuration('2026-10-24T12:10:00Z', '2026-10-24T10:00:00Z')).toBe('');
  });
  it('pretty values', () => {
    expect(prettyValue('{"a":1}')).toBe('{\n  "a": 1\n}');
    expect(prettyValue('plain text')).toBe('plain text');
    expect(prettyValue({ id: 3 })).toBe('{\n  "id": 3\n}');
    expect(prettyValue(null)).toBe('');
  });
});
