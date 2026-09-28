// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
)

func compressDanmakuTest(t *testing.T, version uint16, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if version == 2 {
		w := zlib.NewWriter(&b)
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		w := brotli.NewWriter(&b)
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return danmakuPacket(5, version, b.Bytes())
}
func TestDanmakuPackets(t *testing.T) {
	first := danmakuPacket(5, 0, []byte(`{"cmd":"one"}`))
	second := danmakuPacket(5, 1, []byte(`{"cmd":"two"}`))
	nested := compressDanmakuTest(t, 3, compressDanmakuTest(t, 2, append(first, second...)))
	var got []string
	budget := danmakuMaxExpanded
	err := danmakuDecode(append(nested, 1, 2), 0, &budget, func(op uint32, b []byte) error {
		if op != 5 {
			t.Fatalf("op %d", op)
		}
		got = append(got, string(b))
		return nil
	})
	if err == nil || len(got) != 2 || got[0] != `{"cmd":"one"}` || got[1] != `{"cmd":"two"}` {
		t.Fatalf("got %v, error %v", got, err)
	}
	sentinel := errors.New("storage failed")
	budget = danmakuMaxExpanded
	if err := danmakuDecode(nested, 0, &budget, func(uint32, []byte) error { return sentinel }); err != sentinel {
		t.Fatalf("consumer error changed: %v", err)
	}
}
func TestDanmakuPacketBounds(t *testing.T) {
	valid := danmakuPacket(5, 0, []byte(`{}`))
	for _, size := range []uint32{0, 15, uint32(len(valid) + 1), danmakuMaxPacket + 1} {
		data := append([]byte(nil), valid...)
		binary.BigEndian.PutUint32(data, size)
		budget := danmakuMaxExpanded
		if err := danmakuDecode(data, 0, &budget, func(uint32, []byte) error { t.Fatal("invalid packet delivered"); return nil }); err == nil {
			t.Fatalf("accepted length %d", size)
		}
	}
	data := compressDanmakuTest(t, 3, append(valid, valid...))
	budget := len(valid)
	count := 0
	if err := danmakuDecode(data, 0, &budget, func(uint32, []byte) error { count++; return nil }); err == nil || count != 1 {
		t.Fatalf("expansion bound: count=%d err=%v", count, err)
	}
	data = valid
	for i := 0; i <= danmakuMaxDepth; i++ {
		data = compressDanmakuTest(t, 2, data)
	}
	budget = danmakuMaxExpanded
	if err := danmakuDecode(data, 0, &budget, func(uint32, []byte) error { t.Fatal("excess nesting delivered"); return nil }); err == nil {
		t.Fatal("accepted excessive nesting")
	}
	// 同级包解压后共用同一限额。
	data = append(compressDanmakuTest(t, 2, valid), compressDanmakuTest(t, 3, valid)...)
	budget = len(valid)
	count = 0
	if err := danmakuDecode(data, 0, &budget, func(uint32, []byte) error { count++; return nil }); err == nil || count != 1 {
		t.Fatalf("cumulative budget: count=%d err=%v", count, err)
	}
}
func TestDanmakuWBI(t *testing.T) {
	key, err := danmakuWBIKey("https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png", "https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png")
	if err != nil || key != "ea1db124af3c7062474693fa704f4ff8" {
		t.Fatalf("key %q err %v", key, err)
	}
	query := url.Values{"foo": {"114"}, "bar": {"514"}, "baz": {"1919810"}}
	got := danmakuWBISign(query, key, time.Unix(1702204169, 0))
	// 使用 Python urllib.parse.urlencode + hashlib.md5 独立计算。
	if got.Get("w_rid") != "6149fdadf571698ca7e6a567265cd0ee" {
		t.Fatalf("signature %s", got.Get("w_rid"))
	}
	if query.Get("wts") != "" {
		t.Fatal("mutated input")
	}
	special := url.Values{"foo": {"a!'()* b~"}}
	got = danmakuWBISign(special, key, time.Unix(1, 0))
	if got.Get("foo") != "a b~" || special.Get("foo") != "a!'()* b~" {
		t.Fatal("filter mutated input or failed")
	}
}
