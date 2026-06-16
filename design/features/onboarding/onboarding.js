/* Foryx demo — onboarding 独立首启向导（薄组合）。
   两步向导：(1) 外观 + 语言（主题/语言即时生效）→ (2) API 密钥（大模型 key 必填、搜索 key 可选；隐私脚注强调本地加密存储）。
   像素几乎全在组件库：Segmented(语言/外观/模型字段) + Dropdown(模型选择)；自己只画两栏配置岛与按钮链。
   独立页：自持 <html>，加载 core 令牌/重置 + 用到的组件 <script>，故无 Shell/Intent/SideBar 接入（standalone，无 owned kind）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const $ = s => document.querySelector(s);
  const $$ = s => [...document.querySelectorAll(s)];

  /* ===== 实时 i18n：语言段控重译整页 ===== */
  const T = {
    en: {
      back: 'Back', next: 'Next', enter: 'Enter Foryx',
      name: 'Name', apiKey: 'API key', test: 'Test', testing: 'Testing…', connected: 'Connected',
      providerH: 'Provider', searchProviderH: 'Search provider',
      langLabel: 'Language', appearance: 'Appearance', langEn: 'English', langZh: '中文', langSys: 'System',
      themeLight: 'Light', themeDark: 'Dark', themeSys: 'System',
      s0title: 'Make it yours', s0sub: 'Pick a look and language — both apply instantly',
      s1title: 'Connect a model', s1sub: 'Required to chat — your key stays on this machine',
      model: 'Model', dialogue: 'dialogue', keyPlaceholder: 'Paste your API key',
      utilTitle: 'Also use for utility tasks', utilSub: 'Auto-titles, compaction, search summaries',
      privacy: 'Keys are encrypted (AES-GCM) in local SQLite and never leave your machine.',
      searchH: 'Web search (optional)',
      searchKeyPlaceholder: 'Paste your search API key',
      searchFoot: 'Optional — lets the assistant search the web. Add or change it anytime in Settings.',
    },
    'zh-CN': {
      back: '返回', next: '下一步', enter: '进入 Foryx',
      name: '名称', apiKey: 'API 密钥', test: '测试', testing: '测试中…', connected: '已连接',
      providerH: '供应商', searchProviderH: '搜索供应商',
      langLabel: '语言', appearance: '外观', langEn: 'English', langZh: '中文', langSys: '跟随系统',
      themeLight: '亮色', themeDark: '暗色', themeSys: '跟随系统',
      s0title: '调成你喜欢的样子', s0sub: '挑个外观与语言——都即时生效',
      s1title: '连接模型', s1sub: '对话必需——钥匙只留在你的机器上',
      model: '模型', dialogue: '对话', keyPlaceholder: '粘贴你的 API 密钥',
      utilTitle: '辅助任务也用它', utilSub: '自动标题、上下文压缩、搜索摘要',
      privacy: '密钥以 AES-GCM 加密存于本地 SQLite，绝不离开你的机器。',
      searchH: '网络搜索（可选）',
      searchKeyPlaceholder: '粘贴你的搜索 API 密钥',
      searchFoot: '可选——让助手能联网搜索，可随时在设置中添加或更改。',
    },
  };
  let curLang = 'en';
  const lbl = k => (T[curLang] || T.en)[k] || k;
  const resolveLang = v => v === 'system' ? ((navigator.language || '').toLowerCase().startsWith('zh') ? 'zh-CN' : 'en') : v;

  /* 段控宿主（占位，build 后由 Segmented.mount 填充；语言段控触发整页重译） */
  let langSeg, themeSeg, fmtDD = null, modelDD = null;

  function applyLang(v) {
    curLang = resolveLang(v);
    const d = T[curLang];
    document.documentElement.lang = curLang;
    $$('[data-t]').forEach(el => { if (d[el.dataset.t] != null) el.textContent = d[el.dataset.t]; });
    $$('[data-tp]').forEach(el => { if (d[el.dataset.tp] != null) el.placeholder = d[el.dataset.tp]; });
    // :test 按钮文案随态重译（连通 / 测试中 / 待测）
    $$('.onb-test').forEach(b => b.querySelector('.onb-tt').textContent =
      b.classList.contains('ok') ? d.connected : b.classList.contains('testing') ? d.testing : d.test);
    render();
  }

  /* 主题即时落到 <html data-theme>（system 解析当前系统偏好） */
  function applyTheme(v) {
    const sysDark = matchMedia('(prefers-color-scheme: dark)').matches;
    document.documentElement.dataset.theme = v === 'system' ? (sysDark ? 'dark' : 'light') : v;
  }

  /* 异步引擎启动（mock）：表单先渲染，sidecar 后台唤醒，无状态 chrome */
  let ready = false;
  setTimeout(() => { ready = true; }, 1400);
  function whenReady(btn) {
    if (ready) return Promise.resolve();
    btn.classList.add('waiting');
    return new Promise(res => { const t = setInterval(() => { if (ready) { clearInterval(t); btn.classList.remove('waiting'); res(); } }, 80); });
  }

  /* ===== 共享 :test 接线（mock；仅连通性） ===== */
  function wireTest(btn, keyInput, onPass, onReset) {
    function reset() { btn.className = 'onb-test'; btn.querySelector('.onb-ico').innerHTML = ''; btn.querySelector('.onb-tt').textContent = lbl('test'); onReset && onReset(); syncNext(); }
    btn.onclick = async () => {
      if (!keyInput.value.trim()) { keyInput.focus(); return; }
      btn.className = 'onb-test testing'; btn.querySelector('.onb-ico').innerHTML = icon('spin', 15); btn.querySelector('.onb-tt').textContent = lbl('testing');
      await whenReady(btn);
      await new Promise(r => setTimeout(r, 900));
      btn.className = 'onb-test ok'; btn.querySelector('.onb-ico').innerHTML = icon('check', 15); btn.querySelector('.onb-tt').textContent = lbl('connected');
      onPass && onPass(); syncNext();
    };
    keyInput.addEventListener('input', () => { if (btn.classList.contains('ok')) reset(); else syncNext(); });
    return reset;
  }
  function nameField(input) { input.addEventListener('input', () => input.dataset.touched = '1'); }

  /* ===== 大模型供应商（GET /providers 子集）+ 模型目录（model-capabilities 发现 mock） ===== */
  const fmtCtx = n => n >= 1000000 ? (n / 1000000) + 'M' : n >= 1000 ? Math.round(n / 1000) + 'K' : n;
  const PROVIDERS = [
    { name: 'anthropic', label: 'Anthropic', base: 'https://api.anthropic.com' },
    { name: 'openai', label: 'OpenAI', base: 'https://api.openai.com/v1' },
    { name: 'google', label: 'Google', base: 'https://generativelanguage.googleapis.com/v1beta' },
    { name: 'deepseek', label: 'DeepSeek', base: 'https://api.deepseek.com' },
    { name: 'qwen', label: 'Qwen', base: 'https://dashscope.aliyuncs.com/compatible-mode/v1' },
    { name: 'zhipu', label: 'Zhipu', base: 'https://open.bigmodel.cn/api/paas/v4' },
    { name: 'moonshot', label: 'Moonshot', base: 'https://api.moonshot.cn/v1' },
    { name: 'doubao', label: 'Doubao', base: 'https://ark.cn-beijing.volces.com/api/v3' },
    { name: 'openrouter', label: 'OpenRouter', base: 'https://openrouter.ai/api/v1' },
    { name: 'ollama', label: 'Ollama', base: 'http://localhost:11434' },
    { name: 'custom', label: 'Custom', base: '' },
  ];
  const MODELS = {
    anthropic: [['claude-opus-4-8', 'Claude Opus 4.8', 200000, true], ['claude-sonnet-4-6', 'Claude Sonnet 4.6', 200000, true], ['claude-haiku-4-5', 'Claude Haiku 4.5', 200000, false]],
    openai: [['gpt-5.1', 'GPT-5.1', 256000, true], ['gpt-5.1-mini', 'GPT-5.1 mini', 256000, true], ['o4', 'o4', 200000, false]],
    google: [['gemini-3-pro', 'Gemini 3 Pro', 1000000, true], ['gemini-3-flash', 'Gemini 3 Flash', 1000000, true]],
    deepseek: [['deepseek-v4', 'DeepSeek V4', 128000, false], ['deepseek-v4-flash', 'DeepSeek V4 Flash', 128000, false], ['deepseek-r2', 'DeepSeek R2', 128000, false]],
    qwen: [['qwen3-max', 'Qwen3 Max', 256000, false], ['qwen3-vl', 'Qwen3-VL', 128000, true]],
    zhipu: [['glm-5', 'GLM-5', 128000, false]], moonshot: [['kimi-k2.5', 'Kimi K2.5', 256000, false]],
    doubao: [['doubao-pro-4', 'Doubao Pro 4', 256000, false]], openrouter: [['auto', 'Auto (router)', 200000, false]],
    ollama: [['llama4', 'llama4', 128000, false], ['qwen3', 'qwen3', 128000, false]], custom: [['custom-model', 'from /models', 0, false]],
  };
  let prov = PROVIDERS[0], modelSet = false, resetTest = () => {};

  /* ===== 搜索供应商（websearch.Providers；设工作区默认搜索；可选） ===== */
  const SEARCH = [{ name: 'brave', label: 'Brave' }, { name: 'serper', label: 'Serper' }, { name: 'tavily', label: 'Tavily' }, { name: 'bocha', label: 'Bocha' }];
  let sprov = SEARCH[0], resetSTest = () => {};

  function modelOpts() {
    return (MODELS[prov.name] || []).map(([id, nm, ctx, vis]) => ({ value: id, label: nm, meta: (ctx ? fmtCtx(ctx) : '') + (vis ? ' · vision' : '') }));
  }

  /* ===== 步进器：外观+语言(0) → API 密钥(1) ===== */
  let step = 0;
  const APP = '../../app.html';
  function modelChosen() { return $('#onbTestBtn').classList.contains('ok') && modelSet; }
  function syncNext() {
    const ok = step === 0 ? true : modelChosen();
    $('#onbNext').toggleAttribute('disabled', !ok);
  }
  function render() {
    const d = T[curLang];
    $$('.onb-step').forEach(s => s.classList.toggle('on', +s.dataset.s === step));
    $$('.onb-dots i').forEach(dd => dd.classList.toggle('on', +dd.dataset.d === step));
    $('#onb').classList.toggle('wide', step > 0);
    $('#onbBack').style.visibility = step ? 'visible' : 'hidden';
    $('#onbTitle').textContent = d['s' + step + 'title'];
    $('#onbSub').textContent = d['s' + step + 'sub'];
    $('#onbNext .onb-tt').textContent = step === 1 ? d.enter : d.next;
    syncNext();
  }

  function selectVendor(name) {
    prov = PROVIDERS.find(p => p.name === name);
    $$('#onbVgrid .onb-vendor').forEach(v => v.classList.toggle('on', v.dataset.p === name));
    $('#onbVname').textContent = prov.label;
    if (!$('#onbKeyName').dataset.touched) $('#onbKeyName').value = prov.label;
    resetTest();
  }
  function selectSearch(name) {
    sprov = SEARCH.find(p => p.name === name);
    $$('#onbSgrid .onb-vendor').forEach(v => v.classList.toggle('on', v.dataset.p === name));
    $('#onbSvname').textContent = sprov.label;
    if (!$('#onbSKeyName').dataset.touched) $('#onbSKeyName').value = sprov.label;
    resetSTest();
  }

  /* ===== 装配：注入 SVG → 挂段控/下拉/供应商网格 → 接线 ===== */
  function boot() {
    // 品牌徽 Foryx（像素 F：6 方块；固定黑白品牌资产，非 CSS 不受裸色令牌约束）+ data-ico 占位图标
    $('#onbMark').innerHTML = `<svg viewBox="0 0 512 512" width="44" height="44" role="img" aria-label="Foryx"><rect width="512" height="512" rx="114" fill="#ffffff"/><g fill="#141414"><rect x="106" y="106" width="88" height="88"/><rect x="212" y="106" width="88" height="88"/><rect x="318" y="106" width="88" height="88"/><rect x="106" y="212" width="88" height="88"/><rect x="212" y="212" width="88" height="88"/><rect x="106" y="318" width="88" height="88"/></g></svg>`;
    $$('[data-ico]').forEach(el => el.innerHTML = icon(el.dataset.ico, +el.dataset.n || 14, el.classList.contains('onb-box') ? 2.6 : 1.7));

    // 语言段控（组件）：切换即重译整页
    Segmented.mount($('#onbLang'), [
      { value: 'en', label: T.en.langEn }, { value: 'zh-CN', label: T.en.langZh }, { value: 'system', label: T.en.langSys },
    ], { value: 'en', onPick: applyLang });
    // 外观段控（组件）：切换即落 data-theme
    Segmented.mount($('#onbTheme'), [
      { value: 'light', label: T.en.themeLight }, { value: 'dark', label: T.en.themeDark }, { value: 'system', label: T.en.themeSys },
    ], { value: 'light', onPick: applyTheme });

    // 大模型供应商网格
    $('#onbVgrid').innerHTML = PROVIDERS.map((p, i) => `<div class="onb-vendor${i ? '' : ' on'}" data-p="${p.name}">${p.label}</div>`).join('');
    $$('#onbVgrid .onb-vendor').forEach(v => v.onclick = () => selectVendor(v.dataset.p));

    // 搜索供应商网格
    $('#onbSgrid').innerHTML = SEARCH.map((p, i) => `<div class="onb-vendor${i ? '' : ' on'}" data-p="${p.name}">${p.label}</div>`).join('');
    $$('#onbSgrid .onb-vendor').forEach(v => v.onclick = () => selectSearch(v.dataset.p));

    // :test 接线：大模型通过后揭示模型选择（Dropdown 组件）
    resetTest = wireTest($('#onbTestBtn'), $('#onbApiKey'),
      () => { modelDD = Dropdown.mount($('#onbModelDD'), { options: modelOpts(), onChange: () => syncNext() }); modelSet = true; $('#onbReveal').classList.add('show'); },
      () => { modelSet = false; $('#onbReveal').classList.remove('show'); $('#onbModelDD').innerHTML = ''; modelDD = null; });
    resetSTest = wireTest($('#onbSTestBtn'), $('#onbSApiKey'));
    nameField($('#onbKeyName')); nameField($('#onbSKeyName'));
    $('#onbUtil').onclick = () => $('#onbUtil').classList.toggle('on');

    // 导航
    $('#onbNext').onclick = async () => { await whenReady($('#onbNext')); if (step < 1) { step++; render(); } else location.href = APP; };
    $('#onbBack').onclick = () => { if (step) { step--; render(); } };

    selectVendor('anthropic');
    selectSearch('brave');
    applyTheme('light');
    applyLang('en');   // 落地默认语言（同时调 render）
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
