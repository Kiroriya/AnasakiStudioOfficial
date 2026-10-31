import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';

export interface ChartSeries { key: string; label: string; color: string; values: number[] }

/**
 * Dependency-free SVG line chart: one y-axis, 2px lines, a soft area under the
 * first series, crosshair + tooltip listing every series, legend and a table view.
 */
@Component({
  selector: 'rea-line-chart',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="lc">
      <div class="lc-legend">
        @if (series().length > 1) {
          @for (s of series(); track s.key) { <span><i [style.background]="s.color"></i>{{ s.label }}</span> }
        }
        <span class="grow"></span>
        <button class="link small" (click)="table.set(!table())">{{ table() ? chartLabel() : tableLabel() }}</button>
      </div>
      @if (table()) {
        <div class="lc-table">
          <table class="table compact-table">
            <thead><tr><th>{{ dateLabel() }}</th>@for (s of series(); track s.key) { <th class="num">{{ s.label }}</th> }</tr></thead>
            <tbody>
              @for (l of labels(); track $index; let i = $index) {
                <tr><td>{{ l }}</td>@for (s of series(); track s.key) { <td class="num">{{ fmt(s.values[i]) }}</td> }</tr>
              }
            </tbody>
          </table>
        </div>
      } @else {
        <div class="lc-plot" (pointerleave)="hover.set(-1)"><div class="lc-inner">
          <svg [attr.viewBox]="'0 0 ' + W + ' ' + height()" preserveAspectRatio="none" (pointermove)="move($event)" role="img" [attr.aria-label]="series()[0]?.label">
            @for (t of ticks(); track t.v) {
              <line class="lc-grid" [attr.x1]="P.l" [attr.x2]="W - P.r" [attr.y1]="t.y" [attr.y2]="t.y" />
            }
            @if (series()[0]; as s0) { <path class="lc-area" [attr.d]="area(s0.values)" [attr.fill]="s0.color" /> }
            @for (s of series(); track s.key) {
              <path class="lc-line" [attr.d]="line(s.values)" [attr.stroke]="s.color" vector-effect="non-scaling-stroke" />
            }
            @if (hover() >= 0) {
              <line class="lc-cross" [attr.x1]="x(hover())" [attr.x2]="x(hover())" [attr.y1]="P.t" [attr.y2]="height() - P.b" vector-effect="non-scaling-stroke" />
            }
          </svg>
          <!-- dots + labels in HTML so they don't stretch with the SVG -->
          @if (hover() >= 0) {
            @for (s of series(); track s.key) {
              <span class="lc-dot" [style.left.%]="pctX(hover())" [style.top.px]="y(s.values[hover()])" [style.border-color]="s.color"></span>
            }
            <div class="lc-tip" [class.left]="pctX(hover()) > 60" [style.left.%]="pctX(hover())">
              <div class="lc-tip-date">{{ labels()[hover()] }}</div>
              @for (s of series(); track s.key) {
                <div class="lc-tip-row"><i [style.background]="s.color"></i><b>{{ fmt(s.values[hover()]) }}</b><span>{{ s.label }}</span></div>
              }
            </div>
          }
          @for (t of ticks(); track t.v) { <span class="lc-ytick" [style.top.px]="t.y">{{ fmt(t.v) }}</span> }
          <div class="lc-xticks">
            @for (i of xTicks(); track i) { <span [style.left.%]="pctX(i)">{{ labels()[i] }}</span> }
          </div>
        </div></div>
      }
    </div>
  `,
  styles: `
    .lc-legend { display: flex; align-items: center; gap: 16px; margin-bottom: 8px; font-size: 12.5px; color: var(--muted);
      span { display: inline-flex; align-items: center; gap: 6px; } i { width: 14px; height: 2px; border-radius: 1px; display: inline-block; } }
    .lc-plot { position: relative; padding-left: 40px; }
    svg { display: block; width: 100%; height: var(--h, 240px); overflow: visible; cursor: crosshair; }
    .lc-grid { stroke: #eef0f4; stroke-width: 1; vector-effect: non-scaling-stroke; }
    .lc-line { fill: none; stroke-width: 2; stroke-linejoin: round; stroke-linecap: round; }
    .lc-area { opacity: .08; }
    .lc-cross { stroke: #9aa0b0; stroke-width: 1; stroke-dasharray: 3 3; }
    .lc-inner { position: relative; }
    .lc-ytick { position: absolute; left: -40px; width: 34px; text-align: right; transform: translateY(-50%); font-size: 11px; color: var(--muted); font-variant-numeric: tabular-nums; }
    .lc-xticks { position: relative; height: 20px; margin-top: 4px; span { position: absolute; transform: translateX(-50%); font-size: 11px; color: var(--muted); white-space: nowrap; } }
    .lc-dot { position: absolute; width: 10px; height: 10px; margin: -5px 0 0 -5px; border-radius: 50%; background: #fff; border: 2px solid; pointer-events: none; box-shadow: 0 0 0 2px #fff; }
    .lc-tip { position: absolute; top: 0; margin-left: 14px; min-width: 140px; padding: 8px 12px; background: #fff; border: 1px solid var(--border); border-radius: 10px;
      box-shadow: var(--shadow-lg); pointer-events: none; font-size: 12.5px; z-index: 2;
      &.left { transform: translateX(calc(-100% - 28px)); } }
    .lc-tip-date { color: var(--muted); font-size: 11.5px; margin-bottom: 4px; }
    .lc-tip-row { display: flex; align-items: center; gap: 8px; i { width: 12px; height: 2px; border-radius: 1px; } b { font-variant-numeric: tabular-nums; } span { color: var(--muted); } }
    .lc-table { max-height: 280px; overflow: auto; }
    .compact-table td, .compact-table th { padding: 6px 12px; height: auto; }
  `,
  host: { '[style.--h.px]': 'height()' },
})
export class LineChart {
  readonly series = input<ChartSeries[]>([]);
  readonly labels = input<string[]>([]);
  readonly height = input(240);
  readonly locale = input('th-TH');
  readonly tableLabel = input('Table');
  readonly chartLabel = input('Chart');
  readonly dateLabel = input('Date');

  readonly W = 1000;
  readonly P = { t: 8, r: 0, b: 4, l: 0 };
  readonly hover = signal(-1);
  readonly table = signal(false);

  private readonly max = computed(() => {
    const m = Math.max(1, ...this.series().flatMap(s => s.values));
    const step = niceStep(m / 4);
    return Math.ceil(m / step) * step;
  });

  readonly ticks = computed(() => {
    const max = this.max();
    const step = niceStep(max / 4);
    const out: { v: number; y: number }[] = [];
    for (let v = 0; v <= max + 1e-9; v += step) out.push({ v, y: this.y(v) });
    return out;
  });

  readonly xTicks = computed(() => {
    const n = this.labels().length;
    if (n <= 1) return n ? [0] : [];
    const want = Math.min(n, 6);
    const out = new Set<number>();
    for (let k = 0; k < want; k++) out.add(Math.round((k * (n - 1)) / (want - 1)));
    return [...out];
  });

  private count() { return Math.max(1, this.labels().length); }
  x(i: number) { const n = this.count(); return n === 1 ? this.W / 2 : this.P.l + (i * (this.W - this.P.l - this.P.r)) / (n - 1); }
  pctX(i: number) { return (this.x(i) / this.W) * 100; }
  y(v: number) { const h = this.height(); return this.P.t + (1 - (v || 0) / this.max()) * (h - this.P.t - this.P.b); }

  line(vs: number[]) { return vs.map((v, i) => `${i ? 'L' : 'M'}${this.x(i).toFixed(1)},${this.y(v).toFixed(1)}`).join(''); }
  area(vs: number[]) {
    if (!vs.length) return '';
    const base = this.y(0);
    return `${this.line(vs)}L${this.x(vs.length - 1).toFixed(1)},${base}L${this.x(0).toFixed(1)},${base}Z`;
  }

  move(e: PointerEvent) {
    const r = (e.currentTarget as SVGElement).getBoundingClientRect();
    const n = this.count();
    const i = n === 1 ? 0 : Math.round(((e.clientX - r.left) / r.width) * (n - 1));
    this.hover.set(Math.max(0, Math.min(n - 1, i)));
  }

  fmt(v: number) { return (v ?? 0).toLocaleString(this.locale()); }
}

function niceStep(raw: number) {
  if (raw <= 1) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(raw)));
  const f = raw / p;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * p;
}
