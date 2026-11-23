'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { formula } = require('../../scripts/package-release.cjs');

test('Homebrew formula selects the matching immutable archive and checksum', () => {
  const sums = {};
  for (const platform of ['darwin', 'linux']) for (const arch of ['amd64', 'arm64']) {
    sums[`rian_1.2.3_${platform}_${arch}.tar.gz`] = `${platform}-${arch}`;
  }
  const text = formula('1.2.3', sums);
  for (const [name, hash] of Object.entries(sums)) {
    assert.ok(text.includes(`/v1.2.3/${name}"\n      sha256 "${hash}"`));
  }
  assert.ok(!text.includes('undefined'));
  assert.ok(text.includes('bin.install "rian"'));
  assert.ok(text.includes('prefix.install "LICENSE", "THIRD_PARTY_NOTICES.txt"'));
});
