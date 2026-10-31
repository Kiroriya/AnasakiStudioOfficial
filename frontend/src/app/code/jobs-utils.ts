/**
 * Pure helpers of the Site Code "Jobs" tab (/code/jobs): progress bars,
 * which actions a job offers, query parameters and durations. Unit tested
 * in jobs-utils.spec.ts.
 */
import { CodeJob } from '../core/code-models';

export const JOB_STATUSES = ['pending', 'running', 'done', 'failed', 'cancelled'] as const;
export const JOBS_PER_PAGE = 50;

/** ctx.progress(n | text): a number / "45%" → a bar; "3/10" → 30 %; anything else → text only. */
export function jobProgress(p: unknown): { pct: number | null; text: string } {
  if (p === null || p === undefined || p === '') return { pct: null, text: '' };
  if (typeof p === 'number' && Number.isFinite(p)) return { pct: clamp(p), text: `${clamp(p)}%` };
  const s = String(p).trim();
  const pc = /^(\d+(?:\.\d+)?)\s*%?$/.exec(s);
  if (pc) { const n = clamp(Number(pc[1])); return { pct: n, text: `${n}%` }; }
  const frac = /^(\d+)\s*\/\s*(\d+)$/.exec(s);
  if (frac && Number(frac[2]) > 0) return { pct: clamp((Number(frac[1]) / Number(frac[2])) * 100), text: s };
  return { pct: null, text: s };
}

function clamp(n: number) { return Math.max(0, Math.min(100, Math.round(n))); }

export const canCancel = (j: Pick<CodeJob, 'status'>) => j.status === 'pending' || j.status === 'running';
export const canRetry = (j: Pick<CodeJob, 'status'>) => j.status === 'failed' || j.status === 'cancelled';
export const isActive = (j: Pick<CodeJob, 'status'>) => j.status === 'pending' || j.status === 'running';

/** GET /code/jobs query (empty filters left out). */
export function jobsQuery(status: string, name: string, page: number, perPage = JOBS_PER_PAGE): Record<string, string | number | undefined> {
  return { status: status || undefined, name: name.trim() || undefined, page: Math.max(1, page), perPage };
}

/** "850 ms", "4.2 s", "3 m 05 s", "2 h 10 m". */
export function formatDuration(from?: string | null, to?: string | null): string {
  if (!from || !to) return '';
  const ms = Date.parse(to) - Date.parse(from);
  if (!Number.isFinite(ms) || ms < 0) return '';
  if (ms < 1000) return `${ms} ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(1)} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} m ${String(Math.floor(s % 60)).padStart(2, '0')} s`;
  return `${Math.floor(m / 60)} h ${String(m % 60).padStart(2, '0')} m`;
}

/** Job data / result as readable text (JSON pretty-printed when it parses). */
export function prettyValue(v: unknown): string {
  if (v === null || v === undefined || v === '') return '';
  if (typeof v === 'string') {
    try { return JSON.stringify(JSON.parse(v), null, 2); } catch { return v; }
  }
  try { return JSON.stringify(v, null, 2); } catch { return String(v); }
}
