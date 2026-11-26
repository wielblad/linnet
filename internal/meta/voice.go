package meta

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMediaTransport"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

// VoiceMime is the kind of file a voice message is sent as: AAC sound in an MP4 file.
const VoiceMime = "audio/mp4"

// SendVoice sends a sound file as a voice message, not as a file to be fetched. wave is its
// loudness from beginning to end, each step 0 to 1; chats that are not encrypted carry it
// with the recording, for the wave their own programs draw.
func (c *Client) SendVoice(ctx context.Context, thread store.Thread, path string, duration time.Duration, wave []float64) (SendResult, error) {
	if err := c.waitReady(ctx); err != nil {
		return SendResult{}, err
	}
	a, err := readAttachment(path)
	if err != nil {
		return SendResult{}, err
	}
	a.mime, a.kind = VoiceMime, "audio"
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.sendE2EEVoice(ctx, thread, a, duration)
	}
	media := &httpclient.MercuryUploadMedia{Filename: a.name, MimeType: a.mime, MediaData: a.data, IsVoiceClip: true}
	if len(wave) > 0 {
		per := 1
		if s := int(duration.Seconds()); s > 0 {
			per = max(len(wave)/s, 1)
		}
		media.WaveformData = &httpclient.WaveformData{Amplitudes: wave, SamplingFrequency: per}
	}
	resp, err := c.mx.GetHTTP().SendMercuryUploadRequest(ctx, thread.ID, media)
	if err != nil {
		return SendResult{}, fmt.Errorf("upload voice message: %w", err)
	}
	if resp.Payload.RealMetadata == nil || resp.Payload.RealMetadata.GetFbId() == 0 {
		return SendResult{}, fmt.Errorf("upload voice message: no attachment id in response")
	}
	return c.sendLightspeed(ctx, &socket.SendMessageTask{
		ThreadId:        thread.ID,
		SendType:        table.MEDIA,
		AttachmentFBIds: []int64{resp.Payload.RealMetadata.GetFbId()},
	})
}

func (c *Client) sendE2EEVoice(ctx context.Context, thread store.Thread, a attachment, duration time.Duration) (SendResult, error) {
	if c.e2ee == nil {
		return SendResult{}, fmt.Errorf("e2ee client is not connected")
	}
	uploaded, err := c.e2ee.Upload(ctx, a.data, whatsmeow.MediaAudio)
	if err != nil {
		return SendResult{}, fmt.Errorf("upload e2ee voice message: %w", err)
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
			Mimetype:   proto.String(a.mime),
			ObjectID:   proto.String(uploaded.ObjectID),
		},
	}
	msg := &waConsumerApplication.ConsumerApplication_AudioMessage{PTT: proto.Bool(true)} // a voice message, not a song
	if err := msg.Set(&waMediaTransport.AudioTransport{
		Integral:  &waMediaTransport.AudioTransport_Integral{Transport: transport},
		Ancillary: &waMediaTransport.AudioTransport_Ancillary{Seconds: proto.Uint32(uint32(duration.Seconds()))},
	}); err != nil {
		return SendResult{}, fmt.Errorf("wrap voice message: %w", err)
	}
	content := &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_AudioMessage{AudioMessage: msg}}
	otid := methods.GenerateEpochID()
	msgID := strconv.FormatInt(otid, 10)
	resp, err := c.e2ee.SendFBMessage(ctx, chatJID(thread), consumerContent(content), nil, whatsmeow.SendRequestExtra{ID: msgID})
	if err != nil {
		return SendResult{}, fmt.Errorf("send e2ee voice message: %w", err)
	}
	ts := resp.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return SendResult{MessageID: msgID, Timestamp: ts}, nil
}
