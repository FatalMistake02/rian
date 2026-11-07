'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { setReleaseVersion } = require('../../scripts/release-version.cjs');

test('tag sets matching npm and Go versions without executing lifecycle scripts', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'rian-release-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.mkdirSync(path.join(root, 'cmd/rian'), { recursive: true });
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ name: 'rian', version: '0.2.1' }));
  fs.writeFileSync(path.join(root, 'cmd/rian/main.go'), 'package main\nvar version = "0.2.1"\n');
  assert.equal(setReleaseVersion('v0.3.0', root), '0.3.0');
  assert.equal(JSON.parse(fs.readFileSync(path.join(root, 'package.json'))).version, '0.3.0');
  assert.match(fs.readFileSync(path.join(root, 'cmd/rian/main.go'), 'utf8'), /var version = "0.3.0"/);
  for (const invalid of ['0.3.0', 'v01.0.0', 'v1.0', 'v1.0.0-beta', 'v1.0.0\n', 'v1.0.0;echo bad']) {
    assert.throws(() => setReleaseVersion(invalid, root), /Release tags/);
  }
});
