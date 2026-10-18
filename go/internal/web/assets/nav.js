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
      { label: '适配', value: 1.0, isFit: true },
      { label: '25%', value: 0.25 },
      { label: '50%', value: 0.5 },
      { label: '75%', value: 0.75 },
      { label: '100%', value: 1.0 },
      { label: '150%', value: 1.5 },
      { label: '200%', value: 2.0 },
    ],
    get isFit() { return this.zoom === 1.0 && this.panX === 0 && this.panY === 0; },
    get display() { return this.isFit ? '适配' : `${Math.round(this.zoom * 100)}%`; },
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
      selectedTrackId: null, // 选中轨道 id(轨道头点击;Ducking/Waveform 检查器用)
      pps: 80, // 像素/秒(zoom 驱动)
      drag: null, // 进行中的拖拽 {clip, trackIdx, startX, origStart}
      scrubbing: false, // 播放头拖拽中(scrub)
      // 转场设置弹窗(常驻对象,永不 null → 模板引用天然空安全)
      // {open, track, fromClip, toId, type, duration, existed}
      transitionModal: { open: false, track: null, fromClip: null, toId: null, type: 'dissolve', duration: 0.5, existed: false },
      // 右键上下文菜单(对齐 vidonex ClipItem UContextMenu 四段式;常驻对象避免 null 引用)
      ctxMenu: { open: false, x: 0, y: 0, clip: null, track: null },
      // 拖拽幻影(对齐 vidonex TrackRow ghost:兼容靛蓝/重叠红/不兼容红;常驻对象)
      dragGhost: { show: false, targetKind: '', left: 0, width: 0, duration: 0, snapped: false, snapType: '', overlap: false, compatible: true, title: '' },
      // source → thumbnail_url 映射(filmstrip 胶片条,对齐上游 ClipItem 缩略图背景)
      assetThumbs: {},
      trackWidth: 800, // 滚动区实测宽,做成 clip 区域 min-width
      contentSec: 0, // 内容时长(响应式;标尺 END 高亮宽度 = contentSec × pps)
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
          // 素材缩略图映射(source → thumbnail_url;filmstrip 与右键 Probe 用)
          try {
            const ar = await fetch(`/api/projects/${this.pid}/assets`);
            if (ar.ok) {
              const list = await ar.json();
              this.assetThumbs = {};
              for (const a of list || []) {
                // 按文件名匹配(媒体库里 source 可能带路径,文件名即可对上)
                const base = (a.file_name || '').toLowerCase();
                if (base) this.assetThumbs[base] = (a.thumbnail_url || `/api/assets/${a.id}/frame?t=0`) + '';
              }
            }
          } catch (_) { /* 缩略图可选,失败忽略 */ }
          // 播放状态机同步内容/工作区时长
          window.vp.currentProjectId = this.pid;
          window.vp.syncTimelineDurations(this.tl);
          this.contentSec = (window.vp.playback && window.vp.playback.contentDuration) || 0;
          // 播放头默认停在第一个 clip 起点
          const first = this.firstClipStart();
          this.setPlayhead(first);
          this.updateTrackWidth();
          // 历史栈底 = 加载态(undo 恢复到操作前态)
          this.pushHistory();
          // 视口 DOM 合成层(gizmo/波形)绑定同一 tl 引用 —— 编辑一致落库
          window.dispatchEvent(new CustomEvent('timeline-loaded', { detail: { tl: this.tl, pid: this.pid } }));
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

      // ============ 播放头拖拽 scrub(对齐 vidonex TimelineRuler playhead 拖动)============
      // pointerdown 在播放头/标尺上按下 → 捕获指针 → move 持续 seek,up 释放。
      // 坐标:48 = 轨道头列宽度(左-48 相对时间轴工作区),x/pps 反算秒。
      beginScrub(e) {
        if (e.button !== 0) return;
        e.preventDefault();
        e.stopPropagation();
        this.scrubbing = true;
        // 拖动即暂停自动播放(播放头已由 pointer 接管)
        const pb = window.vp && window.vp.playback;
        if (pb && pb.isPlaying) pb.pause();
        this._scrubMove = (ev) => this.onScrubMove(ev);
        this._scrubEnd = () => this.onScrubEnd();
        window.addEventListener('pointermove', this._scrubMove);
        window.addEventListener('pointerup', this._scrubEnd, { once: true });
        try { e.target.setPointerCapture && e.target.setPointerCapture(e.pointerId); } catch (_) {}
        this.onScrubMove(e);
      },
      onScrubMove(e) {
        if (!this.scrubbing) return;
        // 基准:时间轴滚动容器(与 seek($event) 同坐标)。播放头挂在轨道区,
        // 其 offsetParent 链上第一个 overflow-auto 容器即基准。48 = 轨道头列宽。
        let el = e.target;
        const root = this.$el;
        while (el && el !== root && !(el.classList && (el.classList.contains('overflow-auto') || el.classList.contains('overflow-x-auto') || el.classList.contains('overflow-y-auto')))) {
          el = el.parentElement;
        }
        if (!el || el === root) el = root;
        const rect = el.getBoundingClientRect();
        const x = e.clientX - rect.left - 64; // 减轨道头列宽
        let t = Math.max(0, x / this.pps);
        // 磁吸吸附(对齐上游 useSnapping:吸附 0/播放头/clip 起止,阈值 6px;Snap OFF 忽略)
        if (this.snapEnabled()) t = this.snapScrub(t);
        this.setPlayhead(t, { source: 'timeline' });
      },
      // scrub 磁吸:吸附到 0、当前播放头、各 clip 起止边(上游 snapTime left-edge 分支;Snap OFF 原样返回)
      snapScrub(t) {
        if (!this.snapEnabled()) return t;
        const edges = [0, this.playhead];
        for (const tr of this.tracks()) {
          for (const c of tr.clips) {
            edges.push(c.start, c.start + c.duration);
          }
        }
        const th = 6 / this.pps; // 秒(上游 thresholdPixels=6 ÷ pps)
        let best = t, bestD = th;
        for (const ed of edges) {
          const d = Math.abs(ed - t);
          if (d < bestD) { bestD = d; best = ed; }
        }
        return best;
      },
      onScrubEnd() {
        window.removeEventListener('pointermove', this._scrubMove);
        window.removeEventListener('pointerup', this._scrubEnd);
        this.scrubbing = false;
        this._scrubMove = null;
        this._scrubEnd = null;
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
      // 轨道 dragover:计算拖拽幻影(对齐 vidonex TrackRow handleDragOver:
      // 磁吸吸附 + 重叠/兼容校验 → 靛蓝(可放)/红(重叠)/红(不兼容)三态)
      onTrackDragOver(e, kind) {
        if (!this.hasDraggedAsset(e) && !this.hasDraggedClip(e)) { this.dragGhost.show = false; return; }
        e.preventDefault();
        e.dataTransfer.dropEffect = 'copy';
        const src = window.vp && window.vp.dragSrc;
        if (!src) { this.dragGhost.show = false; return; }
        const rect = e.currentTarget.getBoundingClientRect();
        const mouseX = e.clientX - rect.left;
        let duration = 5.0, excludeId = null, grabOffset = 0;
        if (src.clip) {
          duration = Number(src.clip.duration) || 5.0;
          excludeId = src.clip.id;
          grabOffset = src.grabOffsetTime || 0;
        } else if (src.asset) {
          const a = src.asset;
          if (a.file_type === 'image') duration = 5.0;
          else if (a.duration_seconds > 0) duration = a.duration_seconds;
        }
        const rawTime = Math.max(0, (mouseX - grabOffset * this.pps) / this.pps);
        const { time: candidate, snapped, snapType } = this.snapTimeFor(rawTime, duration, excludeId);
        const track = this.tracks().find((tr) => tr.kind === kind) ||
                      this.tracks().find((tr) => tr.kind === (kind === 'audio' ? 'video' : 'audio'));
        const { hasOverlap } = this.checkTrackOverlap(track, excludeId, candidate, duration);
        const source = src.clip ? src.clip.source : (src.asset ? (src.asset.file_path || src.asset.file_name) : '');
        const compatible = this.isSourceCompatibleWithTrack(source, kind);
        this.dragGhost = {
          show: true, targetKind: kind,
          left: candidate * this.pps, width: Math.max(10, duration * this.pps),
          duration, snapped, snapType, overlap: hasOverlap, compatible,
          title: src.clip ? (src.clip.name || String(src.clip.id)) : (src.asset ? src.asset.file_name : ''),
        };
      },
      // ghost 可见性:当前轨道 kind 匹配(模板每轨调用)
      ghostVisible(kind) {
        return this.dragGhost.show && this.dragGhost.targetKind === kind;
      },
      // 通用磁吸吸附(对齐上游 snapTime:左缘吸附点=0/播放头/clip 起止,右缘=start+duration;阈值 6px)
      snapTimeFor(targetStart, duration, excludeClipId) {
        if (!this.snapEnabled()) return { time: Math.max(0, targetStart), snapped: false, snapType: '' };
        const points = new Set([0, this.playhead]);
        for (const tr of this.tracks()) {
          for (const c of tr.clips) {
            if (c.id === excludeClipId) continue;
            points.add(c.start); points.add(c.start + c.duration);
          }
        }
        const th = 6 / this.pps; // 秒
        let best = Math.max(0, targetStart), minD = Infinity, snapped = false, snapType = '';
        for (const p of points) {
          const d = Math.abs(p - targetStart);
          if (d <= th && d < minD) { minD = d; best = Math.max(0, p); snapped = true; snapType = p === this.playhead ? 'playhead' : 'start'; }
        }
        if (duration > 0) {
          const targetEnd = targetStart + duration;
          for (const p of points) {
            const d = Math.abs(p - targetEnd);
            if (d <= th && d < minD) { minD = d; best = Math.max(0, p - duration); snapped = true; snapType = 'end'; }
          }
        }
        return { time: best, snapped, snapType };
      },
      // 严格重叠校验(对齐上游 checkTrackOverlap:区间交集 > ε=0.005)
      checkTrackOverlap(track, clipIdToExclude, start, duration) {
        if (!track || !track.clips) return { hasOverlap: false };
        const targetStart = Math.max(0, start);
        const targetEnd = targetStart + duration;
        const eps = 0.005;
        for (const clip of track.clips) {
          if (clip.id === clipIdToExclude) continue;
          const cs = Number(clip.start) || 0;
          const ce = cs + (Number(clip.duration) || 0);
          const os = Math.max(targetStart, cs);
          const oe = Math.min(targetEnd, ce);
          if (os < oe - eps) return { hasOverlap: true, overlappingClipId: clip.id };
        }
        return { hasOverlap: false };
      },
      // 素材与轨类型兼容(对齐上游 isSourceCompatibleWithTrack)
      isSourceCompatibleWithTrack(source, targetKind) {
        if (!source) return true;
        const ext = String(source).toLowerCase().split('.').pop() || '';
        const isImage = ['png', 'jpg', 'jpeg', 'webp', 'svg', 'bmp', 'gif'].includes(ext);
        const isAudio = ['mp3', 'wav', 'aac', 'flac', 'ogg', 'm4a'].includes(ext);
        if (isImage) return targetKind === 'video' || targetKind === 'overlay';
        if (isAudio) return targetKind === 'audio' || targetKind === 'waveform';
        return true; // 视频可进任意轨
      },
      hasDraggedClip(e) {
        return e.dataTransfer && e.dataTransfer.types &&
          Array.from(e.dataTransfer.types).includes('application/x-vidproc-clip');
      },
      hasDraggedAsset(e) {
        if (!e.dataTransfer || !e.dataTransfer.types) return false;
        const types = Array.from(e.dataTransfer.types);
        return types.includes('application/x-vidproc-asset') || types.includes('text/plain');
      },
      onTrackDrop(e, kind) {
        if (!this.tl) return;
        this.dragGhost.show = false;
        window.vp.dragSrc = null;
        // 跨轨 clip 拖拽优先(HTML5 DnD)
        if (this.hasDraggedClip(e)) { this.onTrackClipDrop(e, kind); return; }
        const asset = this.parseAsset(e);
        if (!asset) return;
        e.preventDefault();
        // 兼容预检(对齐上游:图片不可进 audio,音频不可进 video;视频全兼容)
        if (!this.isSourceCompatibleWithTrack(asset.file_path || asset.file_name, kind)) return;
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
      // 转场 ribbon 样式(track.transitions 项):横跨 from 结尾 → to 开头,宽 = duration×pps
      transitionRibbonStyle2(track, t) {
        const from = track.clips.find((c) => c.id === t.from);
        const to = track.clips.find((c) => c.id === t.to);
        if (!from || !to) return 'display:none';
        const dur = Number(t.duration) || 0.5;
        const x = (from.start + from.duration - dur / 2) * this.pps;
        const w = Math.max(14, dur * this.pps);
        return `left:${x}px;width:${w}px`;
      },
      // 由转场项取 from clip 对象
      fromClipOf(track, t) {
        return (track.clips || []).find((c) => c.id === t.from) || null;
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
      // ============ 转场设置弹窗(对齐 vidonex TransitionHandle openModal)============
      // 点击转场 ribbon 打开:类型选择 + 时长滑块 + 删除/应用。
      // 单选跨 clip 交界,数据落在 track.transitions。
      openTransitionModal(track, clip) {
        const nextId = this.nextClipId(track, clip);
        if (!nextId) return;
        const cur = this.transitionOf(track, clip);
        this.transitionModal = {
          open: true,
          track,
          fromClip: clip,
          toId: nextId,
          type: cur ? cur.type : 'dissolve',
          duration: cur ? Number(cur.duration) || 0.5 : 0.5,
          existed: !!cur,
        };
      },
      closeTransitionModal() { if (this.transitionModal) this.transitionModal.open = false; },
      // 转场类型库(精简 8 类,对齐上游目录子集)
      transitionTypes() {
        return [
          { value: 'dissolve', label: '交叉溶解', cat: 'fades' },
          { value: 'fade', label: '淡入淡出', cat: 'fades' },
          { value: 'fadeblack', label: '黑场淡入', cat: 'fades' },
          { value: 'wipeleft', label: '左擦除', cat: 'wipes' },
          { value: 'wiperight', label: '右擦除', cat: 'wipes' },
          { value: 'slideleft', label: '左滑入', cat: 'slides' },
          { value: 'slideright', label: '右滑入', cat: 'slides' },
          { value: 'smoothleft', label: '平滑左移', cat: 'slides' },
          { value: 'smoothright', label: '平滑右移', cat: 'slides' },
          { value: 'zoomin', label: '缩放进入', cat: 'zooms' },
          { value: 'circleopen', label: '圆形展开', cat: 'shapes' },
        ];
      },
      transitionTypeLabel(type) {
        const f = this.transitionTypes().find((t) => t.value === type);
        return f ? f.label : type;
      },
      // 转场最长时长 = 两 clip 较短者 ×2(居中覆盖 D/2 各边)
      transitionMaxDuration() {
        const m = this.transitionModal;
        if (!m) return 2;
        const d1 = Number(m.fromClip.duration) || 2;
        const d2 = (m.track.clips.find((c) => c.id === m.toId) || {}).duration || 2;
        return Math.max(0.2, Math.min(10, Math.min(d1, d2) * 2));
      },
      applyTransitionModal() {
        const m = this.transitionModal;
        if (!m) return;
        const dur = Math.min(this.transitionMaxDuration(), Math.max(0.1, Number(m.duration) || 0.5));
        const from = m.fromClip.id, to = m.toId;
        this.addTransition(m.type, from, to);
        // addTransition 默认 0.5s,这里写回设置值
        const tr = this.trackById && this.trackById(m.track.id);
        const t = tr && tr.transitions ? tr.transitions.find((x) => x.from === from && x.to === to) : null;
        if (t) { t.duration = Number(dur.toFixed(2)); this.persistTimeline(); }
        this.closeTransitionModal();
      },
      removeTransitionModal() {
        const m = this.transitionModal;
        if (!m) return;
        this.removeTransition(m.fromClip.id, m.toId);
        this.closeTransitionModal();
      },
      trackById(id) { return this.tracks().find((t) => t.id === id); },
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
      // 拖到另一轨道的 drop:把 clip 挪过去(时间按落点)。
      // 对齐 vidonex handleDrop:兼容校验 + 严格零重叠(重叠则取消移动)
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
        // 兼容校验:素材类型不可进目标轨则取消(对齐 isSourceCompatibleWithTrack)
        if (!this.isSourceCompatibleWithTrack(clip.source, targetKind)) return;
        // 落点时间(相对目标轨内容区)+ 磁吸吸附
        const rect = e.currentTarget ? e.currentTarget.getBoundingClientRect() : null;
        let t = clip.start;
        if (rect) {
          const raw = Math.max(0, (e.clientX - rect.left) / this.pps);
          const snapped = this.snapTimeFor(raw, clip.duration, clip.id);
          t = snapped.time;
        }
        // 严格零重叠:与目标轨既有 clip 相交则取消移动(对齐 checkTrackOverlap)
        const { hasOverlap } = this.checkTrackOverlap(dst, clip.id, t, clip.duration);
        if (hasOverlap) return;
        // 从源轨移除
        srcTrack.clips = srcTrack.clips.filter((c) => c.id !== clip.id);
        clip.start = t;
        if (!dst.clips) dst.clips = [];
        dst.clips.push(clip);
        this.selectedId = clip.id;
        this.updateTrackWidth();
        this.persistTimeline();
      },
      // HTML5 DnD:clip 元素 dragstart 时把 id 挂 transport + 全局 dragSrc(幻影计算用)
      beginClipDnD(e, clip) {
        e.dataTransfer.setData('application/x-vidproc-clip', clip.id);
        e.dataTransfer.effectAllowed = 'move';
        this.selectedId = clip.id;
        if (window.vp) window.vp.dragSrc = { clip, sourceTrackId: this.tracks().find((tr) => tr.clips && tr.clips.some((c) => c.id === clip.id))?.id || null, grabOffsetTime: 0 };
        // dragend 清全局槽(幻影/后续校验)
        e.target.addEventListener('dragend', () => { if (window.vp) window.vp.dragSrc = null; }, { once: true });
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

      // 标尺("/ 轨道点击 → 定位播放头(对齐 vidonex Ruler click)
      seek(e) {
        const rect = e.currentTarget.getBoundingClientRect();
        const x = e.clientX - rect.left;
        this.setPlayhead(Math.max(0, x / this.pps), { source: 'timeline' });
      },
// 标尺动态刻度(对齐上游 TimelineRuler timeMarks:pps<30→5s,<60→2s,否则 1s)
      rulerMarks() {
        const dur = this.trackWidth / Math.max(this.pps, 1);
        let interval = 1;
        if (this.pps < 30) interval = 5;
        else if (this.pps < 60) interval = 2;
        const out = [];
        for (let t = 0; t <= dur; t += interval) {
          const m = Math.floor(t / 60);
          const s = Math.floor(t % 60);
          out.push({
            t,
            pos: t * this.pps,
            label: String(m).padStart(2, '0') + ':' + String(s).padStart(2, '0'),
          });
        }
        return out;
      },
      dispatchPlayhead() {
        window.dispatchEvent(new CustomEvent('playhead-changed', { detail: { t: this.playhead, pid: this.pid, source: 'timeline' } }));
      },

      // ============ 选中 clip → 检查器 ============
      select(clip) {
        this.selectedId = this.selectedId === clip.id ? null : clip.id;
        this.selectedTrackId = null;
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

      // 选中轨道(轨道头点击)→ Ducking/Waveform 检查器。toggle 式。
      selectTrack(track) {
        this.selectedTrackId = this.selectedTrackId === track.id ? null : track.id;
        this.selectedId = null;
        const tr = this.selectedTrackId ? track : null;
        window.dispatchEvent(new CustomEvent('track-selected', {
          detail: {
            track: tr,
            tracks: this.tracks(),
            tl: this.tl,
            editor: this,
          },
        }));
      },

      // ============ 右键上下文菜单(对齐 vidonex ClipItem UContextMenu 四段式)============
      openCtxMenu(e, clip, track) {
        e.preventDefault();
        e.stopPropagation();
        this.select(clip);
        this.ctxMenu = { open: true, x: e.clientX, y: e.clientY, clip, track };
        // 点击别处关闭
        document.addEventListener('click', this._ctxClose = () => { this.ctxMenu.open = false; }, { once: true });
      },
      ctxProbe() {
        // Probe 详情:匹配项目素材(文件名对 assetThumbs 的值是 thumbnail_url,需找 asset 对象)
        // 素材详情弹窗归 mediaLibrary 管,这里广播 probe-open 由 mediaLibrary 打开。
        const m = this.ctxMenu;
        if (!m) return;
        this.ctxMenu.open = false;
        // mediaLibrary 用 detail:{fileName} 自行匹配自己缓存的 assets
        window.dispatchEvent(new CustomEvent('probe-open', { detail: { fileName: (m.clip.source || '').split('/').pop() } }));
      },
      ctxDelete() {
        const m = this.ctxMenu;
        if (!m) return;
        this.ctxMenu.open = false;
        const self = this;
        this.select(m.clip);
        window.vp.confirm({
          title: '删除片段',
          message: `删除 ${m.clip.id}?`,
          confirmText: '删除',
          cancelText: '取消',
          isDanger: true,
        }).then((ok) => { if (ok) self.deleteSelected(); });
      },
      ctxDuplicate() {
        const m = this.ctxMenu;
        if (!m) return;
        this.ctxMenu.open = false;
        this.select(m.clip);
        this.duplicate();
      },
      ctxInspect() {
        const m = this.ctxMenu;
        if (!m) return;
        this.ctxMenu.open = false;
        this.select(m.clip);
        // 选中即广播 clip-selected(timelineEditor.select 已做 → 检查器显示)
      },
      splitAtCtx() {
        const m = this.ctxMenu;
        if (!m) return;
        this.ctxMenu.open = false;
        this.select(m.clip);
        this.split();
      },
      // filmstrip 缩略图 URL(clip.source 文件名 → assets 缩略图;
      // 素材不在库时兜底走 /source-frame 对 spec 源文件抽帧,对齐上游 ClipItem 直接用 clip.source)。
      thumbFor(clip) {
        if (!clip) return null;
        const base = (clip.source || '').split(/[\\/]/).pop().toLowerCase();
        if (this.assetThumbs) {
          if (this.assetThumbs[base]) return this.assetThumbs[base];
          // 上传副本带时间戳前缀(1792252822383_xxx.mp4 → 按去前缀名匹配)
          const stem = base.replace(/^\d+_/, '');
          if (stem !== base) {
            for (const k in this.assetThumbs) {
              if (k.replace(/^\d+_/, '') === stem) return this.assetThumbs[k];
            }
          }
        }
        if (clip.source && this.pid) {
          return `/api/projects/${this.pid}/source-frame?src=${encodeURIComponent(clip.source)}&t=0&w=240`;
        }
        return null;
      },
      // ============ 轨道操作(对齐 vidonex TrackHeader mute/删除)============
      toggleMute(track) {
        if (!track) return;
        track.muted = !track.muted;
        this.persistTimeline();
      },
      removeTrack(track) {
        if (!track || !this.tl) return;
        const self = this;
        window.vp.confirm({
          title: '删除轨道',
          message: `删除轨道 ${track.id}?其所有 clip 将一并移除。`,
          confirmText: '删除',
          cancelText: '取消',
          isDanger: true,
        }).then((ok) => {
          if (!ok) return;
          this.tl.tracks = this.tl.tracks.filter((t) => t !== track);
          if (this.selectedTrackId === track.id) this.selectedTrackId = null;
          this.updateTrackWidth();
          this.persistTimeline();
        });
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

      // yamlBlock:检查器对象 → YAML 缩进行(标量/嵌套对象/对象数组,
      // 前端序列化专用,避免手拼大段)。返回行数组,indent 由调用方给。
      yamlBlock(obj, indent) {
        if (!obj || typeof obj !== 'object') return [];
        const pad = ' '.repeat(indent);
        const out = [];
        for (const [k, v] of Object.entries(obj)) {
          if (v === undefined || v === null || v === '') continue;
          if (Array.isArray(v)) {
            if (!v.length) continue;
            out.push(`${pad}${k}:`);
            for (const item of v) {
              if (typeof item === 'object') {
                out.push(`${pad}  - {${Object.entries(item)
                  .filter(([, iv]) => iv !== undefined && iv !== null && iv !== '')
                  .map(([ik, iv]) => `${ik}: ${this.yamlScalar(iv)}`).join(', ')}}`);
              } else {
                out.push(`${pad}  - ${this.yamlScalar(item)}`);
              }
            }
          } else if (typeof v === 'object') {
            out.push(`${pad}${k}:`);
            for (const [ik, iv] of Object.entries(v)) {
              if (iv === undefined || iv === null || iv === '') continue;
              out.push(`${pad}  ${ik}: ${this.yamlScalar(iv)}`);
            }
          } else {
            out.push(`${pad}${k}: ${this.yamlScalar(v)}`);
          }
        }
        return out;
      },
      // yamlScalar:字符串加引号,数字/布尔原样。
      yamlScalar(v) {
        if (typeof v === 'string') return `"${this.yamlSafe(v)}"`;
        return String(v);
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
            if (tr.muted) lines.push(`    muted: true`);
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
                if (c.fade_in) lines.push(`        fade_in: "${c.fade_in}s"`);
                if (c.fade_out) lines.push(`        fade_out: "${c.fade_out}s"`);
                if (c.has_audio === false) lines.push(`        has_audio: false`);
                // —— 检查器子面板字段(对齐 vidonex chroma/color/ken_burns/subtitles/waveform/crop)——
                if (c.crop && (c.crop.top || c.crop.bottom || c.crop.left || c.crop.right)) {
                  lines.push(`        crop: {top: ${c.crop.top || 0}, bottom: ${c.crop.bottom || 0}, left: ${c.crop.left || 0}, right: ${c.crop.right || 0}}`);
                }
                if (c.chroma_key && c.chroma_key.color) {
                  lines.push('        chroma_key:');
                  for (const ln of this.yamlBlock(c.chroma_key, 10)) lines.push(ln);
                }
                if (c.color_grading && Object.keys(c.color_grading).length) {
                  lines.push('        color_grading:');
                  for (const ln of this.yamlBlock(c.color_grading, 10)) lines.push(ln);
                }
                if (c.ken_burns && c.ken_burns.start_scale !== undefined) {
                  lines.push('        ken_burns:');
                  for (const ln of this.yamlBlock(c.ken_burns, 10)) lines.push(ln);
                }
                if (c.subtitles && (c.subtitles.font_size || c.subtitles.font_color || c.subtitles.cues && c.subtitles.cues.length)) {
                  lines.push('        subtitles:');
                  for (const ln of this.yamlBlock(c.subtitles, 10)) lines.push(ln);
                }
                if (c.waveform && c.waveform.mode) {
                  lines.push('        waveform:');
                  for (const ln of this.yamlBlock(c.waveform, 10)) lines.push(ln);
                }
              }
            }
            // —— 轨道级:ducking(对齐 vidonex track ducking)——
            if (tr.duck_under || tr.ducking) {
              if (tr.duck_under) lines.push(`    duck_under: "${tr.duck_under}"`);
              if (tr.ducking) {
                lines.push('    ducking:');
                for (const ln of this.yamlBlock(tr.ducking, 6)) lines.push(ln);
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
      // 视图/分类(对齐 vidonex MediaLibrary:tree 默认,grid 切换,分类可折叠)
      viewMode: 'tree', // 'tree' | 'grid'
      expanded: { video: true, audio: true, image: true, subtitle: true, other: true },
      isDraggingOver: false,
      dragGhostAsset: null,
      // ProbeDetailsModal(对齐 vidonex media store:activeProbeAsset/isProbeModalOpen)
      activeProbeAsset: null,
      isProbeModalOpen: false,

      // 打开素材详情(点卡片 i 信息钮);7 字段与上游一致
      openProbeModal(a) {
        this.activeProbeAsset = a;
        this.isProbeModalOpen = true;
      },
      closeProbeModal() { this.isProbeModalOpen = false; },

      async init() {
        if (!this.pid) return;
        // timelineEditor 右键菜单 "查看详情" → probe-open{fileName} → 这里打开
        window.addEventListener('probe-open', (e) => {
          const fn = (e.detail || {}).fileName || '';
          const hit = this.assets.find((a) => (a.file_name || '').toLowerCase() === fn.toLowerCase());
          if (hit) this.openProbeModal(hit);
        });
        await this.load();
      },
      get filtered() {
        if (!this.query.trim()) return this.assets;
        const q = this.query.toLowerCase();
        return this.assets.filter((a) => (a.file_name || '').toLowerCase().includes(q));
      },
      // 素材分类(对齐 vidonex getAssetCategory:扩展名判定 video/audio/image/subtitle/other)
      assetCategory(a) {
        if (!a) return 'other';
        const ext = ((a.file_name || a.file_path || '').toLowerCase().split('.').pop() || '').replace(/[^a-z0-9]/g, '');
        if (['srt', 'vtt', 'ass', 'sub'].includes(ext)) return 'subtitle';
        if (a.file_type === 'video' || ['mp4', 'mov', 'mkv', 'webm', 'avi', 'flv'].includes(ext)) return 'video';
        if (a.file_type === 'audio' || ['mp3', 'wav', 'aac', 'flac', 'ogg', 'm4a'].includes(ext)) return 'audio';
        if (a.file_type === 'image' || ['png', 'jpg', 'jpeg', 'webp', 'svg', 'gif', 'bmp'].includes(ext)) return 'image';
        return 'other';
      },
      ASSET_CATEGORIES: [
        { id: 'video', label: '视频', color: 'text-indigo-400', badge: 'bg-indigo-500/15 text-indigo-300' },
        { id: 'audio', label: '音频', color: 'text-emerald-400', badge: 'bg-emerald-500/15 text-emerald-300' },
        { id: 'image', label: '图片', color: 'text-amber-400', badge: 'bg-amber-500/15 text-amber-300' },
        { id: 'subtitle', label: '字幕', color: 'text-violet-400', badge: 'bg-violet-500/15 text-violet-300' },
        { id: 'other', label: '其他', color: 'text-gray-400', badge: 'bg-gray-700/30 text-gray-400' },
      ],
      // 分类分组(仅显示非空分类)
      categorized() {
        const groups = { video: [], audio: [], image: [], subtitle: [], other: [] };
        for (const a of this.filtered) {
          const c = this.assetCategory(a);
          if (groups[c]) groups[c].push(a); else groups.other.push(a);
        }
        return groups;
      },
      activeCategories() {
        const g = this.categorized();
        return this.ASSET_CATEGORIES.filter((c) => (g[c.id] || []).length > 0);
      },
      toggleCategory(id) {
        this.expanded[id] = !this.expanded[id];
      },
      // OS 文件拖入面板 → 上传(对齐 vidonex handleUploadDrop)
      uploadDrop(e) {
        this.isDraggingOver = false;
        const files = Array.from((e.dataTransfer && e.dataTransfer.files) || []);
        if (!files.length || !this.pid || !files.some((f) => f.size > 0 || f.name)) return;
        this.uploadFilesFromList(files).then((n) => { if (n) this.load(); });
      },
      // 文件列表上传(拖入/选择共用)
      async uploadFilesFromList(files) {
        if (!files.length || !this.pid) return 0;
        this.uploading = true;
        let n = 0;
        try {
          for (const f of files) {
            const fd = new FormData();
            fd.append('file', f);
            const r = await fetch(`/api/projects/${this.pid}/assets`, { method: 'POST', body: fd });
            if (r.ok) { n++; }
          }
        } finally { this.uploading = false; }
        return n;
      },
      async load() {
        if (!this.pid) return;
        try {
          const r = await fetch(`/api/projects/${this.pid}/assets`);
          if (!r.ok) throw new Error(r.status);
          this.assets = await r.json();
        } catch (e) { this.error = e.message; }
      },
      // 上传(多文件,文件选择框)
      async uploadFiles(e) {
        const files = Array.from(e.target.files || []);
        e.target.value = '';
        if (!files.length || !this.pid) return;
        const n = await this.uploadFilesFromList(files);
        if (n) this.load();
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
      // 拖拽开始:素材信息挂在 dataTransfer + 自定义拖拽徽章(对齐 vidonex sleek drag ghost)
      onDragStart(ev, a) {
        ev.dataTransfer.setData('application/x-vidproc-asset', JSON.stringify(a));
        ev.dataTransfer.setData('text/plain', a.file_name);
        ev.dataTransfer.effectAllowed = 'copy';
        // 全局 dragSrc:时间轴轨道 dragover 幻影计算读取(对齐 vidonex mediaStore.draggedAsset)
        if (window.vp) window.vp.dragSrc = { asset: a };
        this.dragGhostAsset = a;
        const badge = this.$refs.dragGhost;
        if (ev.dataTransfer.setDragImage && badge) {
          try { ev.dataTransfer.setDragImage(badge, 24, 12); } catch (_) {}
        }
        ev.target.addEventListener('dragend', () => {
          if (window.vp) window.vp.dragSrc = null;
          setTimeout(() => { this.dragGhostAsset = null; }, 150);
        }, { once: true });
      },
    };
  };

  // ============ 检查器(clipInspector)============
  // 挂在 inspector_panel.监听时间轴 clip-selected / track-selected 事件,
  // 持有选中 clip 引用(就是 tl 里的对象),编辑字段 → editor.editClip()
  // 300ms debounce autosave。
  // 对齐 vidonex InspectorPanel 子面板族:
  //   clip 选中 → 时序/变换/音频 + ColorAdjustments + ChromaKey + KenBurns
  //               (+ waveform 轨:WaveformInspector / subtitle 轨:SubtitleInspector)
  //   track 选中 → DuckingInspector(waveform 轨 → WaveformInspector)
  // 子面板字段先镜像进 cfg(默认值),commit 时写回 clip 对象再落库。
  window.clipInspector = function () {
    return {
      clip: null,      // 选中 clip 引用(直接改字段,时间轴同步反映)
      trackKind: null, // video/overlay/audio/waveform/subtitle
      track: null,     // 选中轨道(轨道头点击;track 级检查器用)
      tracks: [],      // 全部轨道(ducking target_track 下拉用)
      _editor: null,   // timelineEditor 引用(提供 editClip/persistTimeline)
      // —— 子面板镜像配置(编辑对象;commit 时写回 clip/track)——
      cfg: {
        chroma_key: { enabled: false, color: '#00FF00', similarity: 0.3, blend: 0.1, despill: true },
        color_grading: { brightness: 0, contrast: 100, saturation: 100, gamma: 100, highlights: 0, shadows: 0, whites: 0, blacks: 0, temperature: 0, tint: 0, blur: 0, sharpen: 0 },
        ken_burns: { enabled: false, start_scale: 1.0, end_scale: 1.25, easing: 'easeInOutQuad' },
        subtitles: { font_size: 32, font_color: '#FFFFFF', outline_width: 2, outline_color: '#000000', box: false, cues: [] },
        waveform: { mode: 'peak_to_peak', color: '#00E6B4', secondary_color: '#6366F1', width: 1200, height: 180, position_x: 0, position_y: 0, opacity: 1.0, glow: true, glow_radius: 8, bar_density: 48, bar_roundness: 6, line_width: 3, fill_area: false, inner_radius: 0.45 },
        ducking: { enabled: false, target_track: '', threshold: -25, duck_amount: -14, attack_ms: 50, release_ms: 500 },
      },

      init() {
        window.addEventListener('clip-selected', (e) => {
          const d = e.detail || {};
          this.clip = d.clip || null;
          this.trackKind = d.trackKind || null;
          this.track = null; // clip 选中掩盖 track 选中
          this.tracks = (d.tl && d.tl.tracks) || this.tracks;
          this._editor = d.editor || null;
          if (this.clip) {
            this.normalizeClip();
            this.syncFromClip();
          } else {
            this.resetCfg();
          }
        });
        window.addEventListener('track-selected', (e) => {
          const d = e.detail || {};
          this.track = d.track || null;
          this.clip = null;
          this.tracks = d.tracks || this.tracks;
          this._editor = d.editor || null;
          this.trackKind = this.track ? this.track.kind : null;
          if (this.track) this.syncFromTrack();
        });
      },

      // 规范化可选字段(x-model.number 需要数值存在)
      normalizeClip() {
        const c = this.clip;
        c.speed = c.speed || 1;
        c.trim = c.trim || 0;
        c.scale = c.scale || 1;
        c.rotation = c.rotation || 0;
        c.opacity = c.opacity !== undefined ? c.opacity : 1;
        c.volume = c.volume !== undefined ? c.volume : 1;
        c.blend_mode = c.blend_mode || 'normal';
        if (!c.position) c.position = { x: 0, y: 0, alignment: 'center' };
        if (!c.position.alignment) c.position.alignment = 'center';
      },

      // —— 子面板镜像同步 ——
      syncFromClip() {
        const c = this.clip;
        // ChromaKey
        const ck = c.chroma_key || null;
        this.cfg.chroma_key = {
          enabled: !!ck,
          color: (ck && (ck.color || ck.key_color)) || '#00FF00',
          similarity: ck ? (ck.similarity ?? 0.3) : 0.3,
          blend: ck ? (ck.blend ?? 0.1) : 0.1,
          despill: ck ? (ck.despill ?? true) : true,
        };
        // ColorGrading(UI 双极值 ↔ spec 值映射,与上游一致)
        const cg = c.color_grading || null;
        this.cfg.color_grading = {
          brightness: Math.round(((cg && cg.brightness !== undefined ? cg.brightness : 0) / 0.5) * 100),
          contrast: Math.round((((cg && cg.contrast !== undefined ? cg.contrast : 1.0) - 1.0) * 125)),
          saturation: Math.round((((cg && cg.saturation !== undefined ? cg.saturation : 1.0) - 1.0) * 100)),
          gamma: Math.round((((cg && cg.gamma !== undefined ? cg.gamma : 1.0) - 1.0) * 133.3)),
          highlights: Math.round((cg && cg.highlights !== undefined ? cg.highlights : 0) * 100),
          shadows: Math.round((cg && cg.shadows !== undefined ? cg.shadows : 0) * 100),
          whites: Math.round((cg && cg.whites !== undefined ? cg.whites : 0) * 100),
          blacks: Math.round((cg && cg.blacks !== undefined ? cg.blacks : 0) * 100),
          temperature: Math.round(((cg && cg.temperature !== undefined ? cg.temperature : 0) / 0.8) * 100),
          tint: Math.round(((cg && cg.tint !== undefined ? cg.tint : 0) / 0.8) * 100),
          blur: cg ? Math.round(cg.blur || 0) : 0,
          sharpen: cg ? Math.round(cg.sharpen || 0) : 0,
        };
        // KenBurns
        const kb = c.ken_burns || null;
        this.cfg.ken_burns = {
          enabled: !!kb,
          start_scale: (kb && kb.start_scale) || 1.0,
          end_scale: (kb && kb.end_scale) || 1.25,
          easing: (kb && kb.easing) || 'easeInOutQuad',
        };
        // Subtitles
        const st = c.subtitles || null;
        this.cfg.subtitles = {
          font_size: (st && st.font_size) || 32,
          font_color: (st && st.font_color) || '#FFFFFF',
          outline_width: st ? (st.outline_width ?? 2) : 2,
          outline_color: (st && st.outline_color) || '#000000',
          box: st ? (st.box ?? false) : false,
          cues: (st && st.cues) || [],
        };
        // Waveform
        const wf = c.waveform || null;
        this.cfg.waveform = {
          mode: (wf && wf.mode) || 'peak_to_peak',
          color: (wf && wf.color) || '#00E6B4',
          secondary_color: (wf && wf.secondary_color) || '#6366F1',
          width: (wf && wf.width) || 1200,
          height: (wf && wf.height) || 180,
          position_x: c.position.x || 0,
          position_y: c.position.y || 0,
          opacity: c.opacity !== undefined ? c.opacity : 1.0,
          glow: wf ? (wf.glow ?? true) : true,
          glow_radius: (wf && wf.glow_radius) || 8,
          bar_density: (wf && wf.bar_density) || 48,
          bar_roundness: (wf && wf.bar_roundness) || 6,
          line_width: (wf && wf.line_width) || 3,
          fill_area: wf ? (wf.fill_area ?? false) : false,
          inner_radius: (wf && wf.inner_radius) || 0.45,
        };
      },
      // track 级同步(ducking / waveform)
      syncFromTrack() {
        const tr = this.track;
        this.cfg.ducking = {
          enabled: !!(tr.ducking || tr.duck_under),
          target_track: tr.duck_under || (this.voiceTracks()[0] ? this.voiceTracks()[0].id : ''),
          threshold: (tr.ducking && tr.ducking.threshold !== undefined) ? tr.ducking.threshold : -25,
          duck_amount: (tr.ducking && tr.ducking.duck_db !== undefined) ? tr.ducking.duck_db : -14,
          attack_ms: (tr.ducking && tr.ducking.attack_ms !== undefined) ? tr.ducking.attack_ms : 50,
          release_ms: (tr.ducking && tr.ducking.release_ms !== undefined) ? tr.ducking.release_ms : 500,
        };
        const wf = tr.waveform || null;
        this.cfg.waveform = {
          mode: (wf && wf.mode) || 'peak_to_peak',
          color: (wf && wf.color) || '#00E6B4',
          secondary_color: (wf && wf.secondary_color) || '#6366F1',
          width: (wf && wf.width) || 1200,
          height: (wf && wf.height) || 180,
          position_x: 0, position_y: 0,
          opacity: (wf && wf.opacity !== undefined) ? wf.opacity : 1.0,
          glow: wf ? (wf.glow ?? true) : true,
          glow_radius: (wf && wf.glow_radius) || 8,
          bar_density: (wf && wf.bar_density) || 48,
          bar_roundness: (wf && wf.bar_roundness) || 6,
          line_width: (wf && wf.line_width) || 3,
          fill_area: wf ? (wf.fill_area ?? false) : false,
          inner_radius: (wf && wf.inner_radius) || 0.45,
        };
      },
      resetCfg() {
        this.cfg.chroma_key = { enabled: false, color: '#00FF00', similarity: 0.3, blend: 0.1, despill: true };
        this.cfg.ken_burns = { enabled: false, start_scale: 1.0, end_scale: 1.25, easing: 'easeInOutQuad' };
      },

      // 可用闪避目标轨道(其它 audio/video 轨)
      voiceTracks() {
        const tid = this.track ? this.track.id : (this.trackKind ? null : null);
        return this.tracks.filter((t) => t.id !== tid && (t.kind === 'audio' || t.kind === 'video'));
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

      // —— 子面板 commit:写回 clip/track → editClip autosave ——
      // ChromaKey:enable 切换删除/重建 chroma_key
      toggleChromaKey() {
        const c = this.clip;
        if (!c) return;
        if (this.cfg.chroma_key.enabled) {
          c.chroma_key = {
            color: this.cfg.chroma_key.color,
            similarity: this.cfg.chroma_key.similarity,
            blend: this.cfg.chroma_key.blend,
            despill: this.cfg.chroma_key.despill,
          };
        } else {
          delete c.chroma_key;
        }
        this.commit('Toggle Chroma Key');
      },
      commitChromaKey() {
        const c = this.clip;
        if (!c || !this.cfg.chroma_key.enabled) return;
        c.chroma_key = {
          color: this.cfg.chroma_key.color,
          similarity: this.cfg.chroma_key.similarity,
          blend: this.cfg.chroma_key.blend,
          despill: this.cfg.chroma_key.despill,
        };
        this.commit('Update Chroma Key');
      },

      // ColorGrading:UI 双极值 → spec 值(与上游映射一致)
      commitColorGrading() {
        const c = this.clip;
        if (!c) return;
        const g = this.cfg.color_grading;
        c.color_grading = {
          brightness: +(g.brightness * 0.005).toFixed(3),
          contrast: +(1.0 + g.contrast * 0.008).toFixed(3),
          saturation: +(1.0 + g.saturation * 0.01).toFixed(3),
          gamma: +(1.0 + g.gamma * 0.0075).toFixed(3),
          temperature: +(g.temperature * 0.008).toFixed(3),
          tint: +(g.tint * 0.008).toFixed(3),
          highlights: +(g.highlights * 0.01).toFixed(3),
          shadows: +(g.shadows * 0.01).toFixed(3),
          whites: +(g.whites * 0.01).toFixed(3),
          blacks: +(g.blacks * 0.01).toFixed(3),
          blur: g.blur,
          sharpen: g.sharpen,
        };
        this.commit('Update Color Adjustments');
      },
      resetColorGrading() {
        const g = this.cfg.color_grading;
        g.brightness = 0; g.contrast = 100; g.saturation = 100; g.gamma = 100;
        g.highlights = 0; g.shadows = 0; g.whites = 0; g.blacks = 0;
        g.temperature = 0; g.tint = 0; g.blur = 0; g.sharpen = 0;
        this.commitColorGrading();
      },

      // KenBurns
      toggleKenBurns() {
        const c = this.clip;
        if (!c) return;
        if (this.cfg.ken_burns.enabled) {
          c.ken_burns = {
            start_scale: this.cfg.ken_burns.start_scale,
            end_scale: this.cfg.ken_burns.end_scale,
            easing: this.cfg.ken_burns.easing,
          };
        } else {
          delete c.ken_burns;
        }
        this.commit('Toggle Ken Burns Animation');
      },
      commitKenBurns() {
        const c = this.clip;
        if (!c || !this.cfg.ken_burns.enabled) return;
        c.ken_burns = {
          start_scale: this.cfg.ken_burns.start_scale,
          end_scale: this.cfg.ken_burns.end_scale,
          easing: this.cfg.ken_burns.easing,
        };
        this.commit('Update Ken Burns');
      },

      // Subtitles(cues 增删改)
      addCue() {
        this.cfg.subtitles.cues.push({ start: 0, end: 3.0, text: 'New Subtitle Caption' });
        this.commitSubtitles();
      },
      removeCue(idx) {
        this.cfg.subtitles.cues.splice(idx, 1);
        this.commitSubtitles();
      },
      commitSubtitles() {
        const c = this.clip;
        if (!c) return;
        const s = this.cfg.subtitles;
        c.subtitles = {
          font_size: s.font_size,
          font_color: s.font_color,
          outline_width: s.outline_width,
          outline_color: s.outline_color,
          box: s.box,
          cues: s.cues,
        };
        this.commit('Update Subtitles');
      },

      // Waveform(clip/waveform 轨 track 写入;position/opacity 同步 clip)
      commitWaveform() {
        const target = this.clip || (this.track && this.track.kind === 'waveform' ? this.track : null);
        if (!target) return;
        const w = this.cfg.waveform;
        const wf = {
          mode: w.mode,
          color: w.color,
          secondary_color: w.secondary_color,
          width: w.width,
          height: w.height,
          opacity: w.opacity,
          glow: w.glow,
          glow_radius: w.glow_radius,
          bar_density: w.bar_density,
          bar_roundness: w.bar_roundness,
          line_width: w.line_width,
          fill_area: w.fill_area,
          inner_radius: w.inner_radius,
          position: (this.clip || this.track) ? { x: w.position_x, y: w.position_y, alignment: 'center' } : undefined,
        };
        if (this.clip) {
          target.waveform = wf;
          target.position = target.position || {};
          target.position.x = w.position_x;
          target.position.y = w.position_y;
          target.opacity = w.opacity;
        } else {
          target.waveform = wf;
        }
        this.commit(this.clip ? 'Update Waveform' : 'Update Track Waveform');
      },
      resetWaveform() {
        const w = this.cfg.waveform;
        w.mode = 'peak_to_peak'; w.color = '#00E6B4'; w.secondary_color = '#6366F1';
        w.width = 1200; w.height = 180; w.position_x = 0; w.position_y = 0; w.opacity = 1.0;
        w.glow = true; w.glow_radius = 8; w.bar_density = 48; w.bar_roundness = 6;
        w.line_width = 3; w.fill_area = false; w.inner_radius = 0.45;
        this.commitWaveform();
      },
      // 预设样式卡
      applyWaveformPreset(id) {
        const w = this.cfg.waveform;
        const presets = {
          cyberpunk: { mode: 'peak_to_peak', color: '#00E6B4', secondary_color: '#6366F1', glow: true, glow_radius: 10, bar_density: 48, bar_roundness: 6 },
          minimal: { mode: 'spectrum', color: '#F43F5E', secondary_color: '#FB923C', glow: false, glow_radius: 4, bar_density: 40, bar_roundness: 4 },
          retro: { mode: 'wave', color: '#10B981', secondary_color: '#06B6D4', glow: true, glow_radius: 12, bar_density: 48, line_width: 3, fill_area: true },
          glow: { mode: 'circular', color: '#A855F7', secondary_color: '#EC4899', glow: true, glow_radius: 10, bar_density: 64, inner_radius: 0.45 },
        };
        if (presets[id]) Object.assign(w, presets[id]);
        this.commitWaveform();
      },
      // 对齐快捷钮
      alignWaveform(pos) {
        const tl = this._editor && this._editor.tl;
        const ch = tl ? tl.height : 1080;
        if (pos === 'center') { this.cfg.waveform.position_x = 0; this.cfg.waveform.position_y = 0; }
        if (pos === 'lower') { this.cfg.waveform.position_x = 0; this.cfg.waveform.position_y = Math.round(ch * 0.28); }
        this.commitWaveform();
      },

      // Ducking(track 级)
      toggleDucking() {
        const tr = this.track;
        if (!tr) return;
        if (this.cfg.ducking.enabled) {
          tr.duck_under = this.cfg.ducking.target_track;
          tr.ducking = {
            threshold: this.cfg.ducking.threshold,
            duck_db: this.cfg.ducking.duck_amount,
            attack_ms: this.cfg.ducking.attack_ms,
            release_ms: this.cfg.ducking.release_ms,
          };
        } else {
          delete tr.duck_under;
          delete tr.ducking;
        }
        this.commit('Toggle Sidechain Ducking');
      },
      commitDucking() {
        const tr = this.track;
        if (!tr || !this.cfg.ducking.enabled) return;
        tr.duck_under = this.cfg.ducking.target_track;
        tr.ducking = {
          threshold: this.cfg.ducking.threshold,
          duck_db: this.cfg.ducking.duck_amount,
          attack_ms: this.cfg.ducking.attack_ms,
          release_ms: this.cfg.ducking.release_ms,
        };
        this.commit('Update Ducking');
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

      // Waveform 预设首色/次色(预设卡圆点预览用)
      presetColor(id, idx) {
        const p = { cyberpunk: ['#00E6B4', '#6366F1'], minimal: ['#F43F5E', '#FB923C'], retro: ['#10B981', '#06B6D4'], glow: ['#A855F7', '#EC4899'] };
        return (p[id] || ['#00E6B4', '#6366F1'])[idx];
      },

      // —— CurveEditor:canvas 画缓动曲线 + 播放扫点(对齐上游 CurveEditor.vue)——
      evaluateEasing(t, easing) {
        const c = Math.max(0, Math.min(1, t));
        switch (easing) {
          case 'linear': return c;
          case 'easeInQuad': return c * c;
          case 'easeOutQuad': return 1 - (1 - c) * (1 - c);
          case 'easeInOutQuad': return c < 0.5 ? 2 * c * c : 1 - Math.pow(-2 * c + 2, 2) / 2;
          case 'easeInOutCubic': return c < 0.5 ? 4 * c * c * c : 1 - Math.pow(-2 * c + 2, 3) / 2;
          default: return c < 0.5 ? 2 * c * c : 1 - Math.pow(-2 * c + 2, 2) / 2;
        }
      },
      drawCurve() {
        const canvas = this.$refs.curveCanvas;
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        if (!ctx) return;
        const rect = canvas.getBoundingClientRect();
        const width = Math.max(10, Math.floor(rect.width || 200));
        const height = Math.max(10, Math.floor(rect.height || 96));
        const dpr = window.devicePixelRatio || 1;
        canvas.width = width * dpr;
        canvas.height = height * dpr;
        ctx.scale(dpr, dpr);
        ctx.clearRect(0, 0, width, height);
        const padL = 14, padR = 14, padT = 12, padB = 12;
        const gw = width - padL - padR, gh = height - padT - padB;
        const easing = this.cfg.ken_burns.easing;
        // 网格
        ctx.strokeStyle = 'rgba(255,255,255,0.05)';
        ctx.lineWidth = 1;
        for (let i = 0; i <= 4; i++) {
          const gx = padL + (gw / 4) * i, gy = padT + (gh / 4) * i;
          ctx.beginPath(); ctx.moveTo(gx, padT); ctx.lineTo(gx, padT + gh); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(padL, gy); ctx.lineTo(padL + gw, gy); ctx.stroke();
        }
        if (!this._curvePreviewing || this._curveProgress === undefined) this._curvePreviewing = false;
        const samples = 60;
        // 渐变面积
        const grad = ctx.createLinearGradient(0, padT, 0, padT + gh);
        grad.addColorStop(0, 'rgba(99,102,241,0.25)');
        grad.addColorStop(1, 'rgba(99,102,241,0.0)');
        ctx.fillStyle = grad;
        ctx.beginPath();
        ctx.moveTo(padL, padT + gh);
        for (let i = 0; i <= samples; i++) {
          const t = i / samples, v = this.evaluateEasing(t, easing);
          ctx.lineTo(padL + t * gw, padT + gh - v * gh);
        }
        ctx.lineTo(padL + gw, padT + gh);
        ctx.closePath();
        ctx.fill();
        // 曲线
        ctx.strokeStyle = '#818cf8';
        ctx.lineWidth = 2.5;
        ctx.beginPath();
        for (let i = 0; i <= samples; i++) {
          const t = i / samples, v = this.evaluateEasing(t, easing);
          const x = padL + t * gw, y = padT + gh - v * gh;
          if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        }
        ctx.stroke();
        // 端点
        ctx.fillStyle = '#6366f1'; ctx.strokeStyle = '#ffffff'; ctx.lineWidth = 1.5;
        ctx.beginPath(); ctx.arc(padL, padT + gh, 3.5, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
        ctx.beginPath(); ctx.arc(padL + gw, padT, 3.5, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
        // 播放扫点
        if (this._curvePreviewing && this._curveProgress !== undefined) {
          const v = this.evaluateEasing(this._curveProgress, easing);
          const x = padL + this._curveProgress * gw, y = padT + gh - v * gh;
          ctx.fillStyle = '#38bdf8';
          ctx.shadowColor = '#38bdf8'; ctx.shadowBlur = 8;
          ctx.beginPath(); ctx.arc(x, y, 4.5, 0, Math.PI * 2); ctx.fill();
          ctx.shadowBlur = 0;
        }
      },
      animateCurve() {
        if (this._curvePreviewing) return;
        this._curvePreviewing = true;
        this._curveProgress = 0;
        const start = performance.now(), dur = 1200;
        const step = (now) => {
          this._curveProgress = Math.min(1, (now - start) / dur);
          this.drawCurve();
          if (this._curveProgress < 1) {
            this._curveAnimId = requestAnimationFrame(step);
          } else {
            setTimeout(() => { this._curvePreviewing = false; this.drawCurve(); }, 200);
          }
        };
        this._curveAnimId = requestAnimationFrame(step);
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
  // 对齐 vidonex CommandPaletteModal 分组:Timeline Editing / Tracks / Aspect Ratios / Playback & Viewport / Project & Modals
  const COMMAND_GROUPS = [
    { id: 'editing', label: '时间轴编辑' },
    { id: 'tracks', label: '轨道管理' },
    { id: 'aspect', label: '画布比例' },
    { id: 'playback', label: '播放与视口' },
    { id: 'project', label: '项目与模态' },
    { id: 'about', label: '关于与更新' },
  ];
  const APP_COMMANDS = [
    // —— 时间轴编辑 ——
    { id: 'split', group: 'editing', name: '分割选中片段', desc: '在播放头处切开', keys: 'S', icon: 'M6 4v16l8-8zM14 12l4-8v16z', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.split(); } },
    { id: 'duplicate', group: 'editing', name: '复制选中片段', desc: '复制到当前轨道末尾', keys: 'Ctrl+D', icon: 'M4 4h12v12H4zM16 8h4v12H8v-4', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.duplicateClip(); } },
    { id: 'delete', group: 'editing', name: '删除选中片段', desc: '从时间轴移除', keys: 'Del', icon: 'M6 7h12l-1 14H7zM9 4h6l1 3H8z', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.deleteSelected(); } },
    { id: 'undo', group: 'editing', name: '撤销', desc: '回退上一步编辑', keys: 'Ctrl+Z', icon: 'M9 14L4 9l5-5M4 9h10a6 6 0 0 1 0 12h-3', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.undo(); } },
    { id: 'redo', group: 'editing', name: '重做', desc: '恢复撤销的编辑', keys: 'Ctrl+Shift+Z', icon: 'M15 14l5-5-5-5M20 9H10a6 6 0 0 0 0 12h3', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.redo(); } },
    { id: 'toggle-snap', group: 'editing', name: '磁吸开关', desc: '吸附对齐(开/关)', keys: 'N', icon: 'M13 2L3 14h7l-1 8 10-12h-7z', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.toggleSnap(); } },
    // —— 轨道管理(对齐上游 addTrack video/audio/waveform)——
    { id: 'add-video-track', group: 'tracks', name: '添加视频轨道', desc: '顶部新建覆盖视频轨', keys: '', icon: 'M8 5v14l11-7z', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.addTrack('video'); } },
    { id: 'add-audio-track', group: 'tracks', name: '添加音频轨道', desc: '新建背景音乐 / 配音轨', keys: '', icon: 'M3 10v4a1 1 0 0 0 1 1h3l4 4V5L7 9H4a1 1 0 0 0-1 1z', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.addTrack('audio'); } },
    { id: 'add-waveform-track', group: 'tracks', name: '添加波形轨道', desc: '动画波形可视化轨', keys: '', icon: 'M4 18v-6M9 18V6m5 12v-9m5 9V9', run: () => { const ed = window.vp.timelineEditorOf; if (ed) ed.addTrack('waveform'); } },
    // —— 画布比例(对齐上游 setAspectRatio 4 预设)——
    { id: 'aspect-16-9', group: 'aspect', name: '16:9 横屏', desc: '1920×1080 · YouTube/网页', keys: '', icon: 'M3 6h18v12H3z', run: () => applyAspect(1920, 1080) },
    { id: 'aspect-9-16', group: 'aspect', name: '9:16 竖屏', desc: '1080×1920 · 抖音/短视频', keys: '', icon: 'M8 3h8v18H8z', run: () => applyAspect(1080, 1920) },
    { id: 'aspect-1-1', group: 'aspect', name: '1:1 方形', desc: '1080×1080 · 社交卡片', keys: '', icon: 'M4 4h16v16H4z', run: () => applyAspect(1080, 1080) },
    { id: 'aspect-21-9', group: 'aspect', name: '21:9 超宽', desc: '2560×1080 · 影院宽银幕', keys: '', icon: 'M2 6h20v12H2z', run: () => applyAspect(2560, 1080) },
    // —— 播放与视口 ——
    { id: 'toggle-play', group: 'playback', name: '播放 / 暂停', desc: '切换传输状态', keys: 'Space', icon: 'M8 5v14l11-7z', run: () => { const pb = window.vp && window.vp.playback; if (pb) pb.togglePlay(); } },
    { id: 'jump-start', group: 'playback', name: '跳到开头', desc: '播放头归零', keys: 'Home', icon: 'M19 5v14l-9-7zM5 5h2v14H5z', run: () => { const pb = window.vp && window.vp.playback; if (pb) pb.jumpToStart(); } },
    { id: 'jump-end', group: 'playback', name: '跳到结尾', desc: '播放头到内容末端', keys: 'End', icon: 'M5 5v14l9-7zM19 5h2v14h-2z', run: () => { const pb = window.vp && window.vp.playback; if (pb) pb.jumpToEnd(); } },
    { id: 'fit-viewport', group: 'playback', name: '视口适配', desc: '重置画布缩放与平移', keys: 'Shift+Z', icon: 'M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5', run: () => { const pv = window.vp.projectViewOf; if (pv) pv.resetViewportZoom(); } },
    // —— 项目与模态 ——
    { id: 'render', group: 'project', name: '渲染导出…', desc: 'FFmpeg 编码 / GPU / 质量', keys: 'Ctrl+E', icon: 'M12 16V4m0 0l-4 4m4-4l4 4M4 16v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2', run: () => { const m = window.renderModal(); m.openFor(window.vp.currentProjectId, (window.vp.projectViewOf || {}).output || ''); } },
    { id: 'new-project', group: 'project', name: '新建项目…', desc: '创建新的视频项目', keys: '', icon: 'M12 5v14M5 12h14', run: () => window.dispatchEvent(new CustomEvent('new-project-open')) },
    { id: 'settings', group: 'project', name: '项目设置…', desc: '画布尺寸 / 背景色 / 帧率', keys: 'P', icon: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zm8 4a8 8 0 0 1-.1 1.4l2 1.5-2 3.5-2.4-.9a8 8 0 0 1-2.4 1.4L14.8 22h-4l-.3-2.6a8 8 0 0 1-2.4-1.4l-2.4.9-2-3.5 2-1.5A8 8 0 0 1 5.6 12a8 8 0 0 1 .1-1.4l-2-1.5 2-3.5 2.4.9a8 8 0 0 1 2.4-1.4L10.8 2h4l.3 2.6a8 8 0 0 1 2.4 1.4l2.4-.9 2 3.5-2 1.5a8 8 0 0 1 .1 1.4z', run: () => window.dispatchEvent(new CustomEvent('project-settings-open', { detail: { tl: (window.vp.timelineEditorOf || {}).tl } })) },
    { id: 'graph', group: 'project', name: '图可视化…', desc: 'FFmpeg 滤镜流程图', keys: '', icon: 'M4 7a3 3 0 1 0 0 6 3 3 0 0 0 0-6zm16 4a3 3 0 1 0 0 6 3 3 0 0 0 0-6zm-8-7a3 3 0 1 0 0 6 3 3 0 0 0 0-6zm0 10a3 3 0 1 0 0 6 3 3 0 0 0 0-6zM7 10l3-3m4 0l3 3M7 11v6m10-4v4', run: () => window.dispatchEvent(new CustomEvent('graph-open')) },
    { id: 'shortcuts', group: 'project', name: '键盘快捷键…', desc: '查看完整快捷键表', keys: 'Ctrl+/', icon: 'M12 8a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3zM7 8a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3zm10 0a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3z', run: () => {} },
    // —— 关于与更新 ——
    { id: 'about', group: 'about', name: '关于 vidproc…', desc: '版本 / 系统诊断 / 更新', keys: '', icon: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm1 15h-2v-6h2zm0-8h-2V7h2z', run: () => {} },
  ];
  // 画布比例:写回 tl + 落库(对齐上游 setAspectRatio → projectStore)
  function applyAspect(w, h) {
    const ed = window.vp.timelineEditorOf;
    if (!ed || !ed.tl) return;
    ed.saveSettings({ width: w, height: h, fps: ed.tl.fps || 30, background_color: ed.tl.background_color || 'black' });
  }
  const APP_SHORTCUTS = [
    { keys: 'Space', desc: '播放 / 暂停' },
    { keys: '← / →', desc: '上一帧 / 下一帧' },
    { keys: 'Home / End', desc: '跳到开头 / 结尾' },
    { keys: 'S', desc: '在播放头分割选中片段' },
    { keys: 'Delete / Backspace', desc: '删除选中片段' },
    { keys: 'Ctrl+D', desc: '复制选中片段' },
    { keys: 'Ctrl+Z / Ctrl+Shift+Z', desc: '撤销 / 重做' },
    { keys: 'N', desc: '磁吸开关' },
    { keys: 'P', desc: '项目设置 (画布/背景/帧率)' },
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
      commandGroups: COMMAND_GROUPS,
      shortcuts: APP_SHORTCUTS,
      shortcutQuery: '',
      palIdx: 0, // 命令面板键盘导航高亮
      version: '0.1.0',
      architecture: 'htmx + Alpine + Go / vidonex 兼容 spec',
      // 系统诊断(对齐 vidonex AboutModal:OS/Arch/Cores/GPU/FFmpeg)
      sys: {
        os: typeof navigator !== 'undefined' ? navigator.userAgent.replace(/\(([^)]*)\).*/, '$1') : '',
        arch: (navigator.userAgent.match(/(x86_64|arm64|aarch64)/i) || [''])[0] || 'x64',
        cores: (navigator.hardwareConcurrency || '—') + '',
        gpu: '',
        ffmpeg: '检测中…',
      },
      sysLoaded: false,
      init() {
        // 事件总线入口(与其它弹窗一致:Alpine 响应式代理要求从组件内改字段;
        // 外部 vp.openShortcuts/openPalette 广播事件,这里收)
        window.addEventListener('app-modals-open', (e) => {
          this.view = (e.detail && e.detail.view) || 'palette';
          this.palIdx = 0;
        });
        // 系统诊断延迟探测(GPU:WebGL renderer)
        setTimeout(() => {
          try {
            const c = document.createElement('canvas');
            const gl = c.getContext('webgl') || c.getContext('experimental-webgl');
            if (gl) {
              const dbg = gl.getExtension('WEBGL_debug_renderer_info');
              if (dbg) this.sys.gpu = gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) || '';
            }
          } catch (_) {}
          this.sysLoaded = true;
          // FFmpeg 探测(/api/sysinfo)
          fetch('/api/sysinfo').then((r) => (r.ok ? r.json() : null)).then((j) => {
            if (j && j.ffmpeg) this.sys.ffmpeg = j.ffmpeg;
          }).catch(() => { this.sys.ffmpeg = '不可用'; });
        }, 300);
        document.addEventListener('keydown', (e) => {
          // 输入框内不劫持
          const tag = (e.target.tagName || '').toLowerCase();
          const inField = ['input', 'textarea', 'select'].includes(tag);
          const mod = e.ctrlKey || e.metaKey;
          if (mod && e.key.toLowerCase() === 'k' && !inField) {
            e.preventDefault();
            this.view = this.view === 'palette' ? null : 'palette';
            this.query = '';
            this.palIdx = 0;
          } else if (mod && e.key === '/' && !inField) {
            e.preventDefault();
            this.view = this.view === 'shortcuts' ? null : 'shortcuts';
          } else if (e.key === 'Escape') {
            if (this.view) this.view = null;
          } else if (this.view === 'palette') {
            // 键盘导航:↑↓ 移动高亮,Enter 执行(对齐 UCommandPalette)
            if (e.key === 'ArrowDown') { e.preventDefault(); this.palIdx = Math.min(this.flatCommands().length - 1, this.palIdx + 1); }
            else if (e.key === 'ArrowUp') { e.preventDefault(); this.palIdx = Math.max(0, this.palIdx - 1); }
            else if (e.key === 'Enter') {
              e.preventDefault();
              const list = this.flatCommands();
              const c = list[this.palIdx] || list[0];
              if (c) this.run(c);
            }
          }
        });
      },
      open(view) { this.view = view; this.palIdx = 0; },
      // 分组命令过滤(对齐上游 commandGroups:仅显示有匹配的分组)
      get filteredGroups() {
        const q = this.query.trim().toLowerCase();
        const match = (c) => !q || c.name.toLowerCase().includes(q) || c.desc.toLowerCase().includes(q) || c.keys.toLowerCase().includes(q);
        return COMMAND_GROUPS
          .map((g) => ({ ...g, items: APP_COMMANDS.filter((c) => c.group === g.id && match(c)) }))
          .filter((g) => g.items.length > 0);
      },
      // 扁平命令列表(键盘导航用)
      flatCommands() {
        return this.filteredGroups.reduce((acc, g) => acc.concat(g.items), []);
      },
      // 命令在扁平列表中的索引(模板高亮判断)
      flatCmdIndex(id) {
        return this.flatCommands().findIndex((c) => c.id === id);
      },
      // 命令执行:自身类(shortcuts/about)切换视图,其它执行后关闭
      run(cmd) {
        if (cmd.id === 'shortcuts') { this.view = 'shortcuts'; return; }
        if (cmd.id === 'about') { this.view = 'about'; return; }
        try { cmd.run && cmd.run(); } catch (_) {}
        this.view = null;
      },
      // 快捷键表过滤(对齐上游搜索)
      get filteredShortcuts() {
        if (!this.shortcutQuery.trim()) return this.shortcuts;
        const q = this.shortcutQuery.toLowerCase();
        return this.shortcuts.filter((s) => s.desc.toLowerCase().includes(q) || s.keys.toLowerCase().includes(q));
      },
      // 快捷键分类(概览:播放/编辑/视图/其他)
      shortcutGroups() {
        const cats = [
          { label: '播放控制', re: /播放|帧|开头|结尾|暂停|关闭/ },
          { label: '时间轴编辑', re: /分割|复制|删除|撤销|重做|磁吸/ },
          { label: '视口与弹窗', re: /设置|命令|快捷键|列表/ },
        ];
        const rest = [];
        const out = cats.map((c) => ({ label: c.label, items: [] }));
        for (const s of this.filteredShortcuts) {
          const hit = cats.findIndex((c) => c.re.test(s.desc));
          const group = hit >= 0 ? out[hit] : null;
          if (group) group.items.push(s); else rest.push(s);
        }
        if (rest.length) out.push({ label: '其他', items: rest });
        return out.filter((g) => g.items.length > 0);
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
      l: 0, r: 0, // 0..1 电平,响应式驱动柱高(兼容旧引用)
      leftDb: -60, rightDb: -60, // dBFS 值(-60..0)
      leftPeakDb: -60, rightPeakDb: -60, // 峰值保持
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
            const rawL = 0.5 + 0.45 * Math.sin(t * 0.011 + 1.7) * Math.sin(t * 0.0043) + (Math.random() - 0.5) * 0.35;
            const rawR = 0.46 + 0.42 * Math.cos(t * 0.012 + 0.8) * Math.sin(t * 0.0051) + (Math.random() - 0.5) * 0.3;
            // dBFS 映射(-36..-2 为主区间,音量越大越响)
            const vol = (pb && pb.volume) || 1;
            this.leftDb = Math.max(-60, Math.min(0, -36 + rawL * 32 + (vol - 1) * 20));
            this.rightDb = Math.max(-60, Math.min(0, -36 + rawR * 32 + (vol - 1) * 20));
            this.l = this.dbToPercent(this.leftDb);
            this.r = this.dbToPercent(this.rightDb);
            // 峰值保持
            if (this.leftDb > this.leftPeakDb) this.leftPeakDb = this.leftDb;
            if (this.rightDb > this.rightPeakDb) this.rightPeakDb = this.rightDb;
          } else {
            // 平滑衰减(对齐上游 decay)
            this.leftDb = Math.max(-60, this.leftDb - 2.5);
            this.rightDb = Math.max(-60, this.rightDb - 2.5);
            this.leftPeakDb = Math.max(-60, this.leftPeakDb - 1.2);
            this.rightPeakDb = Math.max(-60, this.rightPeakDb - 1.2);
            this.l = this.dbToPercent(this.leftDb);
            this.r = this.dbToPercent(this.rightDb);
          }
          this._raf = requestAnimationFrame(this._loop);
        };
        this._raf = requestAnimationFrame(this._loop);
      },
      destroy() { cancelAnimationFrame(this._raf); },
      // dBFS(-60..0) → 0..1(上游 dbToPercent /100)
      dbToPercent(db) {
        if (db <= -60) return 0;
        if (db >= 0) return 1;
        return Math.min(1, Math.max(0, (db + 60) / 60));
      },
      // 峰值刻度位置(0..1)
      peakPos(db) { return this.dbToPercent(db); },
      // 三色渐变(上游 getMeterGradient)
      meterGradient(db) {
        if (db > -3) return 'linear-gradient(90deg, #10b981 0%, #f59e0b 70%, #ef4444 100%)';
        if (db > -12) return 'linear-gradient(90deg, #10b981 0%, #f59e0b 100%)';
        return 'linear-gradient(90deg, #059669 0%, #10b981 100%)';
      },
      // dB 数值文案
      displayDbText() {
        const peak = Math.max(this.leftDb, this.rightDb);
        if (peak <= -59) return '-∞ dB';
        return peak.toFixed(1) + ' dB';
      },
      // 数值颜色类(上游:>-3 红,>-12 黄,否则绿)
      dbColorClass() {
        const peak = Math.max(this.leftDb, this.rightDb);
        if (peak > -3) return 'text-red-400';
        if (peak > -12) return 'text-amber-400';
        return 'text-emerald-400';
      },
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
    { id: 'auto', name: '自动 (按项目规格)' },
    { id: 'none', name: 'CPU 编码 (libx264)' },
    { id: 'nvenc', name: 'NVIDIA NVENC 硬件加速' },
  ];
  const RENDER_PRESETS = [
    { id: '', name: '项目默认' },
    { id: 'tiktok', name: '抖音 9:16' },
    { id: 'instagram_reels', name: 'Instagram 短片' },
    { id: 'instagram_square', name: 'Instagram 方形' },
    { id: 'youtube_1080', name: 'YouTube 1080p' },
    { id: 'youtube_4k', name: 'YouTube 4K' },
    { id: 'web_fast', name: '网络快速 720p' },
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
      // 渲染状态(对齐 vidonex RenderExportModal:rendering/failed/done 三态)
      status: '', // '' | 'rendering' | 'failed' | 'done'
      percent: 0,
      frame: 0,
      speed: 0,
      error: '',
      errorOpen: false,
      copied: false,
      doneInfo: { url: '', sizeText: '', duration: '', elapsed: '' },
      init() {
        // Alpine 对 x-data 返回值做响应式代理,外部直接改单例字段不触发 x-show;
        // 统一走事件总线:openFor() 广播 render-modal-open,这里收。
        window.addEventListener('render-modal-open', (e) => {
          const d = e.detail || {};
          this.pid = d.pid || this.pid;
          if (d.output !== undefined) this.output = d.output;
          this.status = '';
          this.error = '';
          this.open = true;
        });
        // 渲染状态事件(projectView.poll 广播;对齐上游 store 三态)
        window.addEventListener('render-progress', (e) => {
          if (!this.open) return;
          const d = e.detail || {};
          this.status = 'rendering';
          this.percent = d.percent || 0;
          this.frame = d.frame || 0;
          this.speed = d.speed || 0;
        });
        window.addEventListener('render-fail', (e) => {
          if (!this.open) return;
          const d = e.detail || {};
          this.status = 'failed';
          this.error = d.error || '渲染失败';
          this.errorOpen = false;
        });
        window.addEventListener('render-done', (e) => {
          if (!this.open) return;
          const d = e.detail || {};
          this.status = 'done';
          this.percent = 100;
          const mb = Number(d.sizeMB) || 0;
          this.doneInfo = {
            url: d.url || '',
            sizeText: mb > 0 ? (mb >= 1024 ? (mb / 1024).toFixed(1) + ' GB' : mb.toFixed(1) + ' MB') : '',
            duration: d.duration || '',
            elapsed: d.elapsed !== undefined && d.elapsed !== null ? String(d.elapsed) : '',
          };
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
      // 复制错误日志(对齐上游 copyErrorText → clipboard)
      copyError() {
        if (!this.error) return;
        (navigator.clipboard ? navigator.clipboard.writeText(this.error) : Promise.reject())
          .then(() => { this.copied = true; setTimeout(() => { this.copied = false; }, 1500); })
          .catch(() => {});
      },
      // 错误摘要(首行,对齐上游 errorSummary)
      errorSummary() {
        const e = this.error || '';
        const first = e.split('\n')[0] || e;
        return first.length > 90 ? first.slice(0, 90) + '…' : first;
      },
      // 开始渲染:取全局 projectView 实例 → startRender(collect())。
      // 弹窗保持打开显示渲染中态;失败/完成由 render-progress/fail/done 事件驱动。
      start() {
        const host = window.vp && window.vp.projectViewOf;
        if (!host || host.busy) return;
        this.status = 'rendering';
        this.percent = 0;
        this.error = '';
        host.startRender(this.collect());
      },
      // 重开新渲染(完成/失败后;对齐上游 newRender)
      newRender() {
        this.status = '';
        this.error = '';
        this.doneInfo = { url: '', sizeText: '', duration: '' };
      },
    };
    return _renderModal;
  };

  // ============ 新建项目弹窗(newProjectModal)============
  // 对齐 vidonex NewProjectModal:名称 + 5 画布预设(16:9/4K/9:16/1:1/21:9)
  // + fps 选择。创建 = 前端生成 spec(带画布) → POST /api/projects。
  const ASPECT_PRESETS = [
    { id: 'youtube_16_9', name: '16:9 横屏 (1080p)', width: 1920, height: 1080, icon: 'M21 7H3a1 1 0 0 0-1 1v8a1 1 0 0 0 1 1h18a1 1 0 0 0 1-1V8a1 1 0 0 0-1-1zM7 10h2v4H7z' },
    { id: 'youtube_4k', name: '16:9 4K 超高清 (3840×2160)', width: 3840, height: 2160, icon: 'M12 2l8 3v6c0 5-3.4 9.4-8 11-4.6-1.6-8-6-8-11V5z' },
    { id: 'tiktok_9_16', name: '9:16 竖屏 (抖音/短视频)', width: 1080, height: 1920, icon: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm4.5 11.6l-5 3a1 1 0 0 1-1.5-.9V8.3a1 1 0 0 1 1.5-.9l5 3a1 1 0 0 1 0 1.7z' },
    { id: 'instagram_square', name: '1:1 方形 (Instagram)', width: 1080, height: 1080, icon: 'M3 5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zm14 7a5 5 0 1 1-10 0 5 5 0 0 1 10 0zM17 6h2v2h-2z' },
    { id: 'cinema_wide', name: '21:9 超宽影院', width: 2560, height: 1080, icon: 'M4 6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2zm3 4h10v4H7z' },
  ];
  const FPS_OPTIONS = [
    { label: '24 FPS (电影)', value: 24 },
    { label: '25 FPS (PAL 制式)', value: 25 },
    { label: '30 FPS (网络)', value: 30 },
    { label: '60 FPS (游戏)', value: 60 },
  ];
  // 单例:弹窗触发按钮(无 x-data 作用域)也需控制 open
  let _newProjectModal = null;
  window.newProjectModal = function () {
    if (_newProjectModal) return _newProjectModal;
    _newProjectModal = {
      open: false,
      name: '未命名项目',
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
      // —— AppHeader 对齐(上游:项目名内联编辑 + 保存指示 + 宽高比预设)——
      projectName: opts.name || '',
      renameDraft: opts.name || '',
      saving: false, // 保存中指示(saving spinner)
      saved: false,  // 最近保存成功(✓ 2s 后消失)
      aspectPresets: ASPECT_PRESETS,
      aspectMenuOpen: false,
      tl: null, // 响应式时间轴数据(wfLayers/stageDims 依赖;timeline load 后赋值)
      // —— 视口 gizmo(对齐 vidonex TransformGizmo/CropGizmo)——
      // 完全体初始化:clip-selected 广播选中 clip → 读引用直接渲染 + 编辑
      selClip: null,          // 选中 clip 引用(tl 里的对象,直接改字段)
      selClipTrackKind: null, // video/overlay/audio/...
      selClipThumb: '',       // 选中 clip 素材帧 URL(asset frame)
      assets: [],             // 项目素材(匹配 clip.source → nativeW/H + frame url)
      gizmoMode: 'transform', // 'transform' | 'crop'
      // —— 视口平移(Space/中键拖拽,对齐 vidonex useViewportZoom startPan)——
      isSpacePressed: false,
      isPanning: false,
      zoomMenu: { open: false }, // 缩放预设下拉(常驻对象)
      _pan: null,                // 进行中平移状态 {sx, sy, ox, oy}
      showGuide: { cx: false, cy: false, l: false, r: false, t: false, b: false },
      _gizmoDrag: null,       // 进行中的 gizmo 拖拽状态(startClipDrag/Resize/Rotate/Crop)
      gizmoParams: { canvasW: 1920, canvasH: 1080, dispW: 960, dispH: 540 },

      init() {
        // 供渲染导出弹窗等无作用域组件调用(window.vp.projectViewOf)
        window.vp.projectViewOf = this;
        // 视口平移:Space 按住(排除输入框;对齐 vidonex handleKeyDown/Up)
        window.addEventListener('keydown', (e) => {
          const t = e.target;
          if (t && (['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName) || t.isContentEditable)) return;
          if (e.code === 'Space' || e.key === ' ') { this.isSpacePressed = true; e.preventDefault(); }
        });
        window.addEventListener('keyup', (e) => {
          if (e.code === 'Space' || e.key === ' ') this.isSpacePressed = false;
        });
        // 响应式时间码(播放时 rAF 推进 → playhead-changed → 这里刷新)
        this.tc = '00:00:00:00';
        this.tcDur = '00:00:00:00';
        // gizmo:加载项目素材(匹配 clip.source → nativeW/H + frame url)
        this.loadAssets();
        // gizmo:监听时间轴选中(与检查器面板同事件;编辑走同一 tl 引用)
        window.addEventListener('clip-selected', (e) => {
          const d = e.detail || {};
          this.selClip = d.clip || null;
          this.selClipTrackKind = d.trackKind || null;
          this.selClipEditor = d.editor || null;
          if (d.tl) this.tl = d.tl;
          if (this.selClip) this.refreshSelClipThumb();
        });
        // 时间轴工具条"裁剪"按钮 → 切 gizmoMode(对齐 vidonex TimelineContainer Crop)
        window.addEventListener('gizmo-mode', (e) => {
          const m = (e.detail || {}).mode;
          if (m === 'crop' || m === 'transform') this.gizmoMode = m;
        });
        // 时间轴加载完成 → 绑定同一 tl 引用(编辑一致落库,不复制)
        window.addEventListener('timeline-loaded', (e) => {
          const d = e.detail || {};
          if (d.tl) { this.tl = d.tl; }
          this.fps = (d.tl && d.tl.fps) || this.fps;
          this.wfRedrawAll();
        });
        // 页面进入:先取一个默认预览帧(t=0)
        this.refreshPreview(0);
        // 播放头移动 → 刷新画布(防抖 120ms,连拖不连打)
        window.addEventListener('playhead-changed', (e) => {
          const ev = e.detail || {};
          if (ev.pid !== undefined && ev.pid !== pid) return;
          this.playhead = ev.t || 0;
          this.refreshPreview(this.playhead);
          this.wfRedrawAll();
          this.tc = window.vp.playback.formatTimecode(this.playhead, this.fps);
        });
        // 同步项目帧率到播放状态机
        window.vp.currentProjectId = pid;
        fetch(`/api/projects/${pid}/timeline`)
          .then((r) => (r.ok ? r.json() : null))
          .then((tl) => {
            if (!tl) return;
            this.tl = tl;
            this.fps = tl.fps || 30;
            window.vp.syncTimelineDurations(tl);
            this.tcDur = window.vp.playback.formatTimecode(window.vp.playback.contentDuration || 0, this.fps);
            this.wfRedrawAll();
          })
          .catch(() => {});
        // 加载后(含 htmx swap 重新初始化)确保画布有帧 —— 由 playhead-changed 驱动,无需额外
      },

      // ============ 播放工具条/视口缩放(对齐 vidonex PlaybackToolbar + useViewportZoom)============
      pb() { return window.vp.playback; },
      vz() { return window.vp.viewportZoom; },
      onWheel(e) {
        e.preventDefault();
        const factor = e.deltaY < 0 ? 1.15 : 0.87;
        this.vz().setZoom(this.vz().zoom * factor);
      },
      resetViewportZoom() { this.vz().reset(); },
      // 舞台按下:中键或 Space → 平移;空白处 → 取消选中(对齐 vidonex handleStageMouseDown)
      handleStageMouseDown(e) {
        if (this.isSpacePressed || e.button === 1) { this.startPan(e); return; }
        // 点击画布空白背景(非 clip/gizmo)→ 取消选中
        if (e.target === e.currentTarget || e.target.classList.contains('canvas-stage') || e.target === e.currentTarget.querySelector('.canvas-stage')) {
          this.selClip = null;
        }
      },
      startPan(e) {
        if (e.button !== 0 && e.button !== 1) return;
        e.preventDefault();
        this.isPanning = true;
        const vz = this.vz();
        this._pan = { sx: e.clientX, sy: e.clientY, ox: vz.panX, oy: vz.panY };
        const move = (ev) => {
          if (!this._pan) return;
          vz.panX = Math.round(this._pan.ox + (ev.clientX - this._pan.sx));
          vz.panY = Math.round(this._pan.oy + (ev.clientY - this._pan.sy));
        };
        const up = () => {
          this.isPanning = false;
          this._pan = null;
          window.removeEventListener('pointermove', move);
          window.removeEventListener('pointerup', up);
        };
        window.addEventListener('pointermove', move);
        window.addEventListener('pointerup', up, { once: true });
      },
      // 全屏切换(对齐 vidonex toggleFullscreen:视口面板 requestFullscreen)
      async toggleFullscreen() {
        try {
          if (!document.fullscreenElement) {
            const el = this.$refs.viewportPanel || document.documentElement;
            if (el.requestFullscreen) await el.requestFullscreen();
          } else if (document.exitFullscreen) {
            await document.exitFullscreen();
          }
        } catch (_) { /* 全屏被拒/环境不支持 */ }
      },
      // 缩放下拉当前项高亮(对齐 vidonex zoomMenuItems 勾选)
      zoomMenuItemClass(p) {
        const z = this.vz().zoom;
        if (p.isFit) return this.vz().isFit ? 'text-indigo-400' : 'text-gray-300';
        return Math.round(z * 100) === Math.round(p.value * 100) ? 'text-indigo-400' : 'text-gray-300';
      },
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

      // ============ 视口 gizmo(对齐 vidonex useTransformGizmo)============
      loadAssets() {
        fetch(`/api/projects/${pid}/assets`)
          .then((r) => (r.ok ? r.json() : []))
          .then((list) => { this.assets = list || []; })
          .catch(() => { this.assets = []; });
      },
      isVisualClip(kind) { return kind === 'video' || kind === 'overlay'; },
      // 选中 clip 的素材(匹配 clip.source = file_path / file_name / id)
      selAsset() {
        if (!this.selClip) return null;
        return this.assets.find((a) => a.file_path === this.selClip.source ||
          a.file_name === this.selClip.source || String(a.id) === String(this.selClip.source)) || null;
      },
      // 选中 clip 素材帧 URL(资产匹配不到 → 用项目预览帧兜底,仍可编辑位置/缩放)
      refreshSelClipThumb() {
        const a = this.selAsset();
        if (a) {
          const t = Math.max(0, (this.selClip.trim || 0));
          this.selClipThumb = `/api/assets/${a.id}/frame?t=${t}&w=960`;
        } else {
          this.selClipThumb = this.previewSrc || '';
        }
      },
      // 显示坐标系:画布框实际像素 = gizmoParams.dispW/H(由 screenStyle 100% 决定,
      // 读 .canvas-stage rect 更准 —— 兜底 canvas 宽高)
      stageDims() {
        const st = document.querySelector('.canvas-stage');
        if (st) {
          const r = st.getBoundingClientRect();
          if (r.width > 0 && r.height > 0) {
            this.gizmoParams.dispW = r.width;
            this.gizmoParams.dispH = r.height;
          }
        }
        const tl = this.tl;
        if (tl) {
          this.gizmoParams.canvasW = tl.width || 1920;
          this.gizmoParams.canvasH = tl.height || 1080;
        }
        return this.gizmoParams;
      },
      // calculateClipBounds(对齐上游):contain 拟合 → displayScale → position 偏移
      calculateClipBounds(clip, dims) {
        const cw = dims.canvasW, ch = dims.canvasH, dw = dims.dispW, dh = dims.dispH;
        const a = this.selAsset();
        let nativeW = a && a.width ? a.width : cw;
        let nativeH = a && a.height ? a.height : ch;
        const ratio = nativeW / nativeH;
        let baseW = cw, baseH = cw / ratio;
        if (baseH > ch) { baseH = ch; baseW = baseH * ratio; }
        const ds = cw > 0 ? dw / cw : 1;
        const scale = clip.scale || 1.0;
        const posX = (clip.position && clip.position.x || 0) * ds;
        const posY = (clip.position && clip.position.y || 0) * ds;
        const w = baseW * scale * ds, h = baseH * scale * ds;
        return {
          left: (dw - w) / 2 + posX,
          top: (dh - h) / 2 + posY,
          width: w, height: h,
          rotation: clip.rotation || 0,
          opacity: clip.opacity !== undefined ? clip.opacity : 1.0,
        };
      },
      // 选中 clip 画面 DOM style(含 crop 裁剪)
      clipRenderStyle() {
        if (!this.selClip) return 'display:none';
        const d = this.stageDims();
        const b = this.calculateClipBounds(this.selClip, d);
        const c = this.selClip.crop || null;
        const cp = c ? `inset(${c.top || 0}% ${c.right || 0}% ${c.bottom || 0}% ${c.left || 0}%)` : '';
        return `left:${b.left}px;top:${b.top}px;width:${b.width}px;height:${b.height}px;` +
          `transform:rotate(${b.rotation}deg);opacity:${b.opacity};` +
          (cp ? `clip-path:${cp};` : '');
      },
      // gizmo 边框 style(与画面同框 + rotate)
      gizmoFrameStyle() {
        if (!this.selClip) return 'display:none';
        const d = this.stageDims();
        const b = this.calculateClipBounds(this.selClip, d);
        return `left:${b.left}px;top:${b.top}px;width:${b.width}px;height:${b.height}px;` +
          `transform:rotate(${b.rotation}deg);transform-origin:center center;`;
      },

      // —— 拖动移动(对齐上游 startDrag:6 条吸附,阈值 6px)——
      startClipDrag(e) {
        if (!this.selClip || e.button !== 0) return;
        e.preventDefault(); e.stopPropagation();
        const d = this.stageDims();
        const b = this.calculateClipBounds(this.selClip, d);
        const ds = d.canvasW > 0 ? d.dispW / d.canvasW : 1;
        const scaleFactor = 1 / ds;
        const startX = e.clientX, startY = e.clientY;
        const initX = this.selClip.position && this.selClip.position.x || 0;
        const initY = this.selClip.position && this.selClip.position.y || 0;
        const halfW = b.width / 2, halfH = b.height / 2;
        const cw = d.canvasW, ch = d.canvasH;
        const thr = 6 * scaleFactor;
        const show = this.showGuide;
        const clearGuides = () => { show.cx = show.cy = show.l = show.r = show.t = show.b = false; };
        const onMove = (ev) => {
          if (!this.selClip) return;
          const dx = (ev.clientX - startX) * scaleFactor;
          const dy = (ev.clientY - startY) * scaleFactor;
          let nx = initX + dx, ny = initY + dy;
          if (Math.abs(nx) < thr) { nx = 0; show.cx = true; show.l = show.r = false; }
          else if (Math.abs(nx - halfW - (-cw / 2)) < thr) { nx = -cw / 2 + halfW; show.l = true; show.cx = show.r = false; }
          else if (Math.abs(nx + halfW - cw / 2) < thr) { nx = cw / 2 - halfW; show.r = true; show.cx = show.l = false; }
          else { show.cx = show.l = show.r = false; }
          if (Math.abs(ny) < thr) { ny = 0; show.cy = true; show.t = show.b = false; }
          else if (Math.abs(ny - halfH - (-ch / 2)) < thr) { ny = -ch / 2 + halfH; show.t = true; show.cy = show.b = false; }
          else if (Math.abs(ny + halfH - ch / 2) < thr) { ny = ch / 2 - halfH; show.b = true; show.cy = show.t = false; }
          else { show.cy = show.t = show.b = false; }
          this.selClip.position = { ...(this.selClip.position || {}), x: Math.round(nx), y: Math.round(ny), alignment: (this.selClip.position && this.selClip.position.alignment) || 'center' };
        };
        const onUp = () => {
          clearGuides();
          window.removeEventListener('pointermove', onMove);
          window.removeEventListener('pointerup', onUp);
          this.commitGizmo('Move Clip Position');
        };
        window.addEventListener('pointermove', onMove);
        window.addEventListener('pointerup', onUp, { once: true });
      },
      // —— 缩放(对齐上游 startResize:径向距离比,8 柄共用)——
      startResize(e, handle) {
        if (!this.selClip || e.button !== 0) return;
        e.preventDefault(); e.stopPropagation();
        const d = this.stageDims();
        const b = this.calculateClipBounds(this.selClip, d);
        const st = document.querySelector('.canvas-stage');
        const r = st ? st.getBoundingClientRect() : { left: 0, top: 0 };
        const cx = r.left + b.left + b.width / 2;
        const cy = r.top + b.top + b.height / 2;
        const initScale = this.selClip.scale || 1.0;
        const initDist = Math.hypot(e.clientX - cx, e.clientY - cy);
        const onMove = (ev) => {
          if (!this.selClip) return;
          const cur = Math.hypot(ev.clientX - cx, ev.clientY - cy);
          if (initDist > 0) {
            const ratio = cur / initDist;
            let ns = Math.max(0.05, Math.min(10.0, initScale * ratio));
            if (Math.abs(ns - 1.0) < 0.015) { ns = 1.0; this.showGuide.cx = this.showGuide.cy = true; }
            else { this.showGuide.cx = this.showGuide.cy = false; }
            this.selClip.scale = Math.round(ns * 100) / 100;
          }
        };
        const onUp = () => {
          this.showGuide.cx = this.showGuide.cy = false;
          window.removeEventListener('pointermove', onMove);
          window.removeEventListener('pointerup', onUp);
          this.commitGizmo('Resize Clip Scale');
        };
        window.addEventListener('pointermove', onMove);
        window.addEventListener('pointerup', onUp, { once: true });
      },
      // —— 旋转(对齐上游 startRotate:atan2 归一化 + 角度吸附)——
      startRotate(e) {
        if (!this.selClip || e.button !== 0) return;
        e.preventDefault(); e.stopPropagation();
        const d = this.stageDims();
        const b = this.calculateClipBounds(this.selClip, d);
        const st = document.querySelector('.canvas-stage');
        const r = st ? st.getBoundingClientRect() : { left: 0, top: 0 };
        const cx = r.left + b.left + b.width / 2;
        const cy = r.top + b.top + b.height / 2;
        const initAngle = Math.atan2(e.clientY - cy, e.clientX - cx) * (180 / Math.PI);
        const initRot = this.selClip.rotation || 0;
        const onMove = (ev) => {
          if (!this.selClip) return;
          const cur = Math.atan2(ev.clientY - cy, ev.clientX - cx) * (180 / Math.PI);
          let nr = (initRot + (cur - initAngle)) % 360;
          if (nr > 180) nr -= 360;
          if (nr < -180) nr += 360;
          const snaps = [0, 45, 90, 135, 180, -45, -90, -135, -180];
          for (const s of snaps) { if (Math.abs(nr - s) < 4) { nr = s; break; } }
          this.selClip.rotation = Math.round(nr);
        };
        const onUp = () => {
          window.removeEventListener('pointermove', onMove);
          window.removeEventListener('pointerup', onUp);
          this.commitGizmo('Rotate Clip');
        };
        window.addEventListener('pointermove', onMove);
        window.addEventListener('pointerup', onUp, { once: true });
      },
      // —— 裁剪(对齐上游 CropGizmo:角/边拖 → clip.crop 百分比)——
      startCrop(e, handle) {
        if (!this.selClip || e.button !== 0) return;
        e.preventDefault(); e.stopPropagation();
        const d = this.stageDims();
        const b = this.calculateClipBounds(this.selClip, d);
        const init = this.selClip.crop || { top: 0, bottom: 0, left: 0, right: 0 };
        const w = b.width, h = b.height;
        const toPct = (v, total) => Math.max(0, Math.min(100, (v / total) * 100));
        const onMove = (ev) => {
          if (!this.selClip) return;
          const dx = ev.clientX - e.clientX, dy = ev.clientY - e.clientY;
          const c = { ...init };
          if (handle === 'tl' || handle === 'l' || handle === 'bl') c.left = toPct(dx, w);
          if (handle === 'tr' || handle === 'r' || handle === 'br') c.right = toPct(-dx, w);
          if (handle === 'tl' || handle === 't' || handle === 'tr') c.top = toPct(dy, h);
          if (handle === 'bl' || handle === 'b' || handle === 'br') c.bottom = toPct(-dy, h);
          this.selClip.crop = { top: Math.round(c.top), bottom: Math.round(c.bottom), left: Math.round(c.left), right: Math.round(c.right) };
        };
        const onUp = () => {
          window.removeEventListener('pointermove', onMove);
          window.removeEventListener('pointerup', onUp);
          this.commitGizmo('Update Crop');
        };
        window.addEventListener('pointermove', onMove);
        window.addEventListener('pointerup', onUp, { once: true });
      },
      // gizmo 编辑落库(复用 timelineEditor.editClip 300ms debounce,保留选中)
      commitGizmo(label) {
        const ed = this.selClipEditor || window.vp.timelineEditorOf;
        if (ed && ed.editClip) {
          ed.editClip(() => { ed.selectedId = this.selClip ? this.selClip.id : null; });
        }
      },

      // ============ AppHeader(对齐 vidonex AppHeader)============
      // 项目名内联编辑(blur 保存 → PUT name)+ 保存指示 + 宽高比预设下拉。
      // 宽高比预设改 tl 画布 → timelineEditor.saveSettings 落库。
      startRename() { this.renameDraft = this.projectName; },
      commitRename() {
        const name = (this.renameDraft || '').trim();
        if (!name || name === this.projectName) { this.renameDraft = this.projectName; return; }
        const self = this;
        this.saving = true;
        this.saved = false;
        fetch(`/api/projects/${this.pid}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name }),
        })
          .then((r) => {
            if (!r.ok) throw new Error('保存失败 ' + r.status);
            self.projectName = name;
            self.renameDraft = name;
            self.saved = true;
            setTimeout(() => { self.saved = false; }, 2000);
          })
          .catch((e) => { console.error('rename failed', e); })
          .finally(() => { self.saving = false; });
      },
      // 当前宽高比预设 id(按画布尺寸匹配,无匹配 → null 显示 WxH)
      currentAspectId() {
        const tl = this.tl;
        if (!tl || !tl.width || !tl.height) return null;
        const hit = (this.aspectPresets || []).find((p) => p.width === tl.width && p.height === tl.height);
        return hit ? hit.id : null;
      },
      applyAspectPreset(presetId) {
        const p = (this.aspectPresets || []).find((x) => x.id === presetId);
        if (!p) return;
        const ed = window.vp.timelineEditorOf;
        if (ed && ed.saveSettings) ed.saveSettings({ width: p.width, height: p.height });
        this.aspectMenuOpen = false;
      },

      // ============ ViewportWaveformVisualizer(对齐上游:waveform 轨 clip 的
      // canvas 5 模式波形,播放头驱动动画;检查器 waveform 面板配置)============
      // 当前渲染的 waveform 层(所有 waveform 轨 clip,不限播放头重叠,
      // 保证可编辑;检查器写 clip.waveform 后视口即时反映)
      wfLayers() {
        const tl = this.tl;
        if (!tl || !tl.tracks) return [];
        const out = [];
        for (const tr of tl.tracks) {
          if (tr.kind !== 'waveform') continue;
          for (const c of tr.clips || []) out.push({ clip: c, track: tr });
        }
        return out;
      },
      // 波形容器 style(定位/尺寸/不透明度;zIndex 按轨道序)
      wfStyle(clip, track) {
        const d = this.stageDims();
        const ds = d.canvasW > 0 ? d.dispW / d.canvasW : 1;
        const cfg = clip.waveform || track.waveform || {};
        const w = (cfg.width || Math.round(d.canvasW * 0.75)) * ds;
        const h = (cfg.height || (cfg.mode === 'circular' ? Math.min(w, 400 * ds) : 180)) * ds;
        const px = ((clip.position && clip.position.x) || 0) * ds;
        const py = ((clip.position && clip.position.y) || 0) * ds;
        const left = (d.dispW - w) / 2 + px;
        const top = (d.dispH - h) / 2 + py;
        const idx = this.wfLayers().findIndex((l) => l.clip.id === clip.id);
        return `left:${Math.round(left)}px;top:${Math.round(top)}px;width:${Math.round(w)}px;height:${Math.round(h)}px;` +
          `opacity:${clip.opacity !== undefined ? clip.opacity : 1};z-index:${100 + idx};`;
      },
      // 初始化波形 canvas(存引用 → 立即绘制;HiDPI 2x)
      wfInit(el, clip, track) {
        if (!el) return;
        el.__wf = { clip, track };
        this.wfDraw(el);
      },
      // 波形拖动(pure DOM 层自身拖动,不占 TransformGizmo;写回 position)
      startDragWaveform(e, clip) {
        if (e.button !== 0) return;
        e.preventDefault(); e.stopPropagation();
        // 选中该 clip(检查器同步显示 waveform 面板)
        const ed = this.selClipEditor || window.vp.timelineEditorOf;
        if (ed && ed.select) ed.select(clip);
        const d = this.stageDims();
        const ds = d.canvasW > 0 ? d.dispW / d.canvasW : 1;
        const startX = e.clientX, startY = e.clientY;
        const ix = (clip.position && clip.position.x) || 0;
        const iy = (clip.position && clip.position.y) || 0;
        const onMove = (ev) => {
          if (!clip) return;
          let nx = Math.round(ix + (ev.clientX - startX) / ds);
          let ny = Math.round(iy + (ev.clientY - startY) / ds);
          // 居中吸附(对齐上游:阈值 8px)
          if (Math.abs(nx) < 8) { nx = 0; this.showGuide.cx = true; } else this.showGuide.cx = false;
          if (Math.abs(ny) < 8) { ny = 0; this.showGuide.cy = true; } else this.showGuide.cy = false;
          clip.position = { ...(clip.position || {}), x: nx, y: ny, alignment: (clip.position && clip.position.alignment) || 'center' };
        };
        const onUp = () => {
          this.showGuide.cx = this.showGuide.cy = false;
          window.removeEventListener('pointermove', onMove);
          window.removeEventListener('pointerup', onUp);
          if (ed && ed.editClip) ed.editClip(() => { ed.selectedId = clip.id; });
        };
        window.addEventListener('pointermove', onMove);
        window.addEventListener('pointerup', onUp, { once: true });
      },
      // 重绘所有波形 canvas(播放头移动 / 配置变化时)
      wfRedrawAll() {
        document.querySelectorAll('[data-wf-canvas]').forEach((el) => {
          if (el.__wf) this.wfDraw(el);
        });
      },
      // 绘制 5 种模式(对齐上游 drawWaveform;程序化伪音频,播放时随 playhead 动)
      wfDraw(el) {
        const d = el.__wf;
        if (!d || !el.getContext) return;
        const cfg = d.clip.waveform || d.track.waveform || {};
        const mode = (cfg.mode || 'peak_to_peak').toLowerCase();
        const pColor = cfg.color || '#00E6B4';
        const sColor = cfg.secondary_color || '#6366F1';
        const density = Number(cfg.bar_density) || 48;
        const roundness = Number(cfg.bar_roundness) || 6;
        const isGlow = cfg.glow !== undefined ? cfg.glow : true;
        const glowRadius = Number(cfg.glow_radius) || 8;
        const lineWidth = Number(cfg.line_width) || 3;
        const isFill = cfg.fill_area || false;
        const innerR = Number(cfg.inner_radius) || 0.45;
        // HiDPI:canvas 物理像素 = 容器 CSS 尺寸 × 2
        const rect = el.getBoundingClientRect();
        const cssW = Math.max(10, rect.width || 100);
        const cssH = Math.max(10, rect.height || 40);
        const W = Math.round(cssW * 2), H = Math.round(cssH * 2);
        if (el.width !== W) el.width = W;
        if (el.height !== H) el.height = H;
        const ctx = el.getContext('2d');
        if (!ctx) return;
        ctx.clearRect(0, 0, W, H);
        ctx.save();
        const halfH = H / 2;
        const t = this.playhead;
        const playing = window.vp.playback && window.vp.playback.isPlaying;
        const grad = ctx.createLinearGradient(0, 0, W, H);
        grad.addColorStop(0, pColor);
        grad.addColorStop(1, sColor);
        if (isGlow) { ctx.shadowColor = pColor; ctx.shadowBlur = glowRadius * 2; }
        if (mode === 'peak_to_peak' || mode === 'bars' || mode === 'p2p') {
          const n = Math.max(16, Math.min(128, density));
          const sp = W / n, bw = Math.max(3, sp * 0.65);
          const r = Math.min(bw / 2, roundness * 2);
          ctx.fillStyle = grad;
          for (let i = 0; i < n; i++) {
            const seed = i * 0.35 + t * 8;
            const harm = Math.abs(Math.sin(seed) * 0.5 + Math.cos(seed * 2.1) * 0.3 + Math.sin(seed * 0.7) * 0.2);
            const amp = playing ? Math.max(0.08, harm) : Math.max(0.06, Math.sin(i * 0.2 + t) * 0.3 + 0.4);
            const bh = amp * (H * 0.85);
            const x = i * sp + (sp - bw) / 2, y = halfH - bh / 2;
            ctx.beginPath();
            if (ctx.roundRect) ctx.roundRect(x, y, bw, Math.max(4, bh), r); else ctx.rect(x, y, bw, Math.max(4, bh));
            ctx.fill();
          }
        } else if (mode === 'spectrum') {
          const n = Math.max(16, Math.min(96, density));
          const sp = W / n, bw = Math.max(3, sp * 0.7);
          const r = Math.min(bw / 2, roundness * 2);
          ctx.fillStyle = grad;
          for (let i = 0; i < n; i++) {
            const seed = i * 0.4 + t * 9;
            const harm = Math.abs(Math.sin(seed) * 0.6 + Math.cos(seed * 1.8) * 0.4);
            const amp = playing ? Math.max(0.05, harm) : Math.max(0.05, Math.sin(i * 0.25) * 0.4 + 0.4);
            const bh = amp * (H * 0.8);
            const x = i * sp + (sp - bw) / 2, y = H - bh;
            ctx.beginPath();
            if (ctx.roundRect) ctx.roundRect(x, y, bw, Math.max(4, bh), [r, r, 0, 0]); else ctx.rect(x, y, bw, Math.max(4, bh));
            ctx.fill();
            ctx.fillStyle = '#FFFFFF';
            ctx.beginPath();
            ctx.arc(x + bw / 2, Math.max(2, y - 6), Math.max(1.5, bw / 4), 0, Math.PI * 2);
            ctx.fill();
            ctx.fillStyle = grad;
          }
        } else if (mode === 'circular') {
          const cx = W / 2, cy = halfH;
          const base = Math.min(cx, cy) * innerR;
          const n = Math.max(32, Math.min(96, density));
          ctx.strokeStyle = grad;
          ctx.lineWidth = Math.max(2, (W / n) * 0.5);
          ctx.lineCap = 'round';
          ctx.beginPath();
          ctx.arc(cx, cy, base, 0, Math.PI * 2);
          ctx.strokeStyle = pColor + '40';
          ctx.lineWidth = 2;
          ctx.stroke();
          ctx.strokeStyle = grad;
          ctx.lineWidth = Math.max(2, (W / n) * 0.5);
          for (let i = 0; i < n; i++) {
            const ang = (i / n) * Math.PI * 2;
            const seed = i * 0.45 + t * 7;
            const amp = Math.abs(Math.sin(seed) * 0.65 + Math.cos(seed * 2.2) * 0.35);
            const len = playing ? amp * (base * 0.9) + 4 : 8;
            ctx.beginPath();
            ctx.moveTo(cx + Math.cos(ang) * base, cy + Math.sin(ang) * base);
            ctx.lineTo(cx + Math.cos(ang) * (base + len), cy + Math.sin(ang) * (base + len));
            ctx.stroke();
          }
        } else if (mode === 'dots') {
          const cols = Math.max(20, Math.min(80, density));
          const rows = 8;
          const sx = W / cols, sy = H / rows;
          for (let col = 0; col < cols; col++) {
            const seed = col * 0.35 + t * 8;
            const colAmp = playing ? Math.abs(Math.sin(seed) * 0.6 + Math.cos(seed * 1.7) * 0.4) : Math.abs(Math.sin(col * 0.2) * 0.5 + 0.3);
            const active = Math.round(colAmp * rows);
            for (let row = 0; row < rows; row++) {
              const fromBottom = rows - 1 - row;
              const on = fromBottom <= active;
              const x = col * sx + sx / 2, y = row * sy + sy / 2;
              const dr = on ? Math.max(2, sx * 0.35) : Math.max(1, sx * 0.15);
              ctx.fillStyle = on ? grad : pColor + '20';
              ctx.beginPath();
              ctx.arc(x, y, dr, 0, Math.PI * 2);
              ctx.fill();
            }
          }
        } else {
          // wave 模式:平滑曲线 + 可选填充
          ctx.strokeStyle = grad;
          ctx.lineWidth = lineWidth * 2;
          ctx.lineJoin = 'round';
          ctx.lineCap = 'round';
          const pts = 120, step = W / pts;
          const wave = [];
          for (let i = 0; i <= pts; i++) {
            const x = i * step;
            const seed = i * 0.18 + t * 9;
            const amp = playing ? Math.sin(seed) * 0.6 + Math.cos(seed * 2.3) * 0.3 : Math.sin(i * 0.15 + t * 2) * 0.35;
            wave.push({ x, y: halfH + amp * (halfH * 0.75) });
          }
          ctx.beginPath();
          for (let i = 0; i < wave.length; i++) {
            const pt = wave[i];
            if (i === 0) ctx.moveTo(pt.x, pt.y);
            else {
              const prev = wave[i - 1];
              ctx.quadraticCurveTo(prev.x, prev.y, (prev.x + pt.x) / 2, (prev.y + pt.y) / 2);
            }
          }
          ctx.stroke();
          if (isFill) {
            const fg = ctx.createLinearGradient(0, halfH, 0, H);
            fg.addColorStop(0, pColor + '40');
            fg.addColorStop(1, 'transparent');
            ctx.fillStyle = fg;
            ctx.lineTo(W, H);
            ctx.lineTo(0, H);
            ctx.closePath();
            ctx.fill();
          }
        }
        ctx.restore();
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
              if (r.status === 404 || r.status === 204) {
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
        this._renderJobId = id;
        clearInterval(pollTimer);
        pollTimer = setInterval(() => {
          fetch(`/api/render/${id}`)
            .then((r) => r.json())
            .then((j) => {
              this.percent = j.percent || 0;
              this.frame = j.frame || 0;
              this.speed = j.speed || 0;
              // 进度广播(渲染弹窗显示渲染中态)
              window.dispatchEvent(new CustomEvent('render-progress', {
                detail: { percent: j.percent || 0, frame: j.frame || 0, speed: j.speed || 0 },
              }));
              if (j.status === 'failed') {
                clearInterval(pollTimer);
                this.busy = false;
                this.failed = true;
                this.result = { error: j.error };
                window.dispatchEvent(new CustomEvent('render-fail', { detail: { error: j.error } }));
              } else if (j.status === 'done') {
                clearInterval(pollTimer);
                this.busy = false;
                this.done = true;
                this.percent = 100;
                this.result = j;
                window.dispatchEvent(new CustomEvent('render-done', {
                  detail: { output: j.output, sizeMB: j.size_mb, duration: j.duration, elapsed: j.elapsed || '', url: `/api/render/${id}/file` },
                }));
              }
            })
            .catch(() => {}); // 瞬时失败忽略,下一拍再试
        }, 700);
      },
    };
  };

  // ============ 图可视化(graphModal)============
  // 对齐 vidonex GraphVisualizerModal:GET spec/graph → mermaid 文本 →
  // 简易 DAG SVG 渲染(轻量:mermaid 只做源码展示,不引库)。
  // 挂 project_detail 视口顶栏"图"按钮;fetch /api/projects/{id}/spec/graph。
  window.graphModal = function (pid) {
    return {
      pid: pid || null,
      open: false,
      loading: false,
      error: '',
      code: '',
      svgHtml: '',
      isCopied: false,

      init() {
        // 视口顶栏"图"按钮广播 graph-open → 这里打开(事件总线,与其它弹窗一致)
        window.addEventListener('graph-open', () => {
          this.pid = pid || this.pid || (window.vp && window.vp.currentProjectId) || null;
          this.open = true;
          this.reload();
        });
      },
      openGraph() {
        this.open = true;
        this.reload();
      },
      close() { this.open = false; },
      reload() {
        if (!this.pid) { this.error = '无项目'; return; }
        this.loading = true;
        this.error = '';
        this.code = '';
        this.svgHtml = '';
        fetch(`/api/projects/${this.pid}/spec/graph?format=mermaid`)
          .then((r) => {
            if (!r.ok) { throw new Error('请求失败 ' + r.status); }
            return r.text();
          })
          .then((text) => {
            this.code = text || '';
            this.svgHtml = this.renderMermaid(text);
          })
          .catch((e) => { this.error = e.message; })
          .finally(() => { this.loading = false; });
      },
      copyCode() {
        const self = this;
        if (!this.code) return;
        (navigator.clipboard ? navigator.clipboard.writeText(this.code) : Promise.reject())
          .then(() => { this.isCopied = true; setTimeout(() => { self.isCopied = false; }, 1500); })
          .catch(() => {});
      },

      // 轻量 mermaid `graph LR` 解析 + SVG 渲染:节点声明与边声明分层,
      // 从左到右排布(同深度同列),标色 indigo,边带 pad 标签。
      renderMermaid(text) {
        const lines = (text || '').split('\n').map((l) => l.trim()).filter((l) => l);
        if (!lines.length) return '';
        const nodes = new Map(); // id → { id, label, filter }
        const edges = [];
        for (const line of lines) {
          if (!line || line.includes('graph ')) continue;
          // 边:A -->|pad| B
          const m = line.match(/^([A-Za-z0-9_]+)\s+-->\|([^|]*)\|\s+([A-Za-z0-9_]+)$/);
          if (m) { edges.push({ from: m[1], to: m[3], pad: m[2] }); continue; }
          // 节点:id["label<br/><i>filter</i>"]
          const n = line.match(/^([A-Za-z0-9_]+)\s*\["(.*)"\]\s*$/);
          if (n) {
            const parts = n[2].split('<br/>');
            const label = parts[0] || n[1];
            const filter = parts[1] ? parts[1].replace(/<\/?i>/g, '') : '';
            nodes.set(n[1], { id: n[1], label, filter });
          }
        }
        if (!nodes.size) return '';
        // BFS 分层(源在最左)
        const indeg = {};
        for (const e of edges) { indeg[e.to] = (indeg[e.to] || 0) + 1; }
        const roots = [...nodes.keys()].filter((id) => !indeg[id]);
        const layer = {};
        const queue = roots.length ? roots : [...nodes.keys()];
        for (const id of queue) {
          if (layer[id] === undefined) layer[id] = 0;
        }
        for (let qi = 0; qi < queue.length; qi++) {
          const id = queue[qi];
          const cur = layer[id] || 0;
          for (const e of edges) {
            if (e.from === id && layer[e.to] === undefined) {
              layer[e.to] = cur + 1;
              queue.push(e.to);
            }
          }
        }
        for (const id of nodes.keys()) {
          if (layer[id] === undefined) layer[id] = 0;
        }
        const byLayer = {};
        for (const [id, lv] of Object.entries(layer)) {
          (byLayer[lv] = byLayer[lv] || []).push(id);
        }
        const maxLayer = Math.max(0, ...Object.keys(byLayer).map(Number));
        // 列宽/行高
        const NODE_W = 170, NODE_H = 52, GX = 70, GY = 46;
        const positions = {};
        const rowsPerLayer = {};
        const colsW = [];
        for (let lv = 0; lv <= maxLayer; lv++) {
          const ids = byLayer[lv] || [];
          colsW[lv] = NODE_W;
          ids.forEach((id, i) => { positions[id] = { x: lv * (NODE_W + GX), y: i * (NODE_H + GY) }; });
          rowsPerLayer[lv] = ids.length;
        }
        const totalW = (maxLayer + 1) * (NODE_W + GX) - GX + 40;
        let maxH = 120;
        for (let lv = 0; lv <= maxLayer; lv++) {
          maxH = Math.max(maxH, (rowsPerLayer[lv] || 0) * (NODE_H + GY) - GY + 40);
        }
        // 构建 SVG
        const esc = (s) => String(s == null ? '' : s)
          .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
          .replace(/"/g, '&quot;');
        let svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${totalW}" height="${maxH}" viewBox="0 0 ${totalW} ${maxH}" font-family="ui-monospace, SFMono-Regular, monospace">`;
        svg += `<defs><marker id="graphArrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" fill="#6366f1"/></marker></defs>`;
        // 边(先画,节点盖其上)
        for (const e of edges) {
          const a = positions[e.from], b = positions[e.to];
          if (!a || !b) continue;
          const x1 = a.x + NODE_W, y1 = a.y + NODE_H / 2;
          const x2 = b.x, y2 = b.y + NODE_H / 2;
          const mx = (x1 + x2) / 2;
          svg += `<path d="M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}" fill="none" stroke="#6366f1" stroke-width="1.5" marker-end="url(#graphArrow)" opacity="0.7"/>`;
          if (e.pad && e.pad !== 'out') {
            svg += `<text x="${mx}" y="${(y1 + y2) / 2 - 4}" text-anchor="middle" fill="#9ca3af" font-size="10">${esc(e.pad)}</text>`;
          }
        }
        // 节点(圆角矩形 + label + filter)
        for (const [id, pos] of Object.entries(positions)) {
          const nd = nodes.get(id) || { label: id, filter: '' };
          const x = pos.x + 10, y = pos.y + 6;
          svg += `<g><rect x="${x}" y="${y}" width="${NODE_W - 20}" height="${NODE_H - 12}" rx="8" fill="#1f2937" stroke="#6366f1" stroke-width="1.2"/>`;
          svg += `<text x="${x + (NODE_W - 20) / 2}" y="${y + 18}" text-anchor="middle" fill="#e5e7eb" font-size="11" font-weight="600">${esc(nd.label)}</text>`;
          if (nd.filter) {
            svg += `<text x="${x + (NODE_W - 20) / 2}" y="${y + 34}" text-anchor="middle" fill="#818cf8" font-size="9" font-style="italic" font-family="ui-monospace, monospace">${esc(nd.filter)}</text>`;
          }
          svg += `</g>`;
        }
        svg += '</svg>';
        return svg;
      },
    };
  };

  // ============ 底部状态栏(appFooter)============
  // 对齐 vidonex AppFooter:左=时间码/时长/FPS,右=吸附开关 + 缩放滑条(min15 max200 step5)。
  // 数据源全部来自全局 window.vp(playback 状态机 + timelineEditorOf),无 props 注入;
  // playhead-changed / timeline-loaded 事件驱动刷新。
  window.appFooter = function () {
    return {
      timecode: '00:00:00',
      duration: '00:00:00',
      fps: 30,
      pps: 80, // 缩放滑条值(px/s),= timelineEditor.pps
      snapOn: true,

      init() {
        // 对齐 vidonex AppFooter:左=时间码/时长/FPS,右=吸附开关 + 缩放滑条(min15 max200 step5)。
        // 数据源全部来自全局 window.vp(playback 状态机 + timelineEditorOf),无 props 注入;
        // playhead-changed / timeline-loaded 事件驱动刷新。
        window.addEventListener('playhead-changed', () => this.refresh());
        window.addEventListener('timeline-loaded', () => this.refresh());
        this.refresh();
      },
      refresh() {
        const pb = window.vp && window.vp.playback;
        const ed = window.vp && window.vp.timelineEditorOf;
        const fps = (ed && ed.tl && ed.tl.fps) || (window.vp && window.vp.projectViewOf && window.vp.projectViewOf.fps) || 30;
        this.fps = fps;
        // 用 ed.zoom 计算 pps(watch 异步,zoom×80 即时权威;对齐上游 pixelsPerSecond=pps 语义)
        this.pps = ed ? Math.round(80 * ed.zoom) : 80;
        // 时间码读时间轴播放头(scrub/seek/播放都会同步到 ed.playhead),
        // 而非 vp.playback.currentTime——后者是播放状态机,scrub 不更新它。
        this.timecode = pb ? pb.formatTimecode((ed && ed.playhead) || (pb.currentTime || 0), fps) : '00:00:00';
        const dur = (pb && pb.contentDuration) || 0;
        this.duration = pb ? pb.formatTimecode(dur, fps) : '00:00:00';
        const snap = document.querySelector('[data-snap]');
        this.snapOn = !snap || snap.dataset.snap === 'on';
      },
      toggleSnap() {
        const ed = window.vp && window.vp.timelineEditorOf;
        if (ed && ed.toggleSnap) ed.toggleSnap();
        else {
          const snap = document.querySelector('[data-snap]');
          if (snap) snap.dataset.snap = snap.dataset.snap === 'on' ? 'off' : 'on';
        }
        this.refresh();
      },
      zoomIn() {
        const ed = window.vp && window.vp.timelineEditorOf;
        if (ed) ed.zoom = Math.min(200 / 80, ed.zoom + 15 / 80); // +15px/s
        this.refresh();
      },
      zoomOut() {
        const ed = window.vp && window.vp.timelineEditorOf;
        if (ed) ed.zoom = Math.max(15 / 80, ed.zoom - 15 / 80); // -15px/s
        this.refresh();
      },
      setZoom(value) {
        const ed = window.vp && window.vp.timelineEditorOf;
        if (ed && value) ed.zoom = Math.min(200 / 80, Math.max(15 / 80, value / 80));
        this.refresh();
      },
    };
  };
})();
