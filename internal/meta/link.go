package meta

import (
	"net/url"
	"strings"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow/proto/armadilloutil"
	"go.mau.fi/whatsmeow/proto/waArmadilloApplication"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMediaTransport"
)

// sharePreviewVersion is the version of the picture carried by a share in an encrypted chat.
const sharePreviewVersion = 2

// linkPreview is the preview Messenger attaches to a message that contains a link. It is
// stored with the message, so that showing it never needs the web page itself.
type linkPreview struct {
	URL   string // target address
	Title string
	Image string // address of the picture (chats that are not encrypted)
	Ref   string // MediaRef of the picture as JSON (encrypted chats)
}

// encryptedBody is what is stored from the content of an encrypted message.
type encryptedBody struct {
	Text string
	Att  encryptedAttachment
	Link linkPreview
}

// encryptedContent reads the content of an encrypted message, live or archived: a chat
// message with its attachment and link preview, or a share. paths is as in encryptedMedia.
func encryptedContent(message any, paths map[string]string) (encryptedBody, bool) {
	switch typed := message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		text, ok := consumerText(typed)
		if !ok {
			return encryptedBody{}, false
		}
		att, hasMedia := encryptedMedia(typed, paths)
		if hasMedia {
			text = att.Caption // the attachment itself is shown, not a placeholder word
		}
		return encryptedBody{Text: text, Att: att, Link: consumerLink(typed, paths)}, true
	case *waArmadilloApplication.Armadillo:
		text, link, ok := sharedLink(typed, paths)
		return encryptedBody{Text: text, Link: link}, ok
	}
	return encryptedBody{}, false
}

// linkFromWrapped reads the link preview of a message from a chat that is not encrypted.
// It takes the first share that has a picture or a title.
func linkFromWrapped(msg *table.WrappedMessage) linkPreview {
	if msg == nil {
		return linkPreview{}
	}
	for _, xma := range msg.XMAAttachments {
		if xma == nil || xma.LSInsertXmaAttachment == nil {
			continue
		}
		image := firstNonEmpty(webAddress(xma.PreviewUrl), webAddress(xma.PreviewUrlLarge), webAddress(xma.ImageUrl))
		if image == "" && xma.TitleText == "" {
			continue
		}
		link := linkPreview{Title: xma.TitleText, Image: image}
		if xma.CTA != nil {
			link.URL = linkTarget(xma.CTA.ActionUrl)
		}
		if link.URL == "" {
			link.URL = linkTarget(xma.ActionUrl)
		}
		return link
	}
	return linkPreview{}
}

// consumerLink reads the link preview of an encrypted text message. paths is as in
// encryptedMedia.
func consumerLink(msg *waConsumerApplication.ConsumerApplication, paths map[string]string) linkPreview {
	ext := msg.GetPayload().GetContent().GetExtendedTextMessage()
	if ext == nil {
		return linkPreview{}
	}
	link := linkPreview{
		URL:   firstNonEmpty(linkTarget(ext.GetCanonicalURL()), linkTarget(ext.GetMatchedText())),
		Title: ext.GetTitle(),
	}
	if ext.GetThumbnail() == nil {
		return link
	}
	if thumbnail, err := ext.DecodeThumbnail(); err == nil {
		link.Ref, _ = mediaRefJSON(thumbnail.GetIntegral().GetTransport(), "image", paths)
	}
	return link
}

// sharedLink reads a share made with the Share button in an encrypted chat: the text to
// show and the link preview. A share without a web link is not handled.
func sharedLink(msg *waArmadilloApplication.Armadillo, paths map[string]string) (string, linkPreview, bool) {
	content := msg.GetPayload().GetContent().GetExtendedContentMessage()
	if content == nil {
		return "", linkPreview{}, false
	}
	link := linkPreview{Title: content.GetTitleText()}
	for _, cta := range content.GetCtas() {
		link.URL = firstNonEmpty(linkTarget(cta.GetNativeURL()), linkTarget(cta.GetActionURL()))
		if link.URL != "" {
			break
		}
	}
	if link.URL == "" {
		return "", linkPreview{}, false
	}
	for _, raw := range content.GetPreviews() {
		image, err := armadilloutil.Unmarshal(&waMediaTransport.ImageTransport{}, raw, sharePreviewVersion)
		if err != nil {
			continue
		}
		if ref, ok := mediaRefJSON(image.GetIntegral().GetTransport(), "image", paths); ok {
			link.Ref = ref
			break
		}
	}
	text := content.GetMessageText()
	if text == "" {
		text = link.URL // a share without words is still shown
	}
	return text, link, true
}

// linkTarget turns an address found in a preview into the address to open: Facebook's
// redirect is unwrapped, and anything that is not a web address is dropped.
func linkTarget(addr string) string {
	return webAddress(unwrapRedirect(addr))
}

// unwrapRedirect returns the real target of a link that goes through Facebook's redirect
// page (l.php?u=...), and the address itself for any other link.
func unwrapRedirect(addr string) string {
	parsed, err := url.Parse(strings.TrimSpace(addr))
	if err != nil || parsed.Path != "/l.php" || !isFacebookHost(parsed.Hostname()) {
		return addr
	}
	return parsed.Query().Get("u")
}

// isFacebookHost reports whether host serves Facebook's redirect page. An address without
// a host counts too, in case the redirect arrives as a bare path.
func isFacebookHost(host string) bool {
	host = strings.ToLower(host)
	for _, domain := range []string{"facebook.com", "messenger.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return host == ""
}

// webAddress returns addr when it is a full http or https address and "" otherwise, so
// that nothing else is ever stored as a link to open or a picture to fetch.
func webAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	parsed, err := url.Parse(addr)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return addr
}
