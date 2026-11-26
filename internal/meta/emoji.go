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

// EmojiOnly reports whether a message is nothing but emoji, a few at most. Only such a
// message is ever shown large: Messenger marks ordinary words with a size too, and they
// must stay the size of words.
func EmojiOnly(text string) bool {
	seen, n := false, 0
	for _, r := range text {
		n++
		switch {
		case r == 0x200D || r == 0xFE0F || r == 0x20E3 || (r >= 0x1F3FB && r <= 0x1F3FF) || (r >= 0xE0020 && r <= 0xE007F):
			// joiners, the colour form, keycaps, skin tones, flag tags: parts of an emoji
		case (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) || (r >= 0x2300 && r <= 0x23FF) ||
			(r >= 0x2B00 && r <= 0x2BFF) || (r >= 0x2190 && r <= 0x21FF) || r == 0x3030 || r == 0x303D ||
			r == 0x3297 || r == 0x3299 || r == 0x00A9 || r == 0x00AE || r == 0x2122:
			seen = true
		default:
			return false
		}
	}
	return seen && n <= 24
}
