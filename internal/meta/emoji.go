package meta

import (
	"context"
	"fmt"

	"lazyfmess/internal/store"
)

// setEmojiTask changes the emoji of a conversation's button for everyone in it. The
// library the program stands on does not know this task; its label, its fields and its
// queue are the ones other open clients of Messenger send for it.
type setEmojiTask struct {
	ThreadKey   int64  `json:"thread_key"`
	CustomEmoji string `json:"custom_emoji"`
	AvatarKey   *int64 `json:"avatar_sticker_instruction_key_id"`
	SyncGroup   int64  `json:"sync_group"`
}

func (t *setEmojiTask) GetLabel() string { return "100003" }

func (t *setEmojiTask) Create() (any, string) { return t, "thread_quick_reaction" }

// SetEmoji changes the emoji of a conversation's button. It is the conversation's, not
// the person's: everyone in it gets the same one.
func (c *Client) SetEmoji(ctx context.Context, s *store.Store, thread store.Thread, emoji string) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if emoji == "" {
		emoji = ThumbEmoji
	}
	resp, err := c.mx.ExecuteTasks(ctx, &setEmojiTask{ThreadKey: thread.ID, CustomEmoji: emoji, SyncGroup: 1})
	if err != nil {
		return fmt.Errorf("set thread emoji: %w", err)
	}
	if resp != nil {
		for _, failed := range resp.LSHandleFailedTask {
			return fmt.Errorf("set thread emoji rejected: %s", failed.Message)
		}
	}
	return s.SetThreadEmoji(ctx, thread.ID, emoji)
}
