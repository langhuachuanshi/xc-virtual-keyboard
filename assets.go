// Package assets 内嵌设计器资产并提供内存 zip。
//
// go:embed 的匹配只能向下、不能跨出包目录,而 data/ 在仓库根,所以嵌入逻辑
// 放在这个根包里,src/ 的主程序 import 后按需取用。
package assets

import (
	"archive/zip"
	"bytes"
	"embed"
	"io/fs"
	"log"
	"strings"
)

//go:embed all:data
var files embed.FS

// BuildResZip 把 data/ 打进内存 zip:条目名用正斜杠、去掉 data/ 前缀,
// 与设计器导出的包结构一致;布局里的 @ 引用要求 resource.res、main.xml、图片
// 在同一个 zip 包上下文内解析,所以打成单包而不是逐文件注册。
func BuildResZip() []byte {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	err := fs.WalkDir(files, "data", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(path, ".xcproj") { // 工程文件不进运行时包
			return nil
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		w, err := zw.Create(strings.TrimPrefix(path, "data/"))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		log.Panicln("打包资源:", err)
	}
	if err := zw.Close(); err != nil {
		log.Panicln(err)
	}
	return buf.Bytes()
}

// KeySound 返回按键音 mp3 字节(嵌入自 data/sounds/key_press.mp3)。
func KeySound() []byte {
	b, err := files.ReadFile("data/sounds/key_press.mp3")
	if err != nil {
		log.Panicln("读取按键音:", err)
	}
	return b
}
