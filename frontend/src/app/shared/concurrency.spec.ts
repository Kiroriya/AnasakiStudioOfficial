import { mapLimit } from './concurrency';

describe('mapLimit', () => {
  it('keeps order, never exceeds the limit and continues past failures', async () => {
    let running = 0;
    let peak = 0;
    const res = await mapLimit([1, 2, 3, 4, 5, 6, 7], 3, async n => {
      running++;
      peak = Math.max(peak, running);
      await new Promise(r => setTimeout(r, 5 * (8 - n)));
      running--;
      if (n % 3 === 0) throw new Error('bad ' + n);
      return n * 10;
    });
    expect(peak).toBe(3);
    expect(res.map(r => (r.status === 'fulfilled' ? r.value : (r.reason as Error).message)))
      .toEqual([10, 20, 'bad 3', 40, 50, 'bad 6', 70]);
  });

  it('handles an empty list', async () => {
    expect(await mapLimit([], 3, async () => 1)).toEqual([]);
  });
});
