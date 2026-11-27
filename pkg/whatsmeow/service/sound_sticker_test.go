package whatsmeow_service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestUnwrapAudioSticker_Wrapped(t *testing.T) {
	sticker := &waE2E.StickerMessage{Mimetype: stringPtr("image/webp")}
	msg := &waE2E.Message{AudioStickerMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{StickerMessage: sticker}}}

	if got := unwrapAudioSticker(msg).GetStickerMessage(); got != sticker {
		t.Fatalf("expected inner sticker, got %v", got)
	}
}

func TestUnwrapAudioSticker_WrappedLottie(t *testing.T) {
	sticker := &waE2E.StickerMessage{Mimetype: stringPtr("application/was")}
	msg := &waE2E.Message{AudioStickerMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
		LottieStickerMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{StickerMessage: sticker}},
	}}}

	if got := unwrapAudioSticker(msg).GetStickerMessage(); got != sticker {
		t.Fatalf("expected inner sticker, got %v", got)
	}
}

func TestUnwrapAudioSticker_Plain(t *testing.T) {
	msg := &waE2E.Message{StickerMessage: &waE2E.StickerMessage{}}

	if got := unwrapAudioSticker(msg); got != msg {
		t.Fatal("expected same message pointer")
	}
}

func TestUnwrapAudioSticker_Nil(t *testing.T) {
	if got := unwrapAudioSticker(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestUnwrapAudioSticker_Fixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/sound_sticker_raw.json")
	if err != nil {
		t.Fatal(err)
	}
	var msg waE2E.Message
	if err := protojson.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}

	got := unwrapAudioSticker(&msg)

	if got.GetStickerMessage().GetAudioMessage() == nil {
		t.Fatal("expected sticker with nested audio")
	}
	if typ := utils.GetMessageType(got); !strings.HasPrefix(typ, "sticker") {
		t.Fatalf("expected sticker type, got %q", typ)
	}
}

func TestStickerAudioMimeAndExtension(t *testing.T) {
	cases := []struct{ in, mime, ext string }{
		{"audio/ogg; codecs=opus", "audio/ogg; codecs=opus", ".ogg"},
		{"audio/mp4", "audio/mp4", ".m4a"},
		{"", "audio/ogg; codecs=opus", ".ogg"},
		{"audio/mpeg", "audio/mpeg", ".mp3"},
	}
	for _, c := range cases {
		mime, ext := stickerAudioMimeAndExtension(c.in)
		if mime != c.mime || ext != c.ext {
			t.Errorf("%q: got (%q, %q), want (%q, %q)", c.in, mime, ext, c.mime, c.ext)
		}
	}
}

type fakeMediaStorage struct {
	fileName, contentType string
	data                  []byte
	err                   error
}

func (f *fakeMediaStorage) Store(_ context.Context, data []byte, fileName, contentType string) (string, error) {
	f.data, f.fileName, f.contentType = data, fileName, contentType
	if f.err != nil {
		return "", f.err
	}
	return "http://minio:9000/evolution/" + fileName, nil
}

func (f *fakeMediaStorage) Delete(context.Context, string) error { return nil }

func (f *fakeMediaStorage) GetURL(context.Context, string) (string, error) { return "", nil }

func TestApplyStickerAudio_StoresInMinio(t *testing.T) {
	storage := &fakeMediaStorage{}
	messageMap := map[string]interface{}{}

	err := applyStickerAudio(context.Background(), messageMap, "MSGID", "audio/mp4", []byte("abc"), storage)

	if err != nil {
		t.Fatal(err)
	}
	if storage.fileName != "MSGID-sticker-audio.m4a" || storage.contentType != "audio/mp4" {
		t.Fatalf("unexpected store call: %q %q", storage.fileName, storage.contentType)
	}
	if messageMap["stickerAudioUrl"] != "http://minio:9000/evolution/MSGID-sticker-audio.m4a" || messageMap["stickerAudioMimetype"] != "audio/mp4" {
		t.Fatalf("unexpected map: %v", messageMap)
	}
	if _, ok := messageMap["stickerAudioBase64"]; ok {
		t.Fatal("base64 must not be set when stored")
	}
}

func TestApplyStickerAudio_Base64WithoutStorage(t *testing.T) {
	messageMap := map[string]interface{}{}

	err := applyStickerAudio(context.Background(), messageMap, "MSGID", "", []byte("abc"), nil)

	if err != nil {
		t.Fatal(err)
	}
	if messageMap["stickerAudioBase64"] != base64.StdEncoding.EncodeToString([]byte("abc")) || messageMap["stickerAudioMimetype"] != "audio/ogg; codecs=opus" {
		t.Fatalf("unexpected map: %v", messageMap)
	}
	if _, ok := messageMap["stickerAudioUrl"]; ok {
		t.Fatal("url must not be set without storage")
	}
}

func TestApplyStickerAudio_StoreErrorLeavesMapUntouched(t *testing.T) {
	messageMap := map[string]interface{}{"mediaUrl": "x"}

	err := applyStickerAudio(context.Background(), messageMap, "MSGID", "audio/mp4", []byte("abc"), &fakeMediaStorage{err: errors.New("boom")})

	if err == nil {
		t.Fatal("expected error")
	}
	if len(messageMap) != 1 {
		t.Fatalf("map must stay untouched: %v", messageMap)
	}
}

func TestUnwrapAudioSticker_JSONHasStickerAtTop(t *testing.T) {
	raw, _ := os.ReadFile("testdata/sound_sticker_raw.json")
	var msg waE2E.Message
	if err := protojson.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(unwrapAudioSticker(&msg))
	if err != nil {
		t.Fatal(err)
	}
	var node map[string]interface{}
	json.Unmarshal(encoded, &node)

	if _, ok := node["stickerMessage"]; !ok {
		t.Fatalf("expected stickerMessage at top, got keys %v", node)
	}
	if _, ok := node["audioStickerMessage"]; ok {
		t.Fatal("wrapper must be gone")
	}
}
