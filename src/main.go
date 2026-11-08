// 懒獭 - 屏幕键盘:程序入口。
//
// 资产链:data/(仓库根)是设计器输出 —— main.xml 布局、resource.res(颜色变量/字体/图片
// 注册)、default.css(全局样式,内部 @resFile 引 resource.res)。资产由根包 assets 全量嵌入,
// 启动时打成内存 zip 再用 ZipMem 系列 API 加载,发布即单 exe,不依赖磁盘文件。
package main

import (
	"syscall"
	"unsafe"

	assets "github.com/langhuachuanshi/xcgui-virtual-keyboard"

	"github.com/langhuachuanshi/xcgui/xcgui/app"
	"github.com/langhuachuanshi/xcgui/xcgui/wapi"
	"github.com/langhuachuanshi/xcgui/xcgui/window"
	"github.com/langhuachuanshi/xcgui/xcgui/xc"
	"github.com/langhuachuanshi/xcgui/xcgui/xcc"
)

func main() {
	// 1. 初始化 UI 库(失败打印原因并退出)
	app.InitOrExit()

	// 系统分界:Win10+ 启用 D2D 硬件加速;Win7 不启用(炫彩库支持 Win7,
	// 但 SetShadowInfo 在"非 D2D + 自适应 DPI"下 DPI 计算有 BUG,Win7 分支绝不调用它)。
	win10 := isWin10Plus()
	a := app.New(win10)
	a.EnableAutoDPI(true).EnableDPI(true)

	// 2. 内存 zip 加载,顺序不能反:先注册资源(布局解析时要能查到 @ 引用),
	//    再应用 default.css(全局字体字色;其 @resFile 会再次载 res,幂等)。
	res := assets.BuildResZip()
	a.LoadResourceZipMem(res, "resource.res", "")
	xc.XC_LoadStyleZipMem(res, "default.css", "")

	// 3. 由布局创建窗口
	w := window.NewByLayoutZipMem(res, "main.xml", "", 0, 0)
	if win10 {
		w.SetTransparentType(xcc.Window_Transparent_Shadow)
		w.SetShadowInfo(8, 255, 8, false, 0)
	}
	// 不抢焦点(TODO 既定 WS_EX_NOACTIVATE 方案,待真机验证):鼠标点击不激活本
	// 窗口,模拟的按键才能进目标窗口。WS_EX_NOACTIVATE 默认连任务栏图标一起藏
	// (MSDN:要任务栏须加 WS_EX_APPWINDOW),补上;两个扩展样式 Win7 起都支持。
	hwnd := w.GetHWND()
	wapi.SetWindowLongPtrW(hwnd, wapi.GWL_EXSTYLE,
		wapi.GetWindowLongPtrW(hwnd, wapi.GWL_EXSTYLE)|int(xcc.WS_EX_NOACTIVATE|xcc.WS_EX_APPWINDOW))

	// 4. 全部按键:按下播音、单击模拟输入(btn_min/btn_close 是窗口控制钮,不挂)。
	initKeySound(assets.KeySound())
	startKeyWorker(a) // 注入序列后台串行执行,毫秒级延时不再阻塞 UI 线程
	bindKeys(w)
	startCapsSync(a) // Caps 点亮态 300ms 对齐系统真实态,物理键盘切换不失步

	// 5. 显示窗口 → 消息循环 → 释放 UI 库
	w.Show(true)
	// 置顶:NOACTIVATE 下点击永远不会把窗口提前(这正是按键能进目标窗口的代价),
	// 屏幕键盘的惯例是常驻顶层,用户随时够得着。
	wapi.SetWindowPos(hwnd, wapi.HWND_TOPMOST, 0, 0, 0, 0, wapi.SWP_NOMOVE|wapi.SWP_NOSIZE)
	a.Run()
	closeKeySound()
	a.Exit()
}

// isWin10Plus 判断系统是否 Windows 10 及以上。
// 用 RtlGetVersion 取真实版本号:它不受进程兼容性声明影响,Win7 上也一样准。
func isWin10Plus() bool {
	type osVersionInfoW struct {
		OSVersionInfoSize uint32
		MajorVersion      uint32
		MinorVersion      uint32
		BuildNumber       uint32
		PlatformID        uint32
		CSDVersion        [128]uint16
	}
	var vi osVersionInfoW
	vi.OSVersionInfoSize = uint32(unsafe.Sizeof(vi))
	r1, _, _ := syscall.NewLazyDLL(`ntdll.dll`).NewProc(`RtlGetVersion`).Call(uintptr(unsafe.Pointer(&vi)))
	return r1 == 0 && vi.MajorVersion >= 10
}
