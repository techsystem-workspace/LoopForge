// Apply the language before paint; boot-theme.js owns the theme.
// i18n.js replaces this. Boot must still render if that script fails to load.
window.lfT = window.lfT || ((zh) => zh);
(() => {
  const root = document.documentElement;
  let lang = 'vi';
  try {
    // 'source' keeps the untranslated copy; only test harnesses set it.
    const stored = localStorage.getItem('loopforge.lang');
    lang = stored === 'en' || stored === 'source' ? stored : 'vi';
  } catch (_) { /* Restricted storage still permits the default interface. */ }
  root.dataset.lang = lang;
  root.lang = lang;
  try {
    if (sessionStorage.getItem('loopforge.lang.motion') === 'in') {
      if (matchMedia('(prefers-reduced-motion: reduce)').matches) sessionStorage.removeItem('loopforge.lang.motion');
      else root.classList.add('lang-enter');
    }
  } catch (_) { /* Enter animation is optional when storage is blocked. */ }
})();
