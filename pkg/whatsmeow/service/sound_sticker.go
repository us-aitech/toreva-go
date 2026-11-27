package whatsmeow_service

import (
	"context"
	"encoding/base64"
	"strings"

	storage_interfaces "github.com/evolution-foundation/evolution-go/pkg/storage/interfaces"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

const defaultStickerAudioMime = "audio/ogg; codecs=opus"

func unwrapAudioSticker(msg *waE2E.Message) *waE2E.Message {
	for {
		if inner := msg.GetAudioStickerMessage().GetMessage(); inner != nil {
			msg = inner
			continue
		}
		if inner := msg.GetLottieStickerMessage().GetMessage(); inner != nil {
			msg = inner
			continue
		}
		return msg
	}
}

func stickerAudioMimeAndExtension(mime string) (string, string) {
	if strings.TrimSpace(mime) == "" {
		mime = defaultStickerAudioMime
	}
	base := strings.TrimSpace(strings.SplitN(mime, ";", 2)[0])
	if base == "audio/mp4" {
		return mime, ".m4a"
	}
	return mime, getExtensionFromMimeType(base)
}

func applyStickerAudio(ctx context.Context, messageMap map[string]interface{}, msgID, mime string, data []byte, storage storage_interfaces.MediaStorage) error {
	mime, extension := stickerAudioMimeAndExtension(mime)
	if storage == nil {
		messageMap["stickerAudioBase64"] = base64.StdEncoding.EncodeToString(data)
		messageMap["stickerAudioMimetype"] = mime
		return nil
	}
	url, err := storage.Store(ctx, data, msgID+"-sticker-audio"+extension, mime)
	if err != nil {
		return err
	}
	messageMap["stickerAudioUrl"] = url
	messageMap["stickerAudioMimetype"] = mime
	return nil
}

func (mycli *MyClient) attachStickerAudio(ctx context.Context, messageMap map[string]interface{}, msgID string, sticker *waE2E.StickerMessage) {
	audio := sticker.GetAudioMessage()
	if audio == nil {
		return
	}
	messageMap["stickerHasAudio"] = true
	data, err := mycli.WAClient.Download(ctx, audio)
	if err != nil || len(data) == 0 {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to download sticker audio - ID: %s, Error: %v", mycli.userID, msgID, err)
		return
	}
	var storage storage_interfaces.MediaStorage
	if mycli.config.MinioEnabled {
		storage = mycli.mediaStorage
	}
	if err := applyStickerAudio(ctx, messageMap, msgID, audio.GetMimetype(), data, storage); err != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to store sticker audio - ID: %s, Error: %v", mycli.userID, msgID, err)
	}
}
