'use strict';

const fs = require('node:fs');
const path = require('node:path');

function setReleaseVersion(tag, root = path.resolve(__dirname, '..')) {
  const match = /^v((?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*))$/.exec(tag || '');
  if (!match) throw new Error('Release tags must be stable versions such as v0.2.2');
  const version = match[1];
  const packagePath = path.join(root, 'package.json');
  const mainPath = path.join(root, 'cmd/rian/main.go');
  const pkg = JSON.parse(fs.readFileSync(packagePath, 'utf8'));
  const main = fs.readFileSync(mainPath, 'utf8');
  if (!/^var version = "[^"]+"$/m.test(main)) throw new Error('Cannot find the Go version declaration');
  pkg.version = version;
  fs.writeFileSync(packagePath, JSON.stringify(pkg, null, 2) + '\n');
  fs.writeFileSync(mainPath, main.replace(/^var version = "[^"]+"$/m, `var version = "${version}"`));
  return version;
}

if (require.main === module) {
  try { console.log(setReleaseVersion(process.argv[2])); }
  catch (err) { console.error(`rian release: ${err.message}`); process.exitCode = 1; }
}
module.exports = { setReleaseVersion };
