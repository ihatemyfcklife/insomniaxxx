// k6 load test: 1000 concurrent encrypted downloads against an APPROVED staging
// environment and a synthetic seeded cipher blob. Do not use public relays.
import http from 'k6/http';
import { check, sleep } from 'k6';

const base = (__ENV.K6_BASE_URL || '').replace(/\/$/, '');
const hash = __ENV.K6_BLOB_HASH || '';
if (!base || !/^[0-9a-f]{64}$/i.test(hash)) {
  throw new Error('Set K6_BASE_URL and K6_BLOB_HASH for a seeded synthetic staging blob');
}
const vus = Number(__ENV.K6_VUS || '1000');
const duration = __ENV.K6_DURATION || '5m';
const cookie = __ENV.K6_SESSION_COOKIE || '';
export const options = {
  scenarios: {
    verified_cipher_downloads: {
      executor: 'constant-vus',
      vus,
      duration,
      gracefulStop: '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.005'],
    http_req_duration: ['p(95)<30000'],
    checks: ['rate>0.995'],
  },
};

export default function () {
  const headers = cookie ? { Cookie: cookie } : {};
  const res = http.get(`${base}/d/${hash}?cipher=1`, {
    headers,
    responseType: 'none',
    timeout: '3m',
    tags: { transfer: 'encrypted-download' },
  });
  check(res, { 'download completes with 200': (r) => r.status === 200 });
  sleep(0.1);
}
