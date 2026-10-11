// k6 mixed 1,000 virtual users across four HTTP transfer paths.
// Staging only: use synthetic files and PRIVATE Blossom relay infrastructure.
// Browser-based ZK/CAPTCHA and No-JS interoperability must be tested separately.
import http from 'k6/http';
import crypto from 'k6/crypto';
import { check, sleep } from 'k6';

const base = (__ENV.K6_BASE_URL || '').replace(/\/$/, '');
const cipherHash = __ENV.K6_BLOB_HASH || '';
const noJsPath = __ENV.K6_NOJS_SHARE_PATH || '';
if (!base || !/^[0-9a-f]{64}$/i.test(cipherHash) ||
    !/^\/d\/[0-9a-f]{64}\/[A-Za-z0-9_-]+$/.test(noJsPath) ||
    !__ENV.K6_CIPHER_FILE || !__ENV.K6_NOJS_FILE) {
  throw new Error('Synthetic staging URL, ciphertext hash, no-JS share path, and two fixture paths required');
}

const cipherData = open(__ENV.K6_CIPHER_FILE, 'b');
const noJsData = open(__ENV.K6_NOJS_FILE, 'b');
const cipherUploadHash = crypto.sha256(cipherData, 'hex');
// The pre-seeded No-JS share must contain the same bytes as K6_NOJS_FILE.
const plainHash = crypto.sha256(noJsData, 'hex');
const cookie = __ENV.K6_SESSION_COOKIE || '';
const duration = __ENV.K6_DURATION || '5m';
const total = Number(__ENV.K6_VUS || 1000);
if (!Number.isInteger(total) || total < 4 || total % 4 !== 0) {
  throw new Error('K6_VUS must be divisible by 4 and at least 4');
}
const count = total / 4;

export const options = {
  systemTags: ['status', 'method', 'name', 'scenario', 'check', 'expected_response'],
  scenarios: {
    zk_downloads: { exec: 'downloadCipher', executor: 'constant-vus', vus: count, duration },
    nojs_downloads: { exec: 'downloadNoJS', executor: 'constant-vus', vus: count, duration },
    zk_uploads: { exec: 'uploadCipher', executor: 'constant-vus', vus: count, duration },
    nojs_uploads: { exec: 'uploadNoJS', executor: 'constant-vus', vus: count, duration },
  },
  thresholds: {
    checks: ['rate>0.995'],
    http_req_failed: ['rate<0.005'],
  },
};

function opts(tag, responseType = 'binary') {
  return {
    headers: cookie ? { Cookie: cookie } : {},
    timeout: '3m',
    responseType,
    tags: { name: tag, transfer: tag },
  };
}

export function downloadCipher() {
  const r = http.get(`${base}/d/${cipherHash}?cipher=1`, opts('cipher-download'));
  check(r, { 'encrypted download checksum matches': (x) => validBytes(x, cipherHash.toLowerCase()) });
  sleep(0.1);
}

export function downloadNoJS() {
  const r = http.get(`${base}${noJsPath}?download=1`, opts('nojs-download'));
  check(r, { 'plaintext download checksum matches': (x) => validBytes(x, plainHash) });
  sleep(0.1);
}

export function uploadCipher() {
  const fields = { file: http.file(cipherData, 'synthetic-encrypted.blob', 'application/octet-stream') };
  const r = http.post(`${base}/upload-raw`, fields, opts('cipher-upload', 'text'));
  check(r, { 'encrypted upload returns the correct hash': (x) => {
    if (x.status !== 200) return false;
    try { return x.json().hash === cipherUploadHash; } catch (_) { return false; }
  } });
  sleep(0.1);
}

export function uploadNoJS() {
  const fields = { file: http.file(noJsData, 'synthetic.bin', 'application/octet-stream') };
  const r = http.post(`${base}/upload`, fields, opts('nojs-upload', 'text'));
  const share = r.status === 200 && typeof r.body === 'string'
    ? r.body.match(/\/d\/[0-9a-f]{64}\/[A-Za-z0-9_-]{43}(?=[\s"'<&])/)
    : null;
  check(r, { 'plaintext upload returns a share': () => !!share });
  if (share) {
    const downloaded = http.get(`${base}${share[0]}?download=1`, opts('nojs-roundtrip'));
    check(downloaded, { 'new plaintext share checksum matches': (x) => validBytes(x, plainHash) });
  }
  sleep(0.1);
}


function validBytes(response, expectedHash) {
  return response.status === 200 && response.body instanceof ArrayBuffer &&
    crypto.sha256(response.body, 'hex') === expectedHash;
}
