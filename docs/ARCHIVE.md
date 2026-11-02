# 归档记录

> 归档日期:2026-11-02
> 分支:`main`(HEAD 见 README/推送记录)
> 状态:**项目搁置,暂缓开发**

## 决策

本视频处理工具项目(vidproc)核心功能已达成并验证,但基于整体判断,决定
**暂时搁置、归档**。保留代码与完整提交历史,后续如需恢复开发,从当前
`main` HEAD 继续。

## 归档时已完成并验证

| 能力 | 状态 | 证据 |
|---|---|---|
| CLI 26 原子操作 + 管线/YAML 编排 | ✅ | go test 全绿 |
| `render` 声明式渲染(filtergraph DAG) | ✅ | 编译/测试通过 |
| `project` SQLite 持久化 | ✅ | 编译/测试通过 |
| 硬件编码 NVENC / QSV / AMF | ✅(QSV 实测) | `render --gpu qsv` 1.8s 渲染 720p |
| Web Studio(htmx + Alpine,零构建) | ✅ | check_gap 6/6、check_audio 5/5 |
| 真实音频播放(HTMLAudio 池) | ✅ | check_audio 5/5(媒体端点 206) |
| 版本检查 + 更新横幅 | ✅ | `/api/version/check` 返回 0.2.0 |
| 桌面原生对话框(绑定层) | ✅ 编译 | wails3 build 通过 |
| 系统能力前端接入(本地 API) | ✅ | isDesktop 分支 + Web 回退 |
| 三端壳(Windows/Linux/Android)符号 | ✅ | build tag 文件齐全 |

## 遗留待办(如恢复开发)

1. **桌面打包实测**:wails3 编译通过、前端已接绑定,但未打包启动亲眼验证
   原生对话框行为
2. **Android 移动布局**:三端共用桌面界面,无触控/竖屏专属设计;仅响应式断点
3. **AMD AMF 实测**:代码就绪,需 AMD 硬件
4. **CLI shell 补全**:无 bash/zsh/fish completion

## 过程教训

本项目开发过程中,无监管 AI 在方向、工具、路径上多次走歪,耗时远超预期。
完整复盘见 [ai-oversight.md](ai-oversight.md):**AI 只靠自己发展必然走歪,
必须由人定方向 + 其他 AI 独立审查 + 多方向并行尝试**。

## 恢复开发指引

1. `git clone https://github.com/chentianxiong123/vidproc`
2. `cd vidproc/go && go build -o ../vidproc.exe ./cmd/vidproc`
3. 从上述遗留待办逐项推进
