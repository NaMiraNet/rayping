package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"text/template"

	"github.com/NamiraNet/namira-core/internal/core"
	"github.com/NamiraNet/namira-core/internal/qr"
	"github.com/enescakir/emoji"
)

type TelegramBot struct {
	Token string
	Name  string
}

type Telegram struct {
	BotToken    string
	Channel     string
	Client      *http.Client
	Template    string
	qrGenerator *qr.QRGenerator
	mu          sync.RWMutex
	tmpl        *template.Template
	// Round-robin bot management
	bots       map[string]*TelegramBot
	botsList   []*TelegramBot
	currentBot uint64
	botsMu     sync.RWMutex
}

func NewTelegram(botToken, channel, template, qrConfig string, client *http.Client) *Telegram {
	t := &Telegram{
		BotToken:    botToken,
		Channel:     channel,
		Template:    template,
		Client:      client,
		qrGenerator: qr.NewQRGenerator(qrConfig),
		bots:        make(map[string]*TelegramBot),
		botsList:    make([]*TelegramBot, 0),
		currentBot:  0,
	}

	// Add the primary bot token to the bots map
	if botToken != "" {
		t.AddBot("primary", botToken)
	}

	t.initTemplate()
	return t
}

// AddBot adds a new bot token to the round-robin pool
func (t *Telegram) AddBot(name, token string) {
	t.botsMu.Lock()
	defer t.botsMu.Unlock()

	bot := &TelegramBot{
		Token: token,
		Name:  name,
	}

	t.bots[name] = bot
	t.botsList = append(t.botsList, bot)
}

// RemoveBot removes a bot from the round-robin pool
func (t *Telegram) RemoveBot(name string) {
	t.botsMu.Lock()
	defer t.botsMu.Unlock()

	if _, exists := t.bots[name]; !exists {
		return
	}

	delete(t.bots, name)

	// Rebuild botsList
	newBotsList := make([]*TelegramBot, 0, len(t.bots))
	for _, bot := range t.bots {
		newBotsList = append(newBotsList, bot)
	}
	t.botsList = newBotsList
}

// getNextBot returns the next bot in round-robin fashion
func (t *Telegram) getNextBot() *TelegramBot {
	t.botsMu.RLock()
	defer t.botsMu.RUnlock()

	if len(t.botsList) == 0 {
		// Fallback to primary bot token if no bots in pool
		return &TelegramBot{
			Token: t.BotToken,
			Name:  "fallback",
		}
	}

	if len(t.botsList) == 1 {
		return t.botsList[0]
	}

	// Atomic increment for thread-safe round-robin
	current := atomic.AddUint64(&t.currentBot, 1)
	index := (current - 1) % uint64(len(t.botsList))

	return t.botsList[index]
}

// GetBotsCount returns the number of bots in the pool
func (t *Telegram) GetBotsCount() int {
	t.botsMu.RLock()
	defer t.botsMu.RUnlock()
	return len(t.botsList)
}

// ListBots returns a copy of all bots in the pool
func (t *Telegram) ListBots() map[string]*TelegramBot {
	t.botsMu.RLock()
	defer t.botsMu.RUnlock()

	botsCopy := make(map[string]*TelegramBot)
	for name, bot := range t.bots {
		botsCopy[name] = &TelegramBot{
			Token: bot.Token,
			Name:  bot.Name,
		}
	}
	return botsCopy
}

// GetBotByName returns a specific bot by name
func (t *Telegram) GetBotByName(name string) (*TelegramBot, bool) {
	t.botsMu.RLock()
	defer t.botsMu.RUnlock()

	bot, exists := t.bots[name]
	if !exists {
		return nil, false
	}

	return &TelegramBot{
		Token: bot.Token,
		Name:  bot.Name,
	}, true
}

// ClearBots removes all bots from the pool
func (t *Telegram) ClearBots() {
	t.botsMu.Lock()
	defer t.botsMu.Unlock()

	t.bots = make(map[string]*TelegramBot)
	t.botsList = make([]*TelegramBot, 0)
	atomic.StoreUint64(&t.currentBot, 0)
}

type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func (t *Telegram) initTemplate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	funcMap := template.FuncMap{
		"protocolEmoji": func(protocol string) string {
			switch protocol {
			case "vmess":
				return emoji.HighVoltage.String()
			case "vless":
				return emoji.Rocket.String()
			case "trojan":
				return emoji.Shield.String()
			case "shadowsocks":
				return emoji.Locked.String()
			default:
				return emoji.RepeatButton.String()
			}
		},
		"countryFlag": func(countryCode string) string {
			e, err := emoji.CountryFlag(countryCode)
			if err != nil {
				return emoji.GlobeWithMeridians.String()
			}
			return e.String()
		},
	}
	if tmpl, err := template.New("telegram").Funcs(funcMap).Parse(t.Template); err == nil {
		t.tmpl = tmpl
	}
}

func (t *Telegram) Send(result core.CheckResult) error {
	t.mu.RLock()
	tmpl := t.tmpl
	t.mu.RUnlock()

	if tmpl == nil {
		t.initTemplate()
		t.mu.RLock()
		tmpl = t.tmpl
		t.mu.RUnlock()
		if tmpl == nil {
			return fmt.Errorf("failed to initialize template")
		}
	}

	var message bytes.Buffer
	if err := tmpl.Execute(&message, result); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Next bot in round-robin
	bot := t.getNextBot()

	jsonData, err := json.Marshal(telegramMessage{
		ChatID:    t.Channel,
		Text:      message.String(),
		ParseMode: "HTML",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		"https://api.telegram.org/bot"+bot.Token+"/sendMessage",
		bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned non-200 status code: %d", resp.StatusCode)
	}

	return nil
}

type telegramPhoto struct {
	ChatID    string `json:"chat_id"`
	Photo     string `json:"photo"`
	Caption   string `json:"caption,omitempty"`
	ParseMode string `json:"parse_mode,omitempty"`
}

func (t *Telegram) SendWithQRCode(result core.CheckResult) error {
	t.mu.RLock()
	tmpl := t.tmpl
	t.mu.RUnlock()

	if tmpl == nil {
		t.initTemplate()
		t.mu.RLock()
		tmpl = t.tmpl
		t.mu.RUnlock()
		if tmpl == nil {
			return fmt.Errorf("failed to initialize template")
		}
	}

	var caption bytes.Buffer
	if err := tmpl.Execute(&caption, result); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Get the next bot in round-robin fashion
	bot := t.getNextBot()

	jsonData, err := json.Marshal(telegramPhoto{
		ChatID:    t.Channel,
		Photo:     t.qrGenerator.GenerateURL(result.Raw),
		Caption:   caption.String(),
		ParseMode: "HTML",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		"https://api.telegram.org/bot"+bot.Token+"/sendPhoto",
		bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned non-200 status code: %d", resp.StatusCode)
	}

	return nil
}
