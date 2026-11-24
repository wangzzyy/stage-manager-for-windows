# 全屏窗口拖入侧边栏：原生拖动重启设计

## 问题

用户拖动最大化/全屏窗口进入侧边栏时，Windows 会在拖动过程中将窗口恢复为普通窗口。恢复动作会改变窗口尺寸、位置和原生 move-size loop 的状态；Stage Manager 同时尝试启动窗口到卡片的 handoff，二者竞态可能导致拖入失效、窗口跳回或拖动状态残留。

手动操作验证了一个可行方向：先松开鼠标，等窗口完成恢复，再按住鼠标继续拖动，Windows 会重新建立原生拖动状态，随后侧边栏接收拖入正常。

## 目标与非目标

- **目标**：将拖入侧边栏时的全屏窗口拖动转换为可控的普通窗口拖动，复用既有窗口转卡片、飞行、停放与卡片拖动流程。
- **目标**：保留用户的鼠标位置与拖动意图；不重复执行 drop；避免窗口恢复时闪回旧位置。
- **非目标**：重写一般窗口拖动、跨显示器舞台让位、卡片排序或停放机制。
- **不读取/跟进现有 PR**：本方案独立设计，后续实现从当前 main 分支开始。

## 建议流程

1. **捕获拖动上下文**：在 `startDrag` 记录 HWND、起始矩形、原显示器、启动时窗口状态（最大化/全屏候选）、恢复矩形（如可得）与鼠标位置。由于 WinEvent Start 可能在系统已开始 restore 后才到达，不能只依赖 `IsZoomed`；结合恢复位置、窗口矩形与显示器工作区，并保留“是否从最大化状态开始”的可验证证据。
2. **检测侧边栏命中**：在 `dragOver` 命中 strip 时，先判定是否需要“拖动重启”。普通窗口仍走现有 handoff。全屏/最大化窗口进入独立转换分支，避免先置 `converted` 并启动旧 handoff。
3. **释放原生拖动**：注入真实鼠标左键 UP（Win32 `SendInput`），并等待原 move-size loop 的 End 事件/退出确认。UP 应只在确认本次手势确实处于左键拖动时执行；用转换状态避免事件回调递归或重复触发。
4. **恢复窗口状态**：将窗口恢复为普通窗口，应用捕获到的恢复矩形（必要时按当前显示器/DPI修正），更新窗口目录与拖动上下文。恢复窗口要先确保可见且不被停放。
5. **重新建立拖动**：在窗口可用于拖动的标题栏起点重新注入 DOWN，并将拖动路径续接到当前指针位置。必须验证该输入序列确实重新进入 Windows 原生 move-size loop；仅向 HWND 发送 `WM_LBUTTONDOWN` 并不等价于真实标题栏 hit-test。若需要短暂移动鼠标到标题栏锚点再回到当前指针，需避免可见跳动，并保留鼠标当前落点。
6. **刷新后进入既有流程**：确认窗口状态和拖动 loop 已重新初始化后，更新 `a.windows` / `catalog`，再调用现有 `beginWindowCardDrag`、`sendDraggedToStrip` 的路径。卡片捕获/飞行动画仍负责遮盖真实 HWND，handoff 负责隐藏、取消 move loop 与停放；不能在重启分支提前执行两次。
7. **失败回滚**：若 UP、restore、DOWN 或 loop 重启任一步失败，取消本次转换，恢复窗口可见且在安全位置，清除拖动状态并刷新；不能留下鼠标左键被注入为按下、窗口被隐藏或卡片 externalLayer 卡住的状态。

## 状态机

```
DraggingFullscreen
    | strip hit
    v
ReleasePending -- inject LEFTUP --> NativeLoopEnding
    | timeout/failure                    | loop ended
    | rollback                           v
    +----------------------------- RestoreWindow
                                          |
                                   refresh HWND state
                                          v
                                   ReanchorDrag
                                  inject/replay input
                                          |
                              loop re-established?
                                /         \
                              yes         no
                              v           v
                         StripHandoff   Rollback
                              |
                    existing card/flight flow
```

建议在 `drag` 中加入显式转换阶段（而不是复用 `converted` 表示所有状态），例如 `restartPending` / `restarting` / `converted`。所有状态只在 MyGo 主线程读写。异步等待 native loop 结束时通过 `post`/事件回到主线程，不阻塞 UI 主线程。

## Win32 输入与同步注意事项

- 使用 `SendInput` 注入 LEFTUP / LEFTDOWN，避免仅发送窗口消息；Windows 的 move-size loop 依赖实际输入、鼠标捕获与 hit-test。
- LEFTDOWN 需要位于可启动标题栏拖动的锚点；全屏窗口可能是无边框窗口，必须检查其恢复后的 style/frame 与 hit-test。不要假设对任意坐标注入 DOWN 都会启动窗口移动。
- 不能在 WinEvent 主线程回调里等待/阻塞 native loop 退出。采用状态机与超时；等待由事件或短延时调度完成。
- 任何注入的按键/鼠标状态都要有成对释放和回滚保障（退出、异常、窗口销毁、超时）。
- 避免 `EventSystemMoveSizeEnd` 抢先清除正在重启的 `drag`；该事件在转换期间应被状态机识别并消费。
- 真实 HWND 仍应在飞行层首帧就绪后才隐藏，延续现有防闪策略。转换过程尽量避免 DWM cloak 与显式 hide 的重复/失效路径。

## 代码落点

- `stage_mgr_go/internal/app/drag.go`：捕获初始状态、检测 strip hit、转换状态机、End 事件处理、失败回滚、衔接既有 handoff。
- `stage_mgr_go/internal/winapi/`：薄封装 `SendInput` 鼠标事件、窗口 restore/placement、标题栏锚点与 native move-loop 相关检测；不持有 App 状态。
- 如需新的 Win32 消息/结构：声明在 winapi 层并用 Windows build tag；不引入第三方依赖。
- `docs/CODEMAP.md` 与 `AGENTS.md`：实现完成后更新拖动流程说明；用户可见行为无需改 README，除非此修复成为正式功能说明。


## 分阶段诊断日志（实现时必须落地）

目的：一次复现即可判断流程停在哪一步、Windows 原生拖动是否结束、窗口是否成功恢复、重新按下是否真正启动了新的 move-size loop，以及卡片 handoff 是否完成。日志使用项目现有 `debugf`，统一前缀 `fullscreen-drag`；不记录窗口标题、进程命令行等隐私内容。

### 统一字段

每条日志尽量带上：

- `seq`：本次拖动会话唯一递增 ID；同一次转换所有日志共享。
- `step`：固定步骤名，便于过滤。
- `hwnd`：窗口句柄（十六进制）。
- `phase`：状态机当前状态。
- `cursor`：当前屏幕坐标。
- `rect`：窗口外框（left,top,right,bottom）。
- `elapsed_ms`：从本次转换开始经过的毫秒数。
- `ok` / `reason`：结果与失败原因；失败日志必须包含 Win32 错误码（若 API 提供）。

建议统一格式：`fullscreen-drag seq=%d step=%s hwnd=%v phase=%s cursor=(%d,%d) rect=(%d,%d,%d,%d) ok=%t reason=%s`。矩形/坐标无法读取时明确写 `unknown`，不要用零值伪装成功。

### 必须覆盖的日志点

| 步骤 | 日志事件 | 关键判断字段 | 异常时要能看出的结论 |
|---|---|---|---|
| 0. 拖动开始 | `start` | IsZoomed、窗口 style、窗口 rect、恢复 rect、鼠标按键状态 | WinEvent Start 到达时窗口是否已被 Windows 还原 |
| 1. 侧边栏命中 | `strip-hit` | strip/display、命中坐标、是否 fullscreen candidate、是否已转换 | 是否命中正确侧边栏；为何走普通路径/重启路径 |
| 2. 释放鼠标 | `mouse-up` | SendInput 返回数量、GetLastError、注入前后按键状态 | UP 是否真正注入；是否可能误释放用户其他输入 |
| 3. 原生循环结束 | `move-end` | End WinEvent 是否收到、等待耗时、线程 move-size 状态 | Windows 原生拖动循环是否退出；是否超时 |
| 4. 恢复窗口 | `restore` | Restore API 结果、恢复前后 IsZoomed、前后 rect、placement、可见性 | 是否恢复成功；是否跳回旧位置或仍处于最大化 |
| 5. 刷新窗口数据 | `refresh` | catalog 是否找到 HWND、最新 rect/frame、所属 display | Stage Manager 是否拿到恢复后的最新窗口状态 |
| 6. 重新按下 | `mouse-down` | 锚点坐标、SendInput 返回数量、鼠标位置、目标 HWND | DOWN 是否注入；锚点是否位于可拖动标题栏区域 |
| 7. 新拖动确认 | `move-start` | 新 Start WinEvent 是否收到、是否同 HWND、耗时、窗口 rect | Windows 是否真的重新建立原生拖动，而非仅按下鼠标 |
| 8. 卡片接管 | `handoff-start` | beginWindowCardDrag 结果、capture 是否启动、strip/card key | 是否进入既有 Stage Manager 卡片流程 |
| 9. 飞行动画/隐藏 | `handoff-frame`、`window-hide` | 首帧是否 ready、Hide/SetWindowPos 结果、窗口可见性 | 是否在动画首帧前隐藏导致闪烁 |
| 10. 停放 | `park` | CancelMoveSize 结果、ParkImmediateFrom 结果、parkFrom/home rect | 原生循环取消与停放是否成功 |
| 11. 完成 | `complete` | 最终 phase、窗口位置、卡片状态、总耗时 | 是否完整结束且无重复 drop |
| 失败回滚 | `rollback` | 原失败步骤、错误码、是否补发 LEFTUP、窗口是否恢复可见 | 是否留下按键按住、窗口隐藏或 externalLayer 卡住 |

### 事件时序与重复事件诊断

- 每次状态变化记录 `phase-change from=... to=... cause=...`。
- `EventSystemMoveSizeEnd` 在等待 UP 期间记录为原循环结束；在重新按下后记录为新循环开始。用 `seq` 和阶段区分，不能仅按 HWND 判断。
- `EventObjectLocationChange` 在重启期间重复触发时记录 `strip-hit-ignored reason=restarting`，不要刷屏；同一阶段可做限频或只记录首次/计数。
- 同一会话出现第二次 `handoff-start` 或第二次 `complete` 时输出 WARN，附带当前 phase。
- 每个等待点记录开始与结束/超时，日志中包含 timeout 配置值和实际耗时。

### 日志级别与隐私

- 正常阶段使用 `debugf`；失败、超时、状态机非法转换、重复 handoff 使用 `debugf` 的显著 `WARN`/ `ERROR` 文本前缀（遵循项目现有日志设施，不另建日志依赖）。
- 不记录窗口标题、完整路径、命令行或用户输入内容。
- 鼠标注入、窗口恢复、隐藏/停放等改变系统状态的操作，必须同时记录调用结果与操作后的实际状态；不能只记录“准备执行”。
- 每次拖动只生成一个 `seq`，便于从 `start` 到 `complete/rollback` 串起完整链路。

### 通过日志定位问题的速查

- 缺少 `strip-hit`：检查鼠标坐标、strip rect、display 命中与 WinEvent LocationChange。
- 有 `mouse-up` 无 `move-end`：检查 SendInput 注入结果、原生循环退出等待及事件回调线程。
- 有 `move-end` 无 `restore`：检查状态机调度/主线程 post。
- `restore ok` 但 rect 不正确：检查 placement NormalPosition、DPI 与显示器坐标换算。
- 有 `mouse-down` 无新 `move-start`：优先检查标题栏锚点、窗口 hit-test、输入注入和移动事件监听。
- 有新 `move-start` 无 `handoff-start`：检查 catalog 刷新、strip 命中二次判定和状态机阶段。
- 有 `handoff-start` 无 `handoff-frame`：检查窗口捕获、flight layer 初始化与回调。
- 有 `handoff-frame` 无 `window-hide`/`park`：检查 flight 回调是否触发及隐藏/取消循环的结果。
- 有 `rollback`：按其中的 `failed_step` 与 Win32 错误码定位，并确认 `left_button_released=true`、`window_visible=true`。

## 测试/验收矩阵

### 自动化（能纯逻辑覆盖的部分）
- 状态机合法转换、重复 LocationChange、End 事件先后顺序、超时与回滚。
- 普通窗口路径不触发鼠标输入注入。
- 转换期间再次命中 strip 不重复启动；`converted` 不造成第二次 drop。
- 目标 HWND 销毁、刷新后 HWND 不再列出、显示器切换与恢复矩形无效等边界。

### Windows 手动验证
- 最大化窗口与真正无边框全屏窗口分别测试；不同应用（Explorer、浏览器、视频播放器/游戏窗口）。
- 左侧与右侧 strip；固定和浮动侧边栏；100% 与高 DPI；单/多显示器。
- 全屏拖入 strip：鼠标不跳、窗口不闪回旧尺寸/位置、卡片飞行动画只播放一次、落入正确卡片。
- 松开后拖出/取消、拖动时快速经过 strip、窗口原生 End 事件早到/晚到、转换失败回滚。
- 普通非最大化窗口拖入、跨屏拖动、resize 结束在 strip、卡片排序及现有 handoff 回归。
- 确认退出应用后没有遗留鼠标按下状态，所有窗口能正常回到桌面。

## 风险与实现前验证

最关键的不确定性是：在 strip 位置如何重新建立原生标题栏拖动，同时不改变鼠标位置或造成视觉跳跃。需要先做一个小的 Win32 实验验证 `SendInput` UP、窗口 restore、在标题栏锚点 DOWN、再 replay move 的顺序和行为；如果原生 loop 无法可靠重启，备选是由应用接管恢复后的拖动（捕获鼠标并跟踪 pointer），但只有在验证原生方案不可行后才考虑，以避免引入自绘拖动路径。

本 PR 先提交设计说明，作为实现与验证的约束；代码改动、测试与实机验证在后续实现提交中完成。
