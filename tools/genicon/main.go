// genicon 生成应用图标（1.5 名称与图标）：
//   - build/appicon.png、build/windows/icon.ico：浅黄圆角便签 + 右上角斜插红色图钉 + 两条灰色横线
//     （ICO 含 16/24/32/48/64/256；16/24 px 去掉横线，只保留便签和图钉轮廓）
//   - internal/winsys/icons/tray-{light,dark}[-alert].ico：单色线条托盘图标，逾期时右下角加红点
//
// 用法：go run ./tools/genicon
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

const ss = 4 // 超采样倍数

type canvas struct {
	img *image.RGBA
	n   int
}

func newCanvas(size int) *canvas {
	n := size * ss
	return &canvas{img: image.NewRGBA(image.Rect(0, 0, n, n)), n: n}
}

func (c *canvas) f(v float64) float64 { return v * float64(c.n) }

func blend(dst *image.RGBA, x, y int, col color.NRGBA) {
	if x < 0 || y < 0 || x >= dst.Bounds().Dx() || y >= dst.Bounds().Dy() {
		return
	}
	i := dst.PixOffset(x, y)
	a := float64(col.A) / 255
	for k, v := range []uint8{col.R, col.G, col.B} {
		dst.Pix[i+k] = uint8(float64(dst.Pix[i+k])*(1-a) + float64(v)*a)
	}
	dst.Pix[i+3] = uint8(math.Min(255, float64(dst.Pix[i+3])+float64(col.A)*(1-float64(dst.Pix[i+3])/255)))
}

// fill 用 inside(x,y)（单位坐标 0..1）判定填充。
func (c *canvas) fill(col color.NRGBA, inside func(x, y float64) bool) {
	for py := 0; py < c.n; py++ {
		for px := 0; px < c.n; px++ {
			if inside((float64(px)+.5)/float64(c.n), (float64(py)+.5)/float64(c.n)) {
				blend(c.img, px, py, col)
			}
		}
	}
}

func roundRect(x0, y0, x1, y1, r float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		cx := math.Max(x0+r, math.Min(x, x1-r))
		cy := math.Max(y0+r, math.Min(y, y1-r))
		return math.Hypot(x-cx, y-cy) <= r
	}
}

func circle(cx, cy, r float64) func(x, y float64) bool {
	return func(x, y float64) bool { return math.Hypot(x-cx, y-cy) <= r }
}

// segment 是带圆头的线段（粗线）。
func segment(x0, y0, x1, y1, w float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		dx, dy := x1-x0, y1-y0
		t := ((x-x0)*dx + (y-y0)*dy) / (dx*dx + dy*dy)
		t = math.Max(0, math.Min(1, t))
		return math.Hypot(x-(x0+t*dx), y-(y0+t*dy)) <= w/2
	}
}

func (c *canvas) down(size int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a, n float64
			for j := 0; j < ss; j++ {
				for i := 0; i < ss; i++ {
					p := c.img.RGBAAt(x*ss+i, y*ss+j)
					r += float64(p.R)
					g += float64(p.G)
					b += float64(p.B)
					a += float64(p.A)
					n++
				}
			}
			if a > 0 {
				// 预乘 → 非预乘：blend 已按 alpha 混合到透明底，这里按平均 alpha 还原
				out.SetNRGBA(x, y, color.NRGBA{uint8(r / n * 255 / math.Max(a/n, 1)), uint8(g / n * 255 / math.Max(a/n, 1)), uint8(b / n * 255 / math.Max(a/n, 1)), uint8(a / n)})
			}
		}
	}
	return out
}

var (
	paper   = color.NRGBA{0xFF, 0xE9, 0x7A, 255}
	paperBd = color.NRGBA{0xE0, 0xB8, 0x3A, 255}
	lineCol = color.NRGBA{0x8A, 0x8A, 0x8A, 255}
	pinRed  = color.NRGBA{0xE5, 0x39, 0x35, 255}
	pinDark = color.NRGBA{0xB7, 0x1C, 0x1C, 255}
	pinNeed = color.NRGBA{0x77, 0x77, 0x77, 255}
)

func mainIcon(size int, lines bool) image.Image {
	c := newCanvas(size)
	// 便签：圆角方块
	c.fill(paperBd, roundRect(.08, .12, .92, .94, .13))
	inset := .035
	c.fill(paper, roundRect(.08+inset, .12+inset, .92-inset, .94-inset, .10))
	if lines { // 两条灰色横线代表事项
		c.fill(lineCol, segment(.22, .55, .66, .55, .07))
		c.fill(lineCol, segment(.22, .74, .56, .74, .07))
	}
	// 图钉：右上角斜插（从右上向左下 45°）
	c.fill(pinNeed, segment(.70, .30, .56, .44, .05))
	c.fill(pinDark, segment(.80, .20, .68, .32, .16))
	c.fill(pinRed, circle(.80, .20, .13))
	c.fill(color.NRGBA{255, 255, 255, 110}, circle(.765, .165, .04))
	return c.down(size)
}

// trayIcon：单色线条便签 + 图钉；alert 时右下角加红点。
func trayIcon(size int, ink color.NRGBA, alert bool) image.Image {
	c := newCanvas(size)
	w := 0.09
	stroke := func(x0, y0, x1, y1 float64) { c.fill(ink, segment(x0, y0, x1, y1, w)) }
	stroke(.14, .18, .86, .18)
	stroke(.14, .18, .14, .88)
	stroke(.14, .88, .86, .88)
	stroke(.86, .40, .86, .88)
	stroke(.30, .52, .66, .52)
	stroke(.30, .72, .58, .72)
	c.fill(ink, circle(.80, .20, .17))
	if alert {
		c.fill(color.NRGBA{0xE5, 0x39, 0x35, 255}, circle(.80, .80, .22))
	}
	return c.down(size)
}

func encodePNG(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		log.Fatal(err)
	}
	return b.Bytes()
}

// writeICO 写出 PNG 压缩条目的 ICO（Vista+，目标系统 Win10 满足）。
func writeICO(path string, imgs map[int]image.Image, order []int) {
	var head bytes.Buffer
	binary.Write(&head, binary.LittleEndian, [3]uint16{0, 1, uint16(len(order))})
	var data bytes.Buffer
	offset := 6 + 16*len(order)
	for _, s := range order {
		p := encodePNG(imgs[s])
		w := byte(s)
		if s >= 256 {
			w = 0
		}
		head.Write([]byte{w, w, 0, 0})
		binary.Write(&head, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&head, binary.LittleEndian, uint32(len(p)))
		binary.Write(&head, binary.LittleEndian, uint32(offset+data.Len()))
		data.Write(p)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, append(head.Bytes(), data.Bytes()...), 0o644); err != nil {
		log.Fatal(err)
	}
}

func main() {
	sizes := []int{16, 24, 32, 48, 64, 256}
	imgs := map[int]image.Image{}
	for _, s := range sizes {
		imgs[s] = mainIcon(s, s > 24) // 16/24 px 去掉横线
	}
	writeICO("build/windows/icon.ico", imgs, sizes)
	if err := os.WriteFile("build/appicon.png", encodePNG(mainIcon(512, true)), 0o644); err != nil {
		log.Fatal(err)
	}
	// 通知图标（Toast / AUMID 快捷方式）与主图标一致
	if err := os.WriteFile("internal/winsys/icons/notify.png", encodePNG(mainIcon(128, true)), 0o644); err != nil {
		log.Fatal(err)
	}
	// 托盘：浅色任务栏用深色线条，深色任务栏用浅色线条
	for name, ink := range map[string]color.NRGBA{"light": {0x20, 0x20, 0x20, 255}, "dark": {0xF2, 0xF2, 0xF2, 255}} {
		for _, alert := range []bool{false, true} {
			t := map[int]image.Image{}
			ts := []int{16, 24, 32}
			for _, s := range ts {
				t[s] = trayIcon(s, ink, alert)
			}
			suffix := ""
			if alert {
				suffix = "-alert"
			}
			writeICO("internal/winsys/icons/tray-"+name+suffix+".ico", t, ts)
		}
	}
	// 预览图（仅供人工检查，不参与构建）
	prev := image.NewNRGBA(image.Rect(0, 0, 256*2+64, 256))
	draw.Draw(prev, prev.Bounds(), image.NewUniform(color.NRGBA{0x60, 0x70, 0x80, 255}), image.Point{}, draw.Src)
	draw.Draw(prev, image.Rect(0, 0, 256, 256), imgs[256], image.Point{}, draw.Over)
	draw.Draw(prev, image.Rect(288, 96, 288+64, 96+64), mainIcon(64, true), image.Point{}, draw.Over)
	draw.Draw(prev, image.Rect(368, 112, 368+32, 112+32), mainIcon(32, true), image.Point{}, draw.Over)
	draw.Draw(prev, image.Rect(416, 120, 416+16, 120+16), mainIcon(16, false), image.Point{}, draw.Over)
	draw.Draw(prev, image.Rect(448, 112, 448+32, 112+32), trayIcon(32, color.NRGBA{0xF2, 0xF2, 0xF2, 255}, true), image.Point{}, draw.Over)
	if err := os.WriteFile(os.Getenv("ICON_PREVIEW"), encodePNG(prev), 0o644); err != nil && os.Getenv("ICON_PREVIEW") != "" {
		log.Fatal(err)
	}
}
