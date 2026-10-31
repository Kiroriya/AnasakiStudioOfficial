#!/usr/bin/env node
// anasaki-code — sync Anasaki Studio Site Code with a local folder.
import { main } from '../lib/cli.mjs';

const ac = new AbortController();
let interrupted = false;
process.on('SIGINT', () => {
  if (interrupted) process.exit(130);
  interrupted = true;
  ac.abort();
});

main(process.argv.slice(2), { signal: ac.signal }).then(
  (code) => { process.exitCode = code; },
  (err) => {
    process.stderr.write(`anasaki-code: ${err && err.stack ? err.stack : err}\n`);
    process.exitCode = 1;
  },
);
