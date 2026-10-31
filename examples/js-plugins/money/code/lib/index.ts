// Library of the money plugin: `import { baht } from 'money'` bundles this
// file into the importer (it runs with the importer's scopes).
export function baht(n: number): string {
  return '฿' + Number(n).toFixed(2).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

export function round2(n: number): number {
  return Math.round(Number(n) * 100) / 100;
}
