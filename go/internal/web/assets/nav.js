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

  // ============ 播放头总线:时间轴 → 视口预览 广播 ============
  // 事件名 playhead-changed;detail = {t: 秒, pid: 项目ID}
  // 视口 <img>/标尺/状态栏都监听它,避免组件间直接引用。

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
          const res = await fetch(`/api/projects/${this.pid}/timeline`);
          if (!res.ok) throw new Error(await res.text());
          this.tl = await res.json();
          this.pps = 80 * this.zoom;
          // 播放头默认停在第一个 clip 起点
          const first = this.firstClipStart();
          this.playhead = first;
          this.updateTrackWidth();
        } catch (e) {
          this.error = '时间轴加载失败: ' + e.message;
        } finally {
          this.loading = false;
        }
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
        this.playhead = Math.max(0, x / this.pps);
        this.dispatchPlayhead();
      },
      dispatchPlayhead() {
        window.dispatchEvent(new CustomEvent('playhead-changed', { detail: { t: this.playhead, pid: this.pid } }));
      },

      // ============ 拖拽移动 clip ============
      startDrag(e, clip, trackIdx) {
        if (e.button !== 0) return;
        e.preventDefault();
        e.stopPropagation();
        this.selectedId = clip.id;
        this.drag = {
          clip, trackIdx,
          startX: e.clientX,
          origStart: clip.start,
          moved: false,
        };
        // 注意:addEventListener 回调里 this 不绑定组件,用箭头函数包一层
        this._dragMove = (ev) => this.onDragMove(ev);
        this._dragEnd = () => this.onDragEnd();
        window.addEventListener('pointermove', this._dragMove);
        window.addEventListener('pointerup', this._dragEnd, { once: true });
        e.target.setPointerCapture && e.target.setPointerCapture(e.pointerId);
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
          this.playhead = this.drag.clip.start;
          this.dispatchPlayhead();
          this.onClipMoved(this.drag.clip); // 子类/详情页可监听持久化
        }
        this.drag = null;
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
      persistTimeline(cb) {
        if (!this.tl || !this.pid) { cb && cb(); return; }
        const spec = this.buildSpecYAML();
        fetch(`/api/projects/${this.pid}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ spec_yaml: spec }),
        })
          .then((r) => (r.ok ? cb && cb() : cb && cb(new Error('保存失败 ' + r.status))))
          .catch((e) => { cb && cb(e); });
      },

      // 内存 timeline → spec YAML(最小子集:画布/fps/轨道/clips)。
      // 从原始 spec 保留 unknown 字段不现实,这里重建可编辑子集,
      // 对齐 vidonex 编辑保存(所有编辑都重新序列化)。
      buildSpecYAML() {
        const tl = this.tl;
        const lines = [];
        lines.push('version: "1.0"');
        const canvas = `canvas: {width: ${tl.width}, height: ${tl.height}, background_color: "black"}`;
        lines.push(canvas);
        lines.push(`fps: "${tl.fps}"`);
        lines.push('tracks:');
        for (const tr of this.tracks()) {
          lines.push(`  - id: "${tr.id}"`);
          lines.push(`    kind: "${tr.kind}"`);
          lines.push('    clips:');
          for (const c of tr.clips) {
            lines.push(`      - id: "${c.id}"`);
            lines.push(`        source: "${c.source}"`);
            lines.push(`        start: "${c.start}s"`);
            lines.push(`        duration: "${c.duration}s"`);
            if (c.trim > 0) lines.push(`        trim: "${c.trim}s"`);
            if (c.speed && c.speed !== 1) lines.push(`        speed: ${c.speed}`);
            if (c.has_audio === false) lines.push(`        has_audio: false`);
          }
        }
        return lines.join('\n');
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

      init() {
        // 页面进入:先取一个默认预览帧(t=0)
        this.refreshPreview(0);
        // 播放头移动 → 刷新画布(防抖 120ms,连拖不连打)
        window.addEventListener('playhead-changed', (e) => {
          const ev = e.detail || {};
          if (ev.pid !== undefined && ev.pid !== pid) return;
          this.playhead = ev.t || 0;
          this.refreshPreview(this.playhead);
        });
      },

      fmt(sec) {
        if (!isFinite(sec) || sec < 0) return '00:00:00.000';
        const h = Math.floor(sec / 3600);
        const m = Math.floor((sec % 3600) / 60);
        const s = sec % 60;
        const pad = (n, w) => String(n).padStart(w, '0');
        return `${pad(h, 2)}:${pad(m, 2)}:${s.toFixed(3).padStart(6, '0')}`;
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

      startRender() {
        if (this.busy) return;
        this.busy = true;
        this.percent = 0;
        this.frame = 0;
        this.speed = 0;
        this.done = false;
        this.failed = false;
        this.result = {};

        fetch(`/api/projects/${pid}/render`, { method: 'POST' })
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