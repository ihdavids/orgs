/* Making a slide fit when there is too much on it.
 *
 * Nobody writes a deck with a ruler. A slide grows a fourth bullet, a table
 * gains two rows, a code block is pasted in whole - and in every one of these
 * four frameworks the overflow is simply *not drawn*: reveal clips it, deck.js
 * hides it under the next slide, impress runs it off the step, WebSlides lets
 * it scroll where nobody will scroll. The failure is silent, which is the worst
 * part: the deck looks finished right up until it is on a projector.
 *
 * So: measure what is on the slide, and if it does not fit, shrink it until it
 * does. The rules are what make it worth having rather than annoying:
 *
 *  1. **It only ever shrinks.** Scaling a thin slide up to fill the screen
 *     makes a deck with uneven type, which looks worse than the empty space it
 *     was trying to fix. `:FIT: grow` asks for it explicitly.
 *  2. **There is a floor** (`:FIT: 0.3` to lower it; 45% by default). Past that the slide
 *     is not overfull, it is a document, and the honest thing is to leave it
 *     unreadable-but-visible rather than silently shrink it to nothing - you
 *     want to notice while writing the deck, not while giving it.
 *  3. **It measures, it does not reflow.** The content is scaled as a block, so
 *     the line breaks and the layout stay exactly where the author saw them.
 *     Reducing the font size instead re-wraps every line, which moves the
 *     thing somebody was looking at when they decided the slide was finished.
 *  4. **It runs again when anything could have changed size**: on resize, when
 *     a slide is shown, when the fonts arrive, when an image loads, and when
 *     the content itself changes. A picture that loads late is the single most
 *     common reason a slide that fitted a moment ago does not.
 *  5. **It is idempotent and cheap.** Every measurement is taken with the scale
 *     reset, so running it twice gives the same answer, and a slide that fits
 *     costs two reads and no writes.
 */
(function () {
  'use strict';

  var FIT = '.org-fit';
  var MIN_DEFAULT = 0.45;

  function num(el, name, dflt) {
    var v = parseFloat(getComputedStyle(el).getPropertyValue(name));
    return isNaN(v) ? dflt : v;
  }

  // The box a slide's content has to fit inside. Each framework hands us a
  // different element for it, which is the only framework-specific thing here.
  function boxOf(fit) {
    var el = fit.closest('[data-fit-box]') || fit.parentElement;
    return el;
  }

  // One measurement, in rendered pixels, of how much of the box the content is
  // using. Everything here sits inside one or two outer transforms - reveal
  // scales its whole stage to the window, impress scales its canvas - and
  // mixing layout pixels with rendered ones gets the arithmetic wrong by
  // exactly that factor. Rectangles are all in one space, and a ratio is all
  // this needs.
  //
  // Measuring from the wrapper's own top also means whatever sits above it - a
  // padding, a heading left outside the wrapper - is simply room the content
  // does not have, with nothing to calculate.
  function overflow(el, box) {
    var bb = box.getBoundingClientRect();
    var eb = el.getBoundingClientRect();
    if (bb.height <= 0 || eb.height <= 0) { return null; }
    var ratio = bb.height / (box.offsetHeight || bb.height);
    var bottom = bb.bottom;
    if (box.dataset.fitBox === 'viewport') {
      // A WebSlides section *grows* with its content - it is a page rather
      // than a slide - so its own height is never smaller than what is on it
      // and nothing would ever be found to be overfull. What a slide actually
      // shows is a window's worth, so that is what has to be fitted into.
      bottom = Math.min(bb.bottom, bb.top + (window.innerHeight || bb.height));
    }
    var availH = (bottom - num(box, 'padding-bottom', 0) * ratio) - eb.top;
    var availW = (bb.right - num(box, 'padding-right', 0) * ratio) - eb.left;
    if (availH <= 0 || availW <= 0) { return null; }
    return Math.min(availH / eb.height, availW / eb.width);
  }

  function fit(el) {
    if (!el || el.dataset.fitOff === 'yes') { return; }
    var box = boxOf(el);
    if (!box) { return; }

    // Always from a clean slate: a scale left over from the last run would be
    // measured as if it were the content's real size, and the slide would
    // creep smaller every time it was shown.
    el.style.setProperty('--fit-scale', 1);
    el.classList.remove('org-fitted');

    var room = overflow(el, box);
    if (room === null) { return; }
    var grow = el.dataset.fitGrow === 'yes';
    if (room >= 1 && !grow) { return; }

    var min = parseFloat(el.dataset.fitMin || '') || MIN_DEFAULT;
    var max = grow ? parseFloat(el.dataset.fitMax || '1.6') : 1;
    var scale = clamp(room, min, max);

    // Then refine, because shrinking changes what has to fit. The wrapper is
    // laid out at `100% / scale` and scaled back down, so the text keeps its
    // visual width and gets smaller - which means it re-wraps to *fewer* lines
    // and the slide is shorter than the first measurement said. One pass would
    // therefore always shrink too far; three converge to within a percent, and
    // each is one layout of one slide.
    for (var pass = 0; pass < 3; pass++) {
      el.style.setProperty('--fit-scale', scale);
      var k = overflow(el, box);
      if (k === null) { break; }
      if (k >= 0.995 && k <= 1.02) { break; }
      scale = clamp(scale * k, min, max);
    }

    // A whisker under, so a rounding error does not leave the last line
    // clipped - the one failure that looks exactly like the bug this is here
    // to fix.
    scale = Math.floor(scale * 1000) / 1000 * 0.995;
    el.style.setProperty('--fit-scale', scale);
    el.classList.add('org-fitted');
    el.dataset.fitScale = String(Math.round(scale * 100));
  }

  function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }

  var lastFit = 0;

  function fitAll() {
    var all = document.querySelectorAll(FIT);
    for (var i = 0; i < all.length; i++) { fit(all[i]); }
    lastFit = Date.now();
  }

  // Two frames, because the first one is before the browser has finished
  // laying the slide out and would measure the old size.
  //
  // With a timer behind them, because **a background tab never gets a frame**:
  // requestAnimationFrame simply does not fire while the tab is hidden, so a
  // deck opened in a second tab and switched to later would have measured
  // nothing. The flag means whichever arrives first wins and the other is a
  // no-op.
  var queued = false;
  function soon() {
    if (queued) { return; }
    queued = true;
    var run = function () {
      if (!queued) { return; }
      queued = false;
      fitAll();
    };
    requestAnimationFrame(function () { requestAnimationFrame(run); });
    setTimeout(run, 60);
  }

  function watch() {
    window.addEventListener('resize', soon);
    window.addEventListener('load', soon);
    window.addEventListener('orientationchange', soon);
    // Coming back to a tab that was hidden: everything above was starved while
    // it was, and the window may be a different size now.
    document.addEventListener('visibilitychange', function () {
      if (!document.hidden) { soon(); }
    });
    // Every one of the four says "a slide is now showing" differently, and one
    // of them is not listening to any of these - hence the interval below.
    ['slidechanged', 'ready', 'resize', 'overviewhidden'].forEach(function (e) {
      if (window.Reveal && Reveal.on) { Reveal.on(e, soon); }
    });
    document.addEventListener('impress:stepenter', soon);
    document.addEventListener('ws:slide-change', soon);
    if (window.jQuery) { jQuery(document).on('deck.change', soon); }

    // Fonts arrive after the first paint and change every measurement.
    if (document.fonts && document.fonts.ready) { document.fonts.ready.then(soon); }
    // So do pictures, which is the commonest reason a slide that fitted a
    // moment ago does not.
    document.querySelectorAll(FIT + ' img').forEach(function (img) {
      if (!img.complete) { img.addEventListener('load', soon, { once: true }); }
    });
    // And anything that changes the content: a block that has just been run, a
    // mermaid diagram that has just drawn itself, a webfont that swapped in
    // after the first measurement.
    //
    // The cooldown is not a nicety. Fitting *changes the size of the very
    // element being observed*, so without it every fit schedules another one
    // and a slide sits there re-laying itself out for as long as it is on
    // screen - during a presentation, on somebody's laptop battery.
    if (window.ResizeObserver) {
      var ro = new ResizeObserver(function () {
        if (Date.now() - lastFit < 250) { return; }
        soon();
      });
      document.querySelectorAll(FIT).forEach(function (el) { ro.observe(el); });
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { watch(); soon(); });
  } else {
    watch();
    soon();
  }
  // Two late passes for anything that arrived without telling us - a webfont a
  // browser reports as ready before it has swapped it in, mostly, which makes
  // the first measurement of every slide about a tenth too tall.
  setTimeout(soon, 1200);
  setTimeout(soon, 3000);

  window.orgFit = fitAll;
})();
