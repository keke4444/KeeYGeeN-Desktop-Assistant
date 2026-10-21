'use strict';

/* ---------------- 图标 ---------------- */
const ICONS = {
  folder: '<path d="M3 7.6A1.6 1.6 0 0 1 4.6 6h4l1.9 2.4h8.9A1.6 1.6 0 0 1 21 10v8.4A1.6 1.6 0 0 1 19.4 20H4.6A1.6 1.6 0 0 1 3 18.4z"/>',
  home: '<path d="m4 10.5 8-6.5 8 6.5"/><path d="M6.5 9.5V19a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1V9.5"/><path d="M10 20v-5h4v5"/>',
  desktop: '<rect x="3" y="5" width="18" height="12" rx="1.6"/><path d="M9 20h6M12 17v3"/>',
  image: '<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><circle cx="8.6" cy="9.4" r="1.6"/><path d="m4.6 17 4.6-4.6 2.9 2.9 2.4-2.4 4.9 4.9"/>',
  video: '<rect x="3.5" y="6" width="12" height="12" rx="2"/><path d="m16 11 4.5-2.6v7.2L16 13z"/>',
  audio: '<path d="M9 17V6l9-2v11"/><circle cx="6.6" cy="17.4" r="2.4"/><circle cx="15.6" cy="15.4" r="2.4"/>',
  archive: '<path d="M4 7h16v11.5a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 4 18.5z"/><path d="M4 7V5.5A1.5 1.5 0 0 1 5.5 4h13A1.5 1.5 0 0 1 20 5.5V7"/><path d="M11 4v4.5"/><path d="M13 8.5h-2v3h2z"/>',
  exe: '<rect x="4" y="4" width="16" height="16" rx="3"/><path d="m9.2 9 2.4 3-2.4 3"/><path d="M13.4 15h2.6"/>',
  code: '<path d="m9 8-4 4 4 4"/><path d="m15 8 4 4-4 4"/><path d="m13.4 6.5-2.8 11"/>',
  doc: '<path d="M6 3.5h7.5L18 8v12.5H6z"/><path d="M13.5 3.5V8H18"/><path d="M8.8 12h6.4M8.8 15.2h6.4M8.8 18.2h3.8"/>',
  link: '<path d="M10.4 13.6a3 3 0 0 0 4.3 0l3-3a3 3 0 0 0-4.3-4.3l-1 1"/><path d="M13.6 10.4a3 3 0 0 0-4.3 0l-3 3a3 3 0 0 0 4.3 4.3l1-1"/>',
  file: '<path d="M6 3.5h7.5L18 8v12.5H6z"/><path d="M13.5 3.5V8H18"/>',
  clock: '<circle cx="12" cy="12" r="8.4"/><path d="M12 7.3V12l3.1 2"/>',
};

const TYPE_LABEL = {
  folder: '文件夹', image: '图片', video: '视频', audio: '音频',
  archive: '压缩包', exe: '程序', code: '代码', doc: '文档', link: '快捷方式', file: '文件',
};

function svg(name) {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">${ICONS[name] || ICONS.file}</svg>`;
}

// iconMarkup 优先使用 Windows 真实图标（后端返回 data URL），失败时回退到内置矢量图标。
function iconMarkup(kind, iconUrl) {
  if (iconUrl) {
    return `<img class="ico-img" src="${iconUrl}" alt="" draggable="false">`;
  }
  return svg(kind);
}

/* ---------------- 状态 ---------------- */
const state = {
  meta: null,        // GetState
  listing: null,     // ListDir
  selected: new Set(),
  anchor: null,
  busy: false,
  view: localStorage.getItem('dtidy.view') || 'grid',
  settings: null,
  // 「最近」视图：active=true 时文件区展示最近记录；task 为空表示「默认最近」
  recent: { active: false, task: '', groups: [], items: [] },
};

// 搜索状态：active=true 时列表区展示搜索结果；popOpen=true 时才显示候选下拉
const searchState = { active: false, query: '', results: [], timer: null, popIndex: 0, popOpen: false };

// 搜索范围：global=桌面/home/已打开文件夹；current=当前所在文件夹。
// 默认「当前」，让文件夹/分类内的搜索天然带范围筛选；用户手动切换后记住选择。
// 存储键带版本号，避免沿用旧版本遗留的「全局」偏好。
const SEARCH_SCOPE_KEY = 'dtidy.searchScope.v2';
let searchScope = 'current';
try {
  const saved = localStorage.getItem(SEARCH_SCOPE_KEY);
  if (saved === 'global' || saved === 'current') searchScope = saved;
} catch (_) {}

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s == null ? '' : s)
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;');

function toast(msg) {
  const el = $('toast');
  el.textContent = msg;
  el.hidden = false;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => { el.hidden = true; }, 2600);
}

/* ---------------- 数据 ---------------- */
async function loadMeta() {
  const m = await window.go.main.App.GetState();
  applyMeta(m);
}

// applyMeta 统一刷新侧栏、路径与侧栏宽度。
function applyMeta(m) {
  state.meta = m;
  renderSidebar();
  $('desktopPath').textContent = m.desktop;
  $('desktopPath').title = m.desktop;
  $('verText').textContent = 'v' + m.version + ' · ' + m.root;
  document.documentElement.style.setProperty('--sidebar-width', (m.sidebarWidth || 258) + 'px');
  layoutBg();
  if (m.loadError) toast(m.loadError);
}

async function loadDir(path) {
  state.listing = await window.go.main.App.ListDir(path || '');
  state.selected.clear();
  state.anchor = null;
  resetSearch();
  $('searchInput').value = '';
  renderCrumbs();
  renderTable();
  applyView();
  renderSidebarActive();
  renderStatus();
  applySearchScopeUi();
}

/* ---------------- 搜索 ---------------- */
function resetSearch() {
  searchState.active = false;
  searchState.query = '';
  searchState.results = [];
  searchState.popIndex = 0;
  clearTimeout(searchState.timer);
  $('btnClearSearch').hidden = true;
  hideSearchPop();
}

function clearSearch() {
  const had = searchState.active || $('searchInput').value !== '';
  $('searchInput').value = '';
  resetSearch();
  if (had && state.listing) {
    renderCrumbs();
    renderTable();
    renderStatus();
  }
}

// candLimit 返回设置里的候选数量（默认 8）。
function candLimit() {
  const n = state.settings && state.settings.searchCandidates;
  return Math.min(30, Math.max(3, n || 8));
}

async function runSearch(q) {
  if (!q) { clearSearch(); return; }
  try {
    const scopeDir = searchScope === 'current' && state.listing ? state.listing.path : '';
    const list = await window.go.main.App.Search(q, scopeDir);
    // 输入已变化时丢弃过期结果
    if (($('searchInput').value || '').trim() !== q) return;
    searchState.active = true;
    searchState.query = q;
    searchState.results = Array.isArray(list) ? list : [];
    searchState.popIndex = 0;
    state.selected.clear();
    $('btnClearSearch').hidden = false;
    renderCrumbs();
    renderTable();
    applyView();
    renderStatus();
    renderSearchPop();
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

function applySearchScopeUi() {
  for (const b of $('searchScope').querySelectorAll('button')) {
    b.classList.toggle('active', b.dataset.scope === searchScope);
  }
  const input = $('searchInput');
  input.placeholder = searchScope === 'current' ? '在当前文件夹中搜索…' : '搜索桌面和 home…';
  input.title = searchScope === 'current'
    ? '只在当前文件夹内搜索（含子文件夹）；输入后按 Tab 显示候选'
    : '搜索桌面、home 与已打开文件夹；输入后按 Tab 显示候选';
}

// 搜索结果的来源说明（全局 / 当前文件夹，带文件夹名）。
function searchScopeLabel() {
  if (searchScope !== 'current') return '全局';
  const name = state.listing && state.listing.name;
  return name ? `当前：${name}` : '当前文件夹';
}

function hideSearchPop() {
  const pop = $('searchPop');
  searchState.popOpen = false;
  if (pop) pop.hidden = true;
}

// renderSearchPop 渲染搜索框下方的候选列表（渲染全部，超出可视条数后滚动）。
function renderSearchPop() {
  const pop = $('searchPop');
  if (!searchState.active || !searchState.popOpen) { pop.hidden = true; return; }

  const items = searchState.results;
  pop.innerHTML = '';
  // 可视高度按「候选数量」设置，其余靠滚动查看
  pop.style.maxHeight = Math.max(150, candLimit() * 30 + 42) + 'px';
  if (items.length === 0) {
    pop.innerHTML = '<div class="search-empty">没有匹配项</div>';
    pop.hidden = false;
    return;
  }
  if (searchState.popIndex >= items.length) searchState.popIndex = items.length - 1;

  items.forEach((e, i) => {
    const b = document.createElement('button');
    b.className = 'search-item' + (i === searchState.popIndex ? ' on' : '');
    b.innerHTML = `
      <span class="num">${i + 1}.</span>
      <span class="nm" title="${esc(e.path)}">${esc(e.name)}</span>
      <span class="tp">${e.isDir ? '文件夹' : esc((e.ext || '').toUpperCase() || '文件')}</span>`;
    b.onmouseenter = () => {
      searchState.popIndex = i;
      paintCandidate(false);
    };
    b.onmousedown = (ev) => {
      ev.preventDefault();
      activateCandidate(i);
    };
    pop.appendChild(b);
  });

  const foot = document.createElement('div');
  foot.className = 'search-foot';
  pop.appendChild(foot);
  pop.hidden = false;
  paintCandidate(true);
}

// paintCandidate 只更新高亮与底栏文字，不重建列表。
function paintCandidate(scroll) {
  const pop = $('searchPop');
  const els = pop.querySelectorAll('.search-item');
  els.forEach((el, k) => el.classList.toggle('on', k === searchState.popIndex));
  const foot = pop.querySelector('.search-foot');
  if (foot) {
    foot.textContent = `第 ${searchState.popIndex + 1} / ${searchState.results.length} 条 · ↑↓ 或 − + 切换 · Enter 打开 · Esc 关闭`;
  }
  if (scroll) {
    const el = els[searchState.popIndex];
    if (el) el.scrollIntoView({ block: 'nearest' });
  }
}

function moveCandidate(delta) {
  if (!searchState.active || !searchState.popOpen) return;
  const n = searchState.results.length;
  if (n === 0) return;
  const next = Math.min(n - 1, Math.max(0, searchState.popIndex + delta));
  if (next === searchState.popIndex) {
    paintCandidate(true);
    return;
  }
  searchState.popIndex = next;
  paintCandidate(true);
}

function activateCandidate(i) {
  const e = searchState.results[i];
  if (!e) return;
  hideSearchPop();
  if (e.isDir) {
    nav(e.path);
  } else {
    window.go.main.App.OpenFile(e.path).catch((err) => toast(String(err)));
  }
}

function bindSearch() {
  const input = $('searchInput');
  input.addEventListener('input', () => {
    const q = input.value.trim();
    $('btnClearSearch').hidden = q === '';
    clearTimeout(searchState.timer);
    hideSearchPop();
    if (!q) { clearSearch(); return; }
    searchState.timer = setTimeout(() => runSearch(q), 260);
  });
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      e.stopPropagation();
      if (searchState.popOpen) {
        hideSearchPop();
      } else {
        clearSearch();
        input.blur();
      }
      return;
    }
    if (e.key === 'Tab') {
      // 先按 Tab 才会出现候选下拉；再按一次收起
      e.preventDefault();
      if (!searchState.active) return;
      if (searchState.popOpen) {
        hideSearchPop();
      } else {
        searchState.popOpen = true;
        searchState.popIndex = 0;
        renderSearchPop();
      }
      return;
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      if (!searchState.popOpen) return;
      e.preventDefault();
      moveCandidate(e.key === 'ArrowDown' ? 1 : -1);
      return;
    }
    if (e.key === '-' || e.key === '_' || e.key === '+' || e.key === '=') {
      if (!searchState.popOpen) return;
      e.preventDefault();
      moveCandidate(e.key === '-' || e.key === '_' ? -1 : 1);
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      clearTimeout(searchState.timer);
      if (searchState.popOpen) {
        activateCandidate(searchState.popIndex);
      } else {
        runSearch(input.value.trim());
      }
    }
  });
  $('btnClearSearch').onclick = () => {
    clearSearch();
    input.focus();
  };
  $('searchScope').onclick = (e) => {
    const b = e.target.closest('button');
    if (!b) return;
    searchScope = b.dataset.scope === 'current' ? 'current' : 'global';
    try { localStorage.setItem(SEARCH_SCOPE_KEY, searchScope); } catch (_) {}
    applySearchScopeUi();
    if (searchState.active) {
      runSearch(searchState.query);
    } else {
      input.focus();
    }
  };
  applySearchScopeUi();
}

/* ---------------- 配色 ---------------- */
function hexRgb(hex) {
  const h = String(hex || '').replace('#', '');
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return [233, 84, 32];
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}

function mixArr(rgb, other, t) {
  return rgb.map((v, i) => Math.round(v + (other[i] - v) * t));
}

function rgbCss(arr) { return `rgb(${arr.join(', ')})`; }

// darkenHex 把颜色压暗（用于「整体颜色」带出界面底色）。
function darkenHex(hex, amount) {
  const rgb = hexRgb(hex).map((v) => Math.round(v * (1 - amount)));
  return '#' + rgb.map((v) => v.toString(16).padStart(2, '0')).join('');
}

function applyTheme(s) {
  if (!s) return;
  const root = document.documentElement;
  const accent = /^#[0-9a-fA-F]{6}$/.test(s.themeAccent || '') ? s.themeAccent : '#e95420';
  const pageCustom = s.themePageCustom && /^#[0-9a-fA-F]{6}$/.test(s.themePage || '');
  // 未单独指定页面颜色时，整个界面跟随「整体颜色」的深色版本
  const page = pageCustom ? s.themePage : darkenHex(accent, 0.82);
  const a = hexRgb(accent);
  const p = hexRgb(page);

  root.style.setProperty('--orange', accent);
  root.style.setProperty('--orange-rgb', a.join(', '));
  root.style.setProperty('--accent-light', rgbCss(mixArr(a, [255, 255, 255], 0.42)));
  root.style.setProperty('--accent-strong', rgbCss(mixArr(a, [255, 255, 255], 0.12)));

  root.style.setProperty('--bg', page);
  root.style.setProperty('--panel', rgbCss(mixArr(p, [255, 255, 255], 0.06)));
  root.style.setProperty('--panel-2', rgbCss(mixArr(p, [255, 255, 255], 0.11)));
  root.style.setProperty('--panel-elev', rgbCss(mixArr(p, [255, 255, 255], 0.15)));
  root.style.setProperty('--panel-a', `rgba(${p.join(', ')}, 0.86)`);
  root.style.setProperty('--panel-2-a', `rgba(${mixArr(p, [255, 255, 255], 0.11).join(', ')}, 0.86)`);
  root.style.setProperty('--input-bg', rgbCss(mixArr(p, [0, 0, 0], 0.22)));

  const back = (!s.followThemeBack && /^#[0-9a-fA-F]{6}$/.test(s.themeBack || '')) ? s.themeBack : accent;
  root.style.setProperty('--back-color', back);

  // 「最近」任务标签：可跟随整体颜色，也可单独设色
  const recent = (!s.followRecent && /^#[0-9a-fA-F]{6}$/.test(s.themeRecent || '')) ? s.themeRecent : accent;
  root.style.setProperty('--recent-tab-color', recent);
  root.style.setProperty('--recent-tab-rgb', hexRgb(recent).join(', '));
}

/* ---------------- 字体 ---------------- */
// 分区字体覆盖：选择器与设置里的「分区」一一对应
const FONT_PARTS = {
  sidebar: '.sidebar',
  pathbar: '.pathbar, .recent-tabbar',
  file: '.table',
  settings: '.settings-modal',
};
const FONT_FALLBACK = '"Microsoft YaHei UI", "Microsoft YaHei", system-ui, sans-serif';

function fontStack(name) {
  return name ? `"${name}", ${FONT_FALLBACK}` : '';
}

// applyFont 把全局/分区字体、字号、文字颜色写进 CSS 变量（CSS 里所有字号都用 --fs-scale 缩放）
function applyFont(s) {
  if (!s) return;
  const root = document.documentElement;
  const fam = fontStack(s.fontFamily);
  if (fam) root.style.setProperty('--app-font', fam);
  else root.style.removeProperty('--app-font');

  const size = Number(s.fontSize) || 13.5;
  root.style.setProperty('--fs-scale', (size / 13.5).toFixed(3));

  if (s.fontColor) {
    root.style.setProperty('--text', s.fontColor);
    root.style.setProperty('--muted', s.fontColor + 'b3');
  } else {
    root.style.removeProperty('--text');
    root.style.removeProperty('--muted');
  }

  const parts = s.fontParts || {};
  for (const [key, sel] of Object.entries(FONT_PARTS)) {
    const el = document.querySelector(sel);
    if (!el) continue;
    const p = parts[key] || {};
    if (p.family) el.style.setProperty('--part-font', fontStack(p.family));
    else el.style.removeProperty('--part-font');
    if (p.size) el.style.setProperty('--fs-scale', (Number(p.size) / 13.5).toFixed(3));
    else el.style.removeProperty('--fs-scale');
    if (p.color) {
      el.style.setProperty('--text', p.color);
      el.style.setProperty('--muted', p.color + 'b3');
    } else {
      el.style.removeProperty('--text');
      el.style.removeProperty('--muted');
    }
  }
}

function applyBgFit(fit) {
  const layer = $('bgLayer');
  const ok = ['contain', 'cover', 'stretch', 'tile'].includes(fit) ? fit : 'contain';
  layer.classList.remove('fit-contain', 'fit-cover', 'fit-stretch', 'fit-tile');
  layer.classList.add('fit-' + ok);
}

/* ---------------- 背景作用域 ---------------- */
// scope: window=全局，main=仅文件区，custom=拉选区域；rect 为窗口比例 [x,y,w,h]
const bgState = { scope: 'main', rect: [0, 0, 1, 1] };

// layoutBg 按当前作用域计算背景层的位置和大小（文件区会跟随侧边栏位置）。
function layoutBg() {
  const layer = $('bgLayer');
  if (!layer) return;
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  let x = 0, y = 0, w = vw, h = vh;

  if (bgState.scope === 'custom') {
    const r = Array.isArray(bgState.rect) && bgState.rect.length === 4 ? bgState.rect : [0, 0, 1, 1];
    x = r[0] * vw; y = r[1] * vh; w = r[2] * vw; h = r[3] * vh;
  } else if (bgState.scope === 'main') {
    const main = document.querySelector('.main');
    if (main) {
      const rc = main.getBoundingClientRect();
      x = rc.left; y = rc.top; w = rc.width; h = rc.height;
    }
  }
  layer.style.left = Math.round(x) + 'px';
  layer.style.top = Math.round(y) + 'px';
  layer.style.width = Math.round(w) + 'px';
  layer.style.height = Math.round(h) + 'px';
}

// applyView 应用图标/列表视图（不影响数据，只切样式与表头）。
function applyView() {
  const grid = state.view === 'grid';
  const body = $('tbody');
  if (body) body.classList.toggle('grid-mode', grid);
  const head = $('thead');
  if (head) head.hidden = grid;
  $('btnViewGrid').classList.toggle('active', grid);
  $('btnViewList').classList.toggle('active', !grid);
}

function setView(view) {
  state.view = view;
  try { localStorage.setItem('dtidy.view', view); } catch (_) {}
  applyView();
}

/* ---------------- 设置：背景 / 热键 / 分类栏位置 / 缩放 ---------------- */
async function loadSettings() {
  try {
    const s = await window.go.main.App.GetSettings();
    state.settings = s;
    applyUiSettings(s);
    await refreshBackground();
  } catch (err) {
    toast('读取设置失败：' + String(err));
  }
}

function applyUiSettings(s) {
  if (!s) return;
  const root = document.documentElement;
  root.style.setProperty('--bg-opacity', String(s.bgOpacity == null ? 0.35 : s.bgOpacity));
  root.style.setProperty('--icon-scale', String(s.iconScale == null ? 1 : s.iconScale));
  applyTheme(s);
  applyFont(s);
  applyBgFit(s.bgFit);
  bgState.scope = s.bgScope || 'main';
  bgState.rect = Array.isArray(s.bgRect) && s.bgRect.length === 4 ? s.bgRect.slice() : [0, 0, 1, 1];
  const app = $('appRoot');
  app.classList.remove('pos-left', 'pos-right', 'pos-top');
  app.classList.add('pos-' + (s.categoriesPosition || 'left'));
  layoutBg();
}

async function refreshBackground() {
  const layer = $('bgLayer');
  try {
    const url = await window.go.main.App.BackgroundURL();
    if (url) {
      layer.style.backgroundImage = `url("${url}")`;
      layer.classList.add('on');
      $('appRoot').classList.add('has-bg');
    } else {
      layer.style.backgroundImage = '';
      layer.classList.remove('on');
      $('appRoot').classList.remove('has-bg');
    }
  } catch (_) {
    layer.classList.remove('on');
    $('appRoot').classList.remove('has-bg');
  }
}

function openSettings() {
  const s = state.settings || {};
  $('hotkeyInput').value = s.hotkey || '';
  $('hotkeyHint').textContent = s.hotkeyError
    ? '注册失败：' + s.hotkeyError
    : '点击输入框后按下组合键；Esc 清除；保存后立即生效';
  $('bgName').textContent = s.hasBackground ? s.bgName : '未设置';
  const opacity = Math.round((s.bgOpacity == null ? 0.35 : s.bgOpacity) * 100);
  $('opacityRange').value = String(opacity);
  $('opacityVal').textContent = opacity + '%';
  const scale = Math.round((s.iconScale == null ? 1 : s.iconScale) * 100);
  $('scaleRange').value = String(scale);
  $('scaleVal').textContent = scale + '%';
  for (const b of $('posSeg').querySelectorAll('button')) {
    b.classList.toggle('active', b.dataset.pos === (s.categoriesPosition || 'left'));
  }
  for (const b of $('fitSeg').querySelectorAll('button')) {
    b.classList.toggle('active', b.dataset.fit === (s.bgFit || 'contain'));
  }
  setScopeUI(s.bgScope || 'main');
  $('accentColor').value = s.themeAccent || '#e95420';
  pageTouched = s.themePageCustom === true;
  $('pageColor').value = pageTouched
    ? (s.themePage || '#2c001e')
    : darkenHex(s.themeAccent || '#e95420', 0.82);
  $('followBack').checked = s.followThemeBack !== false;
  $('backColor').value = s.themeBack || s.themeAccent || '#e95420';
  $('backColor').hidden = $('followBack').checked;
  $('followRecent').checked = s.followRecent !== false;
  $('recentColor').value = s.themeRecent || s.themeAccent || '#e95420';
  $('recentColor').hidden = $('followRecent').checked;

  fontPartsDraft = JSON.parse(JSON.stringify(s.fontParts || {}));
  fontPartCurrent = 'sidebar';
  $('fontFamily').value = s.fontFamily || '';
  $('fontSize').value = s.fontSize || 13.5;
  $('fontColorFollow').checked = !s.fontColor;
  $('fontColor').value = s.fontColor || '#e6e6e6';
  $('fontColor').hidden = $('fontColorFollow').checked;
  setFontPartUI(fontPartCurrent);

  const desktop = s.desktopPath || (state.meta ? state.meta.desktop : '');
  const home = s.homePath || (state.meta ? state.meta.root : '');
  $('desktopPathVal').textContent = desktop || '未设置';
  $('desktopPathVal').title = desktop;
  $('homePathVal').textContent = home || '未设置';
  $('homePathVal').title = home;
  setCandInput(s.searchCandidates || 8);
  applyRuleSettings(s);
  $('autoStartChk').checked = !!s.autoStart;
  $('closeToTaskbarChk').checked = !!s.closeToTaskbar;
  $('autoStartHint').textContent = s.autoStartError
    ? '上次写入失败：' + s.autoStartError
    : '勾选并保存后写入系统启动项';

  $('settingsMask').hidden = false;
}

function closeSettings() {
  $('settingsMask').hidden = true;
  // 放弃未保存的预览，恢复成已保存的设置
  if (state.settings) applyUiSettings(state.settings);
}

// previewTheme 在设置面板里实时预览配色（不落盘）。
let pageTouched = false;

/* ---------------- 字体设置 ---------------- */
let fontPartsDraft = {};
let fontPartCurrent = 'sidebar';

function setFontPartUI(key) {
  $('fontPart').value = key;
  const p = fontPartsDraft[key] || {};
  $('partFamily').value = p.family || '';
  $('partSize').value = p.size ? String(p.size) : '';
  $('partColorFollow').checked = !p.color;
  $('partColor').value = p.color || '#e6e6e6';
  $('partColor').hidden = $('partColorFollow').checked;
}

function captureFontPart(key) {
  const size = Number($('partSize').value) || 0;
  const p = {
    family: $('partFamily').value,
    size: size,
    color: $('partColorFollow').checked ? '' : $('partColor').value,
  };
  if (!p.family && !p.size && !p.color) delete fontPartsDraft[key];
  else fontPartsDraft[key] = p;
}

function previewFont() {
  applyFont(Object.assign({}, state.settings || {}, {
    fontFamily: $('fontFamily').value,
    fontSize: Number($('fontSize').value) || 13.5,
    fontColor: $('fontColorFollow').checked ? '' : $('fontColor').value,
    fontParts: fontPartsDraft,
  }));
}

function previewTheme() {
  const fitBtn = $('fitSeg').querySelector('button.active');
  const s = Object.assign({}, state.settings || {}, {
    themeAccent: $('accentColor').value,
    themePage: $('pageColor').value,
    themePageCustom: pageTouched,
    followThemeBack: $('followBack').checked,
    themeBack: $('followBack').checked ? '' : $('backColor').value,
    followRecent: $('followRecent').checked,
    themeRecent: $('followRecent').checked ? '' : $('recentColor').value,
    bgFit: fitBtn ? fitBtn.dataset.fit : 'contain',
  });
  applyTheme(s);
  applyFont(s);
  applyBgFit(s.bgFit);
}

/* ---------------- 背景作用域设置 ---------------- */
function setScopeUI(scope) {
  for (const b of $('scopeSeg').querySelectorAll('button')) {
    b.classList.toggle('active', b.dataset.scope === scope);
  }
  $('customScopeWrap').hidden = scope !== 'custom';
  updateRegionName();
}

function updateRegionName() {
  const r = Array.isArray(bgState.rect) && bgState.rect.length === 4 ? bgState.rect : [0, 0, 1, 1];
  $('regionName').textContent = `当前选区 ${Math.round(r[2] * 100)}% × ${Math.round(r[3] * 100)}%`;
}

/* ---------------- 拉选框选背景区域 ---------------- */
const region = { active: false, start: null };

function startRegionSelect() {
  $('settingsMask').hidden = true;
  $('regionMask').hidden = false;
  $('regionBox').hidden = true;
  region.active = true;
  region.start = null;
  document.body.classList.add('region-mode');
}

function cancelRegionSelect() {
  region.active = false;
  region.start = null;
  $('regionMask').hidden = true;
  $('regionBox').hidden = true;
  document.body.classList.remove('region-mode');
}

function setRegionBox(x, y, w, h) {
  const box = $('regionBox');
  box.hidden = false;
  box.style.left = x + 'px';
  box.style.top = y + 'px';
  box.style.width = w + 'px';
  box.style.height = h + 'px';
  box.dataset.size = `${Math.round(w)} × ${Math.round(h)}`;
}

async function saveRegion(rect) {
  try {
    const s = await window.go.main.App.SaveSettings(settingsPayload({ bgScope: 'custom', bgRect: rect }));
    state.settings = s;
    bgState.scope = 'custom';
    bgState.rect = Array.isArray(s.bgRect) && s.bgRect.length === 4 ? s.bgRect : rect;
    layoutBg();
    updateRegionName();
    toast('背景作用域已更新为拉选区域');
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

/* ---------------- 搜索候选数量 ---------------- */
function clampCand(v) {
  const n = Number(v);
  if (!Number.isFinite(n)) return 8;
  return Math.min(30, Math.max(3, Math.round(n)));
}

function setCandInput(v) {
  const n = clampCand(v);
  $('candInput').value = String(n);
  $('candVal').textContent = String(n);
  return n;
}

function candStep(delta) {
  setCandInput(clampCand($('candInput').value) + delta);
  if (searchState.active) renderSearchPop();
}

/* ---------------- 桌面 / home 路径 ---------------- */
async function pickDesktop() {
  try {
    const s = await window.go.main.App.ChooseDesktop();
    state.settings = s;
    $('desktopPathVal').textContent = s.desktopPath || '未设置';
    $('desktopPathVal').title = s.desktopPath || '';
    $('homePathVal').textContent = s.homePath || '未设置';
    $('homePathVal').title = s.homePath || '';
    await loadMeta();
    await loadDir('');
    toast('桌面路径已更新：' + (s.desktopPath || ''));
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function pickHome() {
  try {
    const s = await window.go.main.App.ChooseHome();
    state.settings = s;
    $('homePathVal').textContent = s.homePath || '未设置';
    $('homePathVal').title = s.homePath || '';
    await loadMeta();
    await loadDir('');
    toast('home 路径已更新：' + (s.homePath || ''));
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

/* ---------------- 候选规则文件 ---------------- */
function applyRuleSettings(s) {
  $('ruleName').textContent = s.candRuleName || '未设置';
  $('ruleName').title = s.candRuleFile || '';
  $('ruleHint').textContent = s.candRuleError
    ? '解析失败：' + s.candRuleError
    : '支持 .txt / .json / .csv / .tsv / .ini / .toml；txt 每行：+词=优先，-词=排除，# 注释';
}

async function pickRuleFile() {
  const before = state.settings ? state.settings.candRuleFile : '';
  try {
    const s = await window.go.main.App.ChooseCandidateRules();
    state.settings = s;
    applyRuleSettings(s);
    if (s.candRuleFile && s.candRuleFile !== before) {
      toast('候选规则已启用：' + s.candRuleName);
      if (searchState.active) await runSearch(searchState.query);
    }
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function clearRuleFile() {
  try {
    const s = await window.go.main.App.ClearCandidateRules();
    state.settings = s;
    applyRuleSettings(s);
    toast('已清除候选规则');
    if (searchState.active) await runSearch(searchState.query);
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

function normKey(e) {
  const k = e.key;
  if (['Control', 'Alt', 'Shift', 'Meta'].includes(k)) return '';
  if (k === ' ') return 'Space';
  if (/^[a-zA-Z0-9]$/.test(k)) return k.toUpperCase();
  if (/^F\d{1,2}$/.test(k)) return k.toUpperCase();
  const map = {
    ArrowLeft: 'Left', ArrowRight: 'Right', ArrowUp: 'Up', ArrowDown: 'Down',
    Enter: 'Enter', Tab: 'Tab', Escape: 'Escape', Backspace: 'Backspace', Delete: 'Delete',
    Home: 'Home', End: 'End', PageUp: 'PageUp', PageDown: 'PageDown',
  };
  return map[k] || '';
}

function bindHotkeyCapture() {
  const input = $('hotkeyInput');
  input.addEventListener('keydown', (e) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.key === 'Escape') {
      input.value = '';
      return;
    }
    const mods = [];
    if (e.ctrlKey) mods.push('Ctrl');
    if (e.altKey) mods.push('Alt');
    if (e.shiftKey) mods.push('Shift');
    if (e.metaKey) mods.push('Win');
    const key = normKey(e);
    if (!key) return;
    if (mods.length === 0) {
      $('hotkeyHint').textContent = '需要至少一个修饰键（Ctrl / Alt / Shift / Win）';
      return;
    }
    input.value = mods.concat([key]).join('+');
    $('hotkeyHint').textContent = '保存后立即生效';
  });
}

async function pickBackground() {
  try {
    const s = await window.go.main.App.ChooseBackground();
    state.settings = s;
    $('bgName').textContent = s.hasBackground ? s.bgName : '未设置';
    await refreshBackground();
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function clearBackground() {
  try {
    const s = await window.go.main.App.ClearBackground();
    state.settings = s;
    $('bgName').textContent = '未设置';
    await refreshBackground();
    toast('已清除背景');
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

// settingsPayload 以当前设置为底，叠加本次界面上的改动。
function settingsPayload(overrides) {
  const s = state.settings || {};
  const base = {
    bgImage: s.bgImage || '',
    bgName: s.bgName || '',
    hasBackground: !!s.hasBackground,
    bgOpacity: s.bgOpacity == null ? 0.35 : s.bgOpacity,
    bgFit: s.bgFit || 'contain',
    bgScope: s.bgScope || 'main',
    bgRect: Array.isArray(s.bgRect) && s.bgRect.length === 4 ? s.bgRect : [0, 0, 1, 1],
    categoriesPosition: s.categoriesPosition || 'left',
    iconScale: s.iconScale == null ? 1 : s.iconScale,
    hotkey: s.hotkey || '',
    themeAccent: s.themeAccent || '#e95420',
    themePage: s.themePage || '#2c001e',
    themePageCustom: !!s.themePageCustom,
    themeBack: s.themeBack || '',
    followThemeBack: s.followThemeBack !== false,
    themeRecent: s.themeRecent || '',
    followRecent: s.followRecent !== false,
    fontFamily: s.fontFamily || '',
    fontSize: s.fontSize || 13.5,
    fontColor: s.fontColor || '',
    fontParts: s.fontParts || {},
    searchCandidates: s.searchCandidates || 8,
    autoStart: !!s.autoStart,
    closeToTaskbar: !!s.closeToTaskbar,
  };
  return Object.assign(base, overrides || {});
}

async function saveSettings() {
  const posBtn = $('posSeg').querySelector('button.active');
  const fitBtn = $('fitSeg').querySelector('button.active');
  const scopeBtn = $('scopeSeg').querySelector('button.active');
  const follow = $('followBack').checked;
  const followRecent = $('followRecent').checked;
  captureFontPart(fontPartCurrent);
  const payload = settingsPayload({
    bgOpacity: Number($('opacityRange').value) / 100,
    bgFit: fitBtn ? fitBtn.dataset.fit : 'contain',
    bgScope: scopeBtn ? scopeBtn.dataset.scope : 'main',
    bgRect: bgState.rect,
    categoriesPosition: posBtn ? posBtn.dataset.pos : 'left',
    iconScale: Number($('scaleRange').value) / 100,
    hotkey: $('hotkeyInput').value.trim(),
    themeAccent: $('accentColor').value,
    themePage: $('pageColor').value,
    themePageCustom: pageTouched,
    themeBack: follow ? '' : $('backColor').value,
    followThemeBack: follow,
    themeRecent: followRecent ? '' : $('recentColor').value,
    followRecent,
    fontFamily: $('fontFamily').value,
    fontSize: Number($('fontSize').value) || 13.5,
    fontColor: $('fontColorFollow').checked ? '' : $('fontColor').value,
    fontParts: fontPartsDraft,
    searchCandidates: clampCand($('candInput').value),
    autoStart: $('autoStartChk').checked,
    closeToTaskbar: $('closeToTaskbarChk').checked,
  });
  try {
    const s = await window.go.main.App.SaveSettings(payload);
    state.settings = s;
    applyUiSettings(s);
    if (s.hotkeyError) {
      $('hotkeyHint').textContent = '注册失败：' + s.hotkeyError;
      toast('热键注册失败：' + s.hotkeyError);
      return;
    }
    if (s.autoStartError) {
      $('autoStartHint').textContent = '写入失败：' + s.autoStartError;
      toast('开机启动设置失败：' + s.autoStartError);
    }
    $('settingsMask').hidden = true;
    toast('设置已保存');
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

let zoomTimer = null;

function bindZoom() {
  const body = $('tbody');
  body.addEventListener('wheel', (e) => {
    if (!e.ctrlKey) return;
    e.preventDefault();
    const cur = state.settings && state.settings.iconScale ? state.settings.iconScale : 1;
    const delta = e.deltaY < 0 ? 0.08 : -0.08;
    const next = Math.min(2, Math.max(0.6, Math.round((cur + delta) * 100) / 100));
    if (next === cur) return;

    if (!state.settings) state.settings = {};
    state.settings.iconScale = next;
    document.documentElement.style.setProperty('--icon-scale', String(next));
    $('scaleRange').value = String(Math.round(next * 100));
    $('scaleVal').textContent = Math.round(next * 100) + '%';

    clearTimeout(zoomTimer);
    zoomTimer = setTimeout(persistZoom, 500);
  }, { passive: false });
}

async function persistZoom() {
  if (!state.settings) return;
  try {
    const saved = await window.go.main.App.SaveSettings(
      settingsPayload({ iconScale: state.settings.iconScale || 1 })
    );
    state.settings = saved;
  } catch (_) {}
}

// bindSplitter 拖动侧栏与文件区之间的分隔条。
function bindSplitter() {
  const sp = $('splitter');
  let dragging = false;

  sp.addEventListener('mousedown', (e) => {
    if (e.button !== 0) return;
    e.preventDefault();
    dragging = true;
    document.body.classList.add('col-resizing');
  });

  window.addEventListener('mousemove', (e) => {
    if (!dragging) return;
    const app = $('appRoot').getBoundingClientRect();
    const right = $('appRoot').classList.contains('pos-right');
    const raw = right ? (app.right - e.clientX) : (e.clientX - app.left);
    const width = Math.max(180, Math.min(520, Math.round(raw)));
    document.documentElement.style.setProperty('--sidebar-width', width + 'px');
    layoutBg();
  });

  window.addEventListener('mouseup', () => {
    if (!dragging) return;
    dragging = false;
    document.body.classList.remove('col-resizing');
    const width = parseInt(
      getComputedStyle(document.documentElement).getPropertyValue('--sidebar-width'), 10
    ) || 258;
    window.go.main.App.SaveSidebarWidth(width).catch(() => {});
  });
}

async function refreshAll() {
  if (state.busy) return;
  if (state.recent.active) {
    await loadMeta();
    await reloadRecent();
    return;
  }
  const current = state.listing ? state.listing.path : '';
  const q = searchState.active ? searchState.query : '';
  await loadMeta();
  await loadDir(current);
  if (q) {
    $('searchInput').value = q;
    $('btnClearSearch').hidden = false;
    await runSearch(q);
  }
}

/* ---------------- 侧栏 ---------------- */
async function chooseOpenFolder() {
  try {
    const m = await window.go.main.App.ChooseOpenFolder();
    applyMeta(m);
    toast('已加入侧栏');
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function removeOpenFolder(key) {
  try {
    const m = await window.go.main.App.RemoveOpenFolder(key);
    applyMeta(m);
    toast('已关闭该文件夹（磁盘文件未删除）');
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function deleteCategory(key) {
  const existed = (state.meta && state.meta.categories || []).some((c) => c.key === key);
  try {
    const m = await window.go.main.App.DeleteCategory(key);
    applyMeta(m);
    const still = (m.categories || []).some((c) => c.key === key);
    if (existed && !still) {
      toast('分类已删除');
      await loadDir(m.root || '');
    }
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function removeSidebarItem(key) {
  try {
    const m = await window.go.main.App.RemoveSidebarItem(key);
    applyMeta(m);
    toast('已从侧栏移除（磁盘文件未删除）');
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function chooseItemIcon(key) {
  try {
    const m = await window.go.main.App.ChooseItemIcon(key);
    applyMeta(m);
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function retargetItem(key) {
  try {
    const m = await window.go.main.App.RetargetItem(key);
    applyMeta(m);
    try {
      await loadDir(state.listing ? state.listing.path : '');
    } catch (_) {
      await loadDir('');
    }
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

async function setItemAlias(key, alias) {
  try {
    const m = await window.go.main.App.SetItemAlias(key, alias);
    applyMeta(m);
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

// beginRenameNode 重命名侧栏项（弹窗输入，只改显示名）。
function beginRenameNode(key) {
  const node = [...document.querySelectorAll('#tree .node, #catList .node')]
    .find((n) => n.dataset.key === key);
  if (!node) return;
  const label = node.querySelector('.label');
  if (!label) return;
  const old = label.textContent;

  openModal('重命名', `<input class="set-input" id="renameModalInput" maxlength="40" value="${esc(old)}">`);
  $('modalClose').textContent = '保存';
  const input = $('renameModalInput');
  input.focus();
  input.select();

  const done = (commit) => {
    const val = input.value.trim();
    restoreModalFoot();
    closeModal();
    if (commit && val && val !== old) setItemAlias(key, val);
  };
  $('modalClose').onclick = () => done(true);
  input.onkeydown = (e) => {
    e.stopPropagation();
    if (e.key === 'Enter') {
      e.preventDefault();
      done(true);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      done(false);
    }
  };
}

function showSidebarMenu(ev, key, path) {
  if (!key) return;
  ev.preventDefault();
  ev.stopPropagation();

  const menu = $('ctxMenu');
  menu.innerHTML = '';
  addCtx(menu, `${svg('image')} 自定义图标…`, () => { hideCtx(); chooseItemIcon(key); });
  addCtx(menu, `${svg('doc')} 重命名…`, () => { hideCtx(); beginRenameNode(key); });
  addCtx(menu, `${svg('folder')} 指定文件夹…`, () => { hideCtx(); retargetItem(key); });

  if (key.startsWith('open:')) {
    addCtxSep(menu);
    addCtx(menu, `${svg('file')} 关闭（不删除文件夹）`, () => { hideCtx(); removeOpenFolder(key); });
  } else if (key.startsWith('cat:')) {
    addCtxSep(menu);
    addCtx(menu, `${svg('file')} 删除分类…`, () => { hideCtx(); deleteCategory(key); });
  }

  menu.hidden = false;
  const w = menu.offsetWidth, h = menu.offsetHeight;
  menu.style.left = Math.min(ev.clientX, window.innerWidth - w - 8) + 'px';
  menu.style.top = Math.min(ev.clientY, window.innerHeight - h - 8) + 'px';
}

function renderSidebar() {
  const m = state.meta;
  if (!m) return;

  const opens = (m.openFolders || []).map((o) => `
    <div class="node open-node" role="button" tabindex="0" data-nav="${esc(o.path)}" data-key="${esc(o.key)}" data-drop="${esc(o.path)}">
      <span class="ico">${iconMarkup('folder', o.icon)}</span>
      <span class="label">${esc(o.name)}</span>
      <button class="close-open" data-close="${esc(o.key)}" title="关闭（不删除文件夹）">×</button>
    </div>`).join('');

  $('tree').innerHTML = `
    <div class="node add-open" role="button" tabindex="0" id="btnOpenFolder" title="选择一个文件夹加入侧栏">
      <span class="ico">${svg('folder')}</span>
      <span class="label">打开文件夹</span>
      <span class="cnt">＋</span>
    </div>
    ${opens}
    <div class="node recent-node" role="button" tabindex="0" data-recent="1" id="nodeRecent" title="最近打开的文件（只记录，不动原文件）">
      <span class="ico">${svg('clock')}</span>
      <span class="label">最近</span>
      <span class="cnt">${recentTotal()}</span>
    </div>
    <div class="node" role="button" tabindex="0" data-nav="${esc(m.desktop)}" data-key="desktop" id="nodeDesktop">
      <span class="ico">${m.desktopIcon ? iconMarkup('desktop', m.desktopIcon) : svg('desktop')}</span>
      <span class="label">${esc(m.desktopName || '桌面')}</span>
      <span class="cnt">整理对象</span>
    </div>
    <div class="node" role="button" tabindex="0" data-nav="${esc(m.root)}" data-key="home" id="nodeHome">
      <span class="ico">${m.homeIcon ? iconMarkup('home', m.homeIcon) : svg('home')}</span>
      <span class="label">${esc(m.homeName || 'home')}</span>
      <span class="cnt">分类根目录</span>
    </div>
  `;

  $('btnOpenFolder').onclick = chooseOpenFolder;
  const rn = $('nodeRecent');
  if (rn) rn.onclick = () => openRecent(state.recent.task || '');
  $('tree').querySelectorAll('.node[data-nav]').forEach((n) => {
    n.onclick = (ev) => {
      if (ev.target.closest('.close-open') || ev.target.closest('.rename-input')) return;
      nav(n.dataset.nav);
    };
    n.oncontextmenu = (ev) => showSidebarMenu(ev, n.dataset.key, n.dataset.nav);
    attachDrop(n);
  });
  $('tree').querySelectorAll('.close-open').forEach((b) => {
    b.onclick = (ev) => {
      ev.stopPropagation();
      removeOpenFolder(b.dataset.close);
    };
  });

  const list = $('catList');
  list.innerHTML = '';
  if (!m.categories || m.categories.length === 0) {
    list.innerHTML = '<div class="cat-empty">还没有分类。<br>点上方 <b>+</b> 新建一个，例如「工具」「编程」「取证」。</div>';
  } else {
    for (const c of m.categories) {
      const el = document.createElement('div');
      el.className = 'node cat-node';
      el.setAttribute('role', 'button');
      el.tabIndex = 0;
      el.dataset.nav = c.path;
      el.dataset.drop = c.path;
      el.dataset.key = c.key || ('cat:' + c.name);
      el.innerHTML = `
        <span class="ico">${iconMarkup('folder', c.icon)}</span>
        <span class="label">${esc(c.name)}</span>
        <span class="cnt">${c.count}</span>`;
      el.onclick = (ev) => {
        if (ev.target.closest('.rename-input')) return;
        nav(c.path);
      };
      el.ondblclick = () => window.go.main.App.OpenFolder(c.path).catch(e => toast(String(e)));
      el.oncontextmenu = (ev) => showSidebarMenu(ev, el.dataset.key, c.path);
      attachDrop(el);
      list.appendChild(el);
    }
  }
  renderSidebarActive();
}

function renderSidebarActive() {
  const cur = state.listing ? state.listing.path.toLowerCase() : '';
  const rn = $('nodeRecent');
  if (rn) rn.classList.toggle('active', state.recent.active);
  document.querySelectorAll('.node[data-nav]').forEach((n) => {
    const p = (n.dataset.nav || '').toLowerCase();
    n.classList.toggle('active', !state.recent.active && p === cur);
  });
}

// recentTotal 返回「最近」入口右侧的数量（默认最近 + 各任务）。
function recentTotal() {
  const gs = state.recent.groups || [];
  if (!gs.length) return '';
  return String(gs.reduce((n, g) => n + (g.count || 0), 0));
}

function attachDrop(el) {
  el.addEventListener('dragover', (e) => {
    if (!dragPaths.length) return;
    e.preventDefault();
    el.classList.add('drop-target');
  });
  el.addEventListener('dragleave', () => el.classList.remove('drop-target'));
  el.addEventListener('drop', async (e) => {
    e.preventDefault();
    el.classList.remove('drop-target');
    const dest = el.dataset.drop || el.dataset.nav;
    const paths = readDragPaths(e);
    if (!dest || paths.length === 0) return;
    await doMove(paths, dest);
  });
}

/* ---------------- 面包屑 ---------------- */
function renderCrumbs() {
  const box = $('crumbs');
  box.innerHTML = '';
  if (searchState.active) {
    const b = document.createElement('button');
    b.className = 'crumb current';
    b.textContent = `搜索「${searchState.query}」（${searchScopeLabel()}）· ${searchState.results.length} 项`;
    box.appendChild(b);
    $('recentTabBar').hidden = true;
    $('btnUp').disabled = true;
    $('btnUp').onclick = null;
    return;
  }

  if (state.recent.active) {
    const c = document.createElement('span');
    c.className = 'crumb current';
    c.textContent = '最近';
    box.appendChild(c);

    // 新建任务放在上面（路径栏），任务标签单独一行放在下面
    const add = document.createElement('button');
    add.className = 'btn ghost small recent-new-task';
    add.textContent = '＋ 新建任务';
    add.title = '新建任务分组（只存记录，不创建文件夹）';
    add.onclick = beginCreateTask;
    box.appendChild(add);

    $('recentTabBar').hidden = false;
    renderRecentTabs();
    $('btnUp').disabled = true;
    $('btnUp').onclick = null;
    return;
  }

  $('recentTabBar').hidden = true;
  const l = state.listing;
  if (!l) return;

  const chain = buildChain(l.path);
  chain.forEach((item, i) => {
    if (i > 0) {
      const sep = document.createElement('span');
      sep.className = 'sep';
      sep.textContent = '›';
      box.appendChild(sep);
    }
    const b = document.createElement('button');
    b.className = 'crumb' + (i === chain.length - 1 ? ' current' : '');
    b.textContent = item.label;
    b.onclick = () => nav(item.path);
    box.appendChild(b);
  });

  $('btnUp').disabled = !l.parent;
  $('btnUp').onclick = () => { if (l.parent) nav(l.parent); };
}

function buildChain(path) {
  const m = state.meta;
  const low = path.toLowerCase();
  const dLow = m.desktop.toLowerCase();
  const rLow = m.root.toLowerCase();
  const isUnder = (parent) => low === parent || low.startsWith(parent + '\\') || low.startsWith(parent + '/');
  const restOf = (parent) => path.slice(parent.length).replace(/^[\\/]/, '');

  if (low === dLow) {
    return [{ label: m.desktopName || '桌面', path: m.desktop }];
  }
  if (isUnder(dLow)) {
    const items = [{ label: m.desktopName || '桌面', path: m.desktop }];
    if (low === rLow) {
      items.push({ label: m.homeName || 'home', path: m.root });
      return items;
    }
    if (isUnder(rLow)) {
      items.push({ label: m.homeName || 'home', path: m.root });
      const rest = restOf(m.root);
      if (rest) items.push({ label: rest, path });
      return items;
    }
    const rest = restOf(m.desktop);
    if (rest) items.push({ label: rest, path });
    return items;
  }

  // 侧栏里「打开文件夹」或分类指定文件夹下的路径
  const roots = [];
  for (const o of m.openFolders || []) roots.push({ label: o.name, path: o.path });
  for (const c of m.categories || []) roots.push({ label: c.name, path: c.path });
  for (const r of roots) {
    const rl = r.path.toLowerCase();
    if (isUnder(rl)) {
      const chain = [{ label: r.label, path: r.path }];
      const rest = restOf(r.path);
      if (rest) chain.push({ label: rest, path });
      return chain;
    }
  }

  return [{ label: path.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || path, path }];
}

/* ---------------- 表格 ---------------- */
function renderTable() {
  if (searchState.active) { renderSearchTable(); return; }
  if (state.recent.active) { renderRecentTable(); return; }
  setTableHead(false);
  const l = state.listing;
  const body = $('tbody');
  body.innerHTML = '';

  if (!l || !l.entries || l.entries.length === 0) {
    $('emptyState').hidden = false;
    $('emptyState').querySelector('h2').textContent = '这里没有文件';
    $('emptyState').querySelector('p').textContent = l && l.path.toLowerCase() === state.meta.desktop.toLowerCase()
      ? '桌面上的文件都归位了。'
      : '把文件拖到左侧分类，或从桌面把文件拖进来。';
    return;
  }
  $('emptyState').hidden = true;

  for (const e of l.entries) body.appendChild(buildRow(e));
  renderStatus();
}

// renderSearchTable 把搜索结果渲染到同一个列表区。
function renderSearchTable() {
  setTableHead(false);
  const body = $('tbody');
  body.innerHTML = '';
  if (searchState.results.length === 0) {
    $('emptyState').hidden = false;
    $('emptyState').querySelector('h2').textContent = '没有匹配的文件';
    $('emptyState').querySelector('p').textContent = `没有找到名称包含「${searchState.query}」的文件或文件夹。`;
    return;
  }
  $('emptyState').hidden = true;
  for (const e of searchState.results) body.appendChild(buildRow(e));
  renderStatus();
}

function buildRow(e) {
  const row = document.createElement('div');
  row.className = 'row';
  row.dataset.path = e.path;
  row.dataset.dir = e.isDir ? '1' : '0';
  row.draggable = !e.isDir;
  row.innerHTML = `
    <div class="col-name">
      <span class="ico">${iconMarkup(e.isDir ? 'folder' : (e.kind || 'file'), e.icon)}</span>
      <span class="txt" title="${esc(e.path)}">${esc(e.name)}</span>
    </div>
    <div class="col-type">${esc(TYPE_LABEL[e.isDir ? 'folder' : (e.kind || 'file')] || '文件')}</div>
    <div class="col-size">${e.isDir ? '—' : esc(e.sizeText)}</div>
    <div class="col-time">${esc(e.modTime)}</div>`;

  row.onclick = (ev) => onRowClick(ev, e);
  row.ondblclick = () => onRowDblClick(e);
  row.oncontextmenu = (ev) => onRowContext(ev, e);
  row.addEventListener('dragstart', (ev) => onDragStart(ev, e, row));
  row.addEventListener('dragend', () => { dragPaths = []; });
  return row;
}

function onRowClick(ev, e) {
  if (ev.shiftKey && state.anchor != null) {
    const rows = [...document.querySelectorAll('.row')];
    const a = rows.findIndex((r) => r.dataset.path === state.anchor);
    const b = rows.findIndex((r) => r.dataset.path === e.path);
    if (a >= 0 && b >= 0) {
      const [lo, hi] = a < b ? [a, b] : [b, a];
      state.selected.clear();
      for (let i = lo; i <= hi; i++) {
        if (rows[i].dataset.dir === '0') state.selected.add(rows[i].dataset.path);
      }
      paintSelection();
      renderStatus();
      return;
    }
  }
  if (ev.ctrlKey || ev.metaKey) {
    toggleSel(e);
  } else {
    state.selected.clear();
    if (!e.isDir) state.selected.add(e.path);
    state.anchor = e.path;
  }
  paintSelection();
  renderStatus();
}

function toggleSel(e) {
  if (e.isDir) return;
  if (state.selected.has(e.path)) state.selected.delete(e.path);
  else state.selected.add(e.path);
  state.anchor = e.path;
  paintSelection();
  renderStatus();
}

function paintSelection() {
  document.querySelectorAll('.row').forEach((r) => {
    r.classList.toggle('sel', state.selected.has(r.dataset.path));
  });
}

function onRowDblClick(e) {
  if (e.isDir) {
    nav(e.path);
  } else {
    window.go.main.App.OpenFile(e.path).catch((err) => toast(String(err)));
  }
}

function onDragStart(ev, e, row) {
  if (e.isDir) { ev.preventDefault(); return; }
  if (!state.selected.has(e.path)) {
    state.selected.clear();
    state.selected.add(e.path);
    paintSelection();
    renderStatus();
  }
  dragPaths = [...state.selected];
  ev.dataTransfer.effectAllowed = 'move';
  ev.dataTransfer.setData('text/plain', JSON.stringify(dragPaths));
  row.classList.add('dragging');
}

let dragPaths = [];

function readDragPaths(ev) {
  try {
    const raw = ev.dataTransfer.getData('text/plain');
    if (!raw) return dragPaths;
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? arr : dragPaths;
  } catch (_) {
    return dragPaths;
  }
}

/* ---------------- 右键菜单 ---------------- */
// 原生菜单弹出期间忽略新的右键，避免菜单叠加或排队卡住。
let shellMenuBusy = false;

// 文件区右键：调用 Windows 原生右键菜单（与资源管理器一致）。
async function onRowContext(ev, e) {
  ev.preventDefault();
  if (shellMenuBusy) return;
  if (!state.selected.has(e.path) && !e.isDir) {
    state.selected.clear();
    state.selected.add(e.path);
    paintSelection();
    renderStatus();
  }

  let paths;
  if (e.isDir) {
    paths = [e.path];
  } else if (state.selected.has(e.path) && state.selected.size > 0) {
    paths = [...state.selected];
  } else {
    paths = [e.path];
  }

  shellMenuBusy = true;
  try {
    await window.go.main.App.ShowShellMenu(paths);
    // 原生菜单可能执行了重命名/删除，稍后刷新
    setTimeout(() => { refreshAll().catch(() => {}); }, 500);
  } catch (err) {
    // 取不到原生菜单时回退到内置菜单
    showRowMenu(ev, e);
  } finally {
    shellMenuBusy = false;
  }
}

function showRowMenu(ev, e) {
  ev.preventDefault();
  if (!state.selected.has(e.path) && !e.isDir) {
    state.selected.clear();
    state.selected.add(e.path);
    paintSelection();
    renderStatus();
  }

  const menu = $('ctxMenu');
  menu.innerHTML = '';

  if (!e.isDir) {
    const cats = (state.meta.categories || []).filter((c) => c.path.toLowerCase() !== state.listing.path.toLowerCase());
    if (cats.length > 0) {
      for (const c of cats.slice(0, 8)) {
        addCtx(menu, `${svg('folder')} 移动到「${esc(c.name)}」`, () => {
          hideCtx();
          doMove([...state.selected], c.path);
        });
      }
      addCtxSep(menu);
    }
    if (state.listing.path.toLowerCase() !== state.meta.desktop.toLowerCase()) {
      addCtx(menu, `${svg('desktop')} 移回桌面`, () => {
        hideCtx();
        doMove([...state.selected], state.meta.desktop);
      });
      addCtxSep(menu);
    }
  }

  if (e.isDir) {
    addCtx(menu, `${svg('folder')} 打开`, () => { hideCtx(); nav(e.path); });
  } else {
    addCtx(menu, `${svg('file')} 打开文件`, () => { hideCtx(); window.go.main.App.OpenFile(e.path).catch(err => toast(String(err))); });
  }
  addCtx(menu, `${svg('desktop')} 在资源管理器中显示`, () => {
    hideCtx();
    window.go.main.App.RevealFile(e.path).catch(err => toast(String(err)));
  });

  menu.hidden = false;
  const w = menu.offsetWidth, h = menu.offsetHeight;
  menu.style.left = Math.min(ev.clientX, window.innerWidth - w - 8) + 'px';
  menu.style.top = Math.min(ev.clientY, window.innerHeight - h - 8) + 'px';
}

function addCtx(menu, html, fn) {
  const b = document.createElement('button');
  b.className = 'ctx-item';
  b.innerHTML = html;
  b.onclick = fn;
  menu.appendChild(b);
}

function addCtxSep(menu) {
  const d = document.createElement('div');
  d.className = 'ctx-sep';
  menu.appendChild(d);
}

function hideCtx() { $('ctxMenu').hidden = true; }

/* ---------------- 最近 ---------------- */
const RECENT_MIME = 'application/x-dtidy-recent';

// openRecent 打开「最近」视图；task 为空表示「默认最近」。
async function openRecent(task) {
  hideCtx();
  resetSearch();
  $('searchInput').value = '';
  state.recent.active = true;
  state.listing = null;
  state.selected.clear();
  state.anchor = null;
  state.recent.task = task || '';
  try {
    state.recent.groups = await window.go.main.App.GetRecentGroups();
    if (state.recent.task && !state.recent.groups.some((g) => g.id === state.recent.task)) {
      state.recent.task = '';
    }
    state.recent.items = await window.go.main.App.ListRecent(state.recent.task);
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
  renderSidebar();
  renderCrumbs();
  setTableHead(true);
  renderTable();
  applyView();
  renderStatus();
}

// reloadRecent 静默刷新当前分组与各分组计数。
async function reloadRecent() {
  if (!state.recent.active) return;
  try {
    state.recent.groups = await window.go.main.App.GetRecentGroups();
    state.recent.items = await window.go.main.App.ListRecent(state.recent.task);
  } catch (_) { return; }
  renderSidebar();
  renderCrumbs();
  renderTable();
  renderStatus();
}

// renderRecentTabs 在路径栏下方单独一行渲染任务标签（胶囊样式），并作为拖放目标。
function renderRecentTabs() {
  const box = $('recentTabBar');
  box.innerHTML = '';
  const wrap = document.createElement('div');
  wrap.className = 'recent-tabs';

  for (const g of state.recent.groups) {
    const b = document.createElement('button');
    b.className = 'recent-tab' + (g.id === state.recent.task ? ' active' : '');
    b.title = '点击切换；把记录拖到这里可归入该分组';
    b.innerHTML = `${esc(g.name)}<span class="cnt">${g.count || 0}</span>`;
    b.onclick = () => openRecent(g.id);

    b.addEventListener('dragover', (ev) => {
      if (!ev.dataTransfer.types.includes(RECENT_MIME)) return;
      ev.preventDefault();
      b.classList.add('drop-target');
    });
    b.addEventListener('dragleave', () => b.classList.remove('drop-target'));
    b.addEventListener('drop', async (ev) => {
      ev.preventDefault();
      b.classList.remove('drop-target');
      const path = ev.dataTransfer.getData(RECENT_MIME);
      if (!path) return;
      try {
        await window.go.main.App.MoveRecent(path, g.id);
        toast('已移动到「' + g.name + '」');
        await reloadRecent();
      } catch (err) {
        toast(String(err).replace(/^Error: /, ''));
      }
    });

    if (g.id) {
      b.oncontextmenu = (ev) => showTaskMenu(ev, g);
      b.ondblclick = () => beginRenameTask(b, g);
    }
    wrap.appendChild(b);
  }

  box.appendChild(wrap);
}

function beginCreateTask() {
  if ($('recentTabBar').querySelector('.recent-task-input')) return;
  const input = document.createElement('input');
  input.className = 'recent-task-input';
  input.placeholder = '任务名，例如 渗透';
  input.maxLength = 32;
  $('recentTabBar').querySelector('.recent-tabs').appendChild(input);
  input.focus();

  const finish = async (commit) => {
    if (!input.value.trim()) { input.remove(); return; }
    if (!commit) { input.remove(); return; }
    try {
      await window.go.main.App.CreateRecentTask(input.value.trim());
      input.remove();
      await reloadRecent();
    } catch (err) {
      toast(String(err).replace(/^Error: /, ''));
      input.remove();
    }
  };
  input.onkeydown = (ev) => {
    if (ev.key === 'Enter') finish(true);
    if (ev.key === 'Escape') finish(false);
  };
  input.onblur = () => finish(false);
}

function beginRenameTask(btn, g) {
  if (btn.querySelector('input')) return;
  const input = document.createElement('input');
  input.className = 'recent-task-input';
  input.value = g.name;
  btn.textContent = '';
  btn.appendChild(input);
  input.focus();
  input.select();

  const finish = async (commit) => {
    const name = input.value.trim();
    if (!commit || !name || name === g.name) { await reloadRecent(); return; }
    try {
      await window.go.main.App.RenameRecentTask(g.id, name);
      await reloadRecent();
    } catch (err) {
      toast(String(err).replace(/^Error: /, ''));
      await reloadRecent();
    }
  };
  input.onkeydown = (ev) => {
    ev.stopPropagation();
    if (ev.key === 'Enter') finish(true);
    if (ev.key === 'Escape') finish(false);
  };
  input.onblur = () => finish(true);
}

function showTaskMenu(ev, g) {
  ev.preventDefault();
  ev.stopPropagation();
  const menu = $('ctxMenu');
  menu.innerHTML = '';
  addCtx(menu, '重命名任务…', () => {
    hideCtx();
    const btn = [...document.querySelectorAll('.recent-tab')].find((b) => b.textContent.startsWith(g.name));
    if (btn) beginRenameTask(btn, g);
  });
  addCtx(menu, '删除任务（记录回到默认最近）', async () => {
    hideCtx();
    if (!confirm(`删除任务「${g.name}」？\n组内记录会回到「默认最近」，不会删除任何文件。`)) return;
    try {
      await window.go.main.App.DeleteRecentTask(g.id);
      if (state.recent.task === g.id) state.recent.task = '';
      await reloadRecent();
      if (!state.recent.task) openRecent('');
    } catch (err) {
      toast(String(err).replace(/^Error: /, ''));
    }
  });
  menu.hidden = false;
  const w = menu.offsetWidth, h = menu.offsetHeight;
  menu.style.left = Math.min(ev.clientX, window.innerWidth - w - 8) + 'px';
  menu.style.top = Math.min(ev.clientY, window.innerHeight - h - 8) + 'px';
}

function setTableHead(recent) {
  const head = $('thead');
  if (!head) return;
  head.querySelector('.col-size').textContent = recent ? '最近打开' : '大小';
  head.querySelector('.col-time').textContent = recent ? '次数' : '修改时间';
}

function renderRecentTable() {
  const body = $('tbody');
  body.innerHTML = '';
  const items = state.recent.items || [];
  if (items.length === 0) {
    $('emptyState').hidden = false;
    $('emptyState').querySelector('h2').textContent = '这里还没有记录';
    $('emptyState').querySelector('p').textContent = '双击文件打开后会自动记录；系统里打开的文件稍后也会同步进来。';
    return;
  }
  $('emptyState').hidden = true;
  for (const e of items) body.appendChild(buildRecentRow(e));
  renderStatus();
}

function buildRecentRow(e) {
  const row = document.createElement('div');
  row.className = 'row recent-row' + (e.exists ? '' : ' missing');
  row.dataset.path = e.path;
  row.dataset.dir = '0';
  row.draggable = true;
  row.innerHTML = `
    <div class="col-name">
      <span class="ico">${iconMarkup(e.kind || 'file', e.icon)}</span>
      <span class="txt" title="${esc(e.path)}">${esc(e.name)}</span>
    </div>
    <div class="col-type">${esc(TYPE_LABEL[e.kind] || '文件')}</div>
    <div class="col-size">${esc(e.lastUsedText)}<span class="recent-count">${e.count} 次</span></div>
    <div class="col-time">${e.count} 次</div>`;

  row.ondblclick = () => openRecentItem(e);
  row.oncontextmenu = (ev) => showRecentMenu(ev, e);
  row.addEventListener('dragstart', (ev) => {
    dragPaths = [];
    ev.dataTransfer.effectAllowed = 'copyMove';
    ev.dataTransfer.setData(RECENT_MIME, e.path);
    row.classList.add('dragging');
  });
  row.addEventListener('dragend', () => row.classList.remove('dragging'));
  return row;
}

async function openRecentItem(e) {
  try {
    await window.go.main.App.OpenFile(e.path);
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
    return;
  }
  // 打开后次数与时间会变化，稍等片刻再刷新
  setTimeout(() => { reloadRecent().catch(() => {}); }, 700);
}

function showRecentMenu(ev, e) {
  ev.preventDefault();
  const menu = $('ctxMenu');
  menu.innerHTML = '';

  addCtx(menu, `${svg('file')} 打开文件`, () => {
    hideCtx();
    openRecentItem(e);
  });
  addCtx(menu, `${svg('desktop')} 在资源管理器中显示`, () => {
    hideCtx();
    window.go.main.App.RevealFile(e.path).catch((err) => toast(String(err)));
  });
  addCtxSep(menu);

  const others = (state.recent.groups || []).filter((g) => g.id !== e.task);
  if (others.length > 0) {
    for (const g of others.slice(0, 8)) {
      addCtx(menu, `${svg('folder')} 移动到「${esc(g.name)}」`, async () => {
        hideCtx();
        try {
          await window.go.main.App.MoveRecent(e.path, g.id);
          toast('已移动到「' + g.name + '」');
          await reloadRecent();
        } catch (err) {
          toast(String(err).replace(/^Error: /, ''));
        }
      });
    }
    addCtxSep(menu);
  }

  addCtx(menu, '从最近中移除（不删文件）', async () => {
    hideCtx();
    try {
      await window.go.main.App.RemoveRecent(e.path);
      await reloadRecent();
    } catch (err) {
      toast(String(err).replace(/^Error: /, ''));
    }
  });

  menu.hidden = false;
  const w = menu.offsetWidth, h = menu.offsetHeight;
  menu.style.left = Math.min(ev.clientX, window.innerWidth - w - 8) + 'px';
  menu.style.top = Math.min(ev.clientY, window.innerHeight - h - 8) + 'px';
}

/* ---------------- 移动 ---------------- */
async function doMove(paths, destDir) {
  if (!paths || paths.length === 0) {
    toast('请先选择文件');
    return;
  }
  state.busy = true;
  try {
    const report = await window.go.main.App.MoveTo(paths, destDir);
    state.selected.clear();
    await loadMeta();
    await loadDir(state.listing ? state.listing.path : '');
    showMoveReport(report);
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  } finally {
    state.busy = false;
    renderStatus();
  }
}

async function doUndo() {
  if (state.busy) return;
  state.busy = true;
  try {
    const report = await window.go.main.App.Undo();
    await loadMeta();
    await loadDir(state.listing ? state.listing.path : '');
    showUndoReport(report);
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  } finally {
    state.busy = false;
    renderStatus();
  }
}

function showMoveReport(r) {
  let html = `<div class="target">${esc(r.target || '')}</div>`;
  html += `<div class="sum">
    <span class="ok">成功 ${r.ok}</span>
    <span class="skip">跳过 ${r.skipped}</span>
    <span class="fail">失败 ${r.failed}</span></div>`;
  const problems = (r.items || []).filter((i) => !i.ok);
  if (problems.length) {
    html += '<ul>' + problems.map((p) => `<li>${esc(p.name)}：${esc(p.reason || '未知原因')}</li>`).join('') + '</ul>';
  } else {
    html += '<p class="skip">全部就位。</p>';
  }
  openModal('移动结果', html);
}

function showUndoReport(r) {
  let html = `<div class="target">批次 ${esc(r.batch || '')}</div>`;
  html += `<div class="sum">
    <span class="ok">回到原位 ${r.ok}</span>
    <span class="skip">跳过 ${r.skipped}</span>
    <span class="fail">失败 ${r.failed}</span></div>`;
  const problems = (r.items || []).filter((i) => !i.ok);
  if (problems.length) {
    html += '<ul>' + problems.map((p) => `<li>${esc(p.name)}：${esc(p.reason || '未知原因')}</li>`).join('') + '</ul>';
  }
  openModal('撤销结果', html);
}

/* ---------------- 状态栏 / 移动菜单 ---------------- */
function renderStatus() {
  const n = state.selected.size;
  const info = $('selectionInfo');

  if (searchState.active) {
    info.textContent = n > 0
      ? `搜索「${searchState.query}」（${searchScopeLabel()}）：已选 ${n} 项 / 共 ${searchState.results.length} 项`
      : `搜索「${searchState.query}」（${searchScopeLabel()}）：${searchState.results.length} 个结果，双击打开`;
    $('btnMove').disabled = n === 0 || state.busy;
    $('btnMove').onclick = toggleMoveMenu;
    $('btnRevealDir').onclick = () => {
      window.go.main.App.OpenFolder(state.meta.desktop).catch(e => toast(String(e)));
    };
    return;
  }

  if (state.recent.active) {
    const n = (state.recent.items || []).length;
    info.textContent = `最近：${state.recent.items.length ? n + ' 条记录' : '暂无记录'}（双击打开，拖动可归入其它任务）`;
    $('btnMove').disabled = true;
    $('btnRevealDir').onclick = () => toast('最近记录只是索引，不指向固定目录');
    return;
  }

  if (!state.listing) { info.textContent = '未选择文件'; return; }

  const atDesktop = state.listing.path.toLowerCase() === state.meta.desktop.toLowerCase();
  info.textContent = n > 0
    ? `已选 ${n} 项${atDesktop ? '（桌面）' : '（' + state.listing.name + '）'}`
    : (atDesktop ? '桌面：选中文件后拖到左侧分类' : '当前在分类目录中，可拖回桌面或拖到其它分类');

  $('btnMove').disabled = n === 0 || state.busy;
  $('btnMove').onclick = toggleMoveMenu;
  $('btnRevealDir').onclick = () => {
    if (state.listing) window.go.main.App.OpenFolder(state.listing.path).catch(e => toast(String(e)));
  };
}

function toggleMoveMenu() {
  const menu = $('moveMenu');
  if (!menu.hidden) { menu.hidden = true; return; }
  const targets = [];
  const cur = state.listing ? state.listing.path.toLowerCase() : '';
  const atDesktop = !searchState.active && cur === state.meta.desktop.toLowerCase();
  if (!atDesktop) {
    targets.push({ name: '桌面', path: state.meta.desktop, icon: 'desktop' });
  }
  for (const c of state.meta.categories || []) {
    if (!searchState.active && c.path.toLowerCase() === cur) continue;
    targets.push({ name: c.name, path: c.path, icon: 'folder' });
  }
  if (targets.length === 0) {
    toast('没有可用的目标分类，先在左侧新建一个');
    return;
  }
  menu.innerHTML = '';
  for (const t of targets) {
    const b = document.createElement('button');
    b.className = 'menu-item';
    b.innerHTML = `<span class="ico">${svg(t.icon)}</span><span>${esc(t.name)}</span>`;
    b.onclick = () => { menu.hidden = true; doMove([...state.selected], t.path); };
    menu.appendChild(b);
  }
  menu.hidden = false;
}

/* ---------------- 导航 ---------------- */
async function nav(path) {
  if (!path) return;
  state.recent.active = false;
  try {
    await loadDir(path);
  } catch (err) {
    toast(String(err).replace(/^Error: /, ''));
  }
}

/* ---------------- 新建分类 ---------------- */
function beginCreateCategory() {
  const list = $('catList');
  if (list.querySelector('.cat-input')) return;
  const input = document.createElement('input');
  input.className = 'cat-input';
  input.placeholder = '分类名，例如 工具';
  input.maxLength = 64;
  list.prepend(input);
  input.focus();

  const finish = async (commit) => {
    if (!commit || !input.value.trim()) { input.remove(); return; }
    const name = input.value.trim();
    try {
      state.meta = await window.go.main.App.CreateCategory(name);
      renderSidebar();
      toast('已创建分类：' + name);
    } catch (err) {
      toast(String(err).replace(/^Error: /, ''));
    }
  };
  input.onkeydown = (e) => {
    if (e.key === 'Enter') finish(true);
    if (e.key === 'Escape') finish(false);
  };
  input.onblur = () => finish(false);
}

/* ---------------- 弹层 ---------------- */
function openModal(title, html) {
  $('modalTitle').textContent = title;
  $('modalBody').innerHTML = html;
  $('modalMask').hidden = false;
}
function closeModal() { $('modalMask').hidden = true; }

// restoreModalFoot 恢复通用弹窗的底部按钮（重命名弹窗会临时改成「保存」）。
function restoreModalFoot() {
  $('modalClose').textContent = '知道了';
  $('modalClose').onclick = closeModal;
}

/* ---------------- 事件绑定 ---------------- */
function bind() {
  $('btnAddCat').onclick = beginCreateCategory;
  $('btnUndo').onclick = doUndo;
  $('btnRefresh').onclick = () => refreshAll();
  $('btnViewGrid').onclick = () => setView('grid');
  $('btnViewList').onclick = () => setView('list');
  $('btnSettings').onclick = openSettings;
  $('modalClose').onclick = closeModal;

  // 设置面板
  $('btnSaveSettings').onclick = saveSettings;
  $('btnCloseSettings').onclick = closeSettings;
  $('btnRestart').onclick = () => window.go.main.App.RestartApp();
  $('btnPickBg').onclick = pickBackground;
  $('btnClearBg').onclick = clearBackground;
  $('btnPickDesktop').onclick = pickDesktop;
  $('btnPickHome').onclick = pickHome;
  $('btnPickRule').onclick = pickRuleFile;
  $('btnClearRule').onclick = clearRuleFile;
  $('btnCandMinus').onclick = () => candStep(-1);
  $('btnCandPlus').onclick = () => candStep(1);
  $('candInput').addEventListener('keydown', (e) => {
    if (e.key === 'ArrowUp' || e.key === '+') { e.preventDefault(); candStep(1); }
    if (e.key === 'ArrowDown' || e.key === '-') { e.preventDefault(); candStep(-1); }
    if (e.key === 'Enter') { e.preventDefault(); setCandInput($('candInput').value); }
  });
  $('candInput').addEventListener('change', () => setCandInput($('candInput').value));
  $('autoStartChk').onchange = () => {
    $('autoStartHint').textContent = $('autoStartChk').checked
      ? '保存后写入系统启动项（HKCU\\...\\Run）'
      : '保存后移除系统启动项';
  };
  bindHotkeyCapture();
  bindSearch();
  bindSplitter();

  $('opacityRange').oninput = () => {
    const v = Number($('opacityRange').value);
    $('opacityVal').textContent = v + '%';
    document.documentElement.style.setProperty('--bg-opacity', String(v / 100));
  };
  $('scaleRange').oninput = () => {
    const v = Number($('scaleRange').value);
    $('scaleVal').textContent = v + '%';
    document.documentElement.style.setProperty('--icon-scale', String(v / 100));
  };
  $('posSeg').onclick = (e) => {
    const btn = e.target.closest('button');
    if (!btn) return;
    for (const b of $('posSeg').querySelectorAll('button')) b.classList.remove('active');
    btn.classList.add('active');
    const app = $('appRoot');
    app.classList.remove('pos-left', 'pos-right', 'pos-top');
    app.classList.add('pos-' + btn.dataset.pos);
    layoutBg();
  };
  $('scopeSeg').onclick = (e) => {
    const btn = e.target.closest('button');
    if (!btn) return;
    const scope = btn.dataset.scope;
    for (const b of $('scopeSeg').querySelectorAll('button')) b.classList.toggle('active', b === btn);
    $('customScopeWrap').hidden = scope !== 'custom';
    bgState.scope = scope;
    layoutBg();
  };
  $('btnPickRegion').onclick = startRegionSelect;

  // 背景拉选：按住鼠标拖出矩形
  $('regionMask').addEventListener('mousedown', (e) => {
    if (!region.active || e.button !== 0) return;
    region.start = { x: e.clientX, y: e.clientY };
    setRegionBox(e.clientX, e.clientY, 0, 0);
  });
  window.addEventListener('mousemove', (e) => {
    if (!region.active || !region.start) return;
    const x = Math.min(region.start.x, e.clientX);
    const y = Math.min(region.start.y, e.clientY);
    setRegionBox(x, y, Math.abs(e.clientX - region.start.x), Math.abs(e.clientY - region.start.y));
  });
  window.addEventListener('mouseup', (e) => {
    if (!region.active || !region.start) return;
    const x = Math.min(region.start.x, e.clientX);
    const y = Math.min(region.start.y, e.clientY);
    const w = Math.abs(e.clientX - region.start.x);
    const h = Math.abs(e.clientY - region.start.y);
    region.start = null;
    if (w < 40 || h < 40) {
      $('regionBox').hidden = true;
      toast('选区太小，请重新拖拽');
      return;
    }
    const vw = window.innerWidth || 1;
    const vh = window.innerHeight || 1;
    const rect = [x / vw, y / vh, w / vw, h / vh].map((v) => Math.round(v * 1000) / 1000);
    cancelRegionSelect();
    saveRegion(rect);
  });
  window.addEventListener('resize', layoutBg);
  $('fitSeg').onclick = (e) => {
    const btn = e.target.closest('button');
    if (!btn) return;
    for (const b of $('fitSeg').querySelectorAll('button')) b.classList.remove('active');
    btn.classList.add('active');
    previewTheme();
  };
  $('accentColor').oninput = () => {
    // 整体颜色同时带出整个界面的底色，而不是只改图标球
    pageTouched = false;
    $('pageColor').value = darkenHex($('accentColor').value, 0.82);
    previewTheme();
  };
  $('pageColor').oninput = () => {
    pageTouched = true;
    previewTheme();
  };
  $('backColor').oninput = previewTheme;
  $('followBack').onchange = () => {
    $('backColor').hidden = $('followBack').checked;
    previewTheme();
  };
  $('recentColor').oninput = previewTheme;
  $('followRecent').onchange = () => {
    $('recentColor').hidden = $('followRecent').checked;
    previewTheme();
  };

  $('fontFamily').onchange = previewFont;
  $('fontSize').oninput = previewFont;
  $('fontColor').oninput = previewFont;
  $('fontColorFollow').onchange = () => {
    $('fontColor').hidden = $('fontColorFollow').checked;
    previewFont();
  };
  $('fontPart').onchange = () => {
    captureFontPart(fontPartCurrent);
    fontPartCurrent = $('fontPart').value;
    setFontPartUI(fontPartCurrent);
    previewFont();
  };
  const partFontUpdate = () => {
    captureFontPart(fontPartCurrent);
    previewFont();
  };
  $('partFamily').onchange = partFontUpdate;
  $('partSize').oninput = partFontUpdate;
  $('partColor').oninput = partFontUpdate;
  $('partColorFollow').onchange = () => {
    $('partColor').hidden = $('partColorFollow').checked;
    partFontUpdate();
  };
  $('settingsMask').onclick = (e) => {
    if (e.target === $('settingsMask')) closeSettings();
  };
  bindZoom();

  $('modalMask').onclick = (e) => {
    if (e.target === $('modalMask')) {
      restoreModalFoot();
      closeModal();
    }
  };

  document.addEventListener('click', (e) => {
    hideCtx();
    if (!e.target.closest('.move-wrap')) $('moveMenu').hidden = true;
    if (!e.target.closest('.side-search')) hideSearchPop();
  });
  document.addEventListener('contextmenu', (e) => {
    if (!e.target.closest('.row')) hideCtx();
  });

  // 文件区空白处：弹出当前文件夹的 Windows 原生背景右键菜单
  const fileTable = document.querySelector('.table');
  if (fileTable) {
    fileTable.addEventListener('contextmenu', (ev) => {
      if (ev.target.closest('.row')) return; // 文件行由 onRowContext 处理
      ev.preventDefault();
      hideCtx();
      if (shellMenuBusy) return;
      const dir = state.listing && state.listing.path;
      if (!dir) return;
      shellMenuBusy = true;
      window.go.main.App.ShowShellMenuBackground(dir)
        .then(() => setTimeout(() => { refreshAll().catch(() => {}); }, 500))
        .catch(() => {})
        .finally(() => { shellMenuBusy = false; });
    });
  }

  document.addEventListener('keydown', (e) => {
    if (region.active) {
      if (e.key === 'Escape') cancelRegionSelect();
      return;
    }
    const inField = e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA');

    if (e.key === 'Escape') {
      if (inField) return; // 输入框自己处理 Esc
      if (!$('settingsMask').hidden) { closeSettings(); return; }
      if (searchState.active) { clearSearch(); return; }
      closeModal(); hideCtx(); $('moveMenu').hidden = true;
      if (state.selected.size) { state.selected.clear(); paintSelection(); renderStatus(); }
      return;
    }
    if (inField) return;

    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
      e.preventDefault();
      $('searchInput').focus();
      return;
    }
    if (e.key === 'F5') { e.preventDefault(); refreshAll(); }
    if (e.key === 'Backspace' && state.listing && state.listing.parent) {
      e.preventDefault();
      nav(state.listing.parent);
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'a') {
      e.preventDefault();
      document.querySelectorAll('.row').forEach((r) => {
        if (r.dataset.dir === '0') state.selected.add(r.dataset.path);
      });
      paintSelection(); renderStatus();
    }
  });

  window.addEventListener('dragover', (e) => { if (dragPaths.length) e.preventDefault(); });
  window.addEventListener('drop', (e) => { if (dragPaths.length) e.preventDefault(); });
}

window.addEventListener('DOMContentLoaded', async () => {
  bind();
  applyView();
  try {
    await loadMeta();
    await loadDir('');
    await loadSettings();
  } catch (err) {
    toast('启动失败：' + String(err));
  }
});
