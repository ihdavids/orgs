/* Tufte: each footnote is copied into a sidenote beside the paragraph that
 * calls it. The footnotes stay in the page (hidden on a wide screen) so a
 * narrow screen, which has no margin, still has them at the end. */
(function () {
  var body = document.querySelector('.page-body.with-sidenotes');
  if (!body) return;
  var refs = body.querySelectorAll('sup.footnote-reference a[href^="#footnote-"]');
  if (!refs.length) return;
  refs.forEach(function (a) {
    var id = a.getAttribute('href').slice(1);
    var def = document.getElementById(id);
    if (!def) return;
    var bodyEl = def.parentNode.querySelector('.footnote-body');
    if (!bodyEl) return;
    var num = a.textContent;
    var ref = document.createElement('span');
    ref.className = 'sidenote-ref';
    ref.textContent = num;
    var note = document.createElement('span');
    note.className = 'sidenote';
    note.setAttribute('role', 'note');
    var n = document.createElement('span');
    n.className = 'sn-num';
    n.textContent = num;
    note.appendChild(n);
    // Paragraphs inside a note become runs of text, since a <p> cannot sit
    // inside the <p> the note is attached to.
    var html = bodyEl.innerHTML.replace(/<\/?p>/g, ' ');
    var content = document.createElement('span');
    content.innerHTML = html;
    note.appendChild(content);
    var sup = a.parentNode;
    sup.parentNode.insertBefore(ref, sup.nextSibling);
    ref.parentNode.insertBefore(note, ref.nextSibling);
  });
  body.classList.add('has-sidenotes');
})();
