// 按键音:mp3 启动时一次解码为 PCM 常驻内存,按键即时重触发(掐断上一声、立刻
// 播下一声),播放路径零分配零落盘。winmm 的 waveOut 是系统自带组件,Win7 可用,
// 纯 syscall 无 CGO。程序退出时 closeKeySound 释放设备与常驻缓冲。
package main

import (
	"bytes"
	"io"
	"log"
	"syscall"
	"unsafe"

	"github.com/hajimehoshi/go-mp3"
)

var (
	winmm                      = syscall.NewLazyDLL("winmm.dll")
	procWaveOutOpen            = winmm.NewProc("waveOutOpen")
	procWaveOutPrepareHeader   = winmm.NewProc("waveOutPrepareHeader")
	procWaveOutUnprepareHeader = winmm.NewProc("waveOutUnprepareHeader")
	procWaveOutWrite           = winmm.NewProc("waveOutWrite")
	procWaveOutReset           = winmm.NewProc("waveOutReset")
	procWaveOutClose           = winmm.NewProc("waveOutClose")
)

// WAVE_MAPPER 让系统挑默认播放设备;CALLBACK_NULL 不用回调,写入即异步入队。
const (
	waveMapper    = 0xFFFFFFFF
	callbackNull  = 0
	waveFormatPCM = 1
)

// waveFmtEx 对应 WAVEFORMATEX(MSDN 布局,字段顺序与自然对齐一致,不可乱)。
type waveFmtEx struct {
	formatTag      uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	cbSize         uint16
}

// waveHdr 对应 WAVEHDR。
type waveHdr struct {
	lpData          uintptr
	dwBufferLength  uint32
	dwBytesRecorded uint32
	dwUser          uintptr
	dwFlags         uint32
	dwLoops         uint32
	lpNext          uintptr
	reserved        uintptr
}

// 常驻播放上下文:waveOut 在设备关闭前一直持有 hdr 与 PCM 数据的裸指针,
// 这些变量必须活到程序退出,故放包级变量,不能进局部作用域被 GC 回收。
var (
	hWaveOut   uintptr
	keyHdr     waveHdr
	keyPCM     []byte
	soundReady bool
)

// initKeySound 解码按键音并打开播放设备。失败只降级(静音,按键照常工作),
// 不终止程序:音效是锦上添花,不值得为它拦住键盘。
//
// mp3Bytes: 按键音 mp3 原始字节(嵌入资产 assets.KeySound()).
func initKeySound(mp3Bytes []byte) {
	dec, err := mp3.NewDecoder(bytes.NewReader(mp3Bytes))
	if err != nil {
		log.Println("按键音解码失败:", err)
		return
	}
	pcm, err := io.ReadAll(dec)
	if err != nil || len(pcm) == 0 {
		log.Println("按键音读取失败:", err)
		return
	}
	keyPCM = pcm
	// go-mp3 输出恒为 16bit 小端双声道交错(单声道源也复制成双声道),每帧 4 字节。
	sr := uint32(dec.SampleRate())
	f := waveFmtEx{
		formatTag:      waveFormatPCM,
		channels:       2,
		samplesPerSec:  sr,
		avgBytesPerSec: sr * 4,
		blockAlign:     4,
		bitsPerSample:  16,
	}
	r, _, _ := procWaveOutOpen.Call(
		uintptr(unsafe.Pointer(&hWaveOut)),
		uintptr(waveMapper),
		uintptr(unsafe.Pointer(&f)),
		0, // 无回调
		0,
		uintptr(callbackNull),
	)
	if r != 0 {
		log.Println("打开音频设备失败: MMSYSERR", r)
		return
	}
	soundReady = true
}

// playKeySound 重触发按键音:Reset 掐断上一声,立刻重写缓冲播出新的一声,
// 连打不堆积(新键盖旧键,键盘音该有的手感)。
func playKeySound() {
	if !soundReady {
		return
	}
	hdrSz := uintptr(unsafe.Sizeof(keyHdr))
	procWaveOutReset.Call(hWaveOut)
	// Reset 后旧 header 已标记完成,解除预备才能复用;首播时它从未预备过,
	// 这里报 WAVERR_UNPREPARED,无害,忽略返回值。
	procWaveOutUnprepareHeader.Call(hWaveOut, uintptr(unsafe.Pointer(&keyHdr)), hdrSz)
	keyHdr = waveHdr{
		lpData:         uintptr(unsafe.Pointer(&keyPCM[0])),
		dwBufferLength: uint32(len(keyPCM)),
	}
	procWaveOutPrepareHeader.Call(hWaveOut, uintptr(unsafe.Pointer(&keyHdr)), hdrSz)
	procWaveOutWrite.Call(hWaveOut, uintptr(unsafe.Pointer(&keyHdr)), hdrSz)
}

// closeKeySound 释放播放设备;close 后 hdr/pcm 指针失效,此后不再触碰。
// MSDN 建议顺序:Reset 停播归还缓冲 → Unprepare 解除预备 → Close。
func closeKeySound() {
	if !soundReady {
		return
	}
	soundReady = false
	hdrSz := uintptr(unsafe.Sizeof(keyHdr))
	procWaveOutReset.Call(hWaveOut)
	procWaveOutUnprepareHeader.Call(hWaveOut, uintptr(unsafe.Pointer(&keyHdr)), hdrSz)
	procWaveOutClose.Call(hWaveOut)
	hWaveOut = 0
}
