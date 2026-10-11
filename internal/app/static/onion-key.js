// Convenience only. Submission remains a deliberate action by the recipient.
(function () {
  const input = document.getElementById("onionKey");
  const key = window.location.hash.slice(1);
  if (input && /^[A-Za-z0-9_-]{43}$/.test(key)) input.value = key;
})();
