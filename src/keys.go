// 按键输入模拟:键位用 SendInput 把虚拟键注入系统输入流(官方推荐注入路径,
// Win2000 起可用,WebView2/Chromium 与 IME 对它的兼容性最好)。注入统一经
// keyJobs 单工作协程串行执行:序列里的毫秒级延时(shell 热键对时序敏感,事件
// 贴太近会被丢)不阻塞 UI 线程;channel 保序,快速连打不插队。目标窗口照常
// 收到键盘消息 —— 前提是本键盘不抢焦点(WS_EX_NOACTIVATE,见 main.go)。
//
// 事件分型:
//   - check 型按钮(caps/shift_l/ctrl/fn/win/alt,布局里 buttonType="check")不触发
//     BnClick,事件是 Button_Check,参数即新勾选态;视觉由 DLL 自动翻转,
//     代码只在粘滞消费后 SetCheck(false) 熄灭;
//   - 普通键单个 LButtonDown 干完播音+发键(按下即出,真键盘手感);
//   - min/close 的 buttonType 只管视觉,动作由代码挂 BnClick。
//
// 修饰键语义(2026-11-08 定稿,参考 QtSoftKeyboard 等中文软键盘):
//   - Shift 独立单击 = 完整物理单击,输入法立即切中/英;Ctrl/Alt/Win 点亮时
//     退化为组合按住(Ctrl+Shift 等纯修饰组合照常);
//   - Win 点亮即物理按下;winStartMenuDelay 内无组合键消费则自动弹起 ——
//     无键间隔的按下+弹起,系统自己弹开始菜单;时限内点 E 则 Win+E 照常;
//   - Ctrl/Alt 粘滞 = 物理按住,点亮按下、熄灭弹起。
//   - Fn 只记粘滞态驱动 fnLayer 映射,不改任何 UI 文本(提示图标是设计器资产)。
//
// 状态同步(虚拟键盘通行做法,防合成事件卡修饰键/大小写反转):Caps 不盲发,
// 先在 UI 线程读 GetKeyState 系统真实态,与按钮期望态不一致才补发。注意
// GetKeyState 是每线程快照,只能在 UI 线程读;本窗口不获焦、收不到键盘消息,
// 真机验收若仍失步,再升级 AttachThreadInput 方案(见 TODO)。
package main

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/langhuachuanshi/xcgui/xcgui/app"
	"github.com/langhuachuanshi/xcgui/xcgui/wapi"
	"github.com/langhuachuanshi/xcgui/xcgui/widget"
	"github.com/langhuachuanshi/xcgui/xcgui/window"
	"github.com/langhuachuanshi/xcgui/xcgui/xc"
	"github.com/langhuachuanshi/xcgui/xcgui/xcc"
)

var (
	procSendInput     = syscall.NewLazyDLL("user32.dll").NewProc("SendInput")
	procGetKeyState   = syscall.NewLazyDLL("user32.dll").NewProc("GetKeyState")
	procMapVirtualKey = syscall.NewLazyDLL("user32.dll").NewProc("MapVirtualKeyW")
)

const (
	keyEventF_ExtendedKey = 0x0001
	keyEventF_KeyUp       = 0x0002
)

// INPUT 联合体的 Go 对位:用最大成员 MOUSEINPUT 占位,保证 cbSize 与 C 布局
// 一致(x64=40, x86=28),KEYBDINPUT 经指针重解释写进同一块内存;字段顺序与
// 自然对齐照 MSDN,不可乱。
// INPUT type 常量:0=鼠标 1=键盘(2026-11-08 实踩:写成 0 时每个"抬键"都变成
// 鼠标左键按下注入,全局输入报废)。
const inputKeyboard = 1

type keyboardInput struct {
	wVk       uint16
	wScan     uint16
	dwFlags   uint32
	time      uint32
	extraInfo uintptr
}

type mouseInput struct {
	dx, dy    int32
	mouseData uint32
	dwFlags   uint32
	time      uint32
	extraInfo uintptr
}

type input struct {
	typ uint32
	u   mouseInput
}

// keyJobs 注入任务队列:缓冲 32 保证 UI 线程入队不阻塞(极端连打也只是背压),
// worker 串行消费保证键序。appUI 供任务末尾把 UI 操作(CallUiThreadEx 熄灭
// 粘滞按钮)回投到界面线程。
var (
	keyJobs = make(chan func(), 32)
	appUI   *app.App
)

// startKeyWorker 启动注入工作协程。keybd_event 不碰 UI,可安全在后台执行。
func startKeyWorker(a *app.App) {
	appUI = a
	go func() {
		for job := range keyJobs {
			job()
		}
	}()
}

// vkByName: btn_ 名 → 虚拟键码。字母/数字/F1-F12 程序生成,其余显式列出;
// 修饰键与 btn_fn 无映射(走独立逻辑),btn_min/btn_close 不是键。
var vkByName = buildVkMap()

func buildVkMap() map[string]uint32 {
	m := map[string]uint32{
		"btn_esc":       uint32(xcc.VK_Esc),
		"btn_tab":       uint32(xcc.VK_Tab),
		"btn_backspace": uint32(xcc.VK_Backspace),
		"btn_enter":     uint32(xcc.VK_Enter),
		"btn_space":     uint32(xcc.VK_Space),
		"btn_del":       uint32(xcc.VK_Delete),
		"btn_up":        uint32(xcc.VK_Up),
		"btn_down":      uint32(xcc.VK_Down),
		"btn_left":      uint32(xcc.VK_Left),
		"btn_right":     uint32(xcc.VK_Right),
		"btn_minus":     uint32(xcc.VK_Minus),
		"btn_equal":     uint32(xcc.VK_Equals),
		"btn_comma":     uint32(xcc.VK_Comma),
		"btn_period":    uint32(xcc.VK_FullStop),
		"btn_slash":     uint32(xcc.VK_ForwardSlash),
		"btn_grave":     uint32(xcc.VK_Backquote),
		"btn_bracket_l": uint32(xcc.VK_BracketLeft),
		"btn_bracket_r": uint32(xcc.VK_BracketRight),
		"btn_backslash": uint32(xcc.VK_Backslash),
		"btn_semicolon": uint32(xcc.VK_Semicolon),
		"btn_quote":     uint32(xcc.VK_QuotationMark),
	}
	for i := 0; i < 26; i++ {
		m["btn_"+string(rune('a'+i))] = uint32(xcc.VK_A) + uint32(i)
	}
	for d := 0; d <= 9; d++ {
		m["btn_"+strconv.Itoa(d)] = uint32(xcc.VK_0) + uint32(d)
	}
	for i := 1; i <= 12; i++ {
		m[fmt.Sprintf("btn_f%d", i)] = uint32(xcc.VK_F1) + uint32(i-1)
	}
	return m
}

// stickyMods 粘滞修饰键:name → 虚拟键码;数组序即组合键的按下/释放顺序。
var stickyMods = []struct {
	name string
	vk   uint32
}{
	{"btn_shift_l", uint32(xcc.VK_Shift)},
	{"btn_ctrl", uint32(xcc.VK_Ctrl)},
	{"btn_alt", uint32(xcc.VK_Alt)},
	{"btn_win", uint32(xcc.VK_LWin)},
}

// stickyVKOf 粘滞键名 → 虚拟键码(非粘滞键返回 0)。
func stickyVKOf(name string) uint32 {
	for _, m := range stickyMods {
		if m.name == name {
			return m.vk
		}
	}
	return 0
}

// checkBtns 布局里的 check 型按钮(含非粘滞的 caps/fn),决定事件走 Button_Check。
var checkBtns = map[string]bool{
	"btn_caps": true, "btn_shift_l": true, "btn_ctrl": true,
	"btn_alt": true, "btn_win": true, "btn_fn": true,
}

// extendedVK 发送时必须带 KEYEVENTF_EXTENDEDKEY 的键:方向/导航/Del 不带
// 扩展标志部分程序会收错(当成数字小键盘码)。媒体键不需要。
var extendedVK = map[uint32]bool{
	uint32(xcc.VK_Up):       true,
	uint32(xcc.VK_Down):     true,
	uint32(xcc.VK_Left):     true,
	uint32(xcc.VK_Right):    true,
	uint32(xcc.VK_Delete):   true,
	uint32(xcc.VK_Home):     true,
	uint32(xcc.VK_End):      true,
	uint32(xcc.VK_PageUp):   true,
	uint32(xcc.VK_PageDown): true,
}

// Fn 层媒体/浏览器键(MSDN 公开虚拟键值,绑定 xcc 未收录;Win7 起 keybd_event
// 可发,系统直接出音量浮层/控制媒体)。
const (
	vkBrowserBack    uint32 = 0xA6 // 浏览器后退
	vkBrowserForward uint32 = 0xA7 // 浏览器前进
	vkVolumeMute     uint32 = 0xAD // 静音
	vkVolumeDown     uint32 = 0xAE // 音量减
	vkVolumeUp       uint32 = 0xAF // 音量加
	vkMediaNext      uint32 = 0xB0 // 下一曲
	vkMediaPrev      uint32 = 0xB1 // 上一曲
	vkMediaPlayPause uint32 = 0xB3 // 播放/暂停
)

// fnLayer: Fn 粘滞时的映射层(F 区媒体/浏览器键 + 方向键补导航缺口,对齐
// 笔记本 Fn+方向惯例)。提示图标由设计器资产负责,代码不改动 UI 文本。
var fnLayer = []struct {
	name string
	vk   uint32
}{
	{"btn_f1", vkVolumeMute},
	{"btn_f2", vkVolumeDown},
	{"btn_f3", vkVolumeUp},
	{"btn_f4", vkMediaPlayPause},
	{"btn_f5", vkMediaPrev},
	{"btn_f6", vkMediaNext},
	{"btn_f7", vkBrowserBack},
	{"btn_f8", vkBrowserForward},
	{"btn_up", uint32(xcc.VK_PageUp)},
	{"btn_down", uint32(xcc.VK_PageDown)},
	{"btn_left", uint32(xcc.VK_Home)},
	{"btn_right", uint32(xcc.VK_End)},
}

// 粘滞态以 Go 变量为真源(由 Button_Check 回调同步,DLL 的勾选视觉自动跟随
// 用户点击);点亮即物理按下,熄灭即物理弹起。winGen 是 Win 粘滞代数:每次
// 点亮自增,超时任务凭代数判断自己还有没有效,防止旧超时错误释放新一轮按住。
var (
	stickyOn = map[string]bool{}
	winGen   int
)

// winStartMenuDelay Win 点亮后无组合键消费的自动弹起时长;到点弹起时无键间隔,
// 系统弹开始菜单。时限内点 E 之类的键则照常组合。
const winStartMenuDelay = time.Second

// btnByName 全部按键按钮引用,由 bindKeys 填充:粘滞熄灭、Fn 标签切换、
// Caps 同步都要从代码侧操作按钮。
var btnByName = map[string]*widget.Button{}

// sendKey 用 SendInput 注入单个键事件(带扫描码)。只发虚拟码时部分消费者
// (Chromium/WebView2、shell 热键)不认,扫描码经 MapVirtualKeyW 补齐。
func sendKey(vk uint32, extended, up bool) {
	var in input
	in.typ = inputKeyboard
	ki := keyboardInput{wVk: uint16(vk), wScan: uint16(scanOf(vk))}
	if extended {
		ki.dwFlags |= keyEventF_ExtendedKey
	}
	if up {
		ki.dwFlags |= keyEventF_KeyUp
	}
	*(*keyboardInput)(unsafe.Pointer(&in.u)) = ki
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

// scanOf 虚拟键 → 扫描码。
func scanOf(vk uint32) uintptr {
	s, _, _ := procMapVirtualKey.Call(uintptr(vk), 0)
	return s
}

// keyDown/keyUp sendKey 的语义包装,保留调用点可读性。
func keyDown(vk uint32, extended bool) { sendKey(vk, extended, false) }
func keyUp(vk uint32, extended bool)   { sendKey(vk, extended, true) }

// capsLockOn 读系统真实大写锁定态。必须在 UI 线程调用:GetKeyState 是每线程
// 快照,线程要从自己消息队列读键盘消息才更新。
func capsLockOn() bool {
	r, _, _ := procGetKeyState.Call(uintptr(xcc.VK_CapsLock))
	return int16(r)&1 == 1
}

// takeSticky 在 UI 线程原子收走粘滞态:返回待熄灭的按钮名并立即清零状态。
// 入队时收走而非任务末尾,快速连打的第二键才不会读到上一键未消费的旧粘滞。
func takeSticky() []string {
	var names []string
	for _, m := range stickyMods {
		if stickyOn[m.name] {
			stickyOn[m.name] = false
			names = append(names, m.name)
		}
	}
	if stickyOn["btn_fn"] {
		stickyOn["btn_fn"] = false
		names = append(names, "btn_fn")
	}
	return names
}

// unlightButtons 熄灭按钮勾选态(UI 线程执行)。stickyOn 已在 takeSticky 清零,
// Button_Check 若触发,各分支的物理弹起/单击条件(stickyOn 仍为真)不成立,
// 不会误弹开始菜单/误切输入法。
func unlightButtons(names []string) {
	for _, n := range names {
		if b := btnByName[n]; b != nil {
			b.SetCheck(false)
			b.Redraw(false) // 程序化改外观必须重绘(AGENTS.md 铁律;漏掉会"状态灭了视觉还亮着")
		}
	}
}

// onModCheck check 型按钮选中态变化(DLL 已翻好视觉,参数即新态)。
//
// name: 按钮名.
//
// checked: 新勾选态.
func onModCheck(name string, checked bool) {
	switch name {
	case "btn_caps":
		// 期望态与系统真实态不一致才补发切换,幂等且能自愈物理键盘失步。
		// 连点竞态:第二次点击时队列里的翻转可能还没生效,轮询会把视觉对齐真值。
		if checked != capsLockOn() {
			keyJobs <- func() {
				keyDown(uint32(xcc.VK_CapsLock), false)
				keyUp(uint32(xcc.VK_CapsLock), false)
			}
		}
	case "btn_shift_l":
		// Shift(2026-11-08 定稿,参考 QtSoftKeyboard 等中文软键盘):
		//   - 独立单击 = 一次完整物理单击(按下→50ms→弹起),输入法立即切中/英。
		//     TabTip 式"点亮+熄灭两击才算单击"正是用户痛点,这里一击到位;
		//   - Ctrl/Alt/Win 已点亮时退化为组合修饰键:只注入按下,弹起走熄灭路径
		//     (Ctrl+Shift 切输入法等纯修饰组合照常)。
		if checked {
			if stickyOn["btn_ctrl"] || stickyOn["btn_alt"] || stickyOn["btn_win"] {
				keyJobs <- func() { keyDown(uint32(xcc.VK_Shift), false) }
				stickyOn[name] = true
			} else {
				keyJobs <- func() {
					keyDown(uint32(xcc.VK_Shift), false)
					time.Sleep(50 * time.Millisecond)
					keyUp(uint32(xcc.VK_Shift), false)
				}
				stickyOn[name] = false
				if b := btnByName[name]; b != nil {
					// DLL 已点亮,拍回去;再触发的 checked=false 分支因未粘滞而无动作
					b.SetCheck(false)
					b.Redraw(false)
				}
			}
		} else if stickyOn[name] { // 组合模式熄灭:注入弹起
			keyJobs <- func() { keyUp(uint32(xcc.VK_Shift), false) }
			stickyOn[name] = false
		}
	case "btn_win":
		// Win:点亮即物理按下;winStartMenuDelay 内无组合键消费就自动弹起 ——
		// 无键间隔的按下+弹起,系统自己弹开始菜单,单击即得;时限内点 E 则
		// Win+E 照常组合(发完的弹起有键间隔,不会再弹菜单)。
		if checked {
			keyJobs <- func() { keyDown(uint32(xcc.VK_LWin), false) }
			stickyOn[name] = true
			winGen++
			g := winGen
			time.AfterFunc(winStartMenuDelay, func() {
				appUI.CallUiThreadEx(func(data int) int {
					if !stickyOn[name] || winGen != g {
						return 0 // 已被组合消费/手动熄灭/新一轮按住,本次超时作废
					}
					stickyOn[name] = false
					if b := btnByName[name]; b != nil {
						b.SetCheck(false)
						b.Redraw(false)
					}
					keyJobs <- func() { keyUp(uint32(xcc.VK_LWin), false) }
					return 0
				}, 0)
			})
		} else if stickyOn[name] { // 手动熄灭:物理弹起,无键间隔 → 开始菜单
			keyJobs <- func() { keyUp(uint32(xcc.VK_LWin), false) }
			stickyOn[name] = false
		}
	default:
		// 粘滞 = 物理按住(ctrl/alt):点亮立即注入按下,熄灭注入弹起。
		// 空点(两次点击间没按过任何键)对系统就是一个完整的物理单击。
		if stickyOn[name] && !checked {
			if v := stickyVKOf(name); v != 0 {
				keyJobs <- func() { keyUp(v, false) }
			}
		} else if checked {
			if v := stickyVKOf(name); v != 0 {
				keyJobs <- func() { keyDown(v, false) }
			}
		}
		stickyOn[name] = checked
	}
}

// onKeyClick 普通键按下 → 组装注入任务入队(按下声已在 LButtonDown 响过)。
// 粘滞修饰键在点亮时就已物理按下,这里只发字符本身,再依次弹起全部粘滞键。
func onKeyClick(name string) {
	// Fn 组合优先:媒体/导航键不吃 Shift 等修饰,直接发完释放全部粘滞
	if stickyOn["btn_fn"] {
		for _, k := range fnLayer {
			if k.name == name {
				vk := k.vk
				release := takeSticky()
				keyJobs <- func() {
					keyDown(vk, extendedVK[vk])
					time.Sleep(15 * time.Millisecond)
					keyUp(vk, extendedVK[vk])
					releaseKeyUp(release)
					unlightOnUI(release)
				}
				return
			}
		}
	}
	vk, ok := vkByName[name]
	if !ok {
		return
	}
	release := takeSticky()
	keyJobs <- func() {
		keyDown(vk, extendedVK[vk])
		time.Sleep(15 * time.Millisecond)
		keyUp(vk, extendedVK[vk])
		releaseKeyUp(release)
		unlightOnUI(release)
	}
}

// releaseKeyUp 弹起一批粘滞修饰键(字符/Fn 组合发完后调用)。
//
// names: takeSticky 收走的待熄灭按钮名.
func releaseKeyUp(names []string) {
	for _, n := range names {
		if v := stickyVKOf(n); v != 0 {
			keyUp(v, false)
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// unlightOnUI 把粘滞按钮熄灭回投到 UI 线程执行(worker 协程不直接碰 UI)。
//
// names: takeSticky 收走的待熄灭按钮名.
func unlightOnUI(names []string) {
	appUI.CallUiThreadEx(func(data int) int {
		unlightButtons(names)
		return 0
	}, 0)
}

// startCapsSync 后台定时把 Caps 按钮点亮态对齐系统真实大写锁定态,物理键盘
// 切换 Caps 不再失步。capsLockOn 必须在 UI 线程读(见其注释),所以读+比+设
// 整体放进 CallUiThreadEx;SetCheck 若触发 Button_Check,处理器比对发现与
// 系统态一致不会发键,无回环。
func startCapsSync(a *app.App) {
	go func() {
		for range time.Tick(300 * time.Millisecond) {
			a.CallUiThreadEx(func(data int) int {
				b := btnByName["btn_caps"]
				if b == nil {
					return 0
				}
				if on := capsLockOn(); b.IsCheck() != on {
					b.SetCheck(on)
					b.Redraw(false)
				}
				return 0
			}, 0)
		}
	}()
}

// bindKeys 遍历窗口元素树,按 name 分型挂事件(btn_ 前缀即契约,约定见 AGENTS.md):
//   - btn_min/btn_close:窗口控制钮,挂 BnClick 执行最小化/关闭,不播音不发键;
//   - check 型修饰键:按下播音,Button_Check 驱动粘滞/Caps 逻辑(两个事件是两个
//     不同的信号:声音跟"按下",逻辑跟"勾选态变化",时序由 DLL 定,合一不可靠);
//   - 其余键位:单个 LButtonDown 里播音+发键,按下即出。
func bindKeys(w *window.Window) {
	hwnd := w.GetHWND()
	var walk func(hObj int)
	walk = func(hObj int) {
		switch app.GetObjectType(hObj) {
		case xcc.XC_BUTTON:
			name := xc.XWidget_GetName(hObj)
			if !strings.HasPrefix(name, "btn_") {
				return
			}
			b := widget.NewButtonByHandle(hObj)
			// 布局解析器把 content 里的 { 当数据绑定语法(参考示例 {binding field=...}),
			// "{ [" 没有闭合 } 整串被吃掉不显示 → 运行时补设文本。
			if name == "btn_bracket_l" {
				b.SetText("{ [")
			}
			btnByName[name] = b
			switch {
			case name == "btn_min": // 窗口控制钮:buttonType 只管视觉,动作靠代码
				b.AddEvent_BnClick(func(hEle int, pbHandled *bool) int {
					wapi.ShowWindow(hwnd, xcc.SW_MINIMIZE)
					return 0
				})
			case name == "btn_close":
				b.AddEvent_BnClick(func(hEle int, pbHandled *bool) int {
					w.CloseWindow()
					return 0
				})
			case checkBtns[name]: // 修饰键:选中事件驱动,不挂 BnClick
				b.AddEvent_LButtonDown(func(hEle int, nFlags int, pPt *xc.POINT, pbHandled *bool) int {
					playKeySound()
					return 0
				})
				b.AddEvent_Button_Check(func(hEle int, bCheck bool, pbHandled *bool) int {
					onModCheck(name, bCheck)
					return 0
				})
			default: // 普通键位:一个事件干两件事,按下即播音+发键
				b.AddEvent_LButtonDown(func(hEle int, nFlags int, pPt *xc.POINT, pbHandled *bool) int {
					playKeySound()
					onKeyClick(name)
					return 0
				})
			}
		case xcc.XC_ELE, xcc.XC_ELE_LAYOUT: // 容器继续下钻(设计器当前只出这两类可嵌套)
			e := widget.NewElementByHandle(hObj)
			for i := int32(0); i < e.GetChildCount(); i++ {
				walk(e.GetChildByIndex(i))
			}
		}
	}
	for i := int32(0); i < w.GetChildCount(); i++ {
		walk(w.GetChildByIndex(i))
	}
}
