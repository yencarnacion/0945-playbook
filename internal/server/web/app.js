const state = {
  rows: [],
  filter: 'relevant',
  query: '',
  timer: null,
  volumeFilter: 0,
  history: [],
  historyLimit: 120,
  selectedIndex: -1,
  followLive: true,
  lastSignature: '',
  livePayload: null,
};

const el = {
  project: document.getElementById('project'),
  mode: document.getElementById('mode'),
  clock: document.getElementById('clock'),
  updated: document.getElementById('updated'),
  stats: document.getElementById('stats'),
  rows: document.getElementById('rows'),
  strip: document.getElementById('strip'),
  search: document.getElementById('search'),
  filter: document.getElementById('filter'),
  back: document.getElementById('back'),
  pause: document.getElementById('pause'),
  forward: document.getElementById('forward'),
  live: document.getElementById('live'),
  historyStatus: document.getElementById('historyStatus'),
};

el.search.addEventListener('input', () => {
  state.query = el.search.value.trim().toUpperCase();
  render();
});

el.filter.addEventListener('click', (event) => {
  const button = event.target.closest('button[data-filter]');
  if (!button) return;
  state.filter = button.dataset.filter;
  for (const b of el.filter.querySelectorAll('button')) b.classList.toggle('active', b === button);
  render();
});

el.back.addEventListener('click', () => {
  if (state.history.length === 0) return;
  state.followLive = false;
  state.selectedIndex = clampIndex(state.selectedIndex < 0 ? state.history.length - 1 : state.selectedIndex);
  state.selectedIndex = Math.max(0, state.selectedIndex - 1);
  useSelectedSnapshot();
});

el.pause.addEventListener('click', () => {
  if (state.history.length === 0) return;
  state.followLive = false;
  state.selectedIndex = clampIndex(state.selectedIndex < 0 ? state.history.length - 1 : state.selectedIndex);
  useSelectedSnapshot();
});

el.forward.addEventListener('click', () => {
  if (state.history.length === 0) return;
  state.followLive = false;
  state.selectedIndex = clampIndex(state.selectedIndex < 0 ? state.history.length - 1 : state.selectedIndex);
  state.selectedIndex = Math.min(state.history.length - 1, state.selectedIndex + 1);
  useSelectedSnapshot();
});

el.live.addEventListener('click', () => {
  state.followLive = true;
  state.selectedIndex = state.history.length - 1;
  if (state.livePayload) applyPayload(state.livePayload);
});

async function refresh() {
  try {
    const response = await fetch('/api/state', { cache: 'no-store' });
    const payload = await response.json();
    state.livePayload = payload;
    addHistorySnapshot(payload);
    if (state.followLive) {
      state.selectedIndex = state.history.length - 1;
      applyPayload(payload);
    } else {
      updateHistoryStatus();
    }
  } catch (error) {
    el.updated.textContent = `offline ${new Date().toLocaleTimeString()}`;
  }
}

function applyPayload(payload) {
  state.rows = payload.rows || [];
  state.volumeFilter = Number(payload.volume_filter || 0);
  el.project.textContent = payload.project || '0945-playbook';
  el.mode.textContent = payload.mode || 'mode';
  el.clock.textContent = payload.clock || '--:--:--';
  updateUpdatedText(payload);
  renderStats(payload.stats || {});
  updateHistoryStatus();
  render();
}

function useSelectedSnapshot() {
  if (state.selectedIndex < 0 || state.history.length === 0) return;
  const snapshot = state.history[clampIndex(state.selectedIndex)];
  state.selectedIndex = clampIndex(state.selectedIndex);
  applyPayload(snapshot);
}

function addHistorySnapshot(payload) {
  const snapshot = signalSnapshot(payload);
  const signature = snapshotSignature(snapshot);
  if (signature === state.lastSignature) return;
  state.lastSignature = signature;
  state.history.push(snapshot);
  if (state.history.length > state.historyLimit) {
    const remove = state.history.length - state.historyLimit;
    state.history.splice(0, remove);
    if (!state.followLive) state.selectedIndex = Math.max(0, state.selectedIndex - remove);
  }
  if (state.followLive) state.selectedIndex = state.history.length - 1;
}

function signalSnapshot(payload) {
  const rows = (payload.rows || []).filter((row) => isSignalHistoryRow(row, Number(payload.volume_filter || 0)));
  return {
    ...payload,
    rows,
  };
}

function isSignalHistoryRow(row, volumeFilter) {
  if (row.phase === 'error' || row.error) return true;
  if (volumeFilter && Number(row.first15_vol || 0) < volumeFilter) return false;
  return row.phase === 'likely' ||
    row.phase === 'signal' ||
    row.phase === 'active' ||
    row.phase === 'done' ||
    row.signal;
}

function snapshotSignature(snapshot) {
  const rows = snapshot.rows || [];
  const parts = rows
    .filter((row) => row.phase !== 'error' && !row.error)
    .map((row) => [
      row.symbol,
      row.phase,
      row.status,
      row.action,
      row.branch,
      Math.round(Number(row.ev_score || 0)),
      Number(row.entry || 0).toFixed(3),
      Number(row.target || 0).toFixed(3),
      Number(row.stop || 0).toFixed(3),
      row.shares || 0,
    ].join(':'));
  if (parts.length === 0) return 'empty';
  return parts.join('|');
}

function updateUpdatedText(payload) {
  const viewText = payload.updated ? new Date(payload.updated).toLocaleTimeString() : 'waiting';
  if (state.followLive || !state.livePayload) {
    el.updated.textContent = `updated ${viewText}`;
    return;
  }
  el.updated.textContent = `viewing ${payload.clock || '--:--:--'} · live still calculating ${state.livePayload.clock || '--:--:--'}`;
}

function updateHistoryStatus() {
  const total = state.history.length;
  const current = state.selectedIndex >= 0 ? state.selectedIndex + 1 : total;
  if (state.followLive) {
    const liveClock = state.livePayload ? state.livePayload.clock : '';
    el.historyStatus.textContent = `Live snapshot ${liveClock} ${total ? `${current}/${total}` : ''}`.trim();
  } else {
    const snapshot = state.history[clampIndex(state.selectedIndex)] || {};
    el.historyStatus.textContent = `Viewing snapshot ${snapshot.clock || ''} ${total ? `${current}/${total}` : ''}`.trim();
  }
  el.back.disabled = total <= 1 || clampIndex(state.selectedIndex) <= 0;
  el.forward.disabled = total <= 1 || clampIndex(state.selectedIndex) >= total - 1;
  el.pause.disabled = total === 0 || !state.followLive;
  el.live.disabled = state.followLive;
}

function clampIndex(index) {
  if (state.history.length === 0) return -1;
  if (index < 0) return 0;
  if (index >= state.history.length) return state.history.length - 1;
  return index;
}

function renderStats(stats) {
  const cells = [
    ['Total', stats.total],
    ['Vol >', compact(state.volumeFilter)],
    ['Likely', stats.likely],
    ['Signals', stats.signals],
    ['Active', stats.active],
    ['Done', stats.done],
    ['No Trade', stats.no_trade],
    ['High EV', stats.high_ev],
    ['Errors', stats.errors],
  ];
  el.stats.innerHTML = cells.map(([label, value]) => (
    `<div class="stat"><strong>${value || 0}</strong><span>${label}</span></div>`
  )).join('');
}

function render() {
  const rows = state.rows.filter(rowVisible);
  el.rows.innerHTML = rows.map(rowHTML).join('');
  renderStrip();
  requestAnimationFrame(drawCharts);
}

function rowVisible(row) {
  if (state.query && !row.symbol.includes(state.query)) return false;
  if (!passesVolume(row) && state.filter !== 'problem') return false;
  if (state.filter === 'relevant') return isRelevant(row);
  if (state.filter === 'problem') return row.phase === 'error' || row.error;
  if (state.filter === 'signal') return row.signal;
  return row.phase === state.filter;
}

function renderStrip() {
  const interesting = state.rows
    .filter((row) => row.phase === 'active' || row.phase === 'signal' || row.phase === 'likely' || row.phase === 'done')
    .filter(passesVolume)
    .sort((a, b) => (b.ev_score || 0) - (a.ev_score || 0))
    .slice(0, 18);

  el.strip.innerHTML = interesting.map((row) => {
    const side = row.side < 0 ? 'short' : 'long';
    return `<a class="tile side-${side} phase-${row.phase || 'none'}" href="${row.chart_url}" target="_blank" rel="noreferrer">
      <div class="tile-head">
        <span class="ticker">${escapeHTML(row.symbol)}</span>
        <span class="signal">${escapeHTML(signalText(row))}</span>
      </div>
      <div class="strategy-name">Vol15 ${compact(row.first15_vol) || 'building'}</div>
      <div class="branch-name">${escapeHTML(row.branch || 'No branch yet')}</div>
      <div class="priority-line"><span>${escapeHTML(priorityLabel(row))}</span><span>EV ${fmt(row.ev_score, 0) || '0'}</span></div>
      <div class="levels">
        <span>${escapeHTML(row.action || '')}</span><span>${row.shares ? `${row.shares} sh` : ''}</span>
        <span>Entry ${money(row.entry) || money(row.price)}</span><span>TP ${money(row.target)}</span>
        <span>Stop ${money(row.stop)}</span><span>R ${fmt(row.r_multiple, 2)}</span>
        <span>C/Avg ${fmt(row.ratio, 3)}</span><span>${pct(row.delta_pct)}</span>
      </div>
    </a>`;
  }).join('');
}

function isRelevant(row) {
  return row.phase === 'likely' ||
    row.phase === 'signal' ||
    row.phase === 'active' ||
    row.phase === 'done' ||
    row.signal ||
    row.error;
}

function passesVolume(row) {
  if (!state.volumeFilter) return true;
  if (row.phase === 'error' || row.error) return true;
  return Number(row.first15_vol || 0) >= state.volumeFilter;
}

function rowHTML(row) {
  const sideClass = row.side < 0 ? 'side-short' : row.side > 0 ? 'side-long' : '';
  const evClass = row.ev_score >= 80 ? 'high' : row.ev_score >= 50 ? 'mid' : '';
  const deltaClass = row.delta_pct > 0 ? 'pos' : row.delta_pct < 0 ? 'neg' : 'muted';
  const spark = encodeURIComponent(JSON.stringify(row.spark || []));
  return `<tr class="phase-${row.phase || 'none'} ${sideClass}">
    <td class="sym"><a href="${row.chart_url}" target="_blank" rel="noreferrer">${escapeHTML(row.symbol)}</a><div>${escapeHTML(shortName(row.name))}</div></td>
    <td><span class="pill">${escapeHTML(row.status || '')}</span></td>
    <td class="ev ${evClass}">${fmt(row.ev_score, 0)}</td>
    <td>${escapeHTML(row.action || '')}</td>
    <td>${escapeHTML(row.branch || '-')}</td>
    <td>${fmt(row.ratio, 3)}</td>
    <td class="${deltaClass}">${pct(row.delta_pct)}</td>
    <td>${money(row.price)}</td>
    <td>${money(row.avg15)}</td>
    <td>${compact(row.first15_vol)}</td>
    <td>${pct(row.vwap_reward_pct)}</td>
    <td>${pct(row.hod_risk_pct)}</td>
    <td>${pct(row.distance_pct)}</td>
    <td>${row.shares || ''}</td>
    <td>${money(row.entry)}</td>
    <td>${money(row.target)}</td>
    <td>${money(row.stop)}</td>
    <td class="spark"><canvas width="192" height="52" data-spark="${spark}" data-side="${row.side || 0}"></canvas></td>
    <td class="note">${escapeHTML(row.reason || row.error || '')}</td>
  </tr>`;
}

function drawCharts() {
  for (const canvas of document.querySelectorAll('canvas[data-spark]')) {
    const points = JSON.parse(decodeURIComponent(canvas.dataset.spark || '[]'));
    const ctx = canvas.getContext('2d');
    const w = canvas.width;
    const h = canvas.height;
    ctx.clearRect(0, 0, w, h);
    if (points.length < 2) continue;
    let min = Infinity;
    let max = -Infinity;
    for (const p of points) {
      min = Math.min(min, p.close);
      max = Math.max(max, p.close);
    }
    if (min === max) {
      min *= 0.99;
      max *= 1.01;
    }
    ctx.strokeStyle = '#263140';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, h - 1);
    ctx.lineTo(w, h - 1);
    ctx.stroke();

    const side = Number(canvas.dataset.side || 0);
    ctx.strokeStyle = side < 0 ? '#d55e00' : side > 0 ? '#009e73' : '#58a6ff';
    ctx.lineWidth = 2;
    ctx.beginPath();
    points.forEach((p, i) => {
      const x = points.length === 1 ? 0 : (i / (points.length - 1)) * (w - 2) + 1;
      const y = h - 2 - ((p.close - min) / (max - min)) * (h - 5);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
  }
}

function signalText(row) {
  if (row.side < 0) return row.phase === 'likely' ? 'Likely Sell' : 'Sell Signal';
  if (row.side > 0) return row.phase === 'likely' ? 'Likely Buy' : 'Buy Signal';
  return row.status || 'Watch';
}

function priorityLabel(row) {
  if (row.branch === 'B4 SHORT *') return 'Priority A - Weakness';
  const score = Number(row.ev_score || 0);
  if (score >= 90) return 'Priority A';
  if (score >= 80) return 'Priority B';
  if (score >= 60) return 'Priority C';
  if (row.phase === 'likely') return 'Priority Watch';
  if (row.signal) return 'Priority Review';
  return 'Priority Build';
}

function fmt(value, digits) {
  if (!value || !Number.isFinite(value)) return '';
  return Number(value).toFixed(digits);
}

function money(value) {
  if (!value || !Number.isFinite(value)) return '';
  return value >= 100 ? value.toFixed(2) : value.toFixed(3);
}

function pct(value) {
  if (!value || !Number.isFinite(value)) return '';
  return `${(value * 100).toFixed(2)}%`;
}

function compact(value) {
  if (!value || !Number.isFinite(value)) return '';
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return String(Math.round(value));
}

function shortName(value) {
  if (!value) return '';
  return value.replace(/,? Inc\.?| Corporation| Corp\.?| Class .*/gi, '').slice(0, 22);
}

function escapeHTML(value) {
  return String(value || '').replace(/[&<>"']/g, (ch) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[ch]));
}

refresh();
state.timer = setInterval(refresh, 1000);
