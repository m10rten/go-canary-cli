'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const { EventEmitter } = require('node:events');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');
const { pathToFileURL } = require('node:url');
const zlib = require('node:zlib');

const installer = require('../../npm/install.js');

const sha256 = (content) => crypto.createHash('sha256').update(content).digest('hex');

function tarEntry(name, body = '', typeflag = '0') {
  const header = Buffer.alloc(512);
  header.write(name, 0, 'utf8');
  header.write('0000755\0', 100);
  header.write(`${Buffer.byteLength(body).toString(8).padStart(11, '0')}\0`, 124);
  header.write('        ', 148);
  header.write(typeflag, 156);
  const sum = header.reduce((total, byte) => total + byte, 0);
  header.write(`${sum.toString(8).padStart(6, '0')}\0 `, 148);
  const data = Buffer.alloc(Math.ceil(Buffer.byteLength(body) / 512) * 512);
  data.write(body);
  return Buffer.concat([header, data]);
}

function withSizeField(entry, field) {
  const copy = Buffer.from(entry);
  copy.write(field, 124, 'latin1');
  return copy;
}

function tarGz(...entries) {
  return zlib.gzipSync(Buffer.concat([...entries, Buffer.alloc(1024)]));
}

function zipFile(...entries) {
  const locals = [];
  const centrals = [];
  let offset = 0;
  for (const { name, content, method = 8, flags = 0 } of entries) {
    const data = method === 8 ? zlib.deflateRawSync(content) : Buffer.from(content);
    const nameBuf = Buffer.from(name);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(flags, 6);
    local.writeUInt16LE(method, 8);
    local.writeUInt16LE(nameBuf.length, 26);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(flags, 8);
    central.writeUInt16LE(method, 10);
    central.writeUInt32LE(data.length, 20);
    central.writeUInt32LE(Buffer.byteLength(content), 24);
    central.writeUInt16LE(nameBuf.length, 28);
    central.writeUInt32LE(offset, 42);
    locals.push(local, nameBuf, data);
    centrals.push(central, nameBuf);
    offset += local.length + nameBuf.length + data.length;
  }
  const directory = Buffer.concat(centrals);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(directory.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, directory, end]);
}

const targets = [
  { platform: 'darwin', arch: 'arm64', want: 'canary_1.2.3_darwin_arm64.tar.gz' },
  { platform: 'darwin', arch: 'x64', want: 'canary_1.2.3_darwin_amd64.tar.gz' },
  { platform: 'linux', arch: 'arm64', want: 'canary_1.2.3_linux_arm64.tar.gz' },
  { platform: 'linux', arch: 'x64', want: 'canary_1.2.3_linux_amd64.tar.gz' },
  { platform: 'win32', arch: 'arm64', want: 'canary_1.2.3_windows_arm64.zip' },
  { platform: 'win32', arch: 'x64', want: 'canary_1.2.3_windows_amd64.zip' },
];

test('archiveName maps every release target', () => {
  for (const { platform, arch, want } of targets) {
    assert.equal(installer.archiveName('1.2.3', platform, arch), want, `${platform}/${arch}`);
  }
  assert.equal(installer.archiveName('1.3.0-beta.1', 'linux', 'x64'), 'canary_1.3.0-beta.1_linux_amd64.tar.gz');
});

test('archiveName rejects unsupported platforms', () => {
  const testcases = [
    ['freebsd', 'x64'],
    ['linux', 'ia32'],
    ['aix', 'ppc64'],
  ];
  for (const [platform, arch] of testcases) {
    assert.throws(() => installer.archiveName('1.2.3', platform, arch), new RegExp(`unsupported platform ${platform}/${arch}`));
  }
});

test('binaryPath keeps the binary inside the package under node_modules layouts', () => {
  const testcases = [
    ['linux', path.join('pkg', 'vendor', 'canary')],
    ['win32', path.join('pkg', 'vendor', 'canary.exe')],
  ];
  for (const [platform, want] of testcases) {
    assert.equal(installer.binaryPath('pkg', platform), want);
  }
});

test('fixCommand quotes the install script path for the platform', () => {
  const script = (dir) => path.join(dir, 'install.js');
  const testcases = [
    { platform: 'linux', dir: '/opt/my pkg', want: `node '${script('/opt/my pkg')}'` },
    { platform: 'linux', dir: "/opt/it's", want: `node '/opt/it'\\''s${path.sep}install.js'` },
    { platform: 'win32', dir: 'C:\\Users\\me\\pkg', want: `node "${script('C:\\Users\\me\\pkg')}"` },
  ];
  for (const tc of testcases) {
    assert.equal(installer.fixCommand(tc.dir, tc.platform), tc.want, `${tc.platform} ${tc.dir}`);
  }
});

const digestA = 'a'.repeat(64);
const digestB = 'b'.repeat(64);

test('lookupChecksum finds the line for a name', () => {
  const testcases = [
    { name: 'text mode', text: `${digestA}  one.tar.gz\n${digestB}  two.tar.gz\n`, want: 'one.tar.gz', digest: digestA },
    { name: 'binary mode', text: `${digestA} *one.tar.gz\n`, want: 'one.tar.gz', digest: digestA },
    { name: 'CRLF', text: `${digestA}  one.tar.gz\r\n${digestB}  two.tar.gz\r\n`, want: 'two.tar.gz', digest: digestB },
    { name: 'uppercase digest', text: `${digestA.toUpperCase()}  one.tar.gz\n`, want: 'one.tar.gz', digest: digestA },
  ];
  for (const tc of testcases) {
    assert.equal(installer.lookupChecksum(tc.text, tc.want), tc.digest, tc.name);
  }
});

test('lookupChecksum rejects a missing, malformed or duplicated line', () => {
  const testcases = [
    { name: 'missing', text: `${digestA}  one.tar.gz\n`, want: 'two.tar.gz', error: /has no line for two\.tar\.gz/ },
    { name: 'empty file', text: '', want: 'one.tar.gz', error: /has no line for one\.tar\.gz/ },
    { name: 'short digest', text: `${digestA.slice(1)}  one.tar.gz\n`, want: 'one.tar.gz', error: /line 1 is malformed/ },
    { name: 'no separator', text: `${digestA}one.tar.gz\n`, want: 'one.tar.gz', error: /line 1 is malformed/ },
    { name: 'malformed other line', text: `${digestA}  one.tar.gz\nnonsense\n`, want: 'one.tar.gz', error: /line 2 is malformed/ },
    { name: 'duplicate', text: `${digestA}  one.tar.gz\n${digestB}  one.tar.gz\n`, want: 'one.tar.gz', error: /duplicate entry for one\.tar\.gz/ },
  ];
  for (const tc of testcases) {
    assert.throws(() => installer.lookupChecksum(tc.text, tc.want), tc.error, tc.name);
  }
});

test('verifyChecksum accepts a match and rejects a mismatch', () => {
  const content = Buffer.from('archive bytes');
  const good = `${sha256(content)}  one.tar.gz\n`;
  installer.verifyChecksum(good, 'one.tar.gz', content);
  assert.throws(() => installer.verifyChecksum(good, 'one.tar.gz', Buffer.from('tampered')), /checksum mismatch for one\.tar\.gz/);
  assert.throws(() => installer.verifyChecksum(good, 'other.tar.gz', content), /has no line for other\.tar\.gz/);
});

test('extractTarGz returns the regular file named canary at the archive root', () => {
  const body = 'binary bytes';
  const testcases = [
    { name: 'plain root entry', archive: tarGz(tarEntry('canary', body)), want: body },
    { name: 'dot slash prefix', archive: tarGz(tarEntry('./canary', body)), want: body },
    { name: 'after a PAX header and other files', archive: tarGz(tarEntry('PaxHeader/canary', 'x', 'x'), tarEntry('README', 'read me'), tarEntry('canary', body)), want: body },
    { name: 'body larger than one block', archive: tarGz(tarEntry('canary', 'z'.repeat(1300))), want: 'z'.repeat(1300) },
    { name: 'nested file is ignored', archive: tarGz(tarEntry('dir/canary', 'nested')), error: /canary not found at the archive root/ },
    { name: 'symlink is not a regular file', archive: tarGz(tarEntry('canary', '', '2')), error: /canary not found at the archive root/ },
    { name: 'empty archive', archive: tarGz(), error: /canary not found at the archive root/ },
    { name: 'truncated entry', archive: zlib.gzipSync(tarEntry('canary', 'z'.repeat(1300)).subarray(0, 700)), error: /archive is truncated/ },
    { name: 'not gzip', archive: Buffer.from('plain text'), error: /incorrect header check/ },
    { name: 'size with a non-octal digit', archive: tarGz(withSizeField(tarEntry('canary', 'x'), '0000000009\0\0')), error: /tar: malformed header/ },
    { name: 'size with trailing garbage', archive: tarGz(withSizeField(tarEntry('canary', 'x'), '00000000001x')), error: /tar: malformed header/ },
    { name: 'size padded with spaces', archive: tarGz(withSizeField(tarEntry('canary', 'x'), '       1 \0\0\0')), want: 'x' },
  ];
  for (const tc of testcases) {
    if (tc.error) {
      assert.throws(() => installer.extractTarGz(tc.archive, 'canary'), tc.error, tc.name);
    } else {
      assert.equal(installer.extractTarGz(tc.archive, 'canary').toString(), tc.want, tc.name);
    }
  }
});

test('extractZip returns canary.exe and rejects unsupported entries', () => {
  const body = 'windows binary bytes '.repeat(50);
  const testcases = [
    { name: 'deflated', archive: zipFile({ name: 'canary.exe', content: body }), want: body },
    { name: 'stored', archive: zipFile({ name: 'canary.exe', content: body, method: 0 }), want: body },
    { name: 'after another entry', archive: zipFile({ name: 'README', content: 'read me' }, { name: 'canary.exe', content: body }), want: body },
    { name: 'data descriptor flag', archive: zipFile({ name: 'canary.exe', content: body, flags: 8 }), want: body },
    { name: 'unsupported method', archive: zipFile({ name: 'canary.exe', content: body, method: 14 }), error: /compression method 14, which is not supported/ },
    { name: 'encrypted', archive: zipFile({ name: 'canary.exe', content: body, flags: 1 }), error: /canary\.exe is encrypted/ },
    { name: 'missing entry', archive: zipFile({ name: 'other.exe', content: body }), error: /canary\.exe not found/ },
    { name: 'not a zip', archive: Buffer.from('plain text that is long enough to scan'), error: /end of central directory not found/ },
    { name: 'truncated', archive: zipFile({ name: 'canary.exe', content: body }).subarray(0, 60), error: /end of central directory not found/ },
  ];
  for (const tc of testcases) {
    if (tc.error) {
      assert.throws(() => installer.extractZip(tc.archive, 'canary.exe'), tc.error, tc.name);
    } else {
      assert.equal(installer.extractZip(tc.archive, 'canary.exe').toString(), tc.want, tc.name);
    }
  }
});

test('releaseBaseUrl defaults to GitHub and only accepts https or file mirrors', () => {
  assert.equal(installer.releaseBaseUrl({}), 'https://github.com/m10rten/go-canary-cli/releases/download');
  assert.equal(installer.releaseBaseUrl({ CANARY_DOWNLOAD_BASE_URL: 'https://mirror.example/dl//' }), 'https://mirror.example/dl');
  assert.equal(installer.releaseBaseUrl({ CANARY_DOWNLOAD_BASE_URL: 'file:///tmp/mirror' }), 'file:///tmp/mirror');
  for (const base of ['http://mirror.example', 'ftp://mirror.example', '/tmp/mirror']) {
    assert.throws(() => installer.releaseBaseUrl({ CANARY_DOWNLOAD_BASE_URL: base }), /must start with https:\/\/ or file:\/\//);
  }
});

// Stands in for https.get: respond(href, callNumber) returns { status, location?, body? } or 'idle'.
function fakeGet(respond) {
  const urls = [];
  const get = (target, _options, onResponse) => {
    urls.push(target.href);
    const reply = respond(target.href, urls.length);
    const request = new EventEmitter();
    request.destroy = (err) => request.emit('error', err);
    setImmediate(() => {
      if (reply === 'idle') return request.emit('timeout');
      const res = new EventEmitter();
      res.statusCode = reply.status;
      res.headers = reply.location ? { location: reply.location } : {};
      res.resume = () => {};
      onResponse(res);
      if (reply.status === 200) {
        res.emit('data', Buffer.from(reply.body));
        res.emit('end');
      }
    });
    return request;
  };
  return { get, urls };
}

test('fetchBuffer follows https redirects and rejects unsafe or failing responses', async () => {
  const start = 'https://example.test/start';
  const testcases = [
    { name: 'redirect to http', respond: () => ({ status: 302, location: 'http://example.test/plain' }), error: /refusing a redirect to http:/, calls: 1 },
    { name: 'redirect loop', respond: () => ({ status: 302, location: start }), error: /too many redirects/, calls: installer.MAX_REDIRECTS + 1 },
    { name: '404', respond: () => ({ status: 404 }), error: /HTTP 404 for https:\/\/example\.test\/start/, calls: 1 },
    { name: 'idle timeout', respond: () => 'idle', error: /timed out fetching https:\/\/example\.test\/start/, calls: 1 },
    {
      name: '302 to https then 200',
      respond: (_href, n) => (n === 1 ? { status: 302, location: '/final' } : { status: 200, body: 'payload' }),
      want: 'payload',
      calls: 2,
    },
  ];
  for (const tc of testcases) {
    const { get, urls } = fakeGet(tc.respond);
    const result = installer.fetchBuffer(start, undefined, get);
    if (tc.error) {
      await assert.rejects(result, tc.error, tc.name);
    } else {
      assert.equal((await result).toString(), tc.want, tc.name);
    }
    assert.equal(urls.length, tc.calls, `${tc.name}: requests made`);
  }
});

// install() end to end against a file:// mirror laid out like a release: <base>/v<version>/<asset>.
function stageInstall(t, { platform, archive, checksumLine }) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'canary-npm-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const pkgDir = path.join(root, 'pkg');
  const release = path.join(root, 'mirror', 'v1.2.3');
  fs.mkdirSync(pkgDir);
  fs.mkdirSync(release, { recursive: true });
  fs.writeFileSync(path.join(pkgDir, 'package.json'), JSON.stringify({ version: '1.2.3' }));
  const name = installer.archiveName('1.2.3', platform, 'x64');
  fs.writeFileSync(path.join(release, name), archive);
  fs.writeFileSync(path.join(release, 'checksums.txt'), checksumLine(name));
  const env = { CANARY_DOWNLOAD_BASE_URL: pathToFileURL(path.join(root, 'mirror')).href };
  return { pkgDir, env, binary: installer.binaryPath(pkgDir, platform) };
}

test('install verifies, extracts and writes the binary', async (t) => {
  const testcases = [
    { platform: 'linux', binaryName: 'canary', archive: tarGz(tarEntry('canary', 'linux binary')) },
    { platform: 'win32', binaryName: 'canary.exe', archive: zipFile({ name: 'canary.exe', content: 'windows binary' }) },
  ];
  for (const tc of testcases) {
    const staged = stageInstall(t, { platform: tc.platform, archive: tc.archive, checksumLine: (name) => `${sha256(tc.archive)}  ${name}\n` });
    await installer.install(staged.pkgDir, staged.env, tc.platform, 'x64');
    assert.equal(path.basename(staged.binary), tc.binaryName);
    assert.match(fs.readFileSync(staged.binary, 'utf8'), /binary$/, tc.platform);
    if (tc.platform !== 'win32' && process.platform !== 'win32') {
      assert.equal(fs.statSync(staged.binary).mode & 0o777, 0o755);
    }
    assert.deepEqual(fs.readdirSync(path.dirname(staged.binary)), [tc.binaryName], 'no temp files left behind');
  }
});

test('install leaves no binary when verification or extraction fails', async (t) => {
  const archive = tarGz(tarEntry('canary', 'linux binary'));
  const testcases = [
    { name: 'tampered checksum', archive, checksumLine: (name) => `${'0'.repeat(64)}  ${name}\n`, error: /checksum mismatch/ },
    { name: 'missing checksum line', archive, checksumLine: () => `${sha256(archive)}  other.tar.gz\n`, error: /has no line for/ },
    { name: 'malformed checksums', archive, checksumLine: () => 'nonsense\n', error: /malformed/ },
    { name: 'archive without the binary', archive: tarGz(tarEntry('README', 'x')), checksumLine: null, error: /canary not found/ },
  ];
  for (const tc of testcases) {
    const staged = stageInstall(t, {
      platform: 'linux',
      archive: tc.archive,
      checksumLine: tc.checksumLine ?? ((name) => `${sha256(tc.archive)}  ${name}\n`),
    });
    await assert.rejects(installer.install(staged.pkgDir, staged.env, 'linux', 'x64'), tc.error, tc.name);
    assert.equal(fs.existsSync(staged.binary), false, `${tc.name}: binary exists`);
  }
});

test('install fails for a missing release asset', async (t) => {
  const staged = stageInstall(t, { platform: 'linux', archive: Buffer.alloc(0), checksumLine: () => '' });
  fs.rmSync(path.join(staged.pkgDir, '..', 'mirror', 'v1.2.3', 'checksums.txt'));
  await assert.rejects(installer.install(staged.pkgDir, staged.env, 'linux', 'x64'), /checksums\.txt/);
  assert.equal(fs.existsSync(staged.binary), false);
});

// Runs against a real `make dist` output when CANARY_DIST_DIR is set (PR CI does). It keeps the
// JS copy of the archive naming rule and the archive readers aligned with internal/release.
const distDir = process.env.CANARY_DIST_DIR;
test('archives from make dist match the JS naming rule and extract', { skip: !distDir && 'CANARY_DIST_DIR is not set' }, () => {
  const checksums = fs.readFileSync(path.join(distDir, 'checksums.txt'), 'utf8');
  const version = /^[0-9a-f]{64} [ *]canary_(.+?)_(?:darwin|linux|windows)_/m.exec(checksums)[1];
  const magic = { darwin: 'cffaedfe', linux: '7f454c46', win32: '4d5a' };
  for (const { platform, arch } of targets) {
    const name = installer.archiveName(version, platform, arch);
    const archive = fs.readFileSync(path.join(distDir, name));
    installer.verifyChecksum(checksums, name, archive);
    const extract = platform === 'win32' ? installer.extractZip : installer.extractTarGz;
    const binary = extract(archive, installer.binaryName(platform));
    assert.ok(binary.subarray(0, 4).toString('hex').startsWith(magic[platform]), `${name} does not hold a ${platform} executable`);
  }
});
