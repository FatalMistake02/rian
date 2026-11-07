'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { readGoLicense } = require('../../scripts/build-binaries.cjs');

test('Go license collection supports standard and distribution toolchains', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'rian-license-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const standard = path.join(root, 'LICENSE');
  const distribution = path.join(root, 'copyright');
  fs.writeFileSync(standard, 'Standard toolchain license\n');
  fs.writeFileSync(distribution, 'Distribution toolchain license\n');
  assert.equal(readGoLicense(root, ''), 'Standard toolchain license\n');
  assert.equal(readGoLicense(path.join(root, 'missing-goroot'), distribution), 'Distribution toolchain license\n');
  assert.throws(() => readGoLicense(path.join(root, 'missing-goroot'), ''), /Set RIAN_GO_LICENSE_FILE/);
  assert.throws(() => readGoLicense(root, path.join(root, 'missing-override')), /Set RIAN_GO_LICENSE_FILE/);
  fs.writeFileSync(distribution, '');
  assert.throws(() => readGoLicense(root, distribution), /license is empty/);
});

function debianToolchain(t) {
  const prefix = fs.mkdtempSync(path.join(os.tmpdir(), 'rian-debian-go-'));
  t.after(() => fs.rmSync(prefix, { recursive: true, force: true }));
  const goroot = path.join(prefix, 'lib', 'go-1.26');
  const copyright = path.join(prefix, 'share', 'doc', 'golang-1.26-go', 'copyright');
  fs.mkdirSync(goroot, { recursive: true });
  fs.mkdirSync(path.dirname(copyright), { recursive: true });
  fs.writeFileSync(copyright, 'Debian Go runtime license\n');
  return { prefix, goroot };
}

test('Go license collection automatically finds the versioned Debian notice', t => {
  const { goroot } = debianToolchain(t);
  assert.equal(readGoLicense(goroot, ''), 'Debian Go runtime license\n');
  fs.writeFileSync(path.join(goroot, 'LICENSE'), 'Standard toolchain license\n');
  assert.equal(readGoLicense(goroot, ''), 'Standard toolchain license\n');
});

test('Go license collection resolves symlinked Debian toolchain roots', t => {
  const { prefix, goroot } = debianToolchain(t);
  const alias = path.join(prefix, 'lib', 'go');
  fs.symlinkSync(goroot, alias, process.platform === 'win32' ? 'junction' : 'dir');
  assert.equal(readGoLicense(alias, ''), 'Debian Go runtime license\n');
});

test('Go license collection does not hide empty or unreadable standard notices', t => {
  const { goroot } = debianToolchain(t);
  const standard = path.join(goroot, 'LICENSE');
  fs.writeFileSync(standard, '');
  assert.throws(() => readGoLicense(goroot, ''), /license is empty/);
  fs.rmSync(standard);
  fs.mkdirSync(standard);
  assert.throws(() => readGoLicense(goroot, ''), /Cannot read Go runtime license/);
});

test('Go license collection does not use the Debian metapackage notice', t => {
  const { prefix } = debianToolchain(t);
  const goroot = path.join(prefix, 'lib', 'go');
  fs.mkdirSync(goroot);
  const copyright = path.join(prefix, 'share', 'doc', 'golang-go', 'copyright');
  fs.mkdirSync(path.dirname(copyright), { recursive: true });
  fs.writeFileSync(copyright, 'Metapackage license only\n');
  assert.throws(() => readGoLicense(goroot, ''), /Set RIAN_GO_LICENSE_FILE/);
});
