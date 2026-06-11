/* Host bridge for sandboxed Jaz widgets. Keeps the theme in sync with the
 * app, routes link clicks to the host (links open in the system browser),
 * and reports runtime errors so the loop can fix them on its next run. */
(function () {
  function post(type, data) {
    var msg = { type: type };
    if (data) {
      for (var key in data) msg[key] = data[key];
    }
    try {
      window.parent.postMessage(msg, '*');
    } catch (err) {
      /* host gone */
    }
  }

  window.addEventListener('message', function (event) {
    var msg = event && event.data;
    if (!msg || typeof msg !== 'object') return;
    if (msg.type === 'jaz:theme') {
      document.documentElement.classList.toggle('dark', msg.theme === 'dark');
    }
    if (msg.type === 'jaz:scale' && typeof msg.scale === 'number') {
      var root = document.getElementById('jz-root') || document.body;
      root.style.zoom = String(msg.scale);
    }
  });

  document.addEventListener('click', function (event) {
    var target = event.target;
    var link = target && target.closest ? target.closest('a[href]') : null;
    if (!link) return;
    var href = link.getAttribute('href') || '';
    if (/^https?:\/\//i.test(href)) {
      event.preventDefault();
      post('jaz:link', { href: href });
      return;
    }
    if (href.charAt(0) === '#') return;
    event.preventDefault();
  });

  window.addEventListener('error', function (event) {
    post('jaz:error', { message: String((event && event.message) || 'widget error') });
  });

  /* Broken images render as a broken-glyph icon the author never sees. Hide
   * them (visibility keeps the layout stable) and count them in the layout
   * report so the loop drops or replaces the URL on its next run. Resource
   * error events don't bubble, hence the capture phase; images that failed
   * before this script ran are caught by the sweep in measureLayout. */
  document.addEventListener(
    'error',
    function (event) {
      var el = event && event.target;
      if (el && el.tagName === 'IMG') el.style.visibility = 'hidden';
    },
    true
  );

  function hideBrokenImages() {
    var broken = 0;
    var imgs = document.images;
    for (var i = 0; i < imgs.length; i++) {
      var img = imgs[i];
      if (img.complete && img.naturalWidth === 0 && img.getAttribute('src')) {
        img.style.visibility = 'hidden';
        broken++;
      }
    }
    return broken;
  }

  window.addEventListener('unhandledrejection', function (event) {
    var reason = event && event.reason;
    var message = reason && reason.message ? reason.message : String(reason);
    post('jaz:error', { message: 'unhandled rejection: ' + message });
  });

  /* Layout telemetry: the widget author designs blind, so the host measures
   * what actually rendered — dead space at the bottom, overflow past the
   * tile, and elements that clip their content — and reports it back. The
   * loop sees problems in its next run prompt. */
  function measureLayout() {
    var root = document.getElementById('jz-root');
    if (!root) return;
    var rootRect = root.getBoundingClientRect();
    var overflowPx = Math.max(0, root.scrollHeight - root.clientHeight);
    var nodes = root.querySelectorAll('*');
    var contentBottom = 0;
    var clipped = 0;
    var count = Math.min(nodes.length, 600);
    for (var i = 0; i < count; i++) {
      var el = nodes[i];
      var style = getComputedStyle(el);
      if (
        el.scrollHeight > el.clientHeight + 2 &&
        (style.overflowY === 'hidden' || style.overflowY === 'clip')
      ) {
        clipped++;
      }
      if (el.childElementCount !== 0) continue;
      var rect = el.getBoundingClientRect();
      if (rect.width < 4 || rect.height < 4) continue;
      var visible =
        (el.textContent && el.textContent.trim() !== '') ||
        /^(IMG|SVG|CANVAS|VIDEO)$/.test(el.tagName) ||
        (style.backgroundColor && !/rgba?\(\s*0\s*,\s*0\s*,\s*0\s*,\s*0\s*\)/.test(style.backgroundColor)) ||
        parseFloat(style.borderTopWidth) > 0;
      if (visible && rect.bottom > contentBottom) contentBottom = rect.bottom;
    }
    var available = root.clientHeight - 12; /* bottom padding */
    var deadPx = Math.max(0, available - (contentBottom - rootRect.top));
    var deadPct = overflowPx > 0 ? 0 : Math.round((100 * deadPx) / Math.max(1, root.clientHeight));
    post('jaz:layout', {
      dead_space_pct: deadPct,
      overflow_px: Math.round(overflowPx),
      clipped: clipped,
      img_errors: hideBrokenImages(),
    });
  }

  function onReady() {
    post('jaz:ready');
    /* after fonts/images settle */
    window.setTimeout(measureLayout, 800);
  }

  /* Re-measure when the tile is resized: a user fixing dead space by
   * resizing must replace the stale problem report, not leave it for the
   * loop to "fix" on its next run. ResizeObserver on the document element is
   * the reliable signal for the iframe being resized by the host. */
  var resizeTimer = null;
  function scheduleMeasure() {
    if (resizeTimer) window.clearTimeout(resizeTimer);
    resizeTimer = window.setTimeout(measureLayout, 500);
  }
  if (typeof ResizeObserver === 'function') {
    new ResizeObserver(scheduleMeasure).observe(document.documentElement);
  } else {
    window.addEventListener('resize', scheduleMeasure);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', onReady);
  } else {
    onReady();
  }
})();
