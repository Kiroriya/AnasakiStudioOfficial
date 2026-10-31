// Entry point so that `node --test cli/test` works on every Node version
// (Node 22+ treats the argument as a path to run, Node 20 as a folder to scan).
import './ignore.test.mjs';
import './sync.test.mjs';
import './commands.test.mjs';
import './watch.test.mjs';
