package tui

import (
	"context"
	"image"
	"io"
	"os"
	"strings"
	"testing"

	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"arcana-world/internal/termimage"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestInlineAvatarRecoversTerminalEvictionBeforeFallingBack(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.account = &domain.Account{UID: "1"}
	m.config.ActiveUID = "1"
	m.room = &domain.Room{ID: 2}
	avatar, err := termimage.NewThumbnail(image.NewNRGBA(image.Rect(0, 0, 32, 32)))
	if err != nil {
		t.Fatal(err)
	}
	m.moderation = &moderationState{client: m.client, account: m.config.ActiveUID, roomID: 2, action: moderationRemoveBlock, user: bili.RoomUser{UID: 7}, avatar: avatar}
	m.confirmModeration()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.inlineImage = inlineImage{available: true, cellWidth: 9, cellHeight: 20, thumbnail: avatar, id: 42, placementID: 1, uploaded: true, placed: true}
	m.View()
	response := func(id, placement uint32, payload string) {
		m.handleImageResponse(uv.KittyGraphicsEvent{Options: kitty.Options{ID: int(id), PlacementID: int(placement)}, Payload: []byte(payload)})
	}
	// 终端清屏回收图片后，第一次放置失败应重新上传，不能直接退化成字符头像。
	response(42, 1, "ENOENT:Put command refers to non-existent image")
	if strings.Contains(m.confirmationPrompt(), "▀") {
		t.Fatal("terminal eviction disabled native avatar rendering")
	}
	m.syncInlineImage()
	id := m.inlineImage.id
	response(id, 0, "OK")
	m.syncInlineImage()
	// 旧图的迟到错误不能使新图退化。
	response(42, 1, "EINVAL:stale placement")
	if strings.Contains(m.confirmationPrompt(), "▀") {
		t.Fatal("stale error disabled the replacement image")
	}
	// 同一次恢复仍失败时使用可见的字符预览，不能无限重新上传。
	response(id, m.inlineImage.placementID, "ENOENT:Put command refers to non-existent image")
	if !strings.Contains(m.confirmationPrompt(), "▀") {
		t.Fatal("repeated eviction left an empty avatar instead of falling back")
	}
}

func TestCloseReleasesInlineAvatarWithoutEventLoop(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.speaker = &chatSpeaker{cancel: cancel}
	_, valid := imageOutput("late image upload")
	pending := guardedImageOutput{valid: valid, data: "late image upload"}
	m.inlineImage = inlineImage{id: 42, uploadValid: valid}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	stdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = stdout }()
	err = m.Close()
	writer.Close()
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != termimage.Delete(42) || pending.String() != "" || ctx.Err() == nil {
		t.Fatalf("shutdown retained terminal image, queued output, or avatar request: output=%q pending=%q context=%v", output, pending.String(), ctx.Err())
	}
}
