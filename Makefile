# 懒獭 - 屏幕键盘 —— 日常命令(真源就是这个文件)

.PHONY: help build x86 run check clean

ifeq ($(OS),Windows_NT)
SHELL := C:/Program Files/Git/usr/bin/sh.exe
export PATH := /c/Program Files/Git/usr/bin:$(HOME)/go/bin:/c/Program Files/Go/bin:$(PATH)
endif

# 全仓 Go 只用 go1.20.14(同 lanta-pay):官方最后一个支持 Win7 的版本,Go 1.21 起编译的
# exe 在 Win7 上起不来(runtime 硬断,缺 KERNEL32 入口点)。兼容性由工具链内嵌 runtime 决定,
# 与在哪台机器上编译无关,所以本机 Win10/11 交叉出包即可。go.mod 的 go 指令也钉在 1.18 不能抬。
GO_WIN7 := go1.20.14

# 产物统一放 build/(不入库)。
BUILD_DIR := build
OUT := $(BUILD_DIR)/keyboard.exe

# 发布 ldflags:-s -w 去符号;-H=windowsgui 必须(GUI 子系统,不加会多一个控制台黑框,
# lanta-pay 2026-09-28 实踩);-trimpath 去本地路径。
LDFLAGS := -s -w -H=windowsgui

# UPX 加壳压发布体积(6.5MB -> ~2.3MB)。曾因"加壳版注入失效"短暂移除,后查明真凶是
# 注入需要管理员权限(已用 requireAdministrator 盾牌解决),加壳无辜,恢复。本地解压在
# upx-5.2.0-win64,换机器/换版本只改这里。
UPX := C:/Users/silas/upx-5.2.0-win64/upx.exe

build:
	@mkdir -p $(BUILD_DIR)
	GOTOOLCHAIN=$(GO_WIN7) go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT) ./src
	@go version $(OUT) 2>/dev/null | head -1 | sed 's/^/==> built with: /'
	$(UPX) --best $(OUT)
	@ls -l $(OUT)

# 32 位版(32 位 Win7 老机器):xcgui 绑定同时内嵌 x64/x86 两个 dll,GOARCH=386 直接可用。
x86:
	@mkdir -p $(BUILD_DIR)
	GOTOOLCHAIN=$(GO_WIN7) GOARCH=386 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/keyboard-x86.exe ./src
	@go version $(BUILD_DIR)/keyboard-x86.exe 2>/dev/null | head -1 | sed 's/^/==> built with: /'
	$(UPX) --best $(BUILD_DIR)/keyboard-x86.exe
	@ls -l $(BUILD_DIR)/keyboard-x86.exe

# 日常调试:走本机新 Go(1.26),只在本机 Win10/11 跑,不受 Win7 工具链约束。
run:
	go run ./src

# 自检:vet 与发布同口径(锁 Win7 工具链)。
check:
	GOTOOLCHAIN=$(GO_WIN7) go vet ./...
	@echo "==> check ok"

clean:
	rm -rf $(BUILD_DIR)

# HELP-BEGIN
# 懒獭 - 屏幕键盘 —— 常用命令
#
#   make build   发布构建 -> build/keyboard.exe(x64,UPX 加壳,Win7 x64 ~ Win11)
#   make x86     32 位版 -> build/keyboard-x86.exe(UPX 加壳,32 位 Win7)
#   make run     日常调试(go run,本机新 Go,仅本机 Win10/11)
#   make check   go vet(与发布同口径,锁 1.20.14)
#   make clean   清 build/
#
# HELP-END
#
# help 走 sed 打印上面注释,不写成 @echo:make 会把 recipe 按系统 ANSI 码页转给 sh,
# 中文在命令行里被二次编码成乱码(lanta-pay 2026-09-27 实测;注释里的中文是原字节,sed 原样打出)。
help:
	@sed -n '/^# HELP-BEGIN/,/^# HELP-END/p' Makefile | sed '1d;$$d' | sed 's/^#//; s/^ //'
