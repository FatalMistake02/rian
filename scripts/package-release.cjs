'use strict';

const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { targets, targetFor } = require('../lib/platforms.cjs');
const { checkPackage } = require('./check-package.cjs');

function run(file, args, cwd) {
  const r = spawnSync(file, args, { cwd, stdio: 'inherit' });
  if (r.error || r.status !== 0) throw r.error || new Error(`${file} failed (${r.status})`);
}

function formula(version, sums) {
  const url = `https://github.com/FatalMistake02/rian/releases/download/v${version}`;
  const item = (os, arch, spaces) => {
    const name = `rian_${version}_${os}_${arch}.tar.gz`;
    return `${spaces}url "${url}/${name}"\n${spaces}sha256 "${sums[name]}"`;
  };
  return `class Rian < Formula
  desc "Terminal app for managing development projects, commands, and Git"
  homepage "https://github.com/FatalMistake02/rian"
  version "${version}"
  license "MIT"

  on_macos do
    on_arm do
${item('darwin', 'arm64', '      ')}
    end
    on_intel do
${item('darwin', 'amd64', '      ')}
    end
  end
  on_linux do
    on_arm do
${item('linux', 'arm64', '      ')}
    end
    on_intel do
${item('linux', 'amd64', '      ')}
    end
  end

  def install
    bin.install "rian"
    prefix.install "LICENSE", "THIRD_PARTY_NOTICES.txt"
  end

  test do
    assert_equal "rian #{version}", shell_output("#{bin}/rian --version").strip
  end
end
`;
}

function packageRelease(root = path.resolve(__dirname, '..')) {
  const { version } = checkPackage(root);
  const out = path.join(root, 'dist', 'release');
  fs.rmSync(out, { recursive: true, force: true });
  fs.mkdirSync(out, { recursive: true });
  const sums = {};
  for (const item of targets) {
    const target = targetFor(item.platform, item.arch);
    const stage = fs.mkdtempSync(path.join(root, 'dist', '.archive-'));
    try {
      const names = [target.filename, 'LICENSE', 'THIRD_PARTY_NOTICES.txt'];
      fs.copyFileSync(path.join(root, 'dist', 'binaries', target.key, target.filename), path.join(stage, target.filename));
      fs.chmodSync(path.join(stage, target.filename), 0o755);
      for (const name of names.slice(1)) fs.copyFileSync(path.join(root, name), path.join(stage, name));
      const name = `rian_${version}_${target.goos}_${target.goarch}.${target.platform === 'win32' ? 'zip' : 'tar.gz'}`;
      const dest = path.join(out, name);
      if (target.platform === 'win32') run('zip', ['-q', dest, ...names], stage);
      else run('tar', ['-czf', dest, ...names], stage);
      sums[name] = crypto.createHash('sha256').update(fs.readFileSync(dest)).digest('hex');
    } finally { fs.rmSync(stage, { recursive: true, force: true }); }
  }
  fs.copyFileSync(path.join(root, 'scripts', 'install.sh'), path.join(out, 'install.sh'));
  fs.writeFileSync(path.join(out, 'VERSION'), version + '\n');
  for (const name of ['install.sh', 'VERSION']) sums[name] = crypto.createHash('sha256').update(fs.readFileSync(path.join(out, name))).digest('hex');
  fs.writeFileSync(path.join(out, 'SHA256SUMS'), Object.keys(sums).sort().map(name => `${sums[name]}  ${name}\n`).join(''));
  fs.writeFileSync(path.join(out, 'rian.rb'), formula(version, sums));
  const block = (label, ...commands) => `**${label}**\n\n` + '```sh\n' + commands.join('\n') + '\n```';
  const notes = [
    `Rian ${version} supports macOS, Linux, and Windows on x64 and ARM64.`,
    block('npm', `npm install -g @fatalmistake02/rian@${version}`),
    block('Bun', `bun add -g @fatalmistake02/rian@${version}`, 'bun run --bun rian'),
    block('Standalone installer (macOS/Linux)', `curl -fsSL https://github.com/FatalMistake02/rian/releases/download/v${version}/install.sh | sh -s -- --version ${version}`),
    block('Homebrew', 'brew tap fatalmistake02/rian https://github.com/FatalMistake02/rian', 'brew install fatalmistake02/rian/rian'),
    'For a manual installation, download the archive for your platform below. Each archive contains the executable and licenses. Checksums are listed in SHA256SUMS.',
    `[Source](https://github.com/FatalMistake02/rian/tree/v${version})`,
  ];
  fs.writeFileSync(path.join(out, 'notes.md'), notes.join('\n\n') + '\n');
  return { version, out, sums };
}

if (require.main === module) {
  try { const { version, out } = packageRelease(); console.log(`Rian ${version} standalone archives ready in ${out}`); }
  catch (err) { console.error(`rian release: ${err.message}`); process.exitCode = 1; }
}
module.exports = { packageRelease, formula };
