/* Foryx 原语 — Page(记录页骨架)。居中 --w-content 列 + 唯一滚动区。杀掉各海洋手搓的 .doc-root/.sch-col(SPEC §4.1)。
   header/tabs/sections 全填进 .col。mount(host, {body?}) → {el, col}。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function bindFloatingScroll(root) {
    var scroller = root.querySelector('.fy-page-scroll');
    var rail = root.querySelector('.fy-page-scrollbar');
    var thumb = rail && rail.querySelector('i');
    if (!scroller || !rail || !thumb) return;
    var hideTimer = 0;
    var ticking = false;

    function update(show) {
      var maxScroll = scroller.scrollHeight - scroller.clientHeight;
      if (maxScroll <= 0) {
        root.classList.remove('has-scroll');
        root.classList.remove('is-scrolling');
        return;
      }
      root.classList.add('has-scroll');
      var railH = rail.clientHeight;
      var minH = parseFloat(getComputedStyle(root).getPropertyValue('--ctl-sm')) || parseFloat(getComputedStyle(root).getPropertyValue('--ctl'));
      var thumbH = Math.max(minH, railH * scroller.clientHeight / scroller.scrollHeight);
      var top = (railH - thumbH) * scroller.scrollTop / maxScroll;
      thumb.style.height = thumbH + 'px';
      thumb.style.transform = 'translateY(' + top + 'px)';
      if (show) {
        root.classList.add('is-scrolling');
        clearTimeout(hideTimer);
        hideTimer = setTimeout(function () { root.classList.remove('is-scrolling'); }, 700);
      }
    }

    function requestUpdate(show) {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(function () {
        ticking = false;
        update(show);
      });
    }

    scroller.addEventListener('scroll', function () { requestUpdate(true); }, { passive: true });
    root.addEventListener('mouseenter', function () { requestUpdate(false); });
    requestUpdate(false);
    if (window.ResizeObserver) {
      var ro = new ResizeObserver(function () { requestUpdate(false); });
      ro.observe(scroller);
      ro.observe(root.querySelector('.fy-page-col'));
    }
  }

  function html(o) {
    o = o || {};
    return '<div class="fy-page"><div class="fy-page-scroll"><div class="fy-page-col">' + (o.body || '') + '</div></div><span class="fy-page-scrollbar" aria-hidden="true"><i></i></span></div>';
  }
  function mount(host, o) {
    var e = window.el(html(o));
    if (host) host.appendChild(e);
    bindFloatingScroll(e);
    return { el: e, col: e.querySelector('.fy-page-col') };
  }

  window.FyPage = { html: html, mount: mount };
})();
