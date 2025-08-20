package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

// ---------------------------
// API response types
// ---------------------------

type RandomWordResponse []string

type Definition struct {
	Definition string   `json:"definition"`
	Example    string   `json:"example"`
	Synonyms   []string `json:"synonyms"`
	Antonyms   []string `json:"antonyms"`
}

type Meaning struct {
	PartOfSpeech string       `json:"partOfSpeech"`
	Definitions  []Definition `json:"definitions"`
}

type WordData struct {
	Word     string    `json:"word"`
	Meanings []Meaning `json:"meanings"`
}

// ---------------------------
// Config
// ---------------------------

type Config struct {
	Token     string
	GuildID   string // optional; if empty, registers globally
	ChannelID string // required for scheduled posting
	TZ        string // IANA timezone, e.g. "America/New_York"
	PostAt    string // HH:MM 24h local in TZ
}

func loadConfig() Config {
	_ = godotenv.Load() // ok if .env missing
	cfg := Config{
		Token:     os.Getenv("DISCORD_TOKEN"),
		GuildID:   os.Getenv("GUILD_ID"),
		ChannelID: os.Getenv("CHANNEL_ID"),
		TZ:        os.Getenv("TZ"),
		PostAt:    os.Getenv("POST_AT"),
	}
	return cfg
}

// ---------------------------
// Word helpers
// ---------------------------

func fetchRandomWord() (string, error) {
	resp, err := http.Get("https://random-word-api.herokuapp.com/word?number=1")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("random word api status %d", resp.StatusCode)
	}
	var words RandomWordResponse
	if err := json.NewDecoder(resp.Body).Decode(&words); err != nil {
		return "", err
	}
	if len(words) == 0 {
		return "", fmt.Errorf("no word returned")
	}
	return words[0], nil
}

func fetchDefinition(word string) (string, string, error) {
	url := fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", word)
	resp, err := http.Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return word, "", fmt.Errorf("dictionaryapi status %d", resp.StatusCode)
	}
	var data []WordData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}
	if len(data) == 0 || len(data[0].Meanings) == 0 || len(data[0].Meanings[0].Definitions) == 0 {
		return word, "", fmt.Errorf("no definition for %s", word)
	}
	w := data[0].Word
	pos := data[0].Meanings[0].PartOfSpeech
	def := data[0].Meanings[0].Definitions[0].Definition
	return w, fmt.Sprintf("%s — %s", italics(pos), def), nil
}

func italics(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("*(%s)*", s)
}

// Try up to N random words until one has a definition.
func getWOTD(retries int) (string, error) {
	for i := 0; i < retries; i++ {
		word, err := fetchRandomWord()
		if err != nil {
			continue
		}
		w, def, err := fetchDefinition(word)
		if err == nil {
			return fmt.Sprintf("📖 Word of the Day:\n**%s** %s", strings.Title(w), def), nil
		}
	}
	// fallback: last fetched word without def
	word, err := fetchRandomWord()
	if err != nil {
		return "⚠️ Could not fetch a Word of the Day right now.", nil
	}
	return fmt.Sprintf("📖 Word of the Day:\n**%s**\n(No definition found)", strings.Title(word)), nil
}

// ---------------------------
// Scheduler
// ---------------------------

func scheduleDaily(s *discordgo.Session, channelID, tz, postAt string) {
	if channelID == "" || tz == "" || postAt == "" {
		log.Println("[scheduler] skipped (CHANNEL_ID/TZ/POST_AT not fully set)")
		return
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Printf("[scheduler] invalid TZ %q: %v\n", tz, err)
		return
	}
	parseHM := func(hm string) (int, int, error) {
		var h, m int
		_, err := fmt.Sscanf(hm, "%d:%d", &h, &m)
		return h, m, err
	}
	nextRun := func(now time.Time) (time.Time, error) {
		h, m, err := parseHM(postAt)
		if err != nil {
			return time.Time{}, err
		}
		t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		return t, nil
	}
	go func() {
		for {
			now := time.Now().In(loc)
			next, err := nextRun(now)
			if err != nil {
				log.Printf("[scheduler] bad POST_AT: %v\n", err)
				return
			}
			log.Printf("[scheduler] Next WOTD at %s", next.Format(time.RFC1123))
			time.Sleep(time.Until(next))
			msg, _ := getWOTD(5)
			if _, err := s.ChannelMessageSend(channelID, msg); err != nil {
				log.Printf("[scheduler] send failed: %v\n", err)
			}
		}
	}()
}

// ---------------------------
// main (slash command + scheduler)
// ---------------------------

func main() {
	cfg := loadConfig()
	if cfg.Token == "" {
		log.Fatal("DISCORD_TOKEN is required")
	}

	s, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatal(err)
	}

	// Slash command handler
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		switch i.ApplicationCommandData().Name {
		case "wotd":
			msg, _ := getWOTD(5)
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: msg},
			})
		}
	})

	if err := s.Open(); err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	// Register /wotd (guild if provided, else global)
	cmd := &discordgo.ApplicationCommand{Name: "wotd", Description: "Get a random Word of the Day"}
	appID := s.State.User.ID
	if _, err := s.ApplicationCommandCreate(appID, cfg.GuildID, cmd); err != nil {
		log.Fatalf("cannot create command: %v", err)
	}

	// Start scheduler (only if env vars present)
	scheduleDaily(s, cfg.ChannelID, cfg.TZ, cfg.PostAt)

	log.Println("Bot running. Press CTRL+C to exit.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("Shutting down…")
}
