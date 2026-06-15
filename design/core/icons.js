/* Foryx — 图标集(线性,24 视框,currentColor)。icon(name, size=16, stroke=1.7) → svg 串。
   单一 stroke 默认(`demo` 各处手调 stroke 致漂移 → 收归默认 1.7,特殊再传)。 */
(function () {
  var P = {
    chevr:   '<path d="M9 6l6 6-6 6"/>',
    chevd:   '<path d="M6 9l6 6 6-6"/>',
    more:    '<circle cx="5" cy="12" r="1.4"/><circle cx="12" cy="12" r="1.4"/><circle cx="19" cy="12" r="1.4"/>',
    plus:    '<path d="M12 5v14M5 12h14"/>',
    check:   '<path d="M5 12.5l4.5 4.5L19 6.5"/>',
    close:   '<path d="M6 6l12 12M18 6L6 18"/>',
    search:  '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4-4"/>',
    sliders: '<path d="M4 8h9M17 8h3M4 16h3M11 16h9"/><circle cx="15" cy="8" r="2.2"/><circle cx="9" cy="16" r="2.2"/>',
    doc:     '<path d="M7 3h7l5 5v13H7z"/><path d="M14 3v5h5"/>',
    folder:  '<path d="M3 7h6l2 2h10v10H3z"/>',
    func:    '<path d="M5 20V8a3.5 3.5 0 013.5-3.5M9 13h6"/>',
    handler: '<rect x="4" y="4" width="16" height="16" rx="3"/><path d="M9 12h6"/>',
    agent:   '<circle cx="12" cy="8" r="3.2"/><path d="M5 20a7 7 0 0114 0"/>',
    workflow:'<circle cx="6" cy="6" r="2.4"/><circle cx="18" cy="18" r="2.4"/><path d="M8 6h6a4 4 0 014 4v6"/>',
    bell:    '<path d="M6 9a6 6 0 0112 0c0 5 2 6 2 6H4s2-1 2-6"/><path d="M10 20a2 2 0 004 0"/>',
    gear:    '<circle cx="12" cy="12" r="3.2"/><path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.5 5.5l2 2M16.5 16.5l2 2M18.5 5.5l-2 2M7.5 16.5l-2 2"/>',
    chat:    '<path d="M4 5h16v11H8l-4 4z"/>',
  };
  window.icon = function (name, size, stroke) {
    size = size || 16; stroke = stroke || 1.7;
    return '<svg viewBox="0 0 24 24" width="' + size + '" height="' + size + '" fill="none" stroke="currentColor" stroke-width="'
      + stroke + '" stroke-linecap="round" stroke-linejoin="round">' + (P[name] || '') + '</svg>';
  };
  window.FY_ICONS = P;
})();
