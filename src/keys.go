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
// Shift 对齐物理键盘:点亮时数字/标点键帽切上档符号显示;空点熄灭(本轮没
// 消费过)直控输入法转换模式切中/英(WM_IME_CONTROL,确定性,不赌模拟按键
// 的时序)。Win 空点同理弹开始菜单。
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
	// imm32:中/英切换走 WM_IME_CONTROL 直控转换模式,不模拟 Shift 按键。
	procImmGetDefaultIMEWnd = syscall.NewLazyDLL("imm32.dll").NewProc("ImmGetDefaultIMEWnd")
)

const (
	keyEventF_ExtendedKey = 0x0001
	keyEventF_KeyUp       = 0x0002

	wmImeControl         = 0x0283
	imcGetConversionMode = 0x0001
	imcSetConversionMode = 0x0002
	imeCmodeNative       = 0x0001 // 转换模式的中/英位:1=中文 0=英文
)

// INPUT 联合体的 Go 对位:用最大成员 MOUSEINPUT 占位,保证 cbSize 与 C 布局
// 一致(x64=40, x86=28),KEYBDINPUT 经指针重解释写进同一块内存;字段顺序与
// 自然对齐照 MSDN,不可乱。
const inputKeyboard = 0 // INPUT type: 键盘事件

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

// fnLayer: Fn 粘滞时的映射层 —— 键名 → 改发的虚拟键 + Fn 点亮时临时显示的
// 标签(方向键补导航键缺口,对齐笔记本 Fn+方向惯例)。发送映射与标签提示
// 单表同源,加 Fn 键只改这里。
var fnLayer = []struct {
	name  string
	vk    uint32
	label string
}{
	{"btn_f1", vkVolumeMute, "Mute"},
	{"btn_f2", vkVolumeDown, "Vol-"},
	{"btn_f3", vkVolumeUp, "Vol+"},
	{"btn_f4", vkMediaPlayPause, "Play"},
	{"btn_f5", vkMediaPrev, "Prev"},
	{"btn_f6", vkMediaNext, "Next"},
	{"btn_f7", vkBrowserBack, "Back"},
	{"btn_f8", vkBrowserForward, "Fwd"},
	{"btn_up", uint32(xcc.VK_PageUp), "PgUp"},
	{"btn_down", uint32(xcc.VK_PageDown), "PgDn"},
	{"btn_left", uint32(xcc.VK_Home), "Home"},
	{"btn_right", uint32(xcc.VK_End), "End"},
}

// labelSwap 一条临时标签替换:键名 → 替换文本。
type labelSwap struct {
	name  string
	label string
}

// shiftLayer: Shift 点亮时数字/标点键帽切上档符号显示(字母恒大写不动),
// 熄灭还原 —— 对应物理键盘键帽的上档层。
var shiftLayer = []labelSwap{
	{"btn_grave", "~"},
	{"btn_1", "!"}, {"btn_2", "@"}, {"btn_3", "#"}, {"btn_4", "$"},
	{"btn_5", "%"}, {"btn_6", "^"}, {"btn_7", "&"}, {"btn_8", "*"},
	{"btn_9", "("}, {"btn_0", ")"},
	{"btn_minus", "_"}, {"btn_equal", "+"},
	{"btn_bracket_l", "{"}, {"btn_bracket_r", "}"},
	{"btn_backslash", "|"}, {"btn_semicolon", ":"}, {"btn_quote", `"`},
	{"btn_comma", "<"}, {"btn_period", ">"}, {"btn_slash", "?"},
}

// labelOrig 记录标签换掉前的原文本(Fn 与 Shift 共用,两集合键不相交),
// 还原后清空;空表 = 当前没有换过标签,还原操作幂等。
var labelOrig = map[string]string{}

// 粘滞态以 Go 变量为真源(由 Button_Check 回调同步,DLL 的勾选视觉自动跟随
// 用户点击)。winConsumed/shiftConsumed 标记本轮 Win/Shift 粘滞是否已被组合
// 键消费,决定空点熄灭时要不要补发单击(Win=开始菜单,Shift=输入法中英切换)。
var (
	stickyOn      = map[string]bool{}
	winConsumed   bool
	shiftConsumed bool
)

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

// toggleImeMode 翻转前台窗口输入法的中/英转换模式(等价物理 Shift 单击的
// 效果,但不赌模拟按键被 IME 认不认)。无 IME 的纯英文系统上自然无感。
func toggleImeMode() {
	fg := wapi.GetForegroundWindow()
	ime, _, _ := procImmGetDefaultIMEWnd.Call(fg)
	if ime == 0 {
		return
	}
	mode := wapi.SendMessageW(ime, wmImeControl, imcGetConversionMode, 0)
	wapi.SendMessageW(ime, wmImeControl, imcSetConversionMode, uintptr(mode)^imeCmodeNative)
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
// Button_Check 若触发,空点单击的条件(stickyOn 仍为真)不成立,不会误弹开始
// 菜单/误切输入法;restoreLabels 幂等,没换过标签时是空操作。
func unlightButtons(names []string) {
	for _, n := range names {
		if b := btnByName[n]; b != nil {
			b.SetCheck(false)
		}
	}
	restoreLabels()
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
	default:
		// 粘滞修饰键:记录挂起,不立即发键
		if name == "btn_win" {
			// Win 单击模拟:挂起后没按过任何键就再点熄灭 = 物理键盘单击 Win
			// (按下即弹开始菜单),粘滞模型只能在熄灭时机补发这一下。
			if !checked && stickyOn[name] && !winConsumed {
				keyJobs <- func() {
					keyDown(uint32(xcc.VK_LWin), false)
					time.Sleep(15 * time.Millisecond)
					keyUp(uint32(xcc.VK_LWin), false)
				}
			}
			if checked {
				winConsumed = false // 新一轮挂起,重新计"有没有按过键"
			}
		}
		if name == "btn_shift_l" {
			if checked {
				shiftConsumed = false // 新一轮挂起
				applyShiftLabels()    // 键帽切上档符号显示
			} else {
				// Shift 单击 = 输入法中/英切换:空点熄灭(本轮没消费过)直控前台
				// 输入法转换模式,物理键盘单击 Shift 的等价效果;纯英文系统无 IME,无感。
				if stickyOn[name] && !shiftConsumed {
					toggleImeMode()
				}
				restoreLabels()
			}
		}
		// Fn 点亮/熄灭时切换映射键的临时标签,当场提示这个键会发什么
		if name == "btn_fn" {
			if checked {
				applyFnLabels()
			} else {
				restoreLabels()
			}
		}
		stickyOn[name] = checked
	}
}

// onKeyClick 普通键按下 → 组装注入任务入队(按下声已在 LButtonDown 响过)。
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
	// 入队前在 UI 线程定格组合成员:挂着的修饰键按下 → 字符 → 反序释放,
	// 然后全部自动弹起。注入间隔延时:shell 热键(Win+E 等)对时序敏感。
	var stuck []uint32
	for _, m := range stickyMods {
		if stickyOn[m.name] {
			stuck = append(stuck, m.vk)
			switch m.name { // 被组合消费的键,空点熄灭时不再补发单击
			case "btn_win":
				winConsumed = true
			case "btn_shift_l":
				shiftConsumed = true
			}
		}
	}
	release := takeSticky()
	keyJobs <- func() {
		for _, v := range stuck {
			keyDown(v, false)
			time.Sleep(20 * time.Millisecond)
		}
		keyDown(vk, extendedVK[vk])
		time.Sleep(15 * time.Millisecond)
		keyUp(vk, extendedVK[vk])
		for i := len(stuck) - 1; i >= 0; i-- {
			keyUp(stuck[i], false)
			time.Sleep(10 * time.Millisecond)
		}
		unlightOnUI(release)
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

// applyFnLabels Fn 点亮:映射键换上功能标签并重绘。
func applyFnLabels() {
	for _, k := range fnLayer {
		if b := btnByName[k.name]; b != nil {
			labelOrig[k.name] = b.GetText()
			b.SetText(k.label)
			b.Redraw(false)
		}
	}
}

// applyShiftLabels Shift 点亮:数字/标点键帽切上档符号并重绘。
func applyShiftLabels() {
	for _, s := range shiftLayer {
		if b := btnByName[s.name]; b != nil {
			labelOrig[s.name] = b.GetText()
			b.SetText(s.label)
			b.Redraw(false)
		}
	}
}

// restoreLabels 还原全部被换掉的标签(Fn/Shift 共用一张回执表,两集合键不相
// 交)。幂等:没换过时是空操作。
func restoreLabels() {
	for n, orig := range labelOrig {
		if b := btnByName[n]; b != nil {
			b.SetText(orig)
			b.Redraw(false)
		}
	}
	labelOrig = map[string]string{}
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
