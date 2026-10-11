// Firefox can block HTML meta refreshes through its accessibility settings.
// Retry the *current* URL after the same server-declared delay when JS is
// available; location.reload() preserves a Zero-Knowledge URL fragment.
(() => {
  const meta = document.querySelector('meta[http-equiv="refresh"]');
  if (!meta) return;

  const delay = meta.getAttribute("content") || "";
  if (!/^\d+$/.test(delay)) return;
  const seconds = Number(delay);
  if (!Number.isSafeInteger(seconds) || seconds < 1 || seconds > 3600) return;

  // Give the native no-JS refresh a moment to win before this fallback.
  setTimeout(() => window.location.reload(), seconds * 1000 + 250);
})();
