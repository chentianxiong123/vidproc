# 私有元数据（UDTA Box）注入方法论

> 场景：合并多个小米/红米手机拍摄的 MP4 视频后，源文件里厂商写入的私有元数据
> （`com.android.version` / `com.android.manufacturer` / `com.android.model` /
> `xiaomi.exifInfo.videoinfo`）会全部丢失。ffmpeg 只能读不能写，`-metadata`
> 和 `-metadata_file` 都会静默丢弃这些 tag。
>
> 解决：直接对 MP4 容器做 box 级手术，把源视频的 `udta` box 整个搬进输出的
> `moov`。本文档记录 vidproc 的实现方式与踩坑点。

---

## 一、问题：ffmpeg 为什么写不回去

**先复现**（不要看结论，自己跑一遍才有把握）：

```bash
SRC=first.mp4        # 小米手机原始拍摄
OUT=concat_out.mp4   # ffmpeg concat 产物

# 源里能读到的厂商私有 tag：
ffprobe -v error -show_entries format_tags -of default=noprint_wrappers=1 "$SRC"
#   TAG:com.android.version=13
#   TAG:com.android.manufacturer=Xiaomi
#   TAG:com.android.model=23049RAD8C
#   TAG:xiaomi.exifInfo.videoinfo=N:0 D:undefine V:V14.0.25.0.TMRCNXM ...

# 显式喂回去试试：
ffmpeg -v error -i "$OUT" -c copy \
  -metadata com.android.version=13 \
  -metadata com.android.manufacturer=Xiaomi \
  -y explicit.mp4

ffprobe -v error -show_entries format_tags -of default=noprint_wrappers=1 explicit.mp4
# 结果：一条都没有 —— mp4 muxer 直接丢
```

**结论**：这不是参数没调对，是 ffmpeg 的 mp4 muxer 硬性限制。它只会输出标准
的 `©ART`/`©nam` 等 `ilst` 格式标签，遇到 `udta/mcvr` 这种私有结构就丢弃。
`-map_metadata` 也一样，只是"复制"了 ffmpeg 自己解析得到的 tag，厂商私有项
从来没进 ffmpeg 的内存模型。

**为什么这些 tag 存在但能被读**：小米把它们塞进了 `moov/udta/mcvr`（media
container data）这个私有扩展里，ffmpeg 的 mp4 demuxer 认得读，muxer 不认得写。
`udta` 是 MPEG-4 规范留的"用户数据"槽位，厂商各自往里塞自己格式。

---

## 二、原理：MP4 box 结构手术

### 2.1 box 树布局

```
MP4 file
├── ftyp            (brand/version)
├── free            (padding)
├── mdat            (media data, 通常占大部分体积)
└── moov            (movie metadata，关键就在这里)
    ├── mvhd
    ├── trak (video)
    ├── trak (audio)
    └── udta        ← 厂商私有数据全在这里
        ├── mcvr    (media container data, 小米用这个)
        └── 其他厂商自定义子 box
```

`moov/udta` 是**兄弟节点**并列关系。要让 ffmpeg 保留 `udta`，就必须在拼接
输出中把源文件的整个 `udta` box（含 header + payload）作为 `moov` 的子 box
插入进去。

### 2.2 三步操作

1. **提取**：读源文件 → 顶层扫 box → 找 `moov` → 在 `moov.Payload` 里再扫
   一遍 → 找 `udta` → 拿到 `Payload` 切片
2. **插入**：读输出文件 → 找 `moov` → 把新 `udta` box 的字节拼接到
   `moov.Offset + moov.HdrSize` 之后
3. **更新 size**：MP4 box header 前 4 字节就是 size 字段，插入后要把
   `moov` 的 size 加上 `len(udtaBox)`；否则播放器读到旧 size 会认为 moov
   已经结束，插入的数据被忽略

`moov` 通常 <4GB 走 32-bit size；如果 `moov.Size` 字段是 1（64-bit marker），
则真正 size 在 `moov.Offset+8` 起 8 字节。见附录 A。

### 2.3 为什么"顶层扫不到 udta"

早期实现把 `udta` 当顶层 box 找，`panic: source has no udta box`。原因：
小米的 `udta` 在 `moov` 里面，不是顶层。必须递归扫。同样原因 `moov`
本身可能嵌在 `free` 里面（少见，但存在），递归要覆盖 `moov`/`trak`/`mdia`
/`minf`/`stbl`/`udta` 这一串容器。

---

## 三、vidproc 实现

### 3.1 `internal/mp4/box.go`

已有的 `ScanBoxes` 已经能做完整 box 解析（支持 32/64-bit size marker）。新加
一个函数：

```go
// InsertUDTABox 把一个完整的 udta box（header+payload）作为 moov 的子 box
// 插入到目标文件里，并同步更新 moov size 字段。
func InsertUDTABox(dstPath string, udtaBox []byte) error {
    data, err := os.ReadFile(dstPath)
    if err != nil { return err }
    boxes := ScanBoxes(data)
    var moov *Box
    for i := range boxes {
        if boxes[i].Type == "moov" {
            moov = &boxes[i]
            break
        }
    }
    if moov == nil { return fmt.Errorf("moov box not found") }
    if len(udtaBox) < 8 { return fmt.Errorf("udta box too small: %d bytes", len(udtaBox)) }

    // 在 moov header 之后插入
    insertOff := moov.Offset + 8
    newData := make([]byte, 0, len(data)+len(udtaBox))
    newData = append(newData, data[:insertOff]...)
    newData = append(newData, udtaBox...)
    newData = append(newData, data[insertOff:]...)

    // 更新 moov size
    binary.BigEndian.PutUint32(newData[moov.Offset:moov.Offset+4],
        uint32(moov.Size + uint64(len(udtaBox))))
    return os.WriteFile(dstPath, newData, 0o644)
}
```

大约 25 行，零依赖。

### 3.2 `internal/ffmpeg/ffmpeg.go` — 接入 Concat 管线

在 `Concat` 里 ffmpeg concat 成功后追加：

```go
// 从第一个源文件提 udta，插到输出里。
if len(allInputs) > 0 {
    udtaBox, err := extractFirstInputUDTA(allInputs[0])
    if err == nil {
        if err := mp4.InsertUDTABox(output, udtaBox); err != nil {
            _ = err // 非致命，视频仍可播放
        }
    }
}
```

`extractFirstInputUDTA` 做递归查找，因为源文件的 `moov` 可能嵌套：

```go
func findMoovRecursive(data []byte) *mp4.Box {
    boxes := mp4.ScanBoxes(data)
    for _, b := range boxes {
        if b.Type == "moov" { return &b }
        if isContainer(b.Type) {
            if found := findMoovRecursive(b.Payload); found != nil { return found }
        }
    }
    return nil
}

func isContainer(t string) bool {
    switch t {
    case "free", "trak", "mdia", "minf", "stbl", "udta", "moof", "mfra":
        return true
    }
    return false
}
```

### 3.3 完整管线

```
vidproc concat -o out.mp4 a.mp4 b.mp4 c.mp4 d.mp4
    │
    ├─ ffmpeg -f concat -c copy  → out.mp4        (流复制，画质无损)
    ├─ 读 out.mp4 → 找到 moov
    ├─ 从 a.mp4 读 → moov → udta                  (提取厂商私有数据)
    ├─ 构造 udta box bytes (header+payload)
    └─ InsertUDTABox(out.mp4, udtaBox)             (插入 + 更新 size)
```

用户视角是**无感**：一条命令，私有元数据自动保留。

---

## 四、验证

### 4.1 单元测试

`internal/mp4/box_test.go` 里的 `TestInsertUDTA` 手工构造一个最小 MP4
（ftyp + moov(mvhd) + mdat），插入 udta 后验证：

- 文件字节数 = 原大小 + udta box 大小（size 计算正确）
- `ScanBoxes` 能在新的 `moov.Payload` 里找到 `udta`
- `udta.Payload` 与插入前字节完全相等（payload 保真）

跑法：

```bash
go test ./internal/mp4/ -v
# --- PASS: TestInsertUDTA
# --- PASS: TestInsertUDTABox
```

### 4.2 端到端实测

四条小米视频，28.887 秒，76703647 → 77883645 字节：

```
输入 4 段（每段都有完整 com.android.* / xiaomi.exifInfo.videoinfo）
     │
     └── vidproc concat → 77,883,645 B (+1,180,098 = 1.18 MB udta)
                             │
                             ├─ 867 帧，h264 High 1920x1080 + aac 48kHz 原样
                             ├─ ffprobe 无解码错误
                             ├─ mtime 已同步到源文件 1 的时间
                             └─ creation_time 通过 -map_metadata 注入
```

**关键观察**：插入后 `ffprobe` 显示的 `format_tags` 里**看不到** `com.android.*`
——这不是失败，是 ffmpeg 的 mp4 demuxer 不解析 `udta/mcvr`。数据在文件里，
用 hexdump 或专用 MP4 parser（如 `mp4box` / Python 的 `mp4parse`）能看到。
播放器读不读得懂是播放器的问题，不是我们写的问题。

### 4.3 常见坑

| 症状 | 原因 | 修复 |
|---|---|---|
| `panic: no moov` | `moov` 嵌在 `free` 里 | 递归扫 `free` |
| `moov atom not found` | 插入后 `moov.Size` 没更新 | 别忘了加 `len(udtaBox)` |
| 输出文件比源还小 | 插了 udta 但没插到 `moov` 里（插到 mdat 里去了） | 插入点必须是 `moov.Offset + moov.HdrSize` |
| 文件变大了但 `ffprobe` 读不到新 tag | 正常 | 厂商私有 tag 本来就不走 `format_tags` |
| `index out of range` | 构造 `udtaBox` 时 buf 没预分配长度 | `make([]byte, 8+len(payload))` |

---

## 五、适用性 & 代价

| 维度 | 评估 |
|---|---|
| 通用性 | **完全通用**：任何把厂商数据写进 `moov/udta` 的设备都受益（小米/红米/其他安卓/OEM 相机等） |
| 代码量 | ~50 行（`InsertUDTABox` + `extractFirstInputUDTA`），零新依赖 |
| 单元测试 | ~80 行，覆盖 size / payload / 位置三项 |
| 对现有流程影响 | 零：`Concat` 里 append 一段，其他 ops 不用动 |
| 失败时的降级 | 非致命：udta 提取或插入失败不影响视频本身，仍可播放 |

**封存还是继续？** 继续。理由：

1. 已知缺口：用户明确要求"完整元数据保留"，这是唯一还没补上的部分
2. 一次投入永久可用
3. 以后要改（比如加 64-bit moov size 支持），只改一处

---

## 附录 A：MP4 box header 结构

```
Byte 0-3:  size         (uint32, big-endian)
           若 size == 1  → 后 8 字节是真 size (uint64)，header 共 16 字节
           若 size == 0  → 该 box 直到文件尾
Byte 4-7:  type         (4 字节 ASCII，如 "moov" "udta" "mdat")
Byte 8+:  payload
```

`InsertUDTABox` 当前只处理 32-bit size（<4GB 的 moov 完全够用）。要支持
超大文件，检测 `data[moov.Offset]` 首字节是否为 0x00000001 即可切换到
`binary.BigEndian.PutUint64`。

## 附录 B：小米 `udta/mcvr` 里的实际内容示例

```
udta (size=1180095)
└── mcvr (size=1180087)
    ├── version=13
    ├── manufacturer=Xiaomi
    ├── model=23049RAD8C
    └── exifInfo.videoinfo=N:0 D:undefine V:V14.0.25.0.TMRCNXM T:0 M:0 ...
```

1.18 MB 里绝大部分是 `exifInfo.videoinfo` 的 EXIF 二进制结构，我们整段
原样搬运，不解构不改写，避免破坏厂商格式。
