# 懒獭 - 屏幕键盘

基于自维护 [XCGUI(炫彩界面库)Go 绑定](https://github.com/langhuachuanshi/xcgui) 的 Windows 屏幕虚拟键盘。单文件、免依赖、不抢焦点,支持 Windows 7 SP1 ~ Windows 11。

<!-- 预览截图待补:![preview](docs/screenshot.png) -->

## 功能

- **75 键全事件**:按下即出字 + 按键音(mp3 内嵌,解码常驻内存零延迟)
- **粘滞修饰键**:Shift / Ctrl / Alt / Win 与物理键盘同构 —— 点亮即按下、熄灭即弹起,支持 Ctrl+Shift 等纯修饰键组合(如切换输入法);空点 Shift 由输入法切换中/英,空点 Win 弹开始菜单
- **Caps 真切换**:同步系统真实大小写锁定状态,物理键盘动过也能自愈对齐
- **长按连发**:按住 Backspace/方向键/字母等 500ms 后自动连发,松开停止,物理键盘手感
- **Fn 层**:Fn + F1~F8 = 静音/音量/播放/上下曲/浏览器前后;Fn + 方向键 = Home / End / PgUp / PgDn。点亮 Fn 时按键自动换标签提示,熄灭还原
- **不抢焦点**:点键盘不会夺走目标窗口焦点;窗口常驻顶层,任务栏有图标
- 资产全部内嵌,启动不打磁盘,单 exe 即完整程序

## 下载

[Releases](https://github.com/langhuachuanshi/xcgui-virtual-keyboard/releases) · 当前 [v0.2.1](https://github.com/langhuachuanshi/xcgui-virtual-keyboard/releases/tag/v0.2.1)

| 文件 | 适用 |
|---|---|
| `keyboard.exe` | x64(Windows 7 x64 及以上) |
| `keyboard-x86.exe` | x86(32 位 Windows 7) |

## 构建

```bash
make build   # x64 发布版 -> build/keyboard.exe(Win7 兼容,go1.20.14 工具链,UPX 加壳)
make x86     # x86 发布版 -> build/keyboard-x86.exe(UPX 加壳)
make run     # 日常调试(本机新 Go,仅 Win10/11 本机跑)
make check   # go vet(与发布同口径)
make clean   # 清 build/
```

注意事项:

- **Win7 兼容**:发布产物固定用 Go 1.20.14 编译(Makefile `GOTOOLCHAIN` 自动处理)——Go 1.21+ 的产物在 Win7 上无法启动。
- **资产内嵌**:`data/` 是设计器(炫彩 IDE)资产的唯一副本,改完必须重新编译,运行时不读盘。
- **图标/版本信息**:改 `src/winres/` 后先 `xc res make` 再构建。
- **依赖绑定**:开发机 `go.mod` 里 `replace` 指向本地绑定源码;其他机器构建删除该行,直接走 GitHub 依赖。

## 目录

```
src/        Go 源码:main.go 入口 / keys.go 按键模拟 / sound.go 按键音 / winres/ 图标与版本
assets.go   //go:embed 资产嵌入 + 内存 zip 打包
data/       设计器资产(布局/样式/图片/音效,唯一副本)
Makefile    构建命令唯一真源
```

## 技术要点

- Go(无 CGO),自维护 XCGUI 绑定,xcgui.dll 内嵌、运行时释放
- 设计器优先:布局与外观在 `data/`,Go 只管事件与数据;`btn_` 前缀命名即两侧契约
- 按键注入 `keybd_event`(带扫描码,单 worker 协程串行保序);按键音 winmm waveOut 常驻缓冲重触发
- 不抢焦点:WS_EX_NOACTIVATE(+ APPWINDOW 保任务栏图标),窗口常驻顶层
