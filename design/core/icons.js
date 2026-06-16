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
    /* —— 实体/UI 图标:全部画在居中艺术板内(墨迹光学中心 ≈ 12,12),与点点同心,模版对齐 —— */
    doc:     '<path d="M7 4h6l4 4v12H7z"/><path d="M13 4v4h4"/>',
    folder:  '<path d="M4 7h5l2 2h9v9H4z"/>',
    func:    '<path d="M14.5 5.5H13A2.5 2.5 0 0010.5 8v9"/><path d="M8 11.5h6"/>',
    handler: '<rect x="5" y="5" width="14" height="14" rx="3"/><path d="M10 12h4"/>',
    agent:   '<circle cx="12" cy="8.5" r="3"/><path d="M6.5 19a5.5 5.5 0 0111 0"/>',
    workflow:'<circle cx="6.5" cy="12" r="2.2"/><circle cx="17.5" cy="7.5" r="2.2"/><circle cx="17.5" cy="16.5" r="2.2"/><path d="M8.6 11l6.8-2.6M8.6 13l6.8 2.6"/>',
    bell:    '<path d="M6.5 10a5.5 5.5 0 0111 0c0 4 1.5 5 1.5 5H5s1.5-1 1.5-5"/><path d="M10.5 19a1.7 1.7 0 003 0"/>',
    gear:    '<circle cx="12" cy="12" r="3"/><path d="M12 4v2.5M12 17.5V20M4 12h2.5M17.5 12H20M6.3 6.3l1.8 1.8M15.9 15.9l1.8 1.8M17.7 6.3l-1.8 1.8M8.1 15.9l-1.8 1.8"/>',
    chat:    '<path d="M5 5.5h14v9H9l-4 4z"/>',
    play:    '<path d="M7.5 5.5l10 6.5-10 6.5z"/>',
    panel:   '<rect x="4" y="5" width="16" height="14" rx="2"/><path d="M14 5v14"/>',
    link:    '<path d="M10 14a3.5 3.5 0 005 0l3-3a3.5 3.5 0 00-5-5l-1 1"/><path d="M14 10a3.5 3.5 0 00-5 0l-3 3a3.5 3.5 0 005 5l1-1"/>',
  };
  window.icon = function (name, size, stroke) {
    size = size || 16; stroke = stroke || 1.7;
    return '<svg viewBox="0 0 24 24" width="' + size + '" height="' + size + '" fill="none" stroke="currentColor" stroke-width="'
      + stroke + '" stroke-linecap="round" stroke-linejoin="round">' + (P[name] || '') + '</svg>';
  };
  window.FY_ICONS = P;
})();
