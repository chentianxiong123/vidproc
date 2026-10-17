# vidproc 痛点总结

> 来源：2026-01 批量视频处理任务（11 段游戏录屏 → HEVC 合并）

## 任务背景

将 `C:\Users\a1\Desktop\123\` 下 11 段游戏录屏（6.16 GB, ~2h04m）合并为一个 HEVC 文件，每段带文件名水印（左上角，淡入淡出），GPU 编码。

最终选择 **直接调用 ffmpeg** 而非 vidproc 完成，耗时约 12 分钟。

---

## 痛点

### 1. 多 Pass 架构

Pipeline 每个 op 独立调用一次 ffmpeg，中间产物写临时文件：

```
input → [scale] → tmp1.mp4 → [drawtext] → tmp2.mp4 → [encode] → output.mp4
```

问题：
- **慢**：每次 ffmpeg 启动+读取+编码+关闭，11 段 × 3 pass = 33 次 ffmpeg 调用
- **质量劣化**：每个中间文件都有编码损失（即使 tmp 用无损编码，I/O 开销也大）
- **本次任务实际**：一条 ffmpeg 命令就搞定 `scale + drawtext + hevc_nvenc`，无需 3 次

### 2. 无 Batch 处理

vidproc 只能处理单个视频文件。11 个视频需要手动循环 11 次，或者外部写脚本调用 vidproc 11 次。

对比：ffmpeg 本身也需手动循环，但至少 filter 链可以复用。vidproc 没有任何"批量模式"概念。

### 3. 无项目文件格式

没有 `.vidproj` 或类似的配置保存机制。复杂工作流（比如 11 段 + 不同水印 + 淡入淡出 + 编码参数）无法保存复用。

每次都是从零开始的命令行，或者重写 YAML workflow。

### 4. 跨平台 Filter 转义脆弱

**本次任务最大痛点**：drawtext 的 `fontfile` 路径在 Windows 上包含冒号 (`C:\` 或 `C:/`)，ffmpeg filter 语法把冒号当选项分隔符，导致：

- 单引号包裹路径 → ffmpeg 解析失败（`'C` 被当作值的一部分）
- 反斜杠转义 `\:` → ffmpeg 不支持
- `-filter_complex` 同样问题
- 用 `/tmp/` 相对路径 → Git Bash 路径映射 ffmpeg 不认

最终解决方案：`cd` 到字体所在目录，用纯相对路径 `fontfile=arial.ttf`。

**Fontconfig 错误**：`Fontconfig error: Cannot load default config file` 反复出现。虽然在 Linux/MSYS2 下是 warning，但在 Windows native 模式下可能导致 filter 初始化失败。

这完全是用户不该关心的底层细节。NLE 应该内部处理路径/字体/转义。

### 5. 无单一 filter_complex 渲染

vidproc 的设计是"原子操作组合"，但 ffmpeg 最强大的能力是 **一个 ffmpeg 调用内组合多个 filter**：

```bash
ffmpeg -i input.mp4 \
  -vf "scale=-2:864,drawtext=...,fade=..." \
  -c:v hevc_nvenc ...
```

vidproc 做不到这一点。每个 op 独立，无法在 filter_complex 中共享参数（比如 alpha fade 需要知道 duration，但 drawtext op 无法访问 scale op 的输出参数）。

### 6. nvenc 编码选项受限

vidproc 的 `nvenc.go` 硬编码了 `h264_nvenc`，不支持：
- `hevc_nvenc`（本次任务用的）
- `av1_nvenc`
- CRF / VBR / CQP 等模式切换
- `-preset`、`-tune` 等选项

虽然 `helpers.go` 有 `drawtextOpts`，但编码选项几乎没有可配置性。

### 7. drawtext 无 Alpha Fade

vidproc 的 `text.go` / `title.go` 只有 `-start`/`-end` 硬切换，没有透明度渐变。

本次任务需要每段首尾 2 秒淡入淡出，用 ffmpeg 的 `alpha='min(min(t/2,(D-t)/2),1)'` 表达式实现。vidproc 的 `helpers.go` 虽然加了 `Alpha` 字段，但：
- 是临时加上去的
- 表达式需要用户手写（容易出错）
- 没有基于 duration 的自动计算

### 8. 无时间轴感知 Overlay

无法在单个片段内按时间段显示不同文字（比如 0-5s 显示标题，5-30s 显示副标题）。

NLE 应该有"时间轴"概念：每个片段可以叠加多个文字层，每个层有自己的时间范围。

### 9. Pipeline 和 YAML Workflow 重复

`Workflow.ToPipeline()` 的实现只是把 steps 用 `|` 连接：

```go
func (w *Workflow) ToPipeline() *Pipeline {
    return &Pipeline{Steps: w.Steps}
}
```

两者走同一个 `ExecutePipeline()`，YAML 只是语法糖，没有额外能力。

### 10. 无时间戳支持

没有显示录制时间戳的内置 op。游戏录屏通常需要显示"录制时间"或"当前时间"，需要手动用 drawtext 实现。

---

## 设计方向：轻量 NLE

### 核心理念

vidproc 从"原子操作工具"进化为"轻量非线性编辑器"：

```
当前：input.mp4 → [scale] → [drawtext] → [encode] → output.mp4  （3 次 ffmpeg）
目标：project.vidproj → render → output.mp4                      （1 次 ffmpeg）
```

### 项目文件格式 (`.vidproj`)

```json
{
  "version": 1,
  "output": {
    "file": "output.mp4",
    "codec": "hevc_nvenc",
    "crf": 30,
    "width": 1920,
    "height": 864,
    "fps": 30
  },
  "segments": [
    {
      "source": "input1.mp4",
      "trim": {"in": 0, "out": 174.5},
      "overlays": [
        {
          "type": "text",
          "text": "Segment 1",
          "position": [20, 20],
          "font": "arial",
          "size": 32,
          "color": "white",
          "border": 3,
          "fade_in": 2,
          "fade_out": 2
        }
      ]
    }
  ]
}
```

### 渲染流程

```
1. 读取 project.vidproj
2. 生成 ffmpeg filter_complex：
   - 每个 segment 一个 [input{i}] 分支
   - scale + drawtext + fade 在同一 filter 链中
   - concat filter 合并所有分支
3. 单次 ffmpeg 调用完成全部处理
```

### 保留原子 Ops

当前 13 个原子操作（scale, crop, text, title, concat 等）继续作为独立工具可用：

```
# 单文件处理
vidproc scale -w 1920 -h 864 input.mp4

# 批量处理（新增）
vidproc batch --project batch.vidproj

# NLE 渲染（新增）
vidproc render --project my_project.vidproj
```

---

## 优先级

| 优先级 | 功能 | 理由 |
|--------|------|------|
| P0 | 项目文件格式 | 一切的基础 |
| P0 | 单一 filter_complex 渲染 | 性能 + 质量 |
| P1 | Batch 处理 | 实用需求 |
| P1 | nvenc HEVC/AV1 支持 | 编码选项 |
| P2 | Alpha fade | 视觉效果 |
| P2 | 时间轴 overlay | NLE 核心 |
| P3 | 时间戳 op | 便利性 |
