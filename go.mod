module github.com/langhuachuanshi/xcgui-virtual-keyboard

go 1.18

require github.com/langhuachuanshi/xcgui/xcgui v0.2645.2

require github.com/hajimehoshi/go-mp3 v0.3.4 // indirect

// 开发期指向本地绑定源码,改绑定即时生效;发布给别人用时可删掉本行,直接走 go get。
replace github.com/langhuachuanshi/xcgui/xcgui => ../xcgui/xcgui
