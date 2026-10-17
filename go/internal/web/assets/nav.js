// 导航高亮（零件）：由 URL 推导（可推导不存），htmx swap / 前进后退后同步
// （直接复用 HomeCast nav.js 逻辑，去掉移动端侧边面板——vidproc 无侧边导航）
(function () {
  function sync() {
    const p = location.pathname;
    document.querySelectorAll('.nav-tab').forEach((a) => {
      const href = a.getAttribute('href');
      const active = p === href;
      a.classList.toggle('nav-active', active);
    });
  }
  document.addEventListener('htmx:afterSwap', sync);
  window.addEventListener('popstate', sync);
  sync();
})();
