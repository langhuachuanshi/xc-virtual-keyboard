# AGENTS.md

AI 助手与协作者的项目入口。

## 1. 项目概览

懒獭 - 屏幕键盘:基于上游 Go 绑定 `github.com/twgh/xcgui` 的 Windows 桌面虚拟键盘。设计器优先——外观本体在 `data/` 设计器资产里,Go 代码只管事件与数据。发布目标:Windows 7 x64 及以上。仓库 [langhuachuanshi/xcgui-virtual-keyboard](https://github.com/langhuachuanshi/xcgui-virtual-keyboard)(公开,v0.1.0 已发 Release)。

## 2. 目录结构

```
src/         Go 源码:main.go 程序入口、keys.go 按键模拟(事件/粘滞/Fn 层/注入)、
             sound.go 按键音、winres/ 图标与版本信息
assets.go    根包://go:embed data/ 并提供内存 zip(embed 只能向下匹配,故放根上)
data/        设计器资产(唯一副本):main.xml 布局(77 个按键全带 btn_ name)、
             resource.res(颜色变量/字体/图片注册)、default.css(全局样式)、images/、sounds/
README.md    项目说明(公开仓库门面)
TODO.md      任务索引
Makefile     构建/运行/自检的唯一真源(make help 看命令)
build/       构建产物(不入库)
```

## 3. 项目约定

- **技术栈**:Go(go.mod 指令钉 1.18)、无 CGO、xcgui.dll(x64/x86)内嵌。**发布产物必须用 Go 1.20.14 工具链**(Makefile `GO_WIN7`):Go 1.21+ 编译的程序在 Win7 上起不来(runtime 层硬断),go 指令不能抬过 1.20。
- **系统分界(Win7 兼容)**:`app.New(isWin10Plus)` —— Win7 不启用 D2D(炫彩库支持 Win7);**`SetShadowInfo` 在"非 D2D + 自适应 DPI"下 DPI 计算有 BUG,Win7 分支绝不调用**;阴影由代码在 Win10+ 设置(`XWnd_SetShadowInfo`,大小 8)。main.xml 里的 transparentFlag/shadowSize/shadowAngleSize 待移除(见 TODO),阴影统一由代码接管。
- **内存加载**:资产全量嵌入二进制,启动时打成内存 zip,按 `LoadResourceZipMem(resource.res)` → `XC_LoadStyleZipMem(default.css)` → `NewByLayoutZipMem(main.xml)` 顺序加载,**顺序不能反**;改了 `data/` 必须重新编译(运行时不读盘)。
- **资源加载链**:resource.res 是字典(@变量/字体/图片)→ default.css 样式(内部 `@resFile` 回引 res)→ main.xml 布局按 @名 取值。样式只引用变量名、禁止写死色值。
- **已知 wart**:resource.res 图片注册带旧绝对路径 `dir="D:\...\xcgui\images"`(目录已删);zip 模式不碍事,改磁盘加载前必须清理。
- **命名**:元素 name = `btn_<键名>`(btn_q、btn_f1、btn_shift_l、btn_space…修饰键按内容命名,方向键按图标命名);样式 `<部件>_<变体>`。
- 绑定源码在 `D:\Project\xcgui`(go.mod 本地 replace),查 API 原型以源码为准。
- **构建**:`make build` 发布(产物 `build/keyboard.exe`,Win7 x64+);`make x86` 出 32 位;`make run` 日常调试(走本机新 Go,仅本机运行)。

## 4. xcgui 编码铁律

- 写代码前先检索绑定源码,**禁止凭模型记忆写 API**;检索可用全局技能 `go-xcgui-dev`(联接自 xcgui 仓库,任意项目可用)。
- 事件用 `AddEvent_` 前缀方法(复用回调,可放心用匿名函数);`Event_` 前缀每次新建回调,约 2000 个上限。想拦截事件传递,在回调里设 `*pbHandled = true`。
- 非 UI 线程操作 UI,用 `xc.UI()` / `xc.Auto()` 包裹。
- 改了元素外观要 `Redraw(false)`,否则不刷新。
- List / Tree / ComboBox 先 `CreateAdapter()` 再操作数据。

## 5. 注释

- Go:函数注释 `// 函数名_中文描述, 简要说明.` + 参数逐行说明;非显而易见的决策注释里写**为什么**。
- Makefile:变量与目标上方中文注释写用途与实踩教训;recipe 内只写 ASCII,中文只进注释与 HELP 块(make 按 ANSI 码页转命令,中文会乱码)。

## 6. 任务

任务索引在 [TODO.md](TODO.md):想法一句话入账;完成打勾、标日期、移入"已完成"。

## 7. 行为准则

1. **先想后动**——动手前说出假设;不确定就问;多种理解列出来让用户选;有更简单的方案直说。
2. **简单优先**——用最少代码解决问题;不加没要求的功能与"灵活性";不为不可能的场景做防御。
3. **精准修改**——只动该动的;不顺手改进旁边代码、不重构没坏的东西、匹配已有风格;每行改动可追溯到需求。
4. **目标驱动**——把任务转成可验证目标:修 bug 先写复现;改 UI 观感以"窗口跑起来、人眼验收"为通过标准;**不跑全量 `go test`**(基线测试会弹真窗口),验证用 build + vet。
5. **git 纪律**——日常只 `git commit` 到本地,**`git push` 必须用户明确要求才执行**(2026-11-08 定)。
