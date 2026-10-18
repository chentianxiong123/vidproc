// vidproc studio 前端逻辑(非框架:vanilla JS + Alpine 组件)。
//
// 分工:
//   nav.js       — 导航高亮 + 时间轴编辑器(timelineEditor)+ 渲染任务(renderJob)
//   style.css    — 主题变量
//   templates/   — htmx 片段 + Alpine 组件壳(无业务 JS)
//
// 对齐 vidonex Web Studio:TimelineContainer(真实 clip 块/拖拽/吸附/播放头)+
// CanvasViewport(播放头帧预览)+ 渲染进度条。

(function () {
  // ============ 导航高亮(零件):URL 推导,htmx swap / 前进后退同步 ============
  function syncNav() {
    const p = location.pathname;
    document.querySelectorAll('.nav-tab').forEach((a) => {
      const href = a.getAttribute('href');
      a.classList.toggle('nav-active', p === href);
    });
  }
  document.addEventListener('htmx:afterSwap', syncNav);
  window.addEventListener('popstate', syncNav);
  syncNav();

  // ============ 全局播放状态机(vp.playback)============
  // 对齐 vidonex playback store:currentTime/duration/play/seek/stepFrame/
  // loop/volume/mute。播放 = requestAnimationFrame 推进 → 广播 playhead-changed
  // (画布抽帧预览 + 时间轴播放头同步跟随)。
  window.vp = window.vp || {};
  window.vp.playback = {
    currentTime: 0,
    contentDuration: 0, // 内容末端(导出时长)
    duration: 120,      // 时间轴工作区时长(拖拽留白)
    isPlaying: false,
    isLooping: false,
    volume: 1.0,
    isMuted: false,
    playbackRate: 1.0,
    _raf: null,
    _last: 0,

    play() {
      if (this.isPlaying) return;
      const max = this.contentDuration > 0 ? this.contentDuration : this.duration;
      if (this.currentTime >= max) this.currentTime = 0;
      this.isPlaying = true;
      this._last = performance.now();
      this._raf = requestAnimationFrame((ts) => this._loop(ts));
    },
    pause() {
      this.isPlaying = false;
      if (this._raf) { cancelAnimationFrame(this._raf); this._raf = null; }
    },
    togglePlay() { this.isPlaying ? this.pause() : this.play(); },

    _loop(ts) {
      if (!this.isPlaying) return;
      const dt = (ts - this._last) / 1000;
      this._last = ts;
      this.currentTime += dt * this.playbackRate;
      const max = this.contentDuration > 0 && this.currentTime - dt * this.playbackRate < this.contentDuration
        ? this.contentDuration : this.duration;
      if (this.currentTime >= max) {
        if (this.isLooping) { this.currentTime = 0; }
        else { this.currentTime = max; this.pause(); this._emit(); return; }
      }
      this._emit();
      this._raf = requestAnimationFrame((t2) => this._loop(t2));
    },

    seek(t) {
      this.currentTime = Math.max(0, Math.min(t, this.duration));
      this._emit();
    },
    stepFrame(delta, fps = 30) {
      this.pause();
      this.seek(this.currentTime + delta / fps);
    },
    jumpToStart() { this.pause(); this.currentTime = 0; this._emit(); },
    jumpToEnd() { this.pause(); this.currentTime = this.duration; this._emit(); },

    // 广播播放头变化:画布预览帧 + 时间轴播放头 + 状态栏时间码同步
    _emit() {
      window.dispatchEvent(new CustomEvent('playhead-changed', {
        detail: { t: this.currentTime, pid: window.vp.currentProjectId || null, source: 'playback' },
      }));
    },

    // 对齐 vidonex formatTimecode:HH:MM:SS:FF(帧)
    formatTimecode(seconds, fps = 30) {
      const totalFrames = Math.floor(seconds * fps);
      const frames = totalFrames % fps;
      const totalSecs = Math.floor(seconds);
      const secs = totalSecs % 60;
      const mins = Math.floor(totalSecs / 60) % 60;
      const hours = Math.floor(totalSecs / 3600);
      const pad = (n, z = 2) => String(n).padStart(z, '0');
      return `${pad(hours)}:${pad(mins)}:${pad(secs)}:${pad(frames)}`;
    },
    fmt(sec) { return this.formatTimecode(sec); },
  };

  // 时间轴加载后同步内容时长 → playback.duration
  window.vp.syncTimelineDurations = function (tl) {
    if (!tl) return;
    window.vp.playback.contentDuration = tl.duration || 0;
    window.vp.playback.duration = Math.max(tl.duration + 60, 120);
  };
  window.vp.currentProjectId = null;

  // ============ 视口缩放(vp.viewportZoom)============
  // 对齐 vidonex useViewportZoom:Fit/25/50/75/100/150/200%,滚轮缩放,双击重置。
  window.vp.viewportZoom = {
    zoom: 1.0,
    panX: 0,
    panY: 0,
    ZOOM_PRESETS: [
      { label: 'Fit', value: 1.0, isFit: true },
      { label: '25%', value: 0.25 },
      { label: '50%', value: 0.5 },
      { label: '75%', value: 0.75 },
      { label: '100%', value: 1.0 },
      { label: '150%', value: 1.5 },
      { label: '200%', value: 2.0 },
    ],
    get isFit() { return this.zoom === 1.0 && this.panX === 0 && this.panY === 0; },
    get display() { return this.isFit ? 'Fit' : `${Math.round(this.zoom * 100)}%`; },
    setZoom(v, resetPan = false) {
      this.zoom = Math.min(5, Math.max(0.1, Number(v)));
      if (resetPan) { this.panX = 0; this.panY = 0; }
    },
    zoomIn() { this.setZoom(this.zoom * 1.25); },
    zoomOut() { this.setZoom(this.zoom / 1.25); },
    reset() { this.zoom = 1.0; this.panX = 0; this.panY = 0; },
  };

  // ============ 确认对话框(confirmDialog)============
  // 对齐 vidonex useConfirmDialog:Promise 化的深色确认弹窗,删除/危险操作用。
  window.vp.confirm = function (opts) {
    return new Promise((resolve) => {
      const wrap = document.createElement('div');
      wrap.className = 'fixed inset-0 z-[100] flex items-center justify-center';
      wrap.style.cssText = 'background:rgba(0,0,0,.6);backdrop-filter:blur(2px)';
      wrap.innerHTML = `
        <div class="w-80 rounded-xl bg-gray-900 border border-gray-800 shadow-2xl p-5 text-gray-200">
          <h3 class="text-sm font-semibold mb-2">${opts.title || '确认'}</h3>
          <p class="text-xs text-gray-400 mb-4">${opts.message || ''}</p>
          <div class="flex justify-end gap-2">
            <button class="px-3 py-1.5 rounded-lg text-xs text-gray-300 hover:bg-gray-800 transition" data-act="cancel">${opts.cancelText || '取消'}</button>
            <button class="px-3 py-1.5 rounded-lg text-xs font-medium transition ${opts.isDanger ? 'bg-red-600 hover:bg-red-500 text-white' : 'bg-indigo-500 hover:bg-indigo-400 text-white'}" data-act="ok">${opts.confirmText || '确定'}</button>
          </div>
        </div>`;
      const done = (v) => { wrap.remove(); resolve(v); };
      wrap.querySelector('[data-act="ok"]').addEventListener('click', () => done(true));
      wrap.querySelector('[data-act="cancel"]').addEventListener('click', () => done(false));
      wrap.addEventListener('click', (e) => { if (e.target === wrap) done(false); });
      document.body.appendChild(wrap);
    });
  };


  function fmtTime(sec) {
    if (!isFinite(sec) || sec < 0) return '00:00:00.000';
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    const pad = (n, w) => String(n).padStart(w, '0');
    return `${pad(h, 2)}:${pad(m, 2)}:${s.toFixed(3).padStart(6, '0')}`;
  }

  // ============ 时间轴编辑器(timelineEditor)============
  // 挂到 Alpine:x-data="timelineEditor(pid)"。从 /api/projects/{id}/timeline
  // 拉数值化时间轴,渲染真实 clip 块(宽=时长×pps,绝对定位)并支持:
  //   拖拽移动(pointer events)、Snap 吸附、选中高亮、播放头点击定位。
  // 对齐 vidonex ClipItem:border-2 rounded-lg,选中 indigo-400 + shadow。
  window.timelineEditor = function (pid) {
    return {
      pid: pid || null,
      tl: null, // TimelineJSON
      zoom: 1, // 缩放系数(x1 基准 80px/s)
      playhead: 0, // 当前播放头秒
      selectedId: null, // 选中 clip id
      pps: 80, // 像素/秒(zoom 驱动)
      drag: null, // 进行中的拖拽 {clip, trackIdx, startX, origStart}
      trackWidth: 800, // 滚动区实测宽,做成 clip 区域 min-width
      loading: true,
      error: '',
      busy: false, // 渲染中(详情页按钮联动)
      // 编辑历史(对齐 vidonex undoHistory):操作前快照,50 步上限
      _history: [],
      _historyIdx: -1,

      init() {
        this.$watch('zoom', (v) => { this.pps = 80 * v; });
        this.load();
        // 播放头也可以被外部(标尺/快捷键)跳转
        this.$watch('playhead', (v) => {
          const pid = this.pid;
          window.dispatchEvent(new CustomEvent('playhead-changed', { detail: { t: v, pid } }));
        });
      },

      async load() {
        if (!this.pid) { this.loading = false; return; }
        this.loading = true;
        this.error = '';
        try {
          // 供项目设置弹窗等无作用域组件调用(window.vp.timelineEditorOf)
          window.vp.timelineEditorOf = this;
          const res = await fetch(`/api/projects/${this.pid}/timeline`);
          if (!res.ok) throw new Error(await res.text());
          this.tl = await res.json();
          this.pps = 80 * this.zoom;
          // 播放状态机同步内容/工作区时长
          window.vp.currentProjectId = this.pid;
          window.vp.syncTimelineDurations(this.tl);
          // 播放头默认停在第一个 clip 起点
          const first = this.firstClipStart();
          this.setPlayhead(first);
          this.updateTrackWidth();
          // 历史栈底 = 加载态(undo 恢复到操作前态)
          this.pushHistory();
        } catch (e) {
          this.error = '时间轴加载失败: ' + e.message;
        } finally {
          this.loading = false;
        }
      },

      // setPlayhead 统一入口:播放时由 vp.playback 推,手动 seek 由组件设。
      // 广播 playhead-changed → 画布预览/状态栏同步。循环保护(外部推时不回写播放机)。
      setPlayhead(t, opts) {
        this.playhead = Math.max(0, t);
        const o = opts || {};
        if (!o.silent) {
          window.dispatchEvent(new CustomEvent('playhead-changed', {
            detail: { t: this.playhead, pid: this.pid, source: o.source || 'timeline' },
          }));
        }
      },

      init() {
        this.$watch('zoom', (v) => { this.pps = 80 * v; });
        this.load();
        // 外部播放/seek → 时间轴播放头跟随(来源非自身,避免回环)
        window.addEventListener('playhead-changed', (e) => {
          const d = e.detail || {};
          if (d.source === 'timeline') return;
          if (d.pid !== undefined && d.pid !== this.pid) return;
          this.playhead = d.t || 0;
        });
        // 键盘快捷键
        document.addEventListener('keydown', (e) => this.onKey(e));
      },

      firstClipStart() {
        if (!this.tl || !this.tl.tracks) return 0;
        for (const tr of this.tl.tracks) {
          if (tr.clips && tr.clips.length) return tr.clips[0].start;
        }
        return 0;
      },

      tracks() {
        return (this.tl && this.tl.tracks) || [];
      },
      videoTracks() { return this.tracks().filter((t) => t.kind === 'video' || t.kind === 'overlay'); },
      audioTracks() { return this.tracks().filter((t) => t.kind === 'audio' || t.kind === 'waveform'); },
      otherTracks() { return this.tracks().filter((t) => !['video', 'overlay', 'audio', 'waveform'].includes(t.kind)); },

      updateTrackWidth() {
        // clip 区最小宽度 = 总时长×pps + 播放头余量(IINA 惯例:末尾留一屏)
        const dur = (this.tl && this.tl.duration) || 0;
        this.trackWidth = Math.max(800, dur * this.pps + 160);
      },

      // ============ 素材拖入时间轴(对齐 vidonex addClipToTrack)============
      // 轨道 dragover 显示可放;drop 解析 dataTransfer 素材 → 按落点时间建 clip。
      // 非重叠放置:与轨道已有 clip 冲突时推到最近空档(findNonOverlappingStart)。
      onTrackDragOver(e, kind) {
        if (!this.hasDraggedAsset(e) && !this.hasDraggedClip(e)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'copy';
      },
      hasDraggedClip(e) {
        return e.dataTransfer && e.dataTransfer.types &&
          Array.from(e.dataTransfer.types).includes('application/x-vidproc-clip');
      },
      onTrackDrop(e, kind) {
        if (!this.tl) return;
        // 跨轨 clip 拖拽优先(HTML5 DnD)
        if (this.hasDraggedClip(e)) { this.onTrackClipDrop(e, kind); return; }
        const asset = this.parseAsset(e);
        if (!asset) return;
        e.preventDefault();
        // 落点时间 = 鼠标位置秒 + 轨道区左侧偏移(标尺在轨道头右侧 48px 开始)
        const rect = e.currentTarget.getBoundingClientRect();
        const t = Math.max(0, (e.clientX - rect.left) / this.pps);
        this.addClipFromAsset(asset, kind, t);
      },
      parseAsset(e) {
        try {
          const raw = e.dataTransfer.getData('application/x-vidproc-asset');
          return raw ? JSON.parse(raw) : null;
        } catch (_) { return null; }
      },
      // 素材 → 新 clip(对齐 vidonex addClipToTrack:时长=素材探测值,图片默认 5s)
      addClipFromAsset(asset, trackKind, startTime) {
        const track = this.tracks().find((tr) => tr.kind === trackKind) ||
                      this.tracks().find((tr) => tr.kind === (trackKind === 'audio' ? 'video' : 'audio'));
        if (!track || !this.tl) return;

        let duration = 5.0;
        if (asset.file_type === 'video' && asset.duration_seconds > 0) duration = asset.duration_seconds;
        if (asset.file_type === 'audio' && asset.duration_seconds > 0) duration = asset.duration_seconds;

        // 非重叠:找不与其他 clip 冲突的落点
        const start = this.findNonOverlappingStart(track, startTime !== undefined ? startTime : this.firstFreeStart(track), duration);

        const clip = {
          id: 'clip_' + Date.now().toString(36),
          source: asset.file_path,
          start,
          duration,
          speed: 1,
          scale: 1,
          rotation: 0,
          opacity: 1,
          volume: 1,
          blend_mode: 'normal',
          position: { x: 0, y: 0, alignment: 'center' },
          trim: 0,
        };
        if (!track.clips) track.clips = [];
        track.clips.push(clip);
        this.selectedId = clip.id;
        this.setPlayhead(start, { source: 'timeline' });
        this.updateTrackWidth();
        this.persistTimeline();
      },
      // 轨道下一个可用起点(末尾)
      firstFreeStart(track) {
        let end = 0;
        for (const c of track.clips || []) { if (c.start + c.duration > end) end = c.start + c.duration; }
        return end;
      },
      // 非重叠起点:候选 = 0 与每个 clip 的右边缘 + 左边缘;取最近且不冲突者
      findNonOverlappingStart(track, targetStart, duration) {
        const safeStart = Math.max(0, targetStart);
        const others = (track.clips || []).map((c) => ({
          start: c.start, end: c.start + c.duration,
        })).sort((a, b) => a.start - b.start);
        if (!others.length) return safeStart;
        const hasOverlap = (s) => others.some((c) => Math.max(s, c.start) < Math.min(s + duration, c.end) - 0.001);
        if (!hasOverlap(safeStart)) return safeStart;
        const candidates = [0];
        for (const c of others) {
          candidates.push(c.end);
          if (c.start >= duration) candidates.push(c.start - duration);
        }
        const valid = candidates.filter((p) => p >= 0 && !hasOverlap(p));
        if (valid.length) {
          valid.sort((a, b) => Math.abs(a - safeStart) - Math.abs(b - safeStart));
          return valid[0];
        }
        return others[others.length - 1].end;
      },

      // ============ 轨道管理(对齐 vidonex addTrack:5 类型)============
      // 加轨道:audio/video/overlay/subtitle/waveform。新轨带切片偏移编号,防止 id 冲突。
      addTrack(kind) {
        if (!this.tl) return;
        const exists = this.tracks().filter((t) => t.kind === kind).length;
        const id = `track_${kind}_${Date.now().toString(36)}`;
        const track = { id, kind, clips: [], z_index: exists, volume: 1, muted: false };
        if (kind === 'overlay') track.z_index = this.tracks().filter((t) => ['video', 'overlay'].includes(t.kind)).length + 1;
        if (kind === 'subtitle') track.subtitles = [];
        if (kind === 'waveform') track.waveform = {};
        this.tl.tracks.push(track);
        if (window.vp.timelineEditorOf) window.dispatchEvent(new CustomEvent('track-changed'));
        this.persistTimeline();
      },
      // 复制选中 clip 到同轨(对齐 vidonex duplicateClip:偏移 +1s)
      duplicateClip() {
        if (!this.selectedId || !this.tl) return;
        const src = this.findClip(this.selectedId);
        if (!src) return;
        const track = this.tracks().find((tr) => tr.clips && tr.clips.some((c) => c.id === src.id));
        if (!track) return;
        const copy = JSON.parse(JSON.stringify(src));
        copy.id = src.id + '_dup_' + Date.now().toString(36);
        copy.start = src.start + src.duration;
        track.clips.push(copy);
        this.selectedId = copy.id;
        this.updateTrackWidth();
        this.persistTimeline();
      },

      // ============ 转场(对齐 vidonex TransitionHandle + addTransition)============
      // 转场挂在轨道(from 与 to 交接处),dot 点击循环切换类型,
      // 双击移除。前端序列化回 spec 的 transitions 数组。
      addTransition(type, fromId, toId) {
        if (!this.tl) return;
        // 找包含 from 的轨道(两 clip 相邻才是合法转场)
        const track = this.tracks().find((tr) => tr.clips && tr.clips.some((c) => c.id === fromId));
        if (!track) return;
        const from = track.clips.find((c) => c.id === fromId);
        const to = track.clips.find((c) => c.id === toId);
        if (!from || !to) return;
        // 校验相邻
        if (Math.abs(from.start + from.duration - to.start) > 0.05) return false;
        if (!track.transitions) track.transitions = [];
        // 同接口已存在转场 → 更新类型
        const existing = track.transitions.find((t) => t.from === fromId && t.to === toId);
        const tr = existing || { from: fromId, to: toId };
        if (!existing) track.transitions.push(tr);
        tr.type = type;
        tr.duration = 0.5; // 默认 0.5s
        this.persistTimeline();
        return true;
      },
      removeTransition(fromId, toId) {
        if (!this.tl) return;
        for (const tr of this.tracks()) {
          if (!tr.transitions) continue;
          tr.transitions = tr.transitions.filter((t) => !(t.from === fromId && t.to === toId));
        }
        this.persistTimeline();
      },
      // 转场类型轮换(dot 点击)
      nextTransitionType(cur) {
        const types = ['dissolve', 'fade', 'fadeblack', 'wipeleft', 'wiperight', 'slideleft', 'slideright', 'smoothleft', 'smoothright'];
        const i = types.indexOf(cur);
        return types[(i + 1) % types.length];
      },
      // 转场 dot 样式:位于交接点中央
      transitionStyle(tr, track) {
        const from = track.clips.find((c) => c.id === tr.from);
        if (!from) return 'display:none';
        const x = (from.start + from.duration) * this.pps;
        return `left:${x - 10}px;width:20px`;
      },
      // 轨道上 from=clip 的转场(无 = null)
      transitionOf(track, clip) {
        if (!track || !track.transitions) return null;
        return track.transitions.find((t) => t.from === clip.id) || null;
      },
      // clip 在轨道内的下一个相邻 clip(无 = null)
      nextClipId(track, clip) {
        if (!track || !track.clips) return null;
        const sorted = track.clips.slice().sort((a, b) => a.start - b.start);
        const i = sorted.findIndex((c) => c.id === clip.id);
        const n = sorted[i + 1];
        return n ? n.id : null;
      },
      // 点击转场点:轮换类型 / Shift+click 换 fadeblack / 双击移除
      cycleTransition(track, clip) {
        const next = this.nextClipId(track, clip);
        if (!next) return;
        const cur = this.transitionOf(track, clip);
        const type = cur ? this.nextTransitionType(cur.type) : 'dissolve';
        this.addTransition(type, clip.id, next);
      },
      // 转场 dot 颜色(dissolve=青 / fade=紫 / wipe=橙 / 其它=灰)
      transitionColor(track, clip) {
        const t = this.transitionOf(track, clip);
        if (!t) return 'text-gray-600';
        const c = t.type.startsWith('wipe') ? 'text-orange-400' :
                  t.type.startsWith('slide') ? 'text-yellow-400' :
                  t.type.startsWith('fade') ? 'text-purple-400' : 'text-cyan-400';
        return c;
      },
      // ============ trim 手柄(对齐 vidonex 拖 clip 左右缘改时长)============
      // Left = 改 start+duration(右缘不动);Right = 改 duration(左缘不动)。
      beginTrim(e, clip, edge) {
        if (e.button !== 0) return;
        e.preventDefault();
        e.stopPropagation();
        this.trim = { clip, edge, startX: e.clientX, origStart: clip.start, origDur: clip.duration };
        this._trimMove = (ev) => this.onTrimMove(ev);
        this._trimEnd = () => this.onTrimEnd();
        window.addEventListener('pointermove', this._trimMove);
        window.addEventListener('pointerup', this._trimEnd, { once: true });
        try { e.target.setPointerCapture && e.target.setPointerCapture(e.pointerId); } catch (_) {}
      },
      onTrimMove(e) {
        if (!this.trim) return;
        const dx = (e.clientX - this.trim.startX) / this.pps;
        const { clip, edge, origStart, origDur } = this.trim;
        if (edge === 'left') {
          // 左缘移动:start 平移,duration 补偿(右缘不动)
          let ns = Math.max(0, Math.min(origStart + origDur - 0.1, origStart + dx));
          clip.start = ns;
          clip.duration = origDur + (origStart - ns);
        } else {
          // 右缘:duration 拉长/缩短
          clip.duration = Math.max(0.1, origDur + dx);
        }
        this.updateTrackWidth();
      },
      onTrimEnd() {
        window.removeEventListener('pointermove', this._trimMove);
        window.removeEventListener('pointerup', this._trimEnd);
        if (this.trim && this.trim.clip) {
          this.persistTimeline();
        }
        this.trim = null;
      },

      // 跨轨拖拽:拖 audio clip 到 video 轨(或反之)。clip 移到目标轨,
      // kind 不变但轨道归属变化。由 clip 元素 dragstart/轨道 drop 处理。
      beginClipDrag(e, clip, trackIdx) {
        if (e.button !== 0) return;
        const track = this.videoTracks().concat(this.audioTracks(), this.otherTracks())[trackIdx];
        if (!track) return;
        e.preventDefault();
        e.stopPropagation();
        this.drag = {
          clip, trackIdx, startX: e.clientX, origStart: clip.start, moved: false,
          sourceTrackId: track.id,
        };
        this._dragMove = (ev) => this.onDragMove(ev);
        this._dragEnd = () => this.onDragEnd();
        window.addEventListener('pointermove', this._dragMove);
        window.addEventListener('pointerup', this._dragEnd, { once: true });
        try { e.target.setPointerCapture && e.target.setPointerCapture(e.pointerId); } catch (_) {}
      },
      // 拖到另一轨道的 drop:把 clip 挪过去(时间按落点)
      onTrackClipDrop(e, targetKind) {
        e.preventDefault();
        e.stopPropagation();
        const clip = this.parseDraggedClip(e);
        if (!clip) return;
        const srcTrack = this.tracks().find((tr) => tr.clips && tr.clips.some((c) => c.id === clip.id));
        // 目标轨 = 第一命中;若无该 kind 轨道则新建
        let dst = this.tracks().find((tr) => tr.kind === targetKind);
        if (!dst) {
          this.addTrack(targetKind);
          dst = this.tracks().find((tr) => tr.kind === targetKind);
        }
        if (!dst || !srcTrack || dst.id === srcTrack.id) return;
        // 从源轨移除
        srcTrack.clips = srcTrack.clips.filter((c) => c.id !== clip.id);
        // 落点时间(相对目标轨内容区)
        const rect = e.currentTarget ? e.currentTarget.getBoundingClientRect() : null;
        if (rect) {
          const t = Math.max(0, (e.clientX - rect.left) / this.pps);
          clip.start = t;
        }
        if (!dst.clips) dst.clips = [];
        dst.clips.push(clip);
        this.selectedId = clip.id;
        this.updateTrackWidth();
        this.persistTimeline();
      },
      // HTML5 DnD:clip 元素 dragstart 时把 id 挂 transport
      beginClipDnD(e, clip) {
        e.dataTransfer.setData('application/x-vidproc-clip', clip.id);
        e.dataTransfer.effectAllowed = 'move';
        this.selectedId = clip.id;
        try { e.dataTransfer.setDragImage(e.target, e.offsetX, 10); } catch (_) {}
      },
      parseDraggedClip(e) {
        try {
          const id = e.dataTransfer.getData('application/x-vidproc-clip');
          if (!id) return null;
          return this.findClip(id);
        } catch (_) { return null; }
      },
      finishClipDrag() {
        this.drag = null;
      },

      // 音频 clip 波形(对齐 vidonex ClipWaveform:种子随机伪波形,不读文件)
      drawWaveform(canvas) {
        const seed = canvas.dataset.seed || Math.random().toString(36);
        const kind = canvas.dataset.kind || 'audio';
        // 确定性伪随机(同 seed 同波形)
        let h = 0xdeadbeef;
        for (let i = 0; i < seed.length; i++) h = Math.imul(h ^ seed.charCodeAt(i), 2654435761);
        const rand = () => {
          h = Math.imul(h ^ (h >>> 16), 2246822507);
          h = Math.imul(h ^ (h >>> 13), 3266489909);
          return ((h >>> 0) / 4294967296);
        };
        const dpr = window.devicePixelRatio || 1;
        const w = canvas.clientWidth || 200;
        const cw = canvas.clientHeight || 24;
        canvas.width = w * dpr;
        canvas.height = cw * dpr;
        const ctx = canvas.getContext('2d');
        if (!ctx) return;
        ctx.scale(dpr, dpr);
        ctx.clearRect(0, 0, w, cw);
        const barW = 3, gap = 1.5, step = barW + gap;
        const mid = cw / 2, maxAmp = cw / 2 * 0.85;
        const primary = kind === 'audio' ? '#34d399' : '#c084fc';
        const secondary = kind === 'audio' ? '#059669' : '#7e22ce';
        // 基线
        ctx.strokeStyle = 'rgba(255,255,255,0.15)';
        ctx.beginPath(); ctx.moveTo(0, mid); ctx.lineTo(w, mid); ctx.stroke();
        // 竖条(随机峰 + 包络:中间高两边低的曲子感)
        for (let x = 0; x < w; x += step) {
          const t = x / w;
          const envelope = 0.35 + 0.65 * Math.sin(Math.PI * t) + 0.1 * (rand() - 0.5);
          const peak = Math.min(1, Math.max(0.08, envelope)) * maxAmp;
          const v = Math.max(-peak, Math.min(peak, (rand() * 2 - 1) * peak));
          ctx.fillStyle = v >= 0 ? primary : secondary;
          const bh = Math.max(2, Math.abs(v));
          ctx.fillRect(x, mid - (v >= 0 ? bh : 0), barW, bh);
        }
      },

      // clip 样式:绝对定位,宽/左 = 秒 × pps
      clipStyle(clip) {
        const left = clip.start * this.pps;
        const w = Math.max(12, clip.duration * this.pps);
        return `left:${left}px;width:${w}px`;
      },

      // 选中 clip 的视觉类(对齐 vidonex ClipItem 选中态)
      clipClass(clip) {
        const base = 'absolute top-1.5 bottom-1.5 rounded-lg overflow-hidden select-none cursor-grab active:cursor-grabbing transition-[border-color,box-shadow] duration-150 z-10 ';
        if (this.selectedId === clip.id) {
          return base + 'border-2 border-indigo-400 shadow-lg shadow-indigo-500/30 z-20 bg-indigo-500/15';
        }
        return base + 'border border-gray-700/80 hover:border-gray-500 ' + this.kindBg(clip.kind);
      },
      kindBg(kind) {
        if (kind === 'audio' || kind === 'waveform') return 'bg-gray-800/70';
        return 'bg-gray-800/90';
      },

      select(clip) {
        this.selectedId = this.selectedId === clip.id ? null : clip.id;
      },

      // 标尺/轨道点击 → 定位播放头(对齐 vidonex Ruler click)
      seek(e) {
        const rect = e.currentTarget.getBoundingClientRect();
        const x = e.clientX - rect.left;
        this.setPlayhead(Math.max(0, x / this.pps), { source: 'timeline' });
      },
      dispatchPlayhead() {
        window.dispatchEvent(new CustomEvent('playhead-changed', { detail: { t: this.playhead, pid: this.pid, source: 'timeline' } }));
      },

      // ============ 选中 clip → 检查器 ============
      select(clip) {
        this.selectedId = this.selectedId === clip.id ? null : clip.id;
        // 广播选中的 clip 数据(检查器面板监听渲染属性;未选中 → null 重置)
        // 附带 timelineEditor 引用(editor.editClip 供检查器 autosave 落库)
        const track = this.tracks().find((tr) => tr.clips.some((c) => c.id === clip.id));
        window.dispatchEvent(new CustomEvent('clip-selected', {
          detail: {
            clip: this.selectedId ? clip : null,
            trackKind: this.selectedId && track ? track.kind : null,
            tl: this.selectedId ? this.tl : null,
            editor: this,
          },
        }));
      },

      // ============ 拖拽移动 clip ============
      startDrag(e, clip, trackIdx) {
        if (e.button !== 0) return;
        e.preventDefault();
        e.stopPropagation();
        // 注意:这里不设 selectedId——选中归 click(toggle),否则 mousedown+click
        // 连续触发会选中又取消(第一次点击的 clip 永远选不中)。
        this.drag = {
          clip, trackIdx,
          startX: e.clientX,
          origStart: clip.start,
          moved: false,
        };
        // addEventListener 回调里 this 不绑定组件,用箭头函数包一层
        this._dragMove = (ev) => this.onDragMove(ev);
        this._dragEnd = () => this.onDragEnd();
        window.addEventListener('pointermove', this._dragMove);
        window.addEventListener('pointerup', this._dragEnd, { once: true });
        // 合成 pointer 事件(Playwright/测试)可能没有活动 pointer,忽略即可
        try { e.target.setPointerCapture && e.target.setPointerCapture(e.pointerId); } catch (_) {}
      },

      onDragMove(e) {
        if (!this.drag) return;
        const dx = (e.clientX - this.drag.startX) / this.pps;
        let next = this.drag.origStart + dx;
        this.drag.moved = this.drag.moved || Math.abs(dx) > 0.01;
        // Snap:对齐其他 clip 的起止边(阈值 8px ≈ 8/pps 秒)
        if (this.snapEnabled()) {
          next = this.snapToEdges(next, this.drag.clip.duration);
        }
        this.drag.clip.start = Math.max(0, next);
        this.updateTrackWidth();
      },

      onDragEnd() {
        window.removeEventListener('pointermove', this._dragMove);
        window.removeEventListener('pointerup', this._dragEnd);
        if (this.drag && this.drag.moved) {
          // 拖拽后把播放头移到 clip 新起点(视觉跟随)
          this.setPlayhead(this.drag.clip.start, { source: 'timeline' });
          // 跨轨拖拽:drop 已在轨道上处理(drag 仍挂着);
          // 同一轨内拖拽:在这里收尾持久化。
          if (this.drag.dropped) {
            this.drag = null;
          } else {
            this.onClipMoved(this.drag.clip); // 子类/详情页可监听持久化
            this.drag = null;
          }
        } else {
          this.drag = null;
        }
      },
      // 跨轨 drop 标记:轨道 drop 处理器调用,告诉 onDragEnd 别重复持久化
      markDraggedClipDropped() {
        if (this.drag) this.drag.dropped = true;
      },

      snapEnabled() {
        const snap = document.querySelector('[data-snap]');
        return !snap || snap.dataset.snap === 'on';
      },

      // 吸附:靠最近的 clip 起止边,阈值 8px
      snapToEdges(next, dur) {
        const edges = [];
        for (const tr of this.tracks()) {
          for (const c of tr.clips) {
            if (c === this.drag.clip) continue;
            edges.push(c.start, c.start + c.duration);
          }
        }
        const th = 8 / this.pps; // 秒
        let best = next, bestD = th;
        for (const ed of edges) {
          const d = Math.abs(ed - next);
          if (d < bestD) { bestD = d; best = ed; }
        }
        return best;
      },

      // 拖拽后持久化钩子:默认 no-op,详情页覆盖为 PUT 时间轴
      onClipMoved() {},

      // ============ 键盘快捷键(对齐 vidonex:S分割 / Del删除 / N吸附)============
      onKey(e) {
        const tag = (e.target.tagName || '').toLowerCase();
        if (['input', 'textarea', 'select'].includes(tag)) return;
        // Ctrl+Z 撤销 / Ctrl+Shift+Z 重做(对齐 vidonex undoHistory 快捷键)
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z') {
          e.preventDefault();
          if (e.shiftKey) this.redo(); else this.undo();
          return;
        }
        // Ctrl+D 复制选中 clip(对齐 vidonex duplicateClip 快捷键)
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'd') {
          e.preventDefault();
          this.duplicateClip();
          return;
        }
        // Ctrl+K 命令面板 / Ctrl+/ 快捷键表(全局,见 appModals)
        if ((e.ctrlKey || e.metaKey) && (e.key.toLowerCase() === 'k' || e.key === '/')) {
          e.preventDefault();
          return; // appModals 顶层监听处理
        }
        // 空格播放/暂停(对齐 vidonex:play/pause)
        if (e.code === 'Space') {
          e.preventDefault();
          const pb = window.vp && window.vp.playback;
          if (pb) pb.togglePlay();
          return;
        }
        // 方向键逐帧 / Home/End 跳首尾
        if (e.key === 'ArrowLeft') { window.vp.playback.stepFrame(-1, this.tl ? this.tl.fps : 30); return; }
        if (e.key === 'ArrowRight') { window.vp.playback.stepFrame(1, this.tl ? this.tl.fps : 30); return; }
        if (e.key === 'Home') { window.vp.playback.jumpToStart(); return; }
        if (e.key === 'End') { window.vp.playback.jumpToEnd(); return; }
        if (e.key === 's' || e.key === 'S') { this.split(); }
        else if (e.key === 'Delete' || e.key === 'Backspace') { this.deleteSelected(); }
        else if (e.key === 'n' || e.key === 'N') { this.toggleSnap(); }
      },

      split() {
        if (!this.selectedId || !this.tl) return;
        const clip = this.findClip(this.selectedId);
        if (!clip) return;
        const t = this.playhead;
        if (t <= clip.start || t >= clip.start + clip.duration) return;
        // 前端分割:原地改内存(持久化写回是下一步——Web 编辑写 spec)
        const right = {
          ...clip,
          id: clip.id + '_r',
          start: t,
          duration: clip.start + clip.duration - t,
        };
        clip.duration = t - clip.start;
        this.pushClip(clip, right);
        this.selectedId = right.id;
        this.busy = true;
        this.persistTimeline(() => { this.busy = false; });
      },

      deleteSelected() {
        if (!this.selectedId || !this.tl) return;
        for (const tr of this.tracks()) {
          tr.clips = tr.clips.filter((c) => c.id !== this.selectedId);
        }
        this.selectedId = null;
        this.busy = true;
        this.persistTimeline(() => { this.busy = false; });
      },

      pushClip(orig, extra) {
        // 在 orig 所在轨道的原位插入(分割后右片紧随)
        for (const tr of this.tracks()) {
          const i = tr.clips.findIndex((c) => c.id === orig.id);
          if (i >= 0) { tr.clips.splice(i + 1, 0, extra); return; }
        }
      },

      findClip(id) {
        for (const tr of this.tracks()) {
          const c = tr.clips.find((c) => c.id === id);
          if (c) return c;
        }
        return null;
      },

      // 时间轴写回 spec:把当前内存 clips → 重建 spec YAML → PUT 项目。
      // Web 编辑(拖拽/分割/删除)落库,渲染时才见真实效果——对齐 vidonex
      // timelineStore → projectStore 的持久化链路。默认 no-op(列表页只读);
      // 详情页由 onClipMoved 触发 persistTimeline。
      // 每次写回前自动入历史栈(undo/redo 数据源)。
      persistTimeline(cb) {
        if (!this.tl || !this.pid) { cb && cb(); return; }
        this.pushHistory();
        const spec = this.buildSpecYAML();
        fetch(`/api/projects/${this.pid}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ spec_yaml: spec }),
        })
          .then((r) => (r.ok ? cb && cb() : cb && cb(new Error('保存失败 ' + r.status))))
          .catch((e) => { cb && cb(e); });
      },

      // ============ 撤销/重做(对齐 vidonex undo/redo:Ctrl+Z / Ctrl+Shift+Z)============
      // 快照 = 整个 tl 的深拷贝(JSON round-trip,含 tracks/画布)。
      // persistTimeline 已自动 pushHistory;编辑直接改 tl 后调 persistTimeline 即入栈。
      canUndo() { return this._historyIdx > 0; },
      canRedo() { return this._historyIdx < this._history.length - 1; },
      snapshotTL() {
        return this.tl ? JSON.parse(JSON.stringify(this.tl)) : null;
      },
      pushHistory() {
        const snap = this.snapshotTL();
        if (!snap) return;
        // 相同状态不重复入栈(拖拽结束的瞬态/autosave 重复触发)
        const prev = this._history[this._historyIdx];
        if (prev && JSON.stringify(prev) === JSON.stringify(snap)) return;
        // 丢弃 redo 分支(新编辑后 redo 栈失效)
        this._history = this._history.slice(0, this._historyIdx + 1);
        this._history.push(snap);
        if (this._history.length > 50) this._history.shift();
        this._historyIdx = this._history.length - 1;
      },
      // 恢复快照:替换 tl → 触发 trackWidth/播放头刷新 → 写回 spec
      restoreSnapshot(idx) {
        if (idx < 0 || idx >= this._history.length) return;
        const snap = this._history[idx];
        if (!snap) return;
        // 直接替换引用,Alpine 响应式自动重渲染
        this.tl = JSON.parse(JSON.stringify(snap));
        this._historyIdx = idx;
        this.selectedId = null;
        this.updateTrackWidth();
        this.setPlayhead(0, { source: 'timeline' });
        // 编辑恢复也要落库(内存与 DB 保持一致)
        this.busy = true;
        const spec = this.buildSpecYAML();
        fetch(`/api/projects/${this.pid}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ spec_yaml: spec }),
        })
          .then((r) => r.ok && window.dispatchEvent(new CustomEvent('history-changed')))
          .catch(() => {})
          .finally(() => { this.busy = false; });
      },
      undo() {
        if (!this.canUndo()) return;
        this.restoreSnapshot(this._historyIdx - 1);
      },
      redo() {
        if (!this.canRedo()) return;
        this.restoreSnapshot(this._historyIdx + 1);
      },

      // 内存 timeline → spec YAML(全字段:编辑保存不丢数据)。
      // 对齐 vidonex toVideoSpec:所有编辑重新序列化,渲染用最新 spec。
      // yamlSafe:Windows 反斜杠路径在 YAML 双引号里是转义符(如 \U),
      // 统一转正斜杠(ffmpeg 双平台都接受),避免 spec 解析失败。
      yamlSafe(s) {
        return s ? String(s).replace(/\\/g, '/') : s;
      },
      buildSpecYAML() {
        const tl = this.tl;
        const lines = [];
        lines.push('version: "1.0"');
        lines.push(`canvas: {width: ${tl.width}, height: ${tl.height}, background_color: "${tl.background_color || 'black'}"}`);
        lines.push(`fps: "${tl.fps}"`);
        if (tl.tracks.length === 0) { lines.push('tracks: []'); } else {
          lines.push('tracks:');
          for (const tr of this.tracks()) {
            lines.push(`  - id: "${tr.id}"`);
            lines.push(`    kind: "${tr.kind}"`);
            if (tr.clips && tr.clips.length) {
              lines.push('    clips:');
              for (const c of tr.clips) {
                lines.push(`      - id: "${c.id}"`);
                lines.push(`        source: "${this.yamlSafe(c.source)}"`);
                if (c.start !== undefined && c.start !== 0) lines.push(`        start: "${c.start}s"`);
                lines.push(`        duration: "${c.duration}s"`);
                if (c.trim > 0) lines.push(`        trim: "${c.trim}s"`);
                if (c.speed && c.speed !== 1) lines.push(`        speed: ${c.speed}`);
                // 变换(对齐 vidonex inspector:scale/rotation/opacity/position/blend)
                if (c.scale && c.scale !== 1) lines.push(`        scale: ${c.scale}`);
                if (c.rotation && c.rotation !== 0) lines.push(`        rotation: ${c.rotation}`);
                if (c.opacity !== undefined && c.opacity !== 1) lines.push(`        opacity: ${c.opacity}`);
                if (c.volume !== undefined && c.volume !== 1) lines.push(`        volume: ${c.volume}`);
                if (c.blend_mode && c.blend_mode !== 'normal') lines.push(`        blend_mode: "${c.blend_mode}"`);
                if (c.position && (c.position.x || c.position.y || (c.position.alignment && c.position.alignment !== 'center'))) {
                  lines.push('        position:');
                  if (c.position.x) lines.push(`          x: ${c.position.x}`);
                  if (c.position.y) lines.push(`          y: ${c.position.y}`);
                  if (c.position.alignment && c.position.alignment !== 'center') lines.push(`          alignment: "${c.position.alignment}"`);
                }
                if (c.has_audio === false) lines.push(`        has_audio: false`);
              }
            }
            if (tr.transitions && tr.transitions.length) {
              lines.push('    transitions:');
              for (const tn of tr.transitions) {
                lines.push(`      - type: "${tn.type}"`);
                lines.push(`        duration: "${tn.duration}s"`);
                if (tn.from) lines.push(`        from: "${tn.from}"`);
                if (tn.to) lines.push(`        to: "${tn.to}"`);
              }
            }
          }
        }
        return lines.join('\n');
      },

      // editClip 检查器编辑入口:更新内存 clip → 时间轴自动反映 → autosave。
      // 检查器拿到的 clip 就是 tl 里的引用,直接改字段后调这里落库。
      _autoSaveTimer: null,
      editClip(cb) {
        if (!this.tl || !this.pid) { cb && cb(); return; }
        clearTimeout(this._autoSaveTimer);
        this._autoSaveTimer = setTimeout(() => {
          this.persistTimeline(cb);
        }, 300); // 对齐 vidonex triggerAutoSave 300ms debounce
      },

      // 替换/追加整个时间轴数据(检查器可能整体替换 tracks)
      setTracks(tracks, cb) {
        if (!this.tl) return;
        this.tl.tracks = tracks;
        this.updateTrackWidth();
        this.editClip(cb);
      },

      // 项目设置(画布尺寸/背景色/fps)→ 更新 tl → autosave(对齐 vidonex ProjectSettings)
      saveSettings(settings) {
        if (!this.tl) return;
        if (settings.width) this.tl.width = settings.width;
        if (settings.height) this.tl.height = settings.height;
        if (settings.background_color !== undefined) this.tl.background_color = settings.background_color;
        if (settings.fps) { this.tl.fps = settings.fps; window.vp.syncTimelineDurations(this.tl); }
        this.updateTrackWidth();
        this.editClip();
      },

      toggleSnap() {
        const snap = document.querySelector('[data-snap]');
        if (!snap) return;
        snap.dataset.snap = snap.dataset.snap === 'on' ? 'off' : 'on';
        snap.textContent = snap.dataset.snap === 'on' ? 'Snap ON' : 'Snap OFF';
        snap.classList.toggle('snap-on', snap.dataset.snap === 'on');
      },

      fmt: fmtTime,
    };
  };

  // ============ 媒体库(mediaLibrary)============
  // 挂在 media_panel。打开项目 → 加载素材列表;上传(multipart → assets API);
  // 搜索过滤;拖拽素材到时间轴(HTML5 DnD,drop 由 timelineEditor 处理)。
  // 对齐 vidonex media store:fetchAssets/uploadFile/deleteAsset + draggedAsset。
  window.mediaLibrary = function (pid) {
    return {
      pid: pid || null,
      assets: [],
      query: '',
      uploading: false,
      error: '',

      async init() {
        if (!this.pid) return;
        await this.load();
      },
      get filtered() {
        if (!this.query.trim()) return this.assets;
        const q = this.query.toLowerCase();
        return this.assets.filter((a) => (a.file_name || '').toLowerCase().includes(q));
      },
      async load() {
        if (!this.pid) return;
        try {
          const r = await fetch(`/api/projects/${this.pid}/assets`);
          if (!r.ok) throw new Error(r.status);
          this.assets = await r.json();
        } catch (e) { this.error = e.message; }
      },
      // 上传(多文件)
      async uploadFiles(e) {
        const files = Array.from(e.target.files || []);
        e.target.value = '';
        if (!files.length || !this.pid) return;
        this.uploading = true;
        try {
          for (const f of files) {
            const fd = new FormData();
            fd.append('file', f);
            const r = await fetch(`/api/projects/${this.pid}/assets`, { method: 'POST', body: fd });
            if (r.ok) {
              const a = await r.json();
              this.assets.unshift(a);
            }
          }
        } finally { this.uploading = false; }
      },
      async removeAsset(a) {
        const ok = await window.vp.confirm({
          title: '删除素材',
          message: `确定删除 "${a.file_name}"？(磁盘文件一并移除)`,
          confirmText: '删除',
          cancelText: '取消',
          isDanger: true,
        });
        if (!ok) return;
        const r = await fetch(`/api/assets/${a.id}`, { method: 'DELETE' });
        if (r.ok) this.assets = this.assets.filter((x) => x.id !== a.id);
      },
      // 拖拽开始:素材信息挂在 dataTransfer(时间轴轨道 drop 读取)
      onDragStart(ev, a) {
        ev.dataTransfer.setData('application/x-vidproc-asset', JSON.stringify(a));
        ev.dataTransfer.setData('text/plain', a.file_name);
        ev.dataTransfer.effectAllowed = 'copy';
        if (ev.dataTransfer.setDragImage && ev.target) { try { ev.dataTransfer.setDragImage(ev.target.closest('[draggable]') || ev.target, 20, 20); } catch (_) {} }
      },
    };
  };

  // ============ 检查器(clipInspector)============
  // 挂在 inspector_panel.监听时间轴 clip-selected 事件,持有选中 clip 引用
  // (就是 tl 里的对象),编辑字段 → editor.editClip() 300ms debounce autosave。
  // 对齐 vidonex InspectorPanel:时序/变换/音频分组 + commitChange 历史。
  window.clipInspector = function () {
    return {
      clip: null,      // 选中 clip 引用(直接改字段,时间轴同步反映)
      trackKind: null, // video/overlay/audio/waveform/subtitle
      _editor: null,   // timelineEditor 引用(提供 editClip/persistTimeline)

      init() {
        window.addEventListener('clip-selected', (e) => {
          const d = e.detail || {};
          this.clip = d.clip || null;
          this.trackKind = d.trackKind || null;
          this._editor = d.editor || null;
          // 规范化可选字段(x-model.number 需要数值存在)
          if (this.clip) {
            this.clip.speed = this.clip.speed || 1;
            this.clip.trim = this.clip.trim || 0;
            this.clip.scale = this.clip.scale || 1;
            this.clip.rotation = this.clip.rotation || 0;
            this.clip.opacity = this.clip.opacity !== undefined ? this.clip.opacity : 1;
            this.clip.volume = this.clip.volume !== undefined ? this.clip.volume : 1;
            this.clip.blend_mode = this.clip.blend_mode || 'normal';
            if (!this.clip.position) this.clip.position = { x: 0, y: 0, alignment: 'center' };
            if (!this.clip.position.alignment) this.clip.position.alignment = 'center';
          }
        });
      },

      // 位置 X/Y 代理(clip.position 可能未初始化)
      get posX() { return this.clip && this.clip.position ? (this.clip.position.x || 0) : 0; },
      set posX(v) {
        if (this.clip) {
          if (!this.clip.position) this.clip.position = {};
          this.clip.position.x = v;
        }
      },
      get posY() { return this.clip && this.clip.position ? (this.clip.position.y || 0) : 0; },
      set posY(v) {
        if (this.clip) {
          if (!this.clip.position) this.clip.position = {};
          this.clip.position.y = v;
        }
      },

      // 编辑提交:不重载页面,直接 autosave(300ms debounce)。
      // 时间轴引用与检查器同一对象 → Alpine 响应式自动同步 UI。
      commit(label) {
        const self = this;
        if (this._editor) {
          this._editor.editClip(() => {
            // 保存后保留选中(不丢检查器状态)
            self._editor.selectedId = self.clip ? self.clip.id : null;
          });
        }
      },
    };
  };

  // ============ 全站模态(appModals)============
  // 对齐 vidonex KeyboardShortcutsModal + CommandPalette + AboutModal:
  // - Ctrl+K 命令面板(搜命令执行)
  // - Ctrl+/ 快捷键表(? 或 Ctrl+/ 打开)
  // - About(vp 全局图标 / 命令面板内"关于")
  // 单例 + 事件总线(与其它弹窗一致);挂在 shell 顶层,任何页面可用。
  // 全局 doc keydown 在此处理(Ctrl+K / Ctrl+/ / Esc),避免每组件重复监听。
  const APP_COMMANDS = [
    { id: 'new-project', name: '新建项目', desc: '创建新的视频项目', keys: '', run: () => window.dispatchEvent(new CustomEvent('new-project-open')) },
    { id: 'open-project', name: '返回项目列表', desc: '切换到项目列表', keys: 'Esc', run: () => { location.href = '/'; } },
    { id: 'settings', name: '项目设置', desc: '画布尺寸 / 背景色 / 帧率', keys: 'P', run: () => window.dispatchEvent(new CustomEvent('project-settings-open', { detail: { tl: (window.vp.timelineEditorOf || {}).tl } })) },
    { id: 'render', name: '渲染导出', desc: '编码格式 / GPU / 质量', keys: '', run: () => { const m = window.renderModal(); m.openFor(window.vp.currentProjectId, (window.vp.projectViewOf || {}).output || ''); } },
    { id: 'split', name: '分割选中 clip', desc: '在播放头处切开', keys: 'S', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.split(); } },
    { id: 'duplicate', name: '复制选中 clip', desc: '复制到当前轨道末尾', keys: 'Ctrl+D', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.duplicateClip(); } },
    { id: 'delete', name: '删除选中 clip', desc: '从时间轴移除', keys: 'Del', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.deleteSelected(); } },
    { id: 'undo', name: '撤销', desc: '回退上一步编辑', keys: 'Ctrl+Z', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.undo(); } },
    { id: 'redo', name: '重做', desc: '恢复撤销的编辑', keys: 'Ctrl+Shift+Z', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.redo(); } },
    { id: 'toggle-play', name: '播放 / 暂停', desc: '切换传输状态', keys: 'Space', run: () => { const pb = window.vp && window.vp.playback; if (pb) pb.togglePlay(); } },
    { id: 'toggle-snap', name: '磁吸开关', desc: 'Snap 吸附对齐', keys: 'N', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.toggleSnap(); } },
    { id: 'shortcuts', name: '键盘快捷键', desc: '查看全部快捷键', keys: 'Ctrl+/', run: () => {} },
    { id: 'about', name: '关于 vidproc', desc: '版本与架构信息', keys: '', run: () => {} },
  ];
  const APP_SHORTCUTS = [
    { keys: 'Space', desc: '播放 / 暂停' },
    { keys: '← / →', desc: '上一帧 / 下一帧' },
    { keys: 'Home / End', desc: '跳到开头 / 结尾' },
    { keys: 'S', desc: '在播放头分割选中 clip' },
    { keys: 'Delete / Backspace', desc: '删除选中 clip' },
    { keys: 'Ctrl+D', desc: '复制选中 clip' },
    { keys: 'Ctrl+Z / Ctrl+Shift+Z', desc: '撤销 / 重做' },
    { keys: 'N', desc: '磁吸开关 (Snap)' },
    { keys: 'P', desc: '项目设置 (画布/背景/fps)' },
    { keys: 'Ctrl+K', desc: '命令面板' },
    { keys: 'Ctrl+/', desc: '快捷键列表' },
    { keys: 'Esc', desc: '关闭弹窗 / 返回' },
  ];
  let _appModals = null;
  window.appModals = function () {
    if (_appModals) return _appModals;
    _appModals = {
      view: null, // null | 'palette' | 'shortcuts' | 'about'
      query: '',
      commands: APP_COMMANDS,
      shortcuts: APP_SHORTCUTS,
      version: '0.1.0',
      architecture: 'htmx + Alpine + Go / vidonex 兼容 spec',
      init() {
        // 事件总线入口(与其它弹窗一致:Alpine 响应式代理要求从组件内改字段;
        // 外部 vp.openShortcuts/openPalette 广播事件,这里收)
        window.addEventListener('app-modals-open', (e) => {
          this.view = (e.detail && e.detail.view) || 'palette';
        });
        document.addEventListener('keydown', (e) => {
          // 输入框内不劫持
          const tag = (e.target.tagName || '').toLowerCase();
          const inField = ['input', 'textarea', 'select'].includes(tag);
          const mod = e.ctrlKey || e.metaKey;
          if (mod && e.key.toLowerCase() === 'k' && !inField) {
            e.preventDefault();
            this.view = this.view === 'palette' ? null : 'palette';
            this.query = '';
          } else if (mod && e.key === '/' && !inField) {
            e.preventDefault();
            this.view = this.view === 'shortcuts' ? null : 'shortcuts';
          } else if (e.key === 'Escape') {
            if (this.view) this.view = null;
          }
        });
      },
      open(view) { this.view = view; },
      get filteredCommands() {
        if (!this.query.trim()) return this.commands;
        const q = this.query.toLowerCase();
        return this.commands.filter((c) =>
          c.name.toLowerCase().includes(q) || c.desc.toLowerCase().includes(q) || c.keys.toLowerCase().includes(q));
      },
      // 命令执行:自身类(shortcuts/about)切换视图,其它执行后关闭
      run(cmd) {
        if (cmd.id === 'shortcuts') { this.view = 'shortcuts'; return; }
        if (cmd.id === 'about') { this.view = 'about'; return; }
        try { cmd.run && cmd.run(); } catch (_) {}
        this.view = null;
      },
    };
    return _appModals;
  };
  // 供 shell 顶栏按钮打开快捷键表/命令面板(经事件总线保证 Alpine 响应式)
  window.vp.openShortcuts = function () { window.dispatchEvent(new CustomEvent('app-modals-open', { detail: { view: 'shortcuts' } })); };
  window.vp.openPalette = function () { window.dispatchEvent(new CustomEvent('app-modals-open', { detail: { view: 'palette' } })); };

  // ============ 音频电平表(audioMeter)============
  // 对齐 vidonex AudioPeakMeter:播放时模拟左右声道实时电平(无真实音频解码,
  // 用噪声包络驱动柱形;暂停归零)。挂在播放工具条右侧。
  window.audioMeter = function () {
    return {
      l: 0, r: 0, // 0..1 电平,响应式驱动柱高
      playing: false,
      _raf: null,
      _t0: 0,
      init() {
        this._loop = (t) => {
          // 直接读播放状态(vp.playback 未广播播放事件,轮询最稳)
          const pb = window.vp && window.vp.playback;
          const playing = !!(pb && pb.isPlaying);
          this.playing = playing;
          if (playing) {
            // 播放期间:随机包络(峰值 .15-.95,快速起伏像音乐)
            const e = 0.5 + 0.45 * Math.sin(t * 0.011 + 1.7) * Math.sin(t * 0.0043);
            this.l = Math.max(0.04, Math.min(1, e + (Math.random() - 0.5) * 0.35));
            this.r = Math.max(0.04, Math.min(1, e * 0.92 + (Math.random() - 0.5) * 0.3));
          } else {
            this.l *= 0.86; this.r *= 0.86; // 衰减归零
          }
          this._raf = requestAnimationFrame(this._loop);
        };
        this._raf = requestAnimationFrame(this._loop);
      },
      destroy() { cancelAnimationFrame(this._raf); },
    };
  };
  // ============ 项目设置弹窗(projectSettingsModal)============
  // 对齐 vidonex ProjectSettings:画布尺寸/背景色/fps → timelineEditor.saveSettings。
  // 单例 + 事件总线(与 renderModal 一致,Alpine 响应式代理见 init 注释)。
  const BG_COLORS = [
    { id: 'black', name: '黑' },
    { id: 'white', name: '白' },
    { id: 'green', name: '绿幕' },
    { id: 'blue', name: '蓝幕' },
    { id: 'transparent', name: '透明' },
  ];
  let _projectSettingsModal = null;
  window.projectSettingsModal = function () {
    if (_projectSettingsModal) return _projectSettingsModal;
    _projectSettingsModal = {
      open: false,
      width: 1920,
      height: 1080,
      fps: 30,
      background_color: 'black',
      sizePresets: ASPECT_PRESETS,
      bgColors: BG_COLORS,
      fpsOptions: FPS_OPTIONS,
      init() {
        window.addEventListener('project-settings-open', (e) => {
          const d = e.detail || {};
          const tl = d.tl;
          if (tl) {
            this.width = tl.width || 1920;
            this.height = tl.height || 1080;
            this.fps = tl.fps || 30;
            this.background_color = tl.background_color || 'black';
          }
          this.open = true;
        });
        window.addEventListener('keydown', (e) => { if (e.key === 'Escape' && this.open) this.open = false; });
      },
      applySize(p) {
        this.width = p.width;
        this.height = p.height;
      },
      save() {
        const host = window.vp && window.vp.timelineEditorOf;
        if (!host) { this.open = false; return; }
        host.saveSettings({
          width: Math.max(16, Math.min(7680, this.width | 0)),
          height: Math.max(16, Math.min(4320, this.height | 0)),
          fps: this.fps,
          background_color: this.background_color,
        });
        this.open = false;
      },
    };
    return _projectSettingsModal;
  };

// ============ 渲染导出弹窗(renderModal)============
  // 对齐 vidonex RenderExportModal:格式/输出路径/GPU 加速/平台预设/
  // 质量档(CRF 14-35)/恒定码率 → startRender(params) 起 job。
  // 单例:检查器侧按钮(无 x-data 作用域)直接控制 open。
  const RENDER_FORMATS = [
    { id: 'mp4', name: 'MP4 (H.264/AAC)', ext: 'mp4' },
    { id: 'mov', name: 'MOV (ProRes 兼容)', ext: 'mov' },
    { id: 'webm', name: 'WebM (VP9/Opus)', ext: 'webm' },
  ];
  const RENDER_GPUS = [
    { id: 'auto', name: 'Auto (按 spec)' },
    { id: 'none', name: 'CPU (libx264)' },
    { id: 'nvenc', name: 'NVIDIA NVENC' },
  ];
  const RENDER_PRESETS = [
    { id: '', name: '项目默认' },
    { id: 'tiktok', name: 'TikTok 9:16' },
    { id: 'instagram_reels', name: 'Instagram Reels' },
    { id: 'instagram_square', name: 'Instagram Square' },
    { id: 'youtube_1080', name: 'YouTube 1080p' },
    { id: 'youtube_4k', name: 'YouTube 4K' },
    { id: 'web_fast', name: 'Web Fast 720p' },
  ];
  let _renderModal = null;
  window.renderModal = function () {
    if (_renderModal) return _renderModal;
    _renderModal = {
      open: false,
      pid: null,
      format: 'mp4',
      output: '',
      gpu: 'none',
      preset: '',
      crf: 23,
      bitrate: 0, // kbps;0 = CRF 模式
      useBitrate: false,
      formats: RENDER_FORMATS,
      gpus: RENDER_GPUS,
      presets: RENDER_PRESETS,
      init() {
        // Alpine 对 x-data 返回值做响应式代理,外部直接改单例字段不触发 x-show;
        // 统一走事件总线:openFor() 广播 render-modal-open,这里收。
        window.addEventListener('render-modal-open', (e) => {
          const d = e.detail || {};
          this.pid = d.pid || this.pid;
          if (d.output !== undefined) this.output = d.output;
          this.open = true;
        });
        window.addEventListener('keydown', (e) => { if (e.key === 'Escape' && this.open) this.open = false; });
      },
      // 打开弹窗(检查器侧按钮调用;经事件总线保证 Alpine 响应式)
      openFor(pid, defaultOutput) {
        window.dispatchEvent(new CustomEvent('render-modal-open', {
          detail: { pid: pid || this.pid, output: defaultOutput },
        }));
      },
      get ext() {
        return this.format;
      },
      // CRF 滑块随使用模式联动
      toggleMode(useBit) {
        this.useBitrate = useBit;
        if (useBit && this.bitrate <= 0) this.bitrate = 8000;
      },
      collect() {
        const p = {
          format: this.format,
          gpu: this.gpu === 'auto' ? '' : this.gpu,
          preset: this.preset,
          crf: this.useBitrate ? 0 : (this.crf || 0),
          bitrate: this.useBitrate ? (this.bitrate || 0) : 0,
        };
        if (this.output.trim()) p.output = this.output.trim();
        return p;
      },
      close() { this.open = false; },
      // 开始渲染:取全局 projectView 实例 → startRender(collect())。完成后关闭弹窗,
      // 进度在检查器侧进度条展示。
      start() {
        const host = window.vp && window.vp.projectViewOf;
        if (!host || host.busy) return;
        host.startRender(this.collect());
        this.open = false;
      },
    };
    return _renderModal;
  };

  // ============ 新建项目弹窗(newProjectModal)============
  // 对齐 vidonex NewProjectModal:名称 + 5 画布预设(16:9/4K/9:16/1:1/21:9)
  // + fps 选择。创建 = 前端生成 spec(带画布) → POST /api/projects。
  const ASPECT_PRESETS = [
    { id: 'youtube_16_9', name: '16:9 Landscape (1080p)', width: 1920, height: 1080, icon: 'M21 7H3a1 1 0 0 0-1 1v8a1 1 0 0 0 1 1h18a1 1 0 0 0 1-1V8a1 1 0 0 0-1-1zM7 10h2v4H7z' },
    { id: 'youtube_4k', name: '16:9 4K UHD (3840x2160)', width: 3840, height: 2160, icon: 'M12 2l8 3v6c0 5-3.4 9.4-8 11-4.6-1.6-8-6-8-11V5z' },
    { id: 'tiktok_9_16', name: '9:16 Vertical (Reels/TikTok)', width: 1080, height: 1920, icon: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm4.5 11.6l-5 3a1 1 0 0 1-1.5-.9V8.3a1 1 0 0 1 1.5-.9l5 3a1 1 0 0 1 0 1.7z' },
    { id: 'instagram_square', name: '1:1 Square (Instagram)', width: 1080, height: 1080, icon: 'M3 5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zm14 7a5 5 0 1 1-10 0 5 5 0 0 1 10 0zM17 6h2v2h-2z' },
    { id: 'cinema_wide', name: '21:9 Ultra-Wide Cinema', width: 2560, height: 1080, icon: 'M4 6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2zm3 4h10v4H7z' },
  ];
  const FPS_OPTIONS = [
    { label: '24 FPS (Film)', value: 24 },
    { label: '25 FPS (PAL)', value: 25 },
    { label: '30 FPS (Web)', value: 30 },
    { label: '60 FPS (Gaming)', value: 60 },
  ];
  // 单例:弹窗触发按钮(无 x-data 作用域)也需控制 open
  let _newProjectModal = null;
  window.newProjectModal = function () {
    if (_newProjectModal) return _newProjectModal;
    _newProjectModal = {
      open: false,
      name: 'New Composition',
      presetId: 'youtube_16_9',
      width: 1920,
      height: 1080,
      fps: 30,
      creating: false,
      error: '',
      presets: ASPECT_PRESETS,
      fpsOptions: FPS_OPTIONS,
      init() {
        // Alpine 响应式代理问题:外部直接改单例字段不触发 x-show;
        // 触发按钮广播 new-project-open,这里收。
        window.addEventListener('new-project-open', () => { this.open = true; });
        // 列表页点"新建"(search:?new=1)自动打开
        if (location.hash === '#new') { this.open = true; }
        window.addEventListener('keydown', (e) => { if (e.key === 'Escape' && this.open) this.open = false; });
      },
      applyPreset(p) {
        this.presetId = p.id;
        this.width = p.width;
        this.height = p.height;
      },
      buildSpec() {
        return `version: "1.0"\ncanvas:\n  width: ${this.width}\n  height: ${this.height}\n  background_color: "black"\nfps: "${this.fps}"\noutput: ""\ntracks: []\n`;
      },
      async create() {
        if (!this.name.trim() || this.creating) return;
        this.creating = true;
        this.error = '';
        try {
          const r = await fetch('/api/projects', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: this.name.trim(), spec_yaml: this.buildSpec(), output_path: '' }),
          });
          if (!r.ok) throw new Error(await r.text());
          const p = await r.json();
          this.open = false;
          // 切到新项目(整页跳转避免 htmx 状态残留)
          location.href = `/project/${p.id}`;
        } catch (e) { this.error = '创建失败: ' + e.message; }
        finally { this.creating = false; }
      },
    };
    return _newProjectModal;
  };

  // ============ 渲染任务(renderJob):异步渲染 + 进度轮询 ============
  // POST /api/projects/{id}/render 起 job,setInterval 轮询 /api/render/{id},
  // 实时写进度条(percent)+ 状态栏文字。对齐 vidonex 渲染进度条/RenderModal。
  window.renderJob = function (pid, button) {
    const el = document.querySelector(button);
    const bar = document.querySelector('[data-progress-bar]');
    const pct = document.querySelector('[data-progress-pct]');
    if (!el || !pid) return;
    el.disabled = true;

    fetch(`/api/projects/${pid}/render`, { method: 'POST' })
      .then((r) => {
        if (!r.ok) throw new Error('启动失败 ' + r.status);
        return r.json();
      })
      .then((job) => {
        const id = job.id;
        const timer = setInterval(() => {
          fetch(`/api/render/${id}`)
            .then((r) => r.json())
            .then((j) => {
              if (j.status === 'failed') {
                clearInterval(timer);
                el.disabled = false;
                if (bar) { bar.style.width = '0%'; }
                if (pct) { pct.textContent = '渲染失败'; }
                window.dispatchEvent(new CustomEvent('render-finished', { detail: { ok: false, job: j } }));
                return;
              }
              const p = j.percent || 0;
              if (bar) { bar.style.width = p + '%'; }
              if (pct) { pct.textContent = p.toFixed(0) + '%'; }
              if (j.status === 'done') {
                clearInterval(timer);
                el.disabled = false;
                if (bar) { bar.style.width = '100%'; }
                window.dispatchEvent(new CustomEvent('render-finished', { detail: { ok: true, job: j } }));
              }
            })
            .catch(() => {}); // 轮询瞬时失败忽略,下一拍再试
        }, 700);
      })
      .catch((e) => {
        el.disabled = false;
        if (pct) { pct.textContent = '启动失败: ' + e.message; }
      });
  };

  // ============ 项目视口(projectView):画布预览帧 + 异步渲染进度 ============
  // 挂在 project_detail 视口区。监听时间轴 playhead-changed 事件 → 抽帧;
  // startRender() 起异步 job 并轮询进度。对齐 vidonex CanvasViewport + RenderModal。
  window.projectView = function (opts) {
    const pid = opts.pid;
    const output = opts.output || '';
    let previewTimer = null;
    let pollTimer = null;

    return {
      pid,
      output,
      playhead: 0,
      previewSrc: '',
      previewErr: false,
      loading: false,
      busy: false,
      percent: 0,
      frame: 0,
      speed: 0,
      done: false,
      failed: false,
      result: {},
      fps: 30,
      tc: '00:00:00:00',
      tcDur: '00:00:00:00',

      init() {
        // 供渲染导出弹窗等无作用域组件调用(window.vp.projectViewOf)
        window.vp.projectViewOf = this;
        // 响应式时间码(播放时 rAF 推进 → playhead-changed → 这里刷新)
        this.tc = '00:00:00:00';
        this.tcDur = '00:00:00:00';
        // 页面进入:先取一个默认预览帧(t=0)
        this.refreshPreview(0);
        // 播放头移动 → 刷新画布(防抖 120ms,连拖不连打)
        window.addEventListener('playhead-changed', (e) => {
          const ev = e.detail || {};
          if (ev.pid !== undefined && ev.pid !== pid) return;
          this.playhead = ev.t || 0;
          this.refreshPreview(this.playhead);
          this.tc = window.vp.playback.formatTimecode(this.playhead, this.fps);
        });
        // 同步项目帧率到播放状态机
        window.vp.currentProjectId = pid;
        fetch(`/api/projects/${pid}/timeline`)
          .then((r) => (r.ok ? r.json() : null))
          .then((tl) => {
            if (!tl) return;
            this.fps = tl.fps || 30;
            window.vp.syncTimelineDurations(tl);
            this.tcDur = window.vp.playback.formatTimecode(window.vp.playback.contentDuration || 0, this.fps);
          })
          .catch(() => {});
        // 加载后(含 htmx swap 重新初始化)确保画布有帧 —— 由 playhead-changed 驱动,无需额外
      },

      // ============ 播放工具条/视口缩放(对齐 vidonex PlaybackToolbar + useViewportZoom)============
      pb() { return window.vp.playback; },
      vz() { return window.vp.viewportZoom; },
      cycleZoom() {
        const presets = this.vz().ZOOM_PRESETS;
        const cur = this.vz().zoom;
        const next = presets.find((p) => p.value > cur + 0.001) || presets[0];
        this.vz().setZoom(next.value, true);
      },
      onWheel(e) {
        e.preventDefault();
        const factor = e.deltaY < 0 ? 1.15 : 0.87;
        this.vz().setZoom(this.vz().zoom * factor);
      },
      resetViewportZoom() { this.vz().reset(); },
      get stageStyle() {
        const z = this.vz().zoom;
        return `transform:translate(${this.vz().panX}px, ${this.vz().panY}px) scale(${z})`;
      },
      get screenStyle() {
        // 画布框:Fit = 宽 100%(容器内 4:3 实际按帧比例),缩放时同比例放大
        return `width:100%;height:100%`;
      },
      get isFit() { return this.vz().isFit; },
      get zoomDisplay() { return this.vz().display; },

      fmt(sec) {
        if (!isFinite(sec) || sec < 0) return '00:00:00.000';
        const h = Math.floor(sec / 3600);
        const m = Math.floor((sec % 3600) / 60);
        const s = sec % 60;
        const pad = (n, w) => String(n).padStart(w, '0');
        return `${pad(h, 2)}:${pad(m, 2)}:${s.toFixed(3).padStart(6, '0')}`;
      },

      get outputName() {
        const o = this.output || '';
        return o ? o.split(/[/\\]/).pop() : '—';
      },

      refreshPreview(t) {
        clearTimeout(previewTimer);
        previewTimer = setTimeout(() => {
          this.loading = true;
          fetch(`/api/projects/${pid}/frame?t=${t}&w=960`)
            .then((r) => {
              if (r.status === 404) {
                this.previewSrc = '';
                this.previewErr = true;
                return null;
              }
              if (!r.ok) throw new Error(r.status);
              this.previewErr = false;
              return r.blob();
            })
            .then((blob) => {
              if (!blob) return;
              if (this.previewSrc) URL.revokeObjectURL(this.previewSrc);
              this.previewSrc = URL.createObjectURL(blob);
            })
            .catch(() => { this.previewSrc = ''; this.previewErr = true; })
            .finally(() => { this.loading = false; });
        }, 120);
      },

      startRender(params) {
        if (this.busy) return;
        this.busy = true;
        this.percent = 0;
        this.frame = 0;
        this.speed = 0;
        this.done = false;
        this.failed = false;
        this.result = {};

        fetch(`/api/projects/${pid}/render`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(params || {}),
        })
          .then((r) => {
            if (!r.ok) throw new Error('启动失败 ' + r.status);
            return r.json();
          })
          .then((job) => this.poll(job.id))
          .catch((e) => {
            this.busy = false;
            this.failed = true;
            this.result = { error: e.message };
          });
      },

      poll(id) {
        clearInterval(pollTimer);
        pollTimer = setInterval(() => {
          fetch(`/api/render/${id}`)
            .then((r) => r.json())
            .then((j) => {
              this.percent = j.percent || 0;
              this.frame = j.frame || 0;
              this.speed = j.speed || 0;
              if (j.status === 'failed') {
                clearInterval(pollTimer);
                this.busy = false;
                this.failed = true;
                this.result = { error: j.error };
              } else if (j.status === 'done') {
                clearInterval(pollTimer);
                this.busy = false;
                this.done = true;
                this.percent = 100;
                this.result = j;
              }
            })
            .catch(() => {}); // 瞬时失败忽略,下一拍再试
        }, 700);
      },
    };
  };
})();