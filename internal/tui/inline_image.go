package tui

import (
	"image"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"

	"arcana-world/internal/termimage"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const avatarRows = 4

// 图像只在终端应答成功后放置；业务切换会撤销尚未执行的原始输出。
type inlineImage struct {
	cellWidth, cellHeight       int
	probeID                     uint32
	available                   bool
	thumbnail                   *termimage.Thumbnail
	evicted                     *termimage.Thumbnail
	id, placementID             uint32
	uploaded, placed            bool
	uploadValid, placementValid *atomic.Bool
	rect                        image.Rectangle
	fallbackThumbnail           *termimage.Thumbnail
	fallbackCols                int
	fallback                    string
}

type imageTimeout struct{ id, placementID uint32 }

// Raw 的参数在 Bubble Tea 输出队列消费时才转成字符串，防止取消后迟到的上传或放置。
type guardedImageOutput struct {
	valid *atomic.Bool
	data  string
}

func (o guardedImageOutput) String() string {
	if !o.valid.Load() {
		return ""
	}
	return o.data
}

func imageOutput(data string) (tea.Cmd, *atomic.Bool) {
	valid := &atomic.Bool{}
	valid.Store(true)
	return tea.Raw(guardedImageOutput{valid: valid, data: data}), valid
}

func imageDeadline(id, placementID uint32) tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return imageTimeout{id, placementID} })
}

// 应答解析器使用 int，限制为 31 位以兼容 32 位平台。
func imageID() uint32 {
	for {
		if id := rand.Uint32() >> 1; id != 0 {
			return id
		}
	}
}

func (m *Model) initInlineImage() tea.Cmd {
	geometry := tea.Raw(ansi.WindowOp(16))
	if !termimage.GraphicsSupported() {
		return geometry
	}
	m.inlineImage.probeID = imageID()
	id := m.inlineImage.probeID
	return tea.Batch(geometry, tea.Raw(termimage.Probe(id)), imageDeadline(id, 0))
}

func (m *Model) confirmationAvatar() *termimage.Thumbnail {
	if !m.previewing && m.speakerValid(m.speaker) && (m.mode == "speaker" || m.mode == "confirm" && m.confirmAction == "speaker-apply") {
		return m.speaker.avatar
	}
	if m.mode != "confirm" || m.confirmAction != "moderation-apply" || !m.moderationValid(m.moderation) || m.previewing {
		return nil
	}
	return m.moderation.avatar
}

func (m *Model) avatarColumns() int {
	i := &m.inlineImage
	return termimage.CellGeometry(avatarRows, i.cellWidth, i.cellHeight)
}

func (m *Model) nativeAvatar() bool {
	i := &m.inlineImage
	return i.available && i.cellWidth > 0 && i.cellHeight > 0
}

func (m *Model) confirmationAvatarView() string {
	t := m.confirmationAvatar()
	cols := m.avatarColumns()
	if t == nil || cols > m.view.Width() {
		return ""
	}
	if m.nativeAvatar() {
		return strings.Repeat(strings.Repeat(" ", cols)+"\n", avatarRows-1) + strings.Repeat(" ", cols)
	}
	i := &m.inlineImage
	if i.fallbackThumbnail != t || i.fallbackCols != cols {
		i.fallbackThumbnail, i.fallbackCols = t, cols
		i.fallback = t.Fallback(cols, avatarRows)
	}
	return i.fallback
}

// 只显示完全处于确认框视口内、且没有被通知遮住的头像。
func (m *Model) inlineImageRect() image.Rectangle {
	cols := m.avatarColumns()
	if m.confirmationAvatar() == nil || !m.nativeAvatar() || cols > m.view.Width() || m.view.YOffset() != 0 || m.view.Height() < avatarRows {
		return image.Rectangle{}
	}
	r := image.Rect(m.pickerLeft, m.pickerTop, m.pickerLeft+cols, m.pickerTop+avatarRows)
	n := m.noticeLayer
	if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > m.width || r.Max.Y > m.height ||
		(n.content != "" && r.Overlaps(image.Rect(n.x, n.y, n.x+n.width, n.y+n.height))) {
		return image.Rectangle{}
	}
	return r
}
func (i *inlineImage) invalidatePlacement() {
	if i.placementValid != nil {
		i.placementValid.Store(false)
	}
	i.rect = image.Rectangle{}
}

func (m *Model) clearInlineImage() tea.Cmd {
	i := &m.inlineImage
	// 关闭或切换资料后释放字符预览，并让新会话独立处理终端缓存回收。
	avatar := m.confirmationAvatar()
	if i.fallbackThumbnail != avatar {
		i.fallbackThumbnail, i.fallbackCols, i.fallback = nil, 0, ""
	}
	if i.evicted != avatar {
		i.evicted = nil
	}
	if i.uploadValid != nil {
		i.uploadValid.Store(false)
	}
	if i.placementValid != nil {
		i.placementValid.Store(false)
	}
	id := i.id
	i.thumbnail, i.id, i.placementID = nil, 0, 0
	i.uploaded, i.placed = false, false
	i.uploadValid, i.placementValid = nil, nil
	i.rect = image.Rectangle{}
	if id == 0 {
		return nil
	}
	return tea.Raw(termimage.Delete(id))
}

// 在 View 更新布局之后调用，沿用指针的帧后刷新时机，不在 View 写终端。
func (m *Model) syncInlineImage() tea.Cmd {
	i := &m.inlineImage
	r := m.inlineImageRect()
	if r.Empty() {
		return m.clearInlineImage()
	}
	t := m.confirmationAvatar()
	if i.thumbnail != t {
		clear := m.clearInlineImage()
		i.thumbnail, i.id = t, imageID()
		upload, valid := imageOutput(t.Upload(i.id))
		i.uploadValid = valid
		return tea.Batch(clear, upload, imageDeadline(i.id, 0))
	}
	if !i.uploaded || i.rect == r {
		return nil
	}
	if i.placementValid != nil {
		i.placementValid.Store(false)
	}
	i.placementID++
	i.rect, i.placed = r, false
	place, valid := imageOutput(termimage.Place(i.id, i.placementID, r.Min.X, r.Min.Y, avatarRows))
	i.placementValid = valid
	return tea.Batch(place, imageDeadline(i.id, i.placementID))
}

func (m *Model) handleImageResponse(msg uv.KittyGraphicsEvent) tea.Cmd {
	i := &m.inlineImage
	id, placement := uint32(msg.Options.ID), uint32(msg.Options.PlacementID)
	ok := string(msg.Payload) == "OK"
	if id != 0 && id == i.probeID {
		i.probeID, i.available = 0, ok
		return nil
	}
	if id == 0 || id != i.id || placement != i.placementID {
		return nil
	}
	if !ok {
		// 缩放重绘的清屏可能回收终端图像；缓存失效时重新上传一次，不能误判为不支持协议。
		if placement != 0 && strings.HasPrefix(string(msg.Payload), "ENOENT:") && i.evicted != i.thumbnail {
			i.evicted = i.thumbnail
			return m.clearInlineImage()
		}
		i.available = false
		return m.clearInlineImage()
	}
	if placement == 0 {
		i.uploaded = true
	} else {
		i.placed = true
		i.evicted = nil
	}
	return nil
}

func (m *Model) handleImageTimeout(msg imageTimeout) tea.Cmd {
	i := &m.inlineImage
	if msg.id == i.probeID {
		i.probeID = 0
		return nil
	}
	if msg.id != i.id || msg.placementID != i.placementID ||
		(msg.placementID == 0 && i.uploaded) || (msg.placementID != 0 && i.placed) {
		return nil
	}
	i.available = false
	return m.clearInlineImage()
}
