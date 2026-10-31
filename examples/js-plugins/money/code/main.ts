// Runs as the "money" unit. rate() is listed in code.exports: importers get a
// proxy that calls it here, with money's own settings (money.usdRate).
site.export('rate', (from: string, to: string) => {
  if (from === to) return 1;
  const usd = Number(site.settings('usdRate') ?? 35);
  const r = from === 'USD' && to === 'THB' ? usd : from === 'THB' && to === 'USD' ? 1 / usd : NaN;
  return site.applyFilter('money.format', r, { from, to });
});
