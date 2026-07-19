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
  isReplay: false,
  view: 'playbook',
  extended: null,
  extendedQuery: '',
  extendedIndustry: '',
  extendedFollowLive: true,
  extendedSelectedID: 0,
  extendedSymbols: null,
  soundEnabled: false,
  soundMuted: false,
  sound: null,
  kaneSoundEnabled: false, kaneSoundMuted: false, kaneSymbols: null,
  kaneSound: null,
  kaneVolumeFilter: 100000,
  kaneSelectedClock: 'live',
  refreshing: false,
  stripOrder: [],
  extendedOrder: [],
  extendedOrderSnapshot: 0,
  extendedSort: { key: 'ratio', direction: 'desc' },
  renderedStats: '',
  renderedExtendedSummary: '',
  eventSource: null,
  generations: { playbook: 0, cavg: 0, kane: 0 },
  browserMetrics: { messages: 0, parseMS: [], mergeMS: [], renderMS: [], resyncs: 0 },
  alertIDs: new Set(),
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
  replayTime: document.getElementById('replayTime'),
  replayGo: document.getElementById('replayGo'),
  live: document.getElementById('live'),
  historyStatus: document.getElementById('historyStatus'),
  playbookControls: document.getElementById('playbookControls'),
  extendedControls: document.getElementById('extendedControls'),
  playbookView: document.getElementById('playbookView'),
  extendedView: document.getElementById('extendedView'),
  extendedTab: document.getElementById('extendedTab'),
  extendedRows: document.getElementById('extendedRows'),
  extendedSummary: document.getElementById('extendedSummary'),
  extendedSearch: document.getElementById('extendedSearch'),
  extendedBack: document.getElementById('extendedBack'),
  extendedPrevHit: document.getElementById('extendedPrevHit'),
  extendedNextHit: document.getElementById('extendedNextHit'),
  extendedForward: document.getElementById('extendedForward'),
  extendedLive: document.getElementById('extendedLive'),
  extendedForwardMinute: document.getElementById('extendedForwardMinute'),
  extendedStatus: document.getElementById('extendedStatus'),
  soundToggle: document.getElementById('soundToggle'),
  soundMute: document.getElementById('soundMute'),
  extendedRatioHeading: document.getElementById('extendedRatioHeading'),
  extendedIndustries: document.getElementById('extendedIndustries'),
  kaneView: document.getElementById('kaneView'), kaneRows: document.getElementById('kaneRows'), kaneSummary: document.getElementById('kaneSummary'), kaneControls: document.getElementById('kaneControls'),
  kaneSoundToggle: document.getElementById('kaneSoundToggle'), kaneSoundMute: document.getElementById('kaneSoundMute'),
  kaneVolumeFilter: document.getElementById('kaneVolumeFilter'), kaneVolumePresets: document.getElementById('kaneVolumePresets'),
  kaneSnapshotControls: document.getElementById('kaneSnapshotControls'),
  kaneForwardMinute: document.getElementById('kaneForwardMinute'),
};

document.querySelector('.extended-table thead').addEventListener('click', (event) => {
  const heading = event.target.closest('th[data-sort]');
  if (!heading) return;
  const key = heading.dataset.sort;
  state.extendedSort.direction = state.extendedSort.key === key && state.extendedSort.direction === 'desc' ? 'asc' : 'desc';
  state.extendedSort.key = key;
  renderExtended();
});

document.querySelector('.view-tabs').addEventListener('click', (event) => {
  const button = event.target.closest('button[data-view]');
  if (!button || button.hidden) return;
  selectView(button.dataset.view);
});

el.extendedSearch.addEventListener('input', () => {
  state.extendedQuery = el.extendedSearch.value.trim().toUpperCase();
  renderExtended();
});
el.extendedIndustries.addEventListener('click', (event) => {
  const row = event.target.closest('[data-industry]'); if (!row) return;
  state.extendedIndustry = state.extendedIndustry === row.dataset.industry ? '' : row.dataset.industry;
  renderExtended();
});

el.extendedBack.addEventListener('click', () => moveExtended(-1, false));
el.extendedForward.addEventListener('click', () => moveExtended(1, false));
el.extendedPrevHit.addEventListener('click', () => moveExtended(-1, true));
el.extendedNextHit.addEventListener('click', () => moveExtended(1, true));
el.extendedLive.addEventListener('click', async () => {
  state.extendedFollowLive = true;
  state.extendedSelectedID = 0;
  state.extendedOrder = [];
  state.extendedOrderSnapshot = 0;
  await refreshExtended();
});
el.extendedForwardMinute.addEventListener('click', async () => {
  if (!state.isReplay) return;
  await replayStep(1); await refreshExtended();
});

el.soundToggle.addEventListener('click', () => {
  state.soundEnabled = !state.soundEnabled;
  if (state.soundEnabled && state.extended && !state.sound) {
    state.sound = new Audio(state.extended.sound_url || '/api/extended/sound');
    state.sound.preload = 'auto';
    state.sound.load();
  }
  updateSoundControls();
});

el.soundMute.addEventListener('click', () => {
  state.soundMuted = !state.soundMuted;
  if (state.sound) state.sound.muted = state.soundMuted;
  updateSoundControls();
});

el.kaneSoundToggle.addEventListener('click', () => {
  state.kaneSoundEnabled = !state.kaneSoundEnabled;
  if (state.kaneSoundEnabled && !state.kaneSound) { state.kaneSound = new Audio('/api/extended/sound'); state.kaneSound.preload = 'auto'; state.kaneSound.load(); }
  updateKaneSoundControls();
});
el.kaneSoundMute.addEventListener('click', () => { state.kaneSoundMuted = !state.kaneSoundMuted; updateKaneSoundControls(); });
el.kaneVolumeFilter.addEventListener('input', () => {
  state.kaneVolumeFilter = Math.max(0, Number(el.kaneVolumeFilter.value || 0));
  updateKaneVolumePresets(); syncKaneSymbols(); renderKane((state.livePayload && state.livePayload.kane) || {});
});
el.kaneVolumePresets.addEventListener('click', (event) => {
  const button = event.target.closest('button[data-volume]'); if (!button) return;
  state.kaneVolumeFilter = Number(button.dataset.volume); el.kaneVolumeFilter.value = String(state.kaneVolumeFilter);
  updateKaneVolumePresets(); syncKaneSymbols(); renderKane((state.livePayload && state.livePayload.kane) || {});
});
el.kaneSnapshotControls.addEventListener('click', (event) => {
  const button = event.target.closest('button[data-clock]'); if (!button || button.disabled) return;
  state.kaneSelectedClock = button.dataset.clock; renderKane(selectedKaneState()); updateKaneSnapshotControls();
});
el.kaneForwardMinute.addEventListener('click', async () => {
  if (!state.isReplay) return;
  state.kaneSelectedClock = 'live'; updateKaneSnapshotControls();
  await replayStep(1);
});

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
  if (state.isReplay) {
    replayStep(-1);
    return;
  }
  if (state.history.length === 0) return;
  state.followLive = false;
  state.selectedIndex = clampIndex(state.selectedIndex < 0 ? state.history.length - 1 : state.selectedIndex);
  state.selectedIndex = Math.max(0, state.selectedIndex - 1);
  useSelectedSnapshot();
});

el.pause.addEventListener('click', () => {
  if (state.isReplay) return;
  if (state.history.length === 0) return;
  state.followLive = false;
  state.selectedIndex = clampIndex(state.selectedIndex < 0 ? state.history.length - 1 : state.selectedIndex);
  useSelectedSnapshot();
});

el.forward.addEventListener('click', () => {
  if (state.isReplay) {
    replayStep(1);
    return;
  }
  if (state.history.length === 0) return;
  state.followLive = false;
  state.selectedIndex = clampIndex(state.selectedIndex < 0 ? state.history.length - 1 : state.selectedIndex);
  state.selectedIndex = Math.min(state.history.length - 1, state.selectedIndex + 1);
  useSelectedSnapshot();
});

el.replayGo.addEventListener('click', () => {
  replaySeek(el.replayTime.value);
});

el.replayTime.addEventListener('keydown', (event) => {
  if (event.key !== 'Enter') return;
  event.preventDefault();
  replaySeek(el.replayTime.value);
});

el.live.addEventListener('click', () => {
  if (state.isReplay) return;
  state.followLive = true;
  state.selectedIndex = state.history.length - 1;
  if (state.livePayload) applyPayload(state.livePayload);
});

async function refresh() {
  if (state.refreshing) return;
  state.refreshing = true;
  try {
    const response = await fetch('/api/state', { cache: 'no-store' });
    const payload = await response.json();
    if (payload.mode === 'replay') {
      state.livePayload = payload;
      applyPayload(payload);
      return;
    }
    state.livePayload = payload;
    addHistorySnapshot(payload);
    if (state.followLive) {
      state.selectedIndex = state.history.length - 1;
      applyPayload(payload);
    } else {
      updateHistoryStatus();
    }
    if (state.view === 'extended') await refreshExtended();
  } catch (error) {
    el.updated.textContent = `offline ${new Date().toLocaleTimeString()}`;
  } finally {
    state.refreshing = false;
  }
}

function connectEvents() {
  if (state.isReplay || state.eventSource) return;
  const source = new EventSource('/api/events'); state.eventSource = source;
  source.onmessage = (message) => { const received=performance.now();const parseStart=performance.now();const delta = JSON.parse(message.data);state.browserMetrics.parseMS.push(performance.now()-parseStart);state.browserMetrics.messages++;
    if(delta.type==='resync_required'){state.browserMetrics.resyncs++;source.close();state.eventSource=null;refresh().then(connectEvents);return}
    requestAnimationFrame(() => { const mergeStart=performance.now();
    if (delta.full) { if(delta.playbook_generation<state.generations.playbook)return;state.generations={playbook:delta.playbook_generation||0,cavg:delta.cavg_generation||0,kane:delta.kane_generation||0};state.livePayload = delta.full;state.extended=delta.full.cavg||state.extended;applyPayload(delta.full);requestAnimationFrame(()=>{document.body.dataset.lastRender=String(Date.now());document.body.dataset.playbookGeneration=String(state.generations.playbook);document.body.dataset.cavgGeneration=String(state.generations.cavg);document.body.dataset.kaneGeneration=String(state.generations.kane)});return; }
    if (!state.livePayload) return;
    if(delta.playbook_generation<=state.generations.playbook)return;
    if(delta.playbook_base_generation!==state.generations.playbook){state.browserMetrics.resyncs++;source.close();state.eventSource=null;refresh().then(connectEvents);return}
    const bySymbol = new Map((state.livePayload.rows || []).map((row) => [row.symbol, row]));
    for (const row of delta.rows || []) bySymbol.set(row.symbol, row);
    state.livePayload.rows = [...bySymbol.values()]; state.livePayload.stats = delta.stats || state.livePayload.stats;
    state.livePayload.generation = delta.generation; state.livePayload.published_at = delta.published_at;
    state.generations.playbook=delta.playbook_generation;
    if(delta.cavg&&delta.cavg_generation>state.generations.cavg){if(delta.cavg_base_generation!==state.generations.cavg){state.browserMetrics.resyncs++;source.close();state.eventSource=null;refresh().then(connectEvents);return};detectExtendedAdditions((delta.cavg.selected&&delta.cavg.selected.rows)||[]);state.extended=delta.cavg;state.generations.cavg=delta.cavg_generation;if(state.view==='extended')renderExtended()}
    if(delta.kane&&delta.kane_generation>state.generations.kane){if(delta.kane_base_generation!==state.generations.kane){state.browserMetrics.resyncs++;source.close();state.eventSource=null;refresh().then(connectEvents);return};state.livePayload.kane=delta.kane;state.generations.kane=delta.kane_generation}
    state.browserMetrics.mergeMS.push(performance.now()-mergeStart);applyPayload(state.livePayload);requestAnimationFrame(()=>{state.browserMetrics.renderMS.push(performance.now()-received);document.body.dataset.lastRender=String(Date.now());document.body.dataset.playbookGeneration=String(state.generations.playbook);document.body.dataset.cavgGeneration=String(state.generations.cavg);document.body.dataset.kaneGeneration=String(state.generations.kane);if(location.search.includes('e2e=1')&&state.generations.playbook>0)source.close()})
  });};
  source.onerror = () => { el.updated.textContent = `reconnecting ${new Date().toLocaleTimeString()}`;source.close();state.eventSource=null;setTimeout(()=>refresh().then(connectEvents),1000); };
}

async function replaySeek(clock) {
  if (!clock) return;
  await replayControl('/api/replay/seek', { clock });
}

async function replayStep(minutes) {
  await replayControl('/api/replay/step', { minutes });
}

async function replayControl(url, body) {
  try {
    setReplayBusy(true);
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) throw new Error(await response.text());
    const payload = await response.json();
    state.livePayload = payload;
    applyPayload(payload);
  } catch (error) {
    el.historyStatus.textContent = 'Replay control failed';
  } finally {
    setReplayBusy(false);
  }
}

function setReplayBusy(busy) {
  if (!state.isReplay) return;
  el.back.disabled = busy;
  el.forward.disabled = busy;
  el.replayGo.disabled = busy;
  el.replayTime.disabled = busy;
}

function applyPayload(payload) {
  state.isReplay = payload.mode === 'replay';
  el.kaneForwardMinute.disabled = !state.isReplay;
  const extendedAvailable = payload.mode === 'live' || payload.mode === 'replay';
  state.rows = payload.rows || [];
  if(payload.cavg)state.extended=payload.cavg;
  renderKane(payload.kane || {});
  updateKaneSnapshotControls();
  detectKaneEntries(payload.kane || {});
  state.volumeFilter = Number(payload.volume_filter || 0);
  document.body.classList.toggle('replay-mode', state.isReplay);
  el.extendedTab.hidden = !extendedAvailable;
  el.extendedForwardMinute.hidden = !state.isReplay;
  if (!extendedAvailable && state.view === 'extended') selectView('playbook');
  el.project.textContent = payload.project || '0945-playbook';
  el.mode.textContent = payload.mode || 'mode';
  el.clock.textContent = payload.clock || '--:--:--';
  updateReplayInput(payload);
  updateUpdatedText(payload);
  updateHistoryStatus();
  if (state.view === 'extended') {
    // refreshExtended renders this tab. Do not briefly paint the playbook stats
    // first on every poll; that visible swap was the remaining tab flicker.
    return;
  }
  renderStats(payload.stats || {});
  render();
}

function selectView(view) {
  state.view = view;
  const extended = view === 'extended'; const kane = view === 'kane';
  document.body.classList.toggle('extended-mode', extended);
  document.body.classList.toggle('kane-mode', kane);
  el.playbookView.hidden = extended || kane;
  el.playbookControls.hidden = extended || kane;
  el.extendedView.hidden = !extended;
  el.extendedControls.hidden = !extended;
  el.kaneView.hidden = !kane; el.kaneControls.hidden = !kane;
  for (const button of document.querySelectorAll('.view-tabs button[data-view]')) {
    button.classList.toggle('active', button.dataset.view === view);
  }
  if (extended) {
    renderExtended();
    updateExtendedStatus();
    refreshExtended();
  } else if (kane) {
    renderKane((state.livePayload && state.livePayload.kane) || {});
  } else if (state.livePayload) {
    renderStats(state.livePayload.stats || {});
  }
}

function renderKane(kane) {
  if (kane === (state.livePayload && state.livePayload.kane)) kane = selectedKaneState();
  const allRows = kane.rows || [];
  const rows = allRows.filter((row) => Number(row.volume_from_0400 || 0) >= state.kaneVolumeFilter);
  const preferredSymbols = new Set();
  const preferredSetups = new Set();
  for (const row of rows) {
    if (preferredSetups.has(row.setup)) continue;
    preferredSetups.add(row.setup); preferredSymbols.add(row.symbol);
  }
  const preferred = preferredSymbols.size;
  el.kaneSummary.innerHTML = `<strong>${kane.preliminary ? 'PRELIMINARY' : 'OPEN-LOCKED'} · ${rows.length}/${allRows.length} shown</strong><span>Min volume ${compact(state.kaneVolumeFilter) || 'ALL'} since 04:00 · ${preferred} visible strategy leader${preferred === 1 ? '' : 's'}</span><span>Each setup ranks independently; EV is sample evidence—not ticker-specific predicted EV</span>`;
  reconcileRows(el.kaneRows, rows.map((row) => {
    const preferredOnScreen = preferredSymbols.has(row.symbol);
    return `<tr data-key="${escapeHTML(row.symbol)}" class="${preferredOnScreen ? 'phase-active side-long' : 'side-long'}">
    <td><strong>${escapeHTML(row.setup)} #${row.rank}</strong>${preferredOnScreen ? '<div class="pos">PREFERRED ON SCREEN</div>' : ''}</td>
    <td class="sym"><a href="${row.chart_url}" target="_blank" rel="noreferrer">${escapeHTML(row.symbol)}</a><div>${escapeHTML(shortName(row.name))}</div></td>
    <td>${escapeHTML(row.setup)}</td><td class="${row.gap_pct < 0 ? 'neg' : 'pos'}">${pct(row.gap_pct)}</td><td>${Number(row.gap_atr || 0).toFixed(2)}×</td><td>${money(row.price)}</td><td>${compact(row.volume_from_0400)}</td>
    <td>${pct(row.prior_close_location)}</td><td>${pct(row.sample_ev)}</td><td>${pct(row.win_rate)}</td><td>${pct(row.target_pct)} / ${pct(row.stop_pct)} stop</td><td class="note">${escapeHTML(row.reason)}</td></tr>`;
  }));
}

function selectedKaneState() {
  const live = (state.livePayload && state.livePayload.kane) || {};
  if (state.kaneSelectedClock === 'live') return live;
  const snapshot = (live.history || []).find((point) => point.clock === state.kaneSelectedClock);
  return snapshot || live;
}

function updateKaneSnapshotControls() {
  const history = ((state.livePayload && state.livePayload.kane && state.livePayload.kane.history) || []);
  const available = new Set(history.map((point) => point.clock));
  for (const button of el.kaneSnapshotControls.querySelectorAll('button[data-clock]')) {
    const live = button.dataset.clock === 'live'; button.disabled = !live && !available.has(button.dataset.clock);
    button.classList.toggle('active', button.dataset.clock === state.kaneSelectedClock);
  }
}

function detectKaneEntries(kane) {
  const symbols = new Set((kane.rows || []).filter((row) => Number(row.volume_from_0400 || 0) >= state.kaneVolumeFilter).map((row) => row.symbol));
  const alerts = (kane.rows || []).filter((row) => row.alert_eligible && row.alert_id && !state.alertIDs.has(row.alert_id));
  for (const row of alerts) state.alertIDs.add(row.alert_id);
  if (kane.health === 'READY' && alerts.length && state.kaneSoundEnabled && !state.kaneSoundMuted) playKaneAlertSound();
  state.kaneSymbols = symbols;
}

function updateKaneVolumePresets() {
  for (const button of el.kaneVolumePresets.querySelectorAll('button[data-volume]')) button.classList.toggle('active', Number(button.dataset.volume) === state.kaneVolumeFilter);
}

function syncKaneSymbols() {
  const rows = (state.livePayload && state.livePayload.kane && state.livePayload.kane.rows) || [];
  state.kaneSymbols = new Set(rows.filter((row) => Number(row.volume_from_0400 || 0) >= state.kaneVolumeFilter).map((row) => row.symbol));
}

function playKaneAlertSound() {
  if (!state.kaneSound) return;
  state.kaneSound.currentTime = 0; state.kaneSound.muted = state.kaneSoundMuted;
  state.kaneSound.play().catch(() => { el.kaneSummary.title = 'Click Start sound again to allow browser audio'; });
}

function updateKaneSoundControls() {
  el.kaneSoundToggle.textContent = state.kaneSoundEnabled ? 'Stop sound' : 'Start sound';
  el.kaneSoundMute.disabled = !state.kaneSoundEnabled; el.kaneSoundMute.textContent = state.kaneSoundMuted ? 'Unmute' : 'Mute';
}

async function refreshExtended(selectedID = 0) {
  if (!state.livePayload || (state.livePayload.mode !== 'live' && state.livePayload.mode !== 'replay')) return;
  const requestedID = selectedID || (!state.extendedFollowLive ? state.extendedSelectedID : 0);
  try {
    const response = await fetch('/api/extended', { cache: 'no-store' });
    if (!response.ok) return;
    const latest = await response.json();
    detectExtendedAdditions(latest.selected && latest.selected.rows || []);
    let payload = latest;
    if (requestedID && requestedID !== latest.live_id) {
      const selectedResponse = await fetch(`/api/extended?minute=${encodeURIComponent(requestedID)}`, { cache: 'no-store' });
      if (selectedResponse.ok) payload = await selectedResponse.json();
    }
    state.extended = payload;
    if (!state.extendedFollowLive && payload.selected && payload.selected.id) {
      state.extendedSelectedID = payload.selected.id;
    }
    if (state.extendedFollowLive) state.extendedSelectedID = payload.live_id || 0;
    if (state.view === 'extended') renderExtended();
    updateExtendedStatus();
  } catch (_) {
    if (state.view === 'extended') el.extendedStatus.textContent = 'Extended scan unavailable';
  }
}

function detectExtendedAdditions(rows) {
  const next = new Set(rows.map((row) => row.symbol));
  const alerts = rows.filter((row) => row.alert_eligible && row.alert_id && !state.alertIDs.has(row.alert_id));
  for (const row of alerts) state.alertIDs.add(row.alert_id);
  if (alerts.length && state.soundEnabled && !state.soundMuted) playAlertSound();
  state.extendedSymbols = next;
}

function playAlertSound() {
  if (!state.sound && state.extended) state.sound = new Audio(state.extended.sound_url || '/api/extended/sound');
  if (!state.sound) return;
  state.sound.currentTime = 0;
  state.sound.muted = state.soundMuted;
  state.sound.play().catch(() => {
    el.extendedStatus.textContent = 'Click Start sound again to allow browser audio';
  });
}

async function moveExtended(direction, hitsOnly) {
  const history = state.extended && state.extended.history || [];
  if (!history.length) return;
  let index = history.findIndex((point) => point.id === state.extendedSelectedID);
  if (index < 0) index = history.length - 1;
  do {
    index += direction;
  } while (hitsOnly && index >= 0 && index < history.length && !history[index].count);
  if (index < 0 || index >= history.length) return;
  state.extendedFollowLive = false;
  state.extendedSelectedID = history[index].id;
  await refreshExtended(history[index].id);
}

function renderExtended() {
  const payload = state.extended;
  if (!payload) {
    reconcileRows(el.extendedRows, []);
    el.extendedSummary.textContent = 'Waiting for the first extended-hours scan.';
    return;
  }
  const n = Number(payload.avg_close_bars || 15);
  el.extendedRatioHeading.textContent = `C/Avg${n}`;
  const snapshot = payload.selected || {};
  const candidates = snapshot.rows || [];
  const snapshotID = Number(snapshot.id || 0);
  // A live snapshot is recalculated on every refresh and sorted by the latest
  // ratio. Preserve the established visual order so small quote changes do not
  // shuffle all of the rows under the reader's eyes. Historical snapshots still
  // open in their recorded ranking.
  if (!state.extendedFollowLive && state.extendedOrderSnapshot !== snapshotID) {
    state.extendedOrder = [];
  }
  state.extendedOrderSnapshot = snapshotID;
  const present = new Set(candidates.map((row) => row.symbol));
  state.extendedOrder = state.extendedOrder.filter((symbol) => present.has(symbol));
  const known = new Set(state.extendedOrder);
  for (const row of candidates) {
    if (!known.has(row.symbol)) state.extendedOrder.push(row.symbol);
  }
  const bySymbol = new Map(candidates.map((row) => [row.symbol, row]));
  let rows = state.extendedOrder
    .map((symbol) => bySymbol.get(symbol))
    .filter((row) => !state.extendedQuery || row.symbol.includes(state.extendedQuery));
  const { key, direction } = state.extendedSort;
  rows.sort((a, b) => {
    const stringSort = key === 'symbol' || key === 'clock' || key === 'industry' || key === 'kane_fit';
    const av = stringSort ? String(a[key] || '') : Number(a[key] || 0);
    const bv = stringSort ? String(b[key] || '') : Number(b[key] || 0);
    const result = typeof av === 'string' ? av.localeCompare(bv) : av - bv;
    return direction === 'asc' ? result : -result;
  });
  document.querySelectorAll('.extended-table th[data-sort]').forEach((heading) => {
    heading.classList.toggle('sort-active', heading.dataset.sort === key);
    heading.dataset.direction = heading.dataset.sort === key ? direction : '';
  });
  const industries = new Map();
  for (const row of rows) {
    const industry = row.industry || 'Unknown';
    const counts = industries.get(industry) || { total: 0, up: 0, down: 0 };
    counts.total += 1;
    if (row.side > 0) counts.up += 1;
    if (row.side < 0) counts.down += 1;
    industries.set(industry, counts);
  }
  const industryHTML = [...industries.entries()]
    .sort((a, b) => b[1].total - a[1].total || a[0].localeCompare(b[0]))
    .map(([industry, counts]) => `<div class="industry-row${state.extendedIndustry === industry ? ' selected' : ''}" data-industry="${escapeHTML(industry)}" role="button" title="${state.extendedIndustry === industry ? 'Clear industry filter' : `Show only ${escapeHTML(industry)}`}"><span>${escapeHTML(industry)}</span><span class="industry-counts"><b class="industry-up" title="C/Avg above upper threshold">↑ ${counts.up}</b><b class="industry-down" title="C/Avg below lower threshold">↓ ${counts.down}</b><strong title="Total matches">${counts.total}</strong></span></div>`).join('');
  setHTMLIfChanged(el.extendedIndustries, industryHTML || '<p>No matching groups</p>', 'renderedIndustries');
  if (state.extendedIndustry) rows = rows.filter((row) => (row.industry || 'Unknown') === state.extendedIndustry);
  const summaryHTML = `<strong>${snapshot.rows ? snapshot.rows.length : 0} matches${state.extendedIndustry ? ` · ${rows.length} shown` : ''}</strong>
    <span>C/Avg${n} &lt; ${Number(payload.lower_signal_ratio).toFixed(2)} or &gt; ${Number(payload.upper_signal_ratio).toFixed(2)}</span>
    <span>${state.extendedIndustry ? `Industry: ${escapeHTML(state.extendedIndustry)} · ` : ''}${escapeHTML(payload.window_start)}–${escapeHTML(payload.window_end)} ET${state.isReplay ? ' · replay minute' : ''}</span>`;
  setHTMLIfChanged(el.extendedSummary, summaryHTML, 'renderedExtendedSummary');
  reconcileRows(el.extendedRows, rows.map((row) => {
    const sideClass = row.side < 0 ? 'side-short' : 'side-long';
    const ratioDeltaClass = row.delta_pct > 0 ? 'pos' : row.delta_pct < 0 ? 'neg' : 'muted';
    const changeClass = row.change_pct > 0 ? 'pos' : row.change_pct < 0 ? 'neg' : 'muted';
    return `<tr data-key="${escapeHTML(row.symbol)}" class="${sideClass}">
      <td class="sym"><a href="${row.chart_url}" target="_blank" rel="noreferrer">${escapeHTML(row.symbol)}</a><div>${escapeHTML(shortName(row.name))}</div></td>
      <td>${Number(row.ratio || 0).toFixed(4)}</td>
      <td>${money(row.price)}</td>
      <td class="${changeClass}">${pct(row.change_pct)}</td>
      <td class="${ratioDeltaClass}">${pct(row.delta_pct)}</td>
      <td>${escapeHTML(row.clock)}</td>
      <td>${compact(row.volume)}</td>
      <td class="${row.open_gap_pct > 0 ? 'pos' : row.open_gap_pct < 0 ? 'neg' : 'muted'}">${pct(row.open_gap_pct)}</td>
      <td>${Number(row.gap_atr || 0).toFixed(2)}×</td>
      <td>${pct(row.prior_close_location)}</td>
      <td>${escapeHTML(row.kane_fit || '—')}</td>
      <td class="note">${escapeHTML(row.industry)}</td>
    </tr>`;
  }));
  if (state.view === 'extended') {
    const statsHTML = `<div class="stat"><strong>${snapshot.rows ? snapshot.rows.length : 0}</strong><span>Matches</span></div>
      <div class="stat"><strong>${n}</strong><span>Avg bars</span></div>`;
    setHTMLIfChanged(el.stats, statsHTML, 'renderedStats');
  }
}

function updateExtendedStatus() {
  const payload = state.extended;
  if (!payload || !payload.history || !payload.history.length) {
    el.extendedStatus.textContent = 'Waiting for live data in the configured window';
    return;
  }
  const history = payload.history;
  const selected = payload.selected || {};
  const index = history.findIndex((point) => point.id === selected.id);
  el.extendedStatus.textContent = state.extendedFollowLive
    ? `Live ${selected.clock || ''} · ${history.length} minutes stored`
    : `Viewing ${selected.clock || ''} · ${index + 1}/${history.length}`;
  el.extendedBack.disabled = index <= 0;
  el.extendedPrevHit.disabled = !history.slice(0, Math.max(index, 0)).some((point) => point.count);
  el.extendedForward.disabled = index < 0 || index >= history.length - 1;
  el.extendedNextHit.disabled = index < 0 || !history.slice(index + 1).some((point) => point.count);
  el.extendedLive.disabled = state.extendedFollowLive;
}

function updateSoundControls() {
  el.soundToggle.textContent = state.soundEnabled ? 'Stop sound' : 'Start sound';
  el.soundMute.disabled = !state.soundEnabled;
  el.soundMute.textContent = state.soundMuted ? 'Unmute' : 'Mute';
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
  if (state.isReplay) {
    el.updated.textContent = `selected ${payload.clock || '--:--:--'}`;
    return;
  }
  const viewText = payload.updated ? new Date(payload.updated).toLocaleTimeString() : 'waiting';
  if (state.followLive || !state.livePayload) {
    el.updated.textContent = `updated ${viewText}`;
    return;
  }
  el.updated.textContent = `viewing ${payload.clock || '--:--:--'} · live still calculating ${state.livePayload.clock || '--:--:--'}`;
}

function updateHistoryStatus() {
  if (state.isReplay) {
    el.historyStatus.textContent = `Replay ${state.livePayload ? state.livePayload.clock : el.clock.textContent}`;
    el.back.disabled = false;
    el.forward.disabled = false;
    el.pause.disabled = true;
    el.live.disabled = true;
    el.replayTime.disabled = false;
    el.replayGo.disabled = false;
    return;
  }
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
  el.replayTime.disabled = true;
  el.replayGo.disabled = true;
}

function updateReplayInput(payload) {
  if (!state.isReplay || document.activeElement === el.replayTime) return;
  const clock = String(payload.clock || '');
  const match = clock.match(/^(\d{2}:\d{2})/);
  if (match) el.replayTime.value = match[1];
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
  const html = cells.map(([label, value]) => (
    `<div class="stat"><strong>${value || 0}</strong><span>${label}</span></div>`
  )).join('');
  setHTMLIfChanged(el.stats, html, 'renderedStats');
}

function render() {
  const rows = state.rows.filter(rowVisible);
  reconcileRows(el.rows, rows.map(rowHTML));
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
  const candidates = state.rows
    .filter((row) => row.phase === 'active' || row.phase === 'signal' || row.phase === 'likely' || row.phase === 'done')
    .filter(passesVolume);
  const present = new Set(candidates.map((row) => row.symbol));
  state.stripOrder = state.stripOrder.filter((symbol) => present.has(symbol));
  const known = new Set(state.stripOrder);
  candidates
    .filter((row) => !known.has(row.symbol))
    .sort((a, b) => (b.ev_score || 0) - (a.ev_score || 0))
    .forEach((row) => state.stripOrder.push(row.symbol));
  const bySymbol = new Map(candidates.map((row) => [row.symbol, row]));
  const interesting = state.stripOrder.slice(0, 18).map((symbol) => bySymbol.get(symbol));

  reconcileCards(el.strip, interesting.map((row) => {
    const side = row.side < 0 ? 'short' : 'long';
    return `<a data-key="${escapeHTML(row.symbol)}" class="tile side-${side} phase-${row.phase || 'none'}" href="${row.chart_url}" target="_blank" rel="noreferrer">
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
  }));
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
  return `<tr data-key="${escapeHTML(row.symbol)}" class="phase-${row.phase || 'none'} ${sideClass}">
    <td class="sym"><a href="${row.chart_url}" target="_blank" rel="noreferrer">${escapeHTML(row.symbol)}</a><div>${escapeHTML(shortName(row.name))}</div></td>
    <td><span class="pill">${escapeHTML(row.status || '')}</span></td>
    <td class="ev ${evClass}">${fmt(row.ev_score, 0)}</td>
    <td>${escapeHTML(row.action || '')}</td>
    <td>${escapeHTML(row.branch || '-')}</td>
    <td>${fmt(row.ratio, 3)}</td>
    <td class="${deltaClass}">${pct(row.delta_pct)}</td>
    <td>${money(row.price)}</td>
    <td>${money(row.day_high)}</td>
    <td>${money(row.day_low)}</td>
    <td>${money(row.vwap)}</td>
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

function setHTMLIfChanged(element, html, cacheKey) {
  if (state[cacheKey] === html) return;
  state[cacheKey] = html;
  element.innerHTML = html;
}

// Keep existing row nodes in place. Replacing the entire tbody each second makes
// browsers repaint the whole table and is especially distracting while scrolling.
function reconcileRows(container, htmlRows) {
  const template = document.createElement('tbody');
  template.innerHTML = htmlRows.join('');
  reconcileKeyed(container, [...template.children], true);
}

function reconcileCards(container, htmlCards) {
  const template = document.createElement('div');
  template.innerHTML = htmlCards.join('');
  reconcileKeyed(container, [...template.children], false);
}

function reconcileKeyed(container, desired, patchCells) {
  const existing = new Map([...container.children].map((node) => [node.dataset.key, node]));
  desired.forEach((next, index) => {
    const current = existing.get(next.dataset.key);
    if (!current) {
      container.insertBefore(next, container.children[index] || null);
      return;
    }
    if (patchCells) {
      current.className = next.className;
      [...next.children].forEach((cell, cellIndex) => {
        const oldCell = current.children[cellIndex];
        if (oldCell.className !== cell.className) oldCell.className = cell.className;
        if (oldCell.innerHTML !== cell.innerHTML) oldCell.innerHTML = cell.innerHTML;
      });
    } else if (current.outerHTML !== next.outerHTML) {
      current.className = next.className;
      current.href = next.href;
      current.innerHTML = next.innerHTML;
    }
    if (container.children[index] !== current) container.insertBefore(current, container.children[index] || null);
    existing.delete(next.dataset.key);
  });
  existing.forEach((node) => node.remove());
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

refresh().then(connectEvents);
