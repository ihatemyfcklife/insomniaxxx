// k6 mixed 1,000 virtual users across four HTTP transfer paths.
// Staging only: use synthetic files and PRIVATE Blossom relay infrastructure.
// Browser-based ZK/CAPTCHA and No-JS interoperability must be tested separately.
import http from 'k6/http';
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
const cookie = __ENV.K6_SESSION_COOKIE || '';
const duration = __ENV.K6_DURATION || '5m';
const total = Number(__ENV.K6_VUS || 1000);
if (!Number.isInteger(total) || total < 4 || total % 4 !== 0) {
  throw new Error('K6_VUS must be divisible by 4 and at least 4');
}
const count = total / 4;

export const options = {
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

function opts(tag) {
  return {
    headers: cookie ? { Cookie: cookie } : {},
    timeout: '3m',
    responseType: 'none',
    tags: { name: tag, transfer: tag },
  };
}

export function downloadCipher() {
  const r = http.get(`${base}/d/${cipherHash}?cipher=1`, opts('cipher-download'));
  check(r, { 'encrypted download completes': (x) => x.status === 200 });
  sleep(0.1);
}

export function downloadNoJS() {
  const r = http.get(`${base}${noJsPath}?download=1`, opts('nojs-download'));
  check(r, { 'plaintext download completes': (x) => x.status === 200 });
  sleep(0.1);
}

export function uploadCipher() {
  const fields = { file: http.file(cipherData, 'synthetic-encrypted.blob', 'application/octet-stream') };
  const r = http.post(`${base}/upload-raw`, fields, opts('cipher-upload'));
  check(r, { 'encrypted upload accepted': (x) => x.status === 200 });
  sleep(0.1);
}

export function uploadNoJS() {
  const fields = { file: http.file(noJsData, 'synthetic.bin', 'application/octet-stream') };
  const r = http.post(`${base}/upload`, fields, opts('nojs-upload'));
  check(r, { 'plaintext upload accepted': (x) => x.status === 200 });
  sleep(0.1);
}
