package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMediaTransport"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

// The questions Messenger's own sticker window asks: which packs the account has, and what
// is in one of them. The library does not know them; their names, numbers and what they
// are asked with were read from Messenger's page (26 November 2026).
const (
	stickerPacksQuery = "CometStickerPickerCardQuery"
	stickerPackQuery  = "CometStickerPickerPackContentRootQuery"
)

func init() {
	graphql.GraphQLDocs[stickerPacksQuery] = graphql.GraphQLDoc{DocID: "28475074195489368", FriendlyName: stickerPacksQuery}
	graphql.GraphQLDocs[stickerPackQuery] = graphql.GraphQLDoc{DocID: "25236323532672635", FriendlyName: stickerPackQuery}
}

// StickerPack is one pack of stickers the account has in its tray.
type StickerPack struct {
	ID, Name, Thumb string
}

// Sticker is one sticker of a pack.
type Sticker struct {
	ID            int64
	Label         string
	Image         string // its picture, still
	Width, Height int
}

type gqlErrors []struct {
	Message string `json:"message"`
}

func firstLine(body []byte) []byte {
	if i := bytes.IndexByte(body, '\n'); i > 0 { // more than one answer may follow the first
		return body[:i]
	}
	return body
}

// StickerPacks asks which packs of stickers the account has.
func (c *Client) StickerPacks(ctx context.Context) ([]StickerPack, error) {
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}
	_, body, err := c.mx.GetHTTP().MakeGraphQLRequest(ctx, stickerPacksQuery, map[string]any{
		"feedbackID": nil, "scale": 1, "stickerInterface": "MESSAGES",
	})
	if err != nil {
		return nil, fmt.Errorf("sticker packs: %w", err)
	}
	return ParseStickerPacks(body)
}

// ParseStickerPacks reads the answer to the question about packs.
func ParseStickerPacks(body []byte) ([]StickerPack, error) {
	var resp struct {
		Data struct {
			Plugins struct {
				Picker struct {
					Store struct {
						Packs struct {
							Edges []struct {
								Node struct {
									ID    string `json:"id"`
									Name  string `json:"name"`
									Thumb struct {
										URI string `json:"uri"`
									} `json:"thumbnail_image"`
								} `json:"node"`
							} `json:"edges"`
						} `json:"tray_packs"`
					} `json:"sticker_store"`
				} `json:"sticker_picker"`
			} `json:"picker_plugins"`
		} `json:"data"`
		Errors gqlErrors `json:"errors"`
	}
	if err := json.Unmarshal(firstLine(body), &resp); err != nil {
		return nil, fmt.Errorf("sticker packs: answer not understood: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("sticker packs: %s", resp.Errors[0].Message)
	}
	var out []StickerPack
	for _, e := range resp.Data.Plugins.Picker.Store.Packs.Edges {
		if n := e.Node; n.ID != "" {
			out = append(out, StickerPack{ID: n.ID, Name: n.Name, Thumb: n.Thumb.URI})
		}
	}
	return out, nil
}

// Stickers asks what is in one pack.
func (c *Client) Stickers(ctx context.Context, packID string) ([]Sticker, error) {
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}
	_, body, err := c.mx.GetHTTP().MakeGraphQLRequest(ctx, stickerPackQuery, map[string]any{
		"packID": packID, "stickerWidth": 128, "stickerHeight": 128, "scale": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("stickers: %w", err)
	}
	return ParseStickers(body)
}

// ParseStickers reads the answer to the question about one pack.
func ParseStickers(body []byte) ([]Sticker, error) {
	var resp struct {
		Data struct {
			Pack struct {
				Stickers struct {
					Edges []struct {
						Node struct {
							ID    string `json:"id"`
							Label string `json:"label"`
							Image struct {
								URI    string `json:"uri"`
								Width  int    `json:"width"`
								Height int    `json:"height"`
							} `json:"image"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"stickers"`
			} `json:"sticker_pack"`
		} `json:"data"`
		Errors gqlErrors `json:"errors"`
	}
	if err := json.Unmarshal(firstLine(body), &resp); err != nil {
		return nil, fmt.Errorf("stickers: answer not understood: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("stickers: %s", resp.Errors[0].Message)
	}
	var out []Sticker
	for _, e := range resp.Data.Pack.Stickers.Edges {
		n := e.Node
		id, err := strconv.ParseInt(n.ID, 10, 64)
		if err != nil || id == 0 || !strings.HasPrefix(n.Image.URI, "https://") {
			continue
		}
		w, h := n.Image.Width, n.Image.Height
		if w <= 0 || h <= 0 {
			w, h = 128, 128
		}
		out = append(out, Sticker{ID: id, Label: n.Label, Image: n.Image.URI, Width: w, Height: h})
	}
	return out, nil
}

// SendSticker sends a sticker. A chat that is not encrypted is told its number and shows
// Messenger's own copy of it; an encrypted chat has to be sent the picture itself, which
// is read from path.
func (c *Client) SendSticker(ctx context.Context, thread store.Thread, st Sticker, path string) (SendResult, error) {
	if err := c.waitReady(ctx); err != nil {
		return SendResult{}, err
	}
	if !table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.sendLightspeed(ctx, &socket.SendMessageTask{
			ThreadId:  thread.ID,
			SendType:  table.STICKER,
			StickerId: st.ID,
		})
	}
	if c.e2ee == nil {
		return SendResult{}, fmt.Errorf("e2ee client is not connected")
	}
	a, err := readAttachment(path)
	if err != nil {
		return SendResult{}, err
	}
	w, h := st.Width, st.Height
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(a.data)); err == nil {
		w, h = cfg.Width, cfg.Height
	}
	mime := a.mime
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	uploaded, err := c.e2ee.Upload(ctx, a.data, whatsmeow.MediaImage)
	if err != nil {
		return SendResult{}, fmt.Errorf("upload e2ee sticker: %w", err)
	}
	transport := &waMediaTransport.WAMediaTransport{
		Integral: &waMediaTransport.WAMediaTransport_Integral{
			FileSHA256:        uploaded.FileSHA256,
			MediaKey:          uploaded.MediaKey,
			FileEncSHA256:     uploaded.FileEncSHA256,
			DirectPath:        proto.String(uploaded.DirectPath),
			MediaKeyTimestamp: proto.Int64(time.Now().Unix()),
		},
		Ancillary: &waMediaTransport.WAMediaTransport_Ancillary{
			FileLength: proto.Uint64(uint64(len(a.data))),
			Mimetype:   proto.String(mime),
			Thumbnail: &waMediaTransport.WAMediaTransport_Ancillary_Thumbnail{
				ThumbnailWidth:  proto.Uint32(uint32(w)),
				ThumbnailHeight: proto.Uint32(uint32(h)),
			},
			ObjectID: proto.String(uploaded.ObjectID),
		},
	}
	msg := &waConsumerApplication.ConsumerApplication_StickerMessage{}
	if err := msg.Set(&waMediaTransport.StickerTransport{
		Integral: &waMediaTransport.StickerTransport_Integral{Transport: transport},
		Ancillary: &waMediaTransport.StickerTransport_Ancillary{
			Height:             proto.Uint32(uint32(h)),
			Width:              proto.Uint32(uint32(w)),
			AccessibilityLabel: proto.String(st.Label),
		},
	}); err != nil {
		return SendResult{}, fmt.Errorf("wrap sticker: %w", err)
	}
	content := &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_StickerMessage{StickerMessage: msg}}
	otid := methods.GenerateEpochID()
	msgID := strconv.FormatInt(otid, 10)
	resp, err := c.e2ee.SendFBMessage(ctx, chatJID(thread), consumerContent(content), nil, whatsmeow.SendRequestExtra{ID: msgID})
	if err != nil {
		return SendResult{}, fmt.Errorf("send e2ee sticker: %w", err)
	}
	ts := resp.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return SendResult{MessageID: msgID, Timestamp: ts}, nil
}
