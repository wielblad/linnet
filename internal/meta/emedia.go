package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMediaTransport"
	"google.golang.org/protobuf/proto"
)

// MediaRef is what is needed to fetch and decrypt an attachment of an encrypted chat.
// It is stored with the message as JSON.
type MediaRef struct {
	Type      string `json:"type"` // image, sticker, video, audio or document
	Path      string `json:"path"`
	Key       []byte `json:"key"`
	SHA256    []byte `json:"sha256"`
	EncSHA256 []byte `json:"enc_sha256"`
}

// encryptedAttachment describes the attachment of an encrypted message.
type encryptedAttachment struct {
	Kind       string // image, video, audio or file, as stored with the message
	Mime       string
	Ref        string // MediaRef as JSON
	Caption    string
	Name       string
	DurationMs int64
}

// encryptedMedia reads the attachment of an encrypted message. paths maps attachment object
// ids to fresh download paths, as delivered with archived messages.
func encryptedMedia(msg *waConsumerApplication.ConsumerApplication, paths map[string]string) (encryptedAttachment, bool) {
	content := msg.GetPayload().GetContent()
	if content == nil {
		return encryptedAttachment{}, false
	}
	var transport *waMediaTransport.WAMediaTransport
	var out encryptedAttachment
	refType := ""
	switch typed := content.GetContent().(type) {
	case *waConsumerApplication.ConsumerApplication_Content_ImageMessage:
		decoded, err := typed.ImageMessage.Decode()
		if err != nil {
			return encryptedAttachment{}, false
		}
		transport, out.Kind, refType = decoded.GetIntegral().GetTransport(), "image", "image"
		out.Caption = typed.ImageMessage.GetCaption().GetText()
	case *waConsumerApplication.ConsumerApplication_Content_StickerMessage:
		decoded, err := typed.StickerMessage.Decode()
		if err != nil {
			return encryptedAttachment{}, false
		}
		transport, out.Kind, refType = decoded.GetIntegral().GetTransport(), "image", "sticker"
	case *waConsumerApplication.ConsumerApplication_Content_VideoMessage:
		decoded, err := typed.VideoMessage.Decode()
		if err != nil {
			return encryptedAttachment{}, false
		}
		transport, out.Kind, refType = decoded.GetIntegral().GetTransport(), "video", "video"
		out.Caption = typed.VideoMessage.GetCaption().GetText()
		out.DurationMs = int64(decoded.GetAncillary().GetSeconds()) * 1000
	case *waConsumerApplication.ConsumerApplication_Content_AudioMessage:
		decoded, err := typed.AudioMessage.Decode()
		if err != nil {
			return encryptedAttachment{}, false
		}
		transport, out.Kind, refType = decoded.GetIntegral().GetTransport(), "audio", "audio"
		out.DurationMs = int64(decoded.GetAncillary().GetSeconds()) * 1000
	case *waConsumerApplication.ConsumerApplication_Content_DocumentMessage:
		decoded, err := typed.DocumentMessage.Decode()
		if err != nil {
			return encryptedAttachment{}, false
		}
		transport, out.Kind, refType = decoded.GetIntegral().GetTransport(), "file", "document"
		out.Name = typed.DocumentMessage.GetFileName()
	default:
		return encryptedAttachment{}, false
	}
	integral := transport.GetIntegral()
	if integral == nil {
		return encryptedAttachment{}, false
	}
	path := integral.GetDirectPath()
	if fresh := paths[transport.GetAncillary().GetObjectID()]; fresh != "" {
		path = fresh
	}
	path = downloadPath(path)
	if path == "" || len(integral.GetMediaKey()) == 0 {
		return encryptedAttachment{}, false
	}
	raw, err := json.Marshal(MediaRef{
		Type:      refType,
		Path:      path,
		Key:       integral.GetMediaKey(),
		SHA256:    integral.GetFileSHA256(),
		EncSHA256: integral.GetFileEncSHA256(),
	})
	if err != nil {
		return encryptedAttachment{}, false
	}
	out.Ref, out.Mime = string(raw), transport.GetAncillary().GetMimetype()
	if out.Name == "" {
		out.Name = out.Kind
	}
	return out, true
}

// downloadPath reduces a full address to the path-and-query form the download expects.
func downloadPath(raw string) string {
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.RequestURI()
}

// DownloadEncryptedMedia fetches and decrypts the attachment described by a stored reference.
func (c *Client) DownloadEncryptedMedia(ctx context.Context, ref string) ([]byte, error) {
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}
	if c.e2ee == nil {
		return nil, fmt.Errorf("e2ee client is not connected")
	}
	var r MediaRef
	if err := json.Unmarshal([]byte(ref), &r); err != nil || r.Path == "" {
		return nil, fmt.Errorf("media reference not usable")
	}
	mediaType := whatsmeow.MediaImage
	switch r.Type {
	case "video":
		mediaType = whatsmeow.MediaVideo
	case "audio":
		mediaType = whatsmeow.MediaAudio
	case "document":
		mediaType = whatsmeow.MediaDocument
	}
	data, err := c.e2ee.DownloadFB(ctx, &waMediaTransport.WAMediaTransport_Integral{
		DirectPath:    proto.String(r.Path),
		MediaKey:      r.Key,
		FileSHA256:    r.SHA256,
		FileEncSHA256: r.EncSHA256,
	}, mediaType)
	if err != nil {
		return nil, fmt.Errorf("download encrypted media: %w", err)
	}
	return data, nil
}
