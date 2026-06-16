/* Foryx — shared icon grammar.
   Linear 24-viewBox glyphs, currentColor, one default stroke. Modules consume keys only. */
(function () {
  var P = {
    unknown: '<circle cx="12" cy="12" r="8"/><path d="M9.5 9.5a2.6 2.6 0 0 1 5 1c0 1.8-2.5 2.1-2.5 4"/><path d="M12 18h.01"/>',

    chevr: '<path d="M9 6l6 6-6 6"/>',
    chevd: '<path d="M6 9l6 6 6-6"/>',
    more: '<circle cx="5" cy="12" r="1.4"/><circle cx="12" cy="12" r="1.4"/><circle cx="19" cy="12" r="1.4"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    check: '<path d="M5 12.5l4.5 4.5L19 6.5"/>',
    close: '<path d="M6 6l12 12M18 6L6 18"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4-4"/>',
    sliders: '<path d="M4 8h9M17 8h3M4 16h3M11 16h9"/><circle cx="15" cy="8" r="2.2"/><circle cx="9" cy="16" r="2.2"/>',
    sort: '<path d="M5 6h14M8 12h8M11 18h2"/><path d="M4 15l3 3 3-3"/>',
    grip: '<circle cx="9" cy="6" r="1"/><circle cx="15" cy="6" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="18" r="1"/><circle cx="15" cy="18" r="1"/>',

    side: '<rect x="4" y="5" width="16" height="14" rx="2"/><path d="M10 5v14"/>',
    panel: '<rect x="4" y="5" width="16" height="14" rx="2"/><path d="M14 5v14"/>',
    'panel-left': '<rect x="4" y="5" width="16" height="14" rx="2"/><path d="M10 5v14"/>',
    'panel-right': '<rect x="4" y="5" width="16" height="14" rx="2"/><path d="M14 5v14"/>',
    terminal: '<path d="M6 8l4 4-4 4"/><path d="M12.5 16h5.5"/>',

    doc: '<path d="M7 4h6l4 4v12H7z"/><path d="M13 4v4h4"/><path d="M9.5 13h5M9.5 16.5h3.5"/>',
    folder: '<path d="M4 7h5l2 2h9v9H4z"/>',
    entities: '<rect x="4" y="4" width="6" height="6" rx="1.4"/><rect x="14" y="4" width="6" height="6" rx="1.4"/><rect x="4" y="14" width="6" height="6" rx="1.4"/><rect x="14" y="14" width="6" height="6" rx="1.4"/>',
    function: '<path d="M15 4h-2.2A3.8 3.8 0 0 0 9 7.8V20"/><path d="M6.5 11h8"/>',
    handler: '<rect x="5" y="5" width="14" height="14" rx="3"/><path d="M9 12h6"/>',
    agent: '<circle cx="12" cy="8.5" r="3"/><path d="M6.5 19a5.5 5.5 0 0 1 11 0"/>',
    workflow: '<circle cx="6.5" cy="12" r="2.2"/><circle cx="17.5" cy="7.5" r="2.2"/><circle cx="17.5" cy="16.5" r="2.2"/><path d="M8.6 11l6.8-2.6M8.6 13l6.8 2.6"/>',
    trigger: '<circle cx="12" cy="12" r="2.4"/><path d="M7.2 7.2a6.8 6.8 0 0 0 0 9.6M16.8 7.2a6.8 6.8 0 0 1 0 9.6"/>',
    control: '<circle cx="6" cy="12" r="2"/><circle cx="18" cy="7" r="2"/><circle cx="18" cy="17" r="2"/><path d="M8 12h3.5l4.6-4M11.5 12l4.6 5"/>',
    action: '<rect x="5" y="5" width="14" height="14" rx="3"/><path d="M10 8.5l5.5 3.5L10 15.5z"/>',
    shield: '<path d="M12 4l7 3v5c0 4-2.8 6.6-7 8-4.2-1.4-7-4-7-8V7z"/><path d="M9 12.2l2 2 4-4"/>',
    mcp: '<rect x="6" y="7" width="12" height="8" rx="2.5"/><path d="M9 7V4M15 7V4M12 15v3M9 20h6"/>',
    skill: '<rect x="6" y="4" width="12" height="16" rx="2"/><path d="M9 4v16"/><path d="M12.5 9.5H16M12.5 14.5H16"/>',

    chat: '<path d="M5 5.5h14v9H9l-4 4z"/>',
    conversation: '<path d="M5 5.5h14v9H9l-4 4z"/>',
    scheduler: '<circle cx="12" cy="12" r="8"/><path d="M12 8v4l3 2"/>',
    tasks: '<path d="M10 6h10M10 12h10M10 18h10"/><path d="M4 6.3l1.2 1.2L7.5 5M4 12.3l1.2 1.2L7.5 11M4 18.3l1.2 1.2L7.5 17"/>',
    repo: '<path d="M5 5h14v12H5z"/><path d="M8 20h8M12 17v3"/>',
    bell: '<path d="M6.5 10a5.5 5.5 0 0 1 11 0c0 4 1.5 5 1.5 5H5s1.5-1 1.5-5"/><path d="M10.5 19a1.7 1.7 0 0 0 3 0"/>',
    gear: '<circle cx="12" cy="12" r="3"/><path d="M12 4v2.5M12 17.5V20M4 12h2.5M17.5 12H20M6.3 6.3l1.8 1.8M15.9 15.9l1.8 1.8M17.7 6.3l-1.8 1.8M8.1 15.9l-1.8 1.8"/>',
    moon: '<path d="M20 14.5A8 8 0 0 1 9.5 4a7 7 0 1 0 10.5 10.5z"/>',
    sun: '<circle cx="12" cy="12" r="3.8"/><path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6 7 7M17 17l1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4"/>',

    play: '<path d="M7.5 5.5l10 6.5-10 6.5z"/>',
    enter: '<path d="M9 9l-4 4 4 4"/><path d="M19 5v4a4 4 0 0 1-4 4H5"/>',
    stop: '<rect x="6" y="6" width="12" height="12" rx="2"/>',
    send: '<path d="M20 4L9.5 14.5"/><path d="M20 4l-4.5 16-6-5.5L4 12z"/>',
    spin: '<path d="M20 12a8 8 0 1 1-2.3-5.7"/><path d="M20 5.5v5h-5"/>',
    spark: '<path d="M12 3v5M12 16v5M3 12h5M16 12h5M6.5 6.5l3 3M14.5 14.5l3 3M17.5 6.5l-3 3M9.5 14.5l-3 3"/>',
    zap: '<path d="M13 3L5 13h7l-1 8 8-11h-7z"/>',
    dispatch: '<path d="M4 8h16v10H4z"/><path d="M4 12h6l2 2 2-2h6"/>',
    forge: '<path d="M12 4l7 3v5c0 4-2.8 6.6-7 8-4.2-1.4-7-4-7-8V7z"/><path d="M9 13h6M10.5 10h3"/>',

    edit: '<path d="M12 20h8"/><path d="M15.5 4.5a2 2 0 0 1 3 3L7.5 18.5 4 20l1.5-3.5z"/>',
    copy: '<rect x="9" y="9" width="10" height="10" rx="2"/><path d="M5 15V7a2 2 0 0 1 2-2h8"/>',
    trash: '<path d="M5 7h14M9 7V5h6v2M7 7l1 13h8l1-13"/>',
    link: '<path d="M10 14a3.5 3.5 0 0 0 5 0l3-3a3.5 3.5 0 0 0-5-5l-1 1"/><path d="M14 10a3.5 3.5 0 0 0-5 0l-3 3a3.5 3.5 0 0 0 5 5l1-1"/>',
    at: '<circle cx="12" cy="12" r="4"/><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8"/>',
    tag: '<path d="M11 4H6a2 2 0 0 0-2 2v5l9 9 7-7z"/><path d="M7.5 7.5h.01"/>',
    flag: '<path d="M6 20V5h11l-2 4 2 4H6"/>',

    code: '<path d="M8 8l-4 4 4 4M16 8l4 4-4 4"/>',
    text: '<path d="M5 7h14M5 12h14M5 17h9"/>',
    heading: '<path d="M6 5v14M18 5v14M6 12h12"/>',
    list: '<path d="M9 6h11M9 12h11M9 18h11"/><circle cx="4.5" cy="6" r="1"/><circle cx="4.5" cy="12" r="1"/><circle cx="4.5" cy="18" r="1"/>',
    listol: '<path d="M10 6h10M10 12h10M10 18h10"/><path d="M4 5h1v4M3.5 9h2"/>',
    quote: '<path d="M9 7H5v5h4l-1.5 5M19 7h-4v5h4l-1.5 5"/>',
    table: '<rect x="4" y="5" width="16" height="14" rx="1.5"/><path d="M4 10h16M10 5v14"/>',
    image: '<rect x="4" y="5" width="16" height="14" rx="2"/><circle cx="9" cy="10" r="1.5"/><path d="M5.5 18l4-4 3 2.5 3.5-5 2.5 3.5"/>',
    divider: '<path d="M4 12h16"/>'
  };

  P.func = P.function;

  var missing = {};
  window.icon = function (name, size, stroke) {
    size = size || 16;
    stroke = stroke || 1.7;
    if (!P[name]) missing[name || '(empty)'] = true;
    return '<svg viewBox="0 0 24 24" width="' + size + '" height="' + size + '" fill="none" stroke="currentColor" stroke-width="'
      + stroke + '" stroke-linecap="round" stroke-linejoin="round">' + (P[name] || P.unknown) + '</svg>';
  };
  window.ICONS = P;
  window.FY_ICONS = P;
  window.ICON_MISSING = missing;
})();
