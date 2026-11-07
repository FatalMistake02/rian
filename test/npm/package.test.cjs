'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { checkPackage } = require('../../scripts/check-package.cjs');
const pkg = require('../../package.json');

function packageFixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'rian-package-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const file of ['package.json', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'bin/rian.cjs', 'lib/launcher.cjs', 'lib/platforms.cjs', 'scripts/ensure-binary.cjs']) {
    const destination = path.join(root, file);
    fs.mkdirSync(path.dirname(destination), { recursive: true });
    fs.copyFileSync(path.resolve(__dirname, '../..', file), destination);
  }
  return root;
}

test('package verification uses the inspected package version', t => {
  const root = packageFixture(t);
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ ...pkg, version: '99.0.0' }));
  fs.mkdirSync(path.join(root, 'dist', 'binaries'), { recursive: true });
  fs.writeFileSync(path.join(root, 'dist', 'binaries', 'manifest.json'), JSON.stringify({ version: pkg.version }));
  assert.throws(() => checkPackage(root), /Binary version is stale/);
});

test('package verification rejects missing launcher dependencies', t => {
  const root = packageFixture(t);
  fs.rmSync(path.join(root, 'lib', 'platforms.cjs'));
  assert.throws(() => checkPackage(root), /Missing lib\/platforms.cjs/);
});

test('package verification rejects a corrupt PE header with an actionable error', t => {
  const root = packageFixture(t);
  const binaries = path.join(root, 'dist', 'binaries');
  fs.cpSync(path.resolve(__dirname, '../../dist/binaries'), binaries, { recursive: true });
  const manifestPath = path.join(binaries, 'manifest.json');
  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
  const target = manifest.targets['win32-arm64'];
  const data = fs.readFileSync(path.join(binaries, target.file));
  data.writeUInt32LE(0xffffffff, 0x3c);
  fs.writeFileSync(path.join(binaries, target.file), data);
  target.sha256 = crypto.createHash('sha256').update(data).digest('hex');
  fs.writeFileSync(manifestPath, JSON.stringify(manifest));
  assert.throws(() => checkPackage(root), /Wrong executable format or architecture for win32-arm64/);
});

test('built package includes verified native executables for every supported platform', () => {
  const manifest = checkPackage();
  assert.equal(manifest.version, pkg.version);
  assert.equal(Object.keys(manifest.targets).length, 6);
  assert.equal(pkg.bin.rian, 'bin/rian.cjs');
});

test('npm launcher runs the native executable and returns errors cleanly', () => {
  const root = path.resolve(__dirname, '../..');
  const launcher = path.join(root, 'bin', 'rian.cjs');
  const version = spawnSync(process.execPath, [launcher, '--version'], { encoding: 'utf8', cwd: path.dirname(root), windowsHide: true });
  assert.equal(version.status, 0, version.stderr);
  assert.equal(version.stdout.trim(), `rian ${pkg.version}`);
  const invalid = spawnSync(process.execPath, [launcher, 'invalid-subcommand'], { encoding: 'utf8', windowsHide: true });
  assert.equal(invalid.status, 1);
  assert.match(invalid.stderr, /unknown command/);
});
