'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { targets, targetFor } = require('../../lib/platforms.cjs');
const root = path.resolve(__dirname, '../..');
const { version } = require('../../package.json');

test('all six standalone archives preserve their verified binaries and licenses', { skip: process.platform === 'win32' }, () => {
  const release = path.join(root, 'dist', 'release');
  const sums = fs.readFileSync(path.join(release, 'SHA256SUMS'), 'utf8');
  for (const item of targets) {
    const target = targetFor(item.platform, item.arch);
    const zip = item.platform === 'win32';
    const name = `rian_${version}_${item.goos}_${item.goarch}.${zip ? 'zip' : 'tar.gz'}`;
    const file = path.join(release, name);
    const hash = crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
    assert.ok(sums.includes(`${hash}  ${name}\n`));
    const listing = spawnSync(zip ? 'unzip' : 'tar', zip ? ['-Z1', file] : ['-tzf', file], { encoding: 'utf8' });
    assert.equal(listing.status, 0, listing.stderr);
    assert.deepEqual(listing.stdout.trim().split('\n').sort(), [target.filename, 'LICENSE', 'THIRD_PARTY_NOTICES.txt'].sort());
    const binary = spawnSync(zip ? 'unzip' : 'tar', zip ? ['-p', file, target.filename] : ['-xOzf', file, target.filename], { maxBuffer: 64 * 1024 * 1024 });
    assert.equal(binary.status, 0, String(binary.stderr));
    assert.deepEqual(binary.stdout, fs.readFileSync(path.join(root, 'dist', 'binaries', target.key, target.filename)));
  }
});

test('curl installer verifies archives, installs without Node, and preserves other installs', { skip: process.platform === 'win32' }, t => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rian curl '));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const tools = path.join(dir, 'tools');
  fs.mkdirSync(tools);
  const fixture = path.join(root, 'dist', 'release');
  fs.writeFileSync(path.join(tools, 'curl'), `#!${process.execPath}\nconst fs=require('node:fs'),path=require('node:path');const args=process.argv.slice(2);const url=args.find(x=>x.startsWith('https://'));const source=path.join(${JSON.stringify(fixture)},url.split('/').pop());const dest=args[args.indexOf('-o')+1];if(args.includes('-o'))fs.copyFileSync(source,dest);else process.stdout.write(fs.readFileSync(source));\n`, { mode: 0o755 });
  const install = path.join(dir, 'install');
  const bin = path.join(dir, 'bin');
  const env = { ...process.env, RIAN_INSTALL_DIR: install, RIAN_BIN_DIR: bin, PATH: tools + path.delimiter + process.env.PATH };
  const run = () => spawnSync('sh', [path.join(root, 'scripts', 'install.sh')], { env, encoding: 'utf8' });
  let r = run();
  assert.equal(r.status, 0, r.stderr);
  assert.equal(fs.readlinkSync(path.join(bin, 'rian')), path.join(install, 'rian'));
  r = spawnSync(path.join(bin, 'rian'), ['--version'], { env: { ...env, PATH: '/no-node-or-bun' }, encoding: 'utf8' });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.stdout.trim(), `rian ${version}`);
  const original = fs.readFileSync(path.join(install, 'rian'));
  // Reinstalling a running/previously installed standalone copy is supported.
  assert.equal(run().status, 0);
  fs.unlinkSync(path.join(bin, 'rian'));
  fs.writeFileSync(path.join(bin, 'rian'), 'another installation');
  r = run();
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /another installation/);
  assert.equal(fs.readFileSync(path.join(bin, 'rian'), 'utf8'), 'another installation');
  fs.unlinkSync(path.join(bin, 'rian'));
  fs.symlinkSync(path.join(install, 'rian'), path.join(bin, 'rian'));
  // Replace the transport fixture with one that returns a mismatched checksum.
  fs.writeFileSync(path.join(tools, 'curl'), `#!${process.execPath}\nconst fs=require('node:fs'),path=require('node:path');const a=process.argv.slice(2),u=a.find(x=>x.startsWith('https://')),s=path.join(${JSON.stringify(fixture)},u.split('/').pop());const b=u.endsWith('.tar.gz')?Buffer.from('corrupt'):fs.readFileSync(s);if(a.includes('-o'))fs.writeFileSync(a[a.indexOf('-o')+1],b);else process.stdout.write(b);\n`);
  r = run();
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /checksum mismatch/);
  assert.deepEqual(fs.readFileSync(path.join(install, 'rian')), original);
});
