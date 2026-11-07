'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { EventEmitter } = require('node:events');
const { targets, targetFor } = require('../../lib/platforms.cjs');
const { binaryPath, launch } = require('../../lib/launcher.cjs');

test('dispatches all six platform/architecture combinations', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'rian-launcher-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const item of targets) {
    const target = targetFor(item.platform, item.arch);
    const file = path.join(root, 'dist', 'binaries', target.key, target.filename);
    fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, 'fixture');
    assert.equal(binaryPath(root, item.platform, item.arch), file);
  }
  assert.throws(() => binaryPath(root, 'freebsd', 'x64'), /does not support/);
  assert.throws(() => binaryPath(root, 'darwin', 'ia32'), /does not support/);
});

test('missing binaries give an actionable install error', () => {
  assert.throws(() => binaryPath(path.join(os.tmpdir(), 'rian-missing-install')), /npm install -g @fatalmistake02\/rian/);
});

test('a directory in place of the executable gives an actionable install error', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'rian-launcher-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const target = targetFor();
  fs.mkdirSync(path.join(root, 'dist', 'binaries', target.key, target.filename), { recursive: true });
  assert.throws(() => binaryPath(root), /npm install -g @fatalmistake02\/rian/);
});

test('preserves arguments, terminal stdio, working directory, and child exit status', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'rian-launcher-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const target = targetFor();
  const file = path.join(root, 'dist', 'binaries', target.key, target.filename);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, 'fixture');
  const child = new EventEmitter(); child.exitCode = null; child.signalCode = null;
  const oldCode = process.exitCode;
  const oldHandlerCount = process.listenerCount('SIGTERM');
  let invocation;
  const args = ['add', '/directory with spaces', '--yes'];
  launch(root, args, (file, passed, options) => { invocation = { file, passed, options }; return child; });
  assert.deepEqual(invocation.passed, args);
  assert.equal(invocation.options.stdio, 'inherit');
  assert.equal(invocation.options.shell, false);
  assert.equal(invocation.options.cwd, undefined);
  child.emit('exit', 7, null);
  assert.equal(process.exitCode, 7);
  assert.equal(process.listenerCount('SIGTERM'), oldHandlerCount);
  process.exitCode = oldCode;
});
