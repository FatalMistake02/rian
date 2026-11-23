'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const root = path.resolve(__dirname, '..');
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'rian-bun-'));
const env = { ...process.env, BUN_INSTALL: path.join(tmp, 'bun'), BUN_INSTALL_BIN: path.join(tmp, 'bun', 'bin'), BUN_INSTALL_GLOBAL_DIR: path.join(tmp, 'bun', 'install', 'global'), BUN_INSTALL_CACHE_DIR: path.join(tmp, 'cache') };
function run(file, args, options = {}) {
  const r = spawnSync(file, args, { cwd: tmp, env, encoding: 'utf8', ...options });
  if (r.error || r.status !== 0) throw r.error || new Error(`${file}: ${r.stderr || r.stdout}`);
  return r.stdout;
}
try {
  run('bun', ['pm', 'pack', '--ignore-scripts', '--destination', tmp], { cwd: root });
  const tarball = path.join(tmp, fs.readdirSync(tmp).find(name => name.endsWith('.tgz')));
  run('bun', ['add', '-g', '--ignore-scripts', tarball]);
  const dir = run('bun', ['pm', 'bin', '-g']).trim();
  env.PATH = dir + path.delimiter + env.PATH;
  const expected = `rian ${require('../package.json').version}`;
  if (run('bun', ['run', '--bun', 'rian', '--version']).trim() !== expected) throw new Error('Bun launcher version mismatch');
  // Exercise install ownership through the same public command without changing it.
  const info = JSON.parse(run('bun', ['run', '--bun', 'rian', '--installation']));
  if (info.method !== 'bun') throw new Error(`Bun installation was detected as ${info.method}`);
  console.log('Bun global installation, native launcher, and update ownership verified.');
} finally { fs.rmSync(tmp, { recursive: true, force: true }); }
