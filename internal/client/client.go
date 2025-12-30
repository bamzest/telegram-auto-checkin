package client

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/rs/zerolog"
	"golang.org/x/net/proxy"
)

type Client struct {
	tgClient          *telegram.Client
	api               *tg.Client
	appID             int
	appHash           string
	log               zerolog.Logger
	replyWaitSeconds  int // Seconds to wait for bot reply
	replyHistoryLimit int // Number of historical messages to fetch
}

func NewClient(appID int, appHash string, sessionFile string, proxyAddr string, log zerolog.Logger, replyWaitSeconds, replyHistoryLimit int) (*Client, error) {
	// Ensure session directory exists
	sessionDir := "session"
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	// Adjust session file path to session directory
	if sessionFile != "" && !strings.Contains(sessionFile, string(os.PathSeparator)) {
		sessionFile = filepath.Join(sessionDir, sessionFile)
	}

	// telegram.FileSessionStorage supports specifying full path
	// Session file will be saved to the specified path
	opts := telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{
			Path: sessionFile,
		},
	}

	clientLog := log.With().Int("app_id", appID).Logger()

	// Output session file path (debug level)
	absPath, _ := filepath.Abs(sessionFile)
	clientLog.Debug().Str("session_file", sessionFile).Str("abs_path", absPath).Msg("Session file path")

	// Set default values
	if replyWaitSeconds <= 0 {
		replyWaitSeconds = 3
	}
	if replyHistoryLimit <= 0 {
		replyHistoryLimit = 10
	}

	if proxyAddr != "" {
		clientLog.Info().Str("proxy", proxyAddr).Msg("Using proxy connection")
		dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
		}
		opts.Resolver = dcs.Plain(dcs.PlainOptions{
			Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		})
	}

	client := telegram.NewClient(appID, appHash, opts)

	return &Client{
		tgClient:          client,
		api:               tg.NewClient(client),
		appID:             appID,
		appHash:           appHash,
		log:               clientLog,
		replyWaitSeconds:  replyWaitSeconds,
		replyHistoryLimit: replyHistoryLimit,
	}, nil
}

func (c *Client) Auth(ctx context.Context, phone, password string) error {
	return c.Run(ctx, func(ctx context.Context) error {
		return c.AuthInRun(ctx, phone, password)
	})
}

func (c *Client) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return c.tgClient.Run(ctx, fn)
}

func (c *Client) AuthInRun(ctx context.Context, phone, password string) error {
	status, err := c.tgClient.Auth().Status(ctx)
	if err != nil {
		return err
	}
	if status.Authorized {
		c.log.Debug().Msg("✓ Already authorized")
		return nil
	}

	if phone != "" {
		 c.log.Info().Msg("Logging in with phone number...")
		flow := auth.NewFlow(
			auth.Constant(phone, password, auth.CodeAuthenticatorFunc(func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
				fmt.Printf("Please enter verification code for %s: ", phone)
				code, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				return strings.TrimSpace(code), nil
			})),
			auth.SendCodeOptions{},
		)
		return c.tgClient.Auth().IfNecessary(ctx, flow)
	}

	// QR code login
	c.log.Info().Msg("No phone number provided, trying QR code login")
	qr := qrlogin.NewQR(c.api, c.appID, c.appHash, qrlogin.Options{})
	token, err := qr.Export(ctx)
	if err != nil {
		return err
	}

	c.log.Info().Str("url", token.URL()).Msg("Please scan this link with Telegram on your phone")

	authorization, err := qr.Accept(ctx, token)
	if err != nil {
		return err
	}

	if authorization.PasswordPending {
		return fmt.Errorf("2FA password is required but not supported via QR login in this tool yet, please use phone login")
	}

	c.log.Info().Msg("Login successful")
	return nil
}

func (c *Client) resolvePeer(ctx context.Context, target string) (tg.InputPeerClass, error) {
	peer, err := c.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: strings.TrimPrefix(target, "@"),
	})
	if err != nil {
		return nil, err
	}

	if len(peer.Users) > 0 {
		user := peer.Users[0].(*tg.User)
		return &tg.InputPeerUser{
			UserID:     user.ID,
			AccessHash: user.AccessHash,
		}, nil
	}

	if len(peer.Chats) > 0 {
		chat := peer.Chats[0].(*tg.Channel)
		return &tg.InputPeerChannel{
			ChannelID:  chat.ID,
			AccessHash: chat.AccessHash,
		}, nil
	}

	return nil, fmt.Errorf("could not resolve peer")
}

func randInt64() int64 {
	var b [8]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		panic(err)
	}
	return int64(binary.LittleEndian.Uint64(b[:]))
}

// CheckInMessage sends text message for check-in
func (c *Client) CheckInMessage(ctx context.Context, target string, message string) error {
	return c.Run(ctx, func(ctx context.Context) error {
		return c.CheckInMessageInRun(ctx, target, message)
	})
}

// CheckInButton clicks button in latest message
func (c *Client) CheckInButton(ctx context.Context, target string, buttonText string) error {
	return c.Run(ctx, func(ctx context.Context) error {
		return c.CheckInButtonInRun(ctx, target, buttonText)
	})
}

func (c *Client) CheckInMessageInRun(ctx context.Context, target string, message string) error {
	taskLog := c.log.With().Str("target", target).Logger()
	peer, err := c.resolvePeer(ctx, target)
	if err != nil {
		return err
	}

	updates, err := c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  message,
		RandomID: randInt64(),
	})
	if err != nil {
		return err
	}

	logSendMessageResult(taskLog, updates)

	// Wait for bot reply
	 taskLog.Info().Int("wait_seconds", c.replyWaitSeconds).Msg("Waiting for reply...")
	time.Sleep(time.Duration(c.replyWaitSeconds) * time.Second)

	// Get latest messages
	history, err := c.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: c.replyHistoryLimit,
	})
	if err != nil {
		taskLog.Warn().Err(err).Msg("Failed to get message history")
		return nil // Don't block main flow
	}

	var msgs []tg.MessageClass
	switch h := history.(type) {
	case *tg.MessagesMessages:
		msgs = h.Messages
	case *tg.MessagesMessagesSlice:
		msgs = h.Messages
	case *tg.MessagesChannelMessages:
		msgs = h.Messages
	}

	// Find the message ID we sent
	var sentMsgID int
	switch u := updates.(type) {
	case *tg.Updates:
		if len(u.Updates) > 0 {
			for _, upd := range u.Updates {
				if msgUpdate, ok := upd.(*tg.UpdateMessageID); ok {
					sentMsgID = msgUpdate.ID
					break
				}
				if newMsg, ok := upd.(*tg.UpdateNewMessage); ok {
					if m, ok := newMsg.Message.(*tg.Message); ok && m.Out {
						sentMsgID = m.ID
						break
					}
				}
			}
		}
	case *tg.UpdateShortSentMessage:
		sentMsgID = u.ID
	}

	// Extract bot's reply (find latest message not sent by us)
	var botReply string
	for _, m := range msgs {
		if msg, ok := m.(*tg.Message); ok {
			if !msg.Out && (sentMsgID == 0 || msg.ID > sentMsgID) {
				botReply = msg.Message
				break
			}
		}
	}

	if botReply != "" {
		taskLog.Info().Str("reply", botReply).Msg("Received reply")
	} else {
		taskLog.Info().Msg("Sent (no reply)")
	}

	return nil
}

// CheckInMessageInRunWithLogger Send text message for check-in (with task logger)
func (c *Client) CheckInMessageInRunWithLogger(ctx context.Context, target string, message string, taskLogger zerolog.Logger) error {
	taskLog := taskLogger.With().Str("target", target).Logger()
	peer, err := c.resolvePeer(ctx, target)
	if err != nil {
		return err
	}

	updates, err := c.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  message,
		RandomID: randInt64(),
	})
	if err != nil {
		return err
	}

	logSendMessageResult(taskLog, updates)

	// Wait for bot reply
	taskLog.Info().Int("wait_seconds", c.replyWaitSeconds).Msg("Waiting for reply...")
	time.Sleep(time.Duration(c.replyWaitSeconds) * time.Second)
	history, err := c.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: c.replyHistoryLimit,
	})
	if err != nil {
		taskLog.Warn().Err(err).Msg("Failed to get message history")
		return nil // Don't block main flow
	}

	var msgs []tg.MessageClass
	switch h := history.(type) {
	case *tg.MessagesMessages:
		msgs = h.Messages
	case *tg.MessagesMessagesSlice:
		msgs = h.Messages
	case *tg.MessagesChannelMessages:
		msgs = h.Messages
	}

	// Find the message ID we sent
	var sentMsgID int
	switch u := updates.(type) {
	case *tg.Updates:
		if len(u.Updates) > 0 {
			for _, upd := range u.Updates {
				if msgUpdate, ok := upd.(*tg.UpdateMessageID); ok {
					sentMsgID = msgUpdate.ID
					break
				}
				if newMsg, ok := upd.(*tg.UpdateNewMessage); ok {
					if m, ok := newMsg.Message.(*tg.Message); ok && m.Out {
						sentMsgID = m.ID
						break
					}
				}
			}
		}
	case *tg.UpdateShortSentMessage:
		sentMsgID = u.ID
	}

	// Extract bot's reply (find latest message not sent by us)
	var botReply string
	for _, m := range msgs {
		if msg, ok := m.(*tg.Message); ok {
			if !msg.Out && (sentMsgID == 0 || msg.ID > sentMsgID) {
				botReply = msg.Message
				break
			}
		}
	}

	if botReply != "" {
		// Write to both main log and task log
		c.log.Info().Str("target", target).Str("reply", botReply).Msg("Received reply")
		taskLog.Info().Str("reply", botReply).Msg("Received reply")
	} else {
		taskLog.Info().Msg("Sent (no reply)")
	}

	return nil
}

func (c *Client) CheckInButtonInRun(ctx context.Context, target string, buttonText string) error {
	taskLog := c.log.With().Str("target", target).Logger()
	peer, err := c.resolvePeer(ctx, target)
	if err != nil {
		return err
	}

	// Get the latest message
	history, err := c.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: 1,
	})
	if err != nil {
		return err
	}

	var msgs []tg.MessageClass
	switch h := history.(type) {
	case *tg.MessagesMessages:
		msgs = h.Messages
	case *tg.MessagesMessagesSlice:
		msgs = h.Messages
	case *tg.MessagesChannelMessages:
		msgs = h.Messages
	default:
		return fmt.Errorf("unexpected history type: %T", history)
	}

	if len(msgs) == 0 {
		return fmt.Errorf("no messages found")
	}

	msg, ok := msgs[0].(*tg.Message)
	if !ok || msg.ReplyMarkup == nil {
		return fmt.Errorf("latest message has no buttons")
	}

	markup, ok := msg.ReplyMarkup.(*tg.ReplyInlineMarkup)
	if !ok {
		return fmt.Errorf("no inline markup found")
	}

	for _, row := range markup.Rows {
		for _, btn := range row.Buttons {
			inlineBtn, ok := btn.(*tg.KeyboardButtonCallback)
			if ok && inlineBtn.Text == buttonText {
				answer, err := c.api.MessagesGetBotCallbackAnswer(ctx, &tg.MessagesGetBotCallbackAnswerRequest{
					Peer:  peer,
					MsgID: msg.ID,
					Data:  inlineBtn.Data,
					Game:  false,
				})
				if err != nil {
					return err
				}

				logCallbackAnswer(taskLog, answer)
				return nil
			}
		}
	}

	return fmt.Errorf("button with text %q not found", buttonText)
}

// CheckInButtonInRunWithLogger Click button for check-in (with task logger)
func (c *Client) CheckInButtonInRunWithLogger(ctx context.Context, target string, buttonText string, taskLogger zerolog.Logger) error {
	taskLog := taskLogger.With().Str("target", target).Logger()
	peer, err := c.resolvePeer(ctx, target)
	if err != nil {
		return err
	}

	// Get the latest message
	history, err := c.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: 1,
	})
	if err != nil {
		return err
	}

	var msgs []tg.MessageClass
	switch h := history.(type) {
	case *tg.MessagesMessages:
		msgs = h.Messages
	case *tg.MessagesMessagesSlice:
		msgs = h.Messages
	case *tg.MessagesChannelMessages:
		msgs = h.Messages
	default:
		return fmt.Errorf("unexpected history type: %T", history)
	}

	if len(msgs) == 0 {
		return fmt.Errorf("no messages found")
	}

	msg, ok := msgs[0].(*tg.Message)
	if !ok || msg.ReplyMarkup == nil {
		return fmt.Errorf("latest message has no buttons")
	}

	markup, ok := msg.ReplyMarkup.(*tg.ReplyInlineMarkup)
	if !ok {
		return fmt.Errorf("no inline markup found")
	}

	for _, row := range markup.Rows {
		for _, btn := range row.Buttons {
			inlineBtn, ok := btn.(*tg.KeyboardButtonCallback)
			if ok && inlineBtn.Text == buttonText {
				answer, err := c.api.MessagesGetBotCallbackAnswer(ctx, &tg.MessagesGetBotCallbackAnswerRequest{
					Peer:  peer,
					MsgID: msg.ID,
					Data:  inlineBtn.Data,
					Game:  false,
				})
				if err != nil {
					return err
				}

				// Write to both main log and task log
				logCallbackAnswer(c.log.With().Str("target", target).Logger(), answer)
				logCallbackAnswer(taskLog, answer)
				return nil
			}
		}
	}

	return fmt.Errorf("button with text %q not found", buttonText)
}

func logSendMessageResult(log zerolog.Logger, updates tg.UpdatesClass) {
	switch u := updates.(type) {
	case *tg.UpdateShortSentMessage:
		 log.Info().Str("response_type", "updateShortSentMessage").Int("message_id", u.ID).Msg("Message sent")
	 case *tg.Updates:
		 log.Info().Str("response_type", "updates").Int("updates", len(u.Updates)).Msg("Message sent")
	 case *tg.UpdatesCombined:
		 log.Info().Str("response_type", "updatesCombined").Int("updates", len(u.Updates)).Msg("Message sent")
	 default:
		 log.Info().Str("response_type", fmt.Sprintf("%T", updates)).Msg("Message sent")
	 }
}

func logCallbackAnswer(log zerolog.Logger, answer *tg.MessagesBotCallbackAnswer) {
	if answer == nil {
		 log.Info().Msg("Button clicked (no reply)")
		 return
	 }

	 if answer.Message != "" {
		 log.Info().Str("reply", answer.Message).Msg("Received reply")
	 } else if answer.URL != "" {
		 log.Info().Str("url", answer.URL).Msg("Received URL")
	 } else {
		 log.Info().Msg("Button clicked")
	 }

	 log.Info().
		 Str("response_type", "messagesBotCallbackAnswer").
		 Bool("alert", answer.Alert).
		 Bool("has_url", answer.HasURL).
		 Bool("native_ui", answer.NativeUI).
		 Str("message", answer.Message).
		 Str("url", answer.URL).
		 Int("cache_time", answer.CacheTime).
		 Msg("API response details")
}
