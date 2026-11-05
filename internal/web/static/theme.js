// Applies the colour theme before the page is drawn, so it does not flash.
// Five themes from light to dark; the choice is kept per browser.
(function () {
  var THEMES = ['light', 'gray', 'slate', 'navy', 'black'];
  var DARK = {slate: 1, navy: 1, black: 1};
  var OLD = {bright: 'light', dim: 'black', dark: 'black', auto: 'gray', teal: 'gray', orange: 'light'};
  function get() {
    var v = 'gray';
    try { v = localStorage.getItem('t66.theme') || 'gray'; } catch (e) {}
    if (OLD[v]) { v = OLD[v]; try { localStorage.setItem('t66.theme', v); } catch (e) {} }
    return THEMES.indexOf(v) < 0 ? 'gray' : v;
  }
  function apply(x) {
    if (x === 'gray') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme', x);
    // the built-in logo has a dark and a light version
    var q = '/logo?dark=' + (DARK[x] ? 1 : 0);
    var set = function () { document.querySelectorAll('img.logo').forEach(function (i) { if (i.getAttribute('src').split('&')[0] !== q) i.src = q; }); };
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', set); else set();
  }
  apply(get());
  window.t66Theme = {
    list: THEMES, get: get,
    set: function (v) { try { localStorage.setItem('t66.theme', v); } catch (e) {} apply(v); },
    next: function () { var v = THEMES[(THEMES.indexOf(get()) + 1) % THEMES.length]; this.set(v); return v; }
  };
})();
