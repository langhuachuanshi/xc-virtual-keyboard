# TODO

## 待办

- [ ] Win7 真机验收(用户确认遗留):跑一遍打字/修饰键/音效/置顶/任务栏图标
- [ ] main.xml 移除窗口阴影属性(transparentFlag="shadow" shadowSize shadowAngleSize,设计器里改)——Win7 上 DLL 解析布局仍可能内部走带 BUG 的阴影路径,阴影统一由代码按系统接管
- [ ] resource.res 清理 stale dir 绝对路径(dir 指向已删除的旧 xcgui\images)

## 进行中

(无)

## 已完成

- [x] v0.2.3 发版(2026-11-08 完成):单实例锁(命名互斥体,再次双击静默退出,已实测)
- [x] v0.2.2 发版(2026-11-08 完成):manifest requireAdministrator(盾牌)根治 WebView2/Electron 注入失效 —— 注入需管理员权限,普通权限被系统静默丢弃;代码回滚 v0.2.1(撤销扫描码实验,Win 恢复正常);UPX 加壳平反恢复
- [x] v0.2.1 发版(2026-11-08 完成):长按连发 + 自审修复(退出弹起卡键、注释对齐);winres 版本 0.2.1.0
- [x] 长按连发(2026-11-08 完成):按住 500ms 后 33ms/次连发,松开停止;连发期间粘滞修饰键保持按下,松开统一弹起;WebView2 焦点输入经用户实测确认可用(SendInput 生效)
- [x] Caps 真机确认同步正常(2026-11-08 用户确认),AttachThreadInput 后备方案销案
- [x] v0.2.0 验收通过并发布(2026-11-08 完成):切中英/Win 开始菜单/复制粘贴/WebView2 输入/Fn 五项全过;winres 版本升 0.2.0.0,补齐公司/版权/作者信息
- [x] Shift 单击切中英(2026-11-08 定稿,参考 QtSoftKeyboard;TabTip 两击式是公认痛点):独立单击=完整物理单击立即切中英,Ctrl/Alt/Win 点亮时退化为组合修饰键;修复程序化熄灭丢 Redraw 导致的"视觉还亮着"(违反 AGENTS.md 铁律实踩)
- [x] Win 单击弹开始菜单(2026-11-08 定稿):点亮即物理按下,1 秒内无组合键消费自动弹起(无键间隔,系统弹开始菜单);时限内点 E 则 Win+E 照常;Fn 动态标签按用户要求撤销(提示图标归设计器资产)
- [x] 粘滞模型物理同构(2026-11-08 定稿):点亮修饰键立即注入按下、熄灭注入弹起 —— 修复 Ctrl+Shift 纯修饰组合发不出、QQ拼音 Shift 空点切英文无效;键帽上档显示按用户要求撤销
- [x] git init + 首次提交 + tag v0.1.0 + 双架构发布产物(build/keyboard.exe x64、keyboard-x86.exe,均 go1.20.14)(2026-11-08 完成)
- [x] 代码自审六项全修:注入 worker 化+连打竞态、GetKeyState 收 UI 线程、fnLayer 单表、sendKey 合并、注释清理、waveOut 关闭顺序(2026-11-08 完成)
- [x] Fn 层(2026-11-08 完成):Fn 粘滞 + F1-F8 = 音量/媒体/浏览器键(VK 本地定义,xcc 未收录)、Fn+方向键 = Home/End/PgUp/PgDn;Fn 点亮时 F 区/方向键动态换标签提示,熄灭还原(设计器无需加图标)
- [x] Caps 点亮态实时同步:300ms 轮询 GetKeyState 对齐按钮(2026-11-08 完成)
- [x] 任务栏图标:WS_EX_NOACTIVATE 默认藏任务栏按钮,补 WS_EX_APPWINDOW(2026-11-08 完成)
- [x] 窗口常驻顶层:HWND_TOPMOST(NOACTIVATE 点击永不激活的补偿,屏幕键盘惯例)(2026-11-08 完成)
- [x] Win 组合修复:发送补扫描码(MapVirtualKey)+ 注入延时;Win 单击(挂起未消费即熄灭)补发单击弹开始菜单(2026-11-08 完成)
- [x] btn_bracket_l 文本修复:content 的 { 被布局解析器当数据绑定语法吃掉,运行时 SetText 补设(2026-11-08 完成)
- [x] 键盘不抢焦点:WS_EX_NOACTIVATE(wapi.SetWindowLongPtrW,双系统分支都设),待真机验证(2026-11-08 完成)
- [x] 修饰键行为(2026-11-08 二次修复):check 型按钮走 Button_Check 事件(BnClick 对 check 型不触发,首版批量用 BnClick 是错的);Caps 与系统真实态比对(GetKeyState 低位)不一致才补发切换,可自愈物理键盘失步;Shift/Ctrl/Alt/Win 粘滞一键,字符键发出后 SetCheck(false) 自动弹起;Fn 仅记录状态(Fn 层未定义)
- [x] 按键事件绑定(2026-11-08 二次修复):普通键单个 LButtonDown 里播音+发键(按下即出);btn_min/btn_close 挂显式 BnClick 执行最小化/关闭(buttonType 只管视觉,DLL 无原生动作,首版删掉验证过的处理器是错的);keybd_event 模拟(2026-11-08 完成)
- [x] 键音:`data/sounds/key_press.mp3`(已随 zip 嵌入)按键播声(2026-11-08 完成)
- [x] 程序图标 + 版本信息:winres(src/winres,xc res 工具链),版本 0.1.0.0(2026-11-08 完成)
- [x] 接入自维护绑定(本地 replace)+ 内存 zip 加载 data/,窗口生命周期跑通(2026-11-08 完成)
- [x] Win7 兼容构建:Makefile / build.bat 锁 GOTOOLCHAIN=go1.20.14,产物进 build/(2026-11-08 完成)
- [x] AGENTS.md 项目规范 + TODO.md 任务索引,源码迁入 src/(2026-11-08 完成)
