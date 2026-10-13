package termimage

import "image"

// imageViewport 仅改变取样范围，不复制像素或修改待上传图片。
type imageViewport struct {
	image.Image
	rect image.Rectangle
	zoom int
}

func (p *imageViewport) Bounds() image.Rectangle { return p.rect }

func (p *imageViewport) key(key byte) bool {
	before, zoom := p.rect, p.zoom
	bounds := p.Image.Bounds()
	switch key {
	case '+', '=':
		if p.zoom*2 > min(8, bounds.Dx(), bounds.Dy()) {
			return false
		}
		p.zoom *= 2
	case '-':
		p.zoom = max(1, p.zoom/2)
	case '0':
		p.zoom, p.rect = 1, bounds
	case 'h':
		p.rect = p.rect.Add(image.Pt(-max(1, p.rect.Dx()/8), 0))
	case 'l':
		p.rect = p.rect.Add(image.Pt(max(1, p.rect.Dx()/8), 0))
	case 'k':
		p.rect = p.rect.Add(image.Pt(0, -max(1, p.rect.Dy()/8)))
	case 'j':
		p.rect = p.rect.Add(image.Pt(0, max(1, p.rect.Dy()/8)))
	default:
		return false
	}
	if p.zoom != zoom && key != '0' {
		// 缩放围绕当前视口中心；到达边缘时只平移，不缩小可视区域。
		w, h := bounds.Dx()/p.zoom, bounds.Dy()/p.zoom
		x := p.rect.Min.X + (p.rect.Dx()-w)/2
		y := p.rect.Min.Y + (p.rect.Dy()-h)/2
		p.rect = image.Rect(x, y, x+w, y+h)
	}
	x := max(bounds.Min.X, min(p.rect.Min.X, bounds.Max.X-p.rect.Dx()))
	y := max(bounds.Min.Y, min(p.rect.Min.Y, bounds.Max.Y-p.rect.Dy()))
	p.rect = p.rect.Add(image.Pt(x-p.rect.Min.X, y-p.rect.Min.Y))
	return p.rect != before
}
