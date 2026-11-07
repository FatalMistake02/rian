'use strict';

const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { targets, targetFor } = require('../lib/platforms.cjs');
const { binaryPath } = require('../lib/launcher.cjs');

const root = path.resolve(__dirname, '..');

function verifyArchitecture(data, target) {
  let valid;
  if (target.platform === 'darwin') {
    valid = data.readUInt32LE(0) === 0xfeedfacf &&
      data.readUInt32LE(4) === (target.arch === 'arm64' ? 0x100000c : 0x1000007);
  } else if (target.platform === 'linux') {
    valid = data.subarray(0, 4).toString('hex') === '7f454c46' && data[4] === 2 && data[5] === 1 &&
      data.readUInt16LE(18) === (target.arch === 'arm64' ? 183 : 62);
  } else {
    const header = data.readUInt32LE(0x3c);
    valid = data.subarray(0, 2).toString() === 'MZ' && header <= data.length - 6 &&
      data.readUInt32LE(header) === 0x4550 &&
      data.readUInt16LE(header + 4) === (target.arch === 'arm64' ? 0xaa64 : 0x8664);
  }
  if (!valid) throw new Error(`Wrong executable format or architecture for ${target.key}`);
}

function checkPackage(packageRoot = root) {
  const pkg = JSON.parse(fs.readFileSync(path.join(packageRoot, 'package.json'), 'utf8'));
  for (const file of ['LICENSE', 'THIRD_PARTY_NOTICES.txt', 'bin/rian.cjs', 'lib/launcher.cjs', 'lib/platforms.cjs', 'scripts/ensure-binary.cjs']) {
    const fullPath = path.join(packageRoot, file);
    if (!fs.existsSync(fullPath) || !fs.statSync(fullPath).isFile()) throw new Error(`Missing ${file}`);
  }
  const assetRoot = path.join(packageRoot, 'dist', 'binaries');
  const manifest = JSON.parse(fs.readFileSync(path.join(assetRoot, 'manifest.json'), 'utf8'));
  if (manifest.version !== pkg.version) throw new Error('Binary version is stale; run npm run build');
  if (Object.keys(manifest.targets).length !== targets.length) throw new Error('The binary manifest must contain all six platforms');
  for (const item of targets) {
    const target = targetFor(item.platform, item.arch);
    const expected = `${target.key}/${target.filename}`;
    const info = manifest.targets[target.key];
    if (!info || info.file !== expected) throw new Error(`Missing ${target.key} executable`);
    const data = fs.readFileSync(path.join(assetRoot, expected));
    if (data.length < 1024) throw new Error(`${target.key} is not a built executable`);
    const actual = crypto.createHash('sha256').update(data).digest('hex');
    if (actual !== info.sha256) throw new Error(`${target.key} checksum does not match`);
    verifyArchitecture(data, target);
  }
  const result = spawnSync(binaryPath(packageRoot), ['--version'], { encoding: 'utf8', windowsHide: true });
  if (result.error || result.status !== 0 || result.stdout.trim() !== `rian ${pkg.version}`) throw new Error('The current-platform executable failed the version check');
  return manifest;
}

if (require.main === module) {
  try {
    const manifest = checkPackage(process.argv[2] && path.resolve(process.argv[2]));
    console.log(`Rian ${manifest.version}: all six binaries, checksums, architectures, and package files verified.`);
  }
  catch (err) { console.error(`rian package: ${err.message}`); process.exitCode = 1; }
}

module.exports = { checkPackage };
