'use strict';

const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { targets, targetFor } = require('../lib/platforms.cjs');

const root = path.resolve(__dirname, '..');
const pkg = require('../package.json');
const output = path.join(root, 'dist', 'binaries');

function runGo(args, env = {}, capture = false) {
  const result = spawnSync('go', args, { cwd: root, env: { ...process.env, ...env },
    encoding: 'utf8', stdio: capture ? 'pipe' : 'inherit', maxBuffer: 10 * 1024 * 1024 });
  if (result.error || result.status !== 0) {
    throw new Error(result.error?.message || result.stderr?.trim() || `go ${args[0]} failed (${result.status})`);
  }
  return result.stdout || '';
}

function readGoLicense(goroot, override = process.env.RIAN_GO_LICENSE_FILE) {
  const files = [path.resolve(override || path.join(goroot, 'LICENSE'))];
  if (!override) {
    let resolvedRoot = path.resolve(goroot);
    try { resolvedRoot = fs.realpathSync(resolvedRoot); } catch { /* Report missing or unreadable paths below. */ }
    const version = /^go-(\d+\.\d+)$/.exec(path.basename(resolvedRoot));
    if (version && path.basename(path.dirname(resolvedRoot)) === 'lib') {
      files.push(path.resolve(resolvedRoot, '..', '..', 'share', 'doc', `golang-${version[1]}-go`, 'copyright'));
    }
  }
  for (const file of files) {
    let license;
    try {
      license = fs.readFileSync(file, 'utf8');
    } catch (err) {
      if (!override && (err.code === 'ENOENT' || err.code === 'ENOTDIR')) continue;
      throw new Error(`Cannot read Go runtime license at ${file}. Set RIAN_GO_LICENSE_FILE to your toolchain's license file (${err.code}).`);
    }
    if (!license.trim()) throw new Error(`Go runtime license is empty: ${file}`);
    return license;
  }
  throw new Error(`Cannot find Go runtime license (checked ${files.join(', ')}). Set RIAN_GO_LICENSE_FILE to your toolchain's license file.`);
}

function collectLicenses() {
  const goroot = runGo(['env', 'GOROOT'], {}, true).trim();
  const parts = ['Rian bundles the Go runtime and the following third-party modules.\nTheir original copyright and license notices are reproduced below.\n',
    `Go runtime\n${runGo(['version'], {}, true).trim()}\n\n${readGoLicense(goroot)}`];
  const modules = runGo(['list', '-m', '-f', '{{.Path}}|{{.Version}}|{{.Dir}}', 'all'], {}, true).trim().split(/\r?\n/);
  for (const entry of modules) {
    const [name, version, dir] = entry.split('|');
    if (!version || !dir) continue;
    const files = fs.readdirSync(dir).filter(name => /^(LICENSE|COPYING|NOTICE)([.-].*)?$/i.test(name)).sort();
    if (!files.length) throw new Error(`Missing license notice for ${name}@${version}`);
    for (const file of files) {
      if (!fs.statSync(path.join(dir, file)).isFile()) continue;
      parts.push(`${name}@${version} — ${file}\n\n${fs.readFileSync(path.join(dir, file), 'utf8')}`);
    }
  }
  return parts.join('\n\n' + '='.repeat(72) + '\n\n');
}

function build() {
  if (!/^\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?$/.test(pkg.version)) throw new Error('Invalid package version');
  const notices = collectLicenses();
  fs.rmSync(output, { recursive: true, force: true });
  fs.mkdirSync(output, { recursive: true });
  const manifest = { version: pkg.version, targets: {} };
  for (const item of targets) {
    const target = targetFor(item.platform, item.arch);
    const relative = `${target.key}/${target.filename}`;
    const file = path.join(output, relative);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    console.log(`Building Rian ${pkg.version} for ${target.key}…`);
    runGo(['build', '-trimpath', '-buildvcs=false', '-ldflags', `-s -w -X main.version=${pkg.version}`, '-o', file, './cmd/rian'],
      { GOOS: target.goos, GOARCH: target.goarch, CGO_ENABLED: '0' });
    fs.chmodSync(file, 0o755);
    manifest.targets[target.key] = { file: relative, sha256: crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex') };
  }
  fs.writeFileSync(path.join(root, 'THIRD_PARTY_NOTICES.txt'), notices);
  fs.writeFileSync(path.join(output, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
  console.log('All six executables and dependency licenses are ready.');
}

if (require.main === module) {
  try { build(); } catch (err) { console.error(`rian build: ${err.message}`); process.exitCode = 1; }
}

module.exports = { readGoLicense };
