package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

type Settings struct {
    token_secret_path string
    cmd_prefix byte
}

var (
    settings Settings
)

func main() {
    var err error
    settings, err = load_settings()
    if err != nil {
        log.Fatalf("error loading settings: %s\n", err.Error())
    }

    // Read secret file
    tok_file_contents, err := os.ReadFile(settings.token_secret_path)
    if err != nil {
        log.Fatalf("error reading token file: %s\n", err.Error())
    }

    // Initialize discord bot
    bot, err := discordgo.New("Bot "+strings.Trim(string(tok_file_contents), " \n"))
    if err != nil {
        log.Fatalf("Error creating discord bot: %s\n", err.Error())
    }

    // Configure event handlers for bot
    bot.AddHandler(message_create)

    // Set intents of bot
    bot.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds | discordgo.IntentsGuildVoiceStates

    // Open connection to discord
    err = bot.Open()
    if err != nil {
        log.Fatalf("Error connecting to discord: %s\n", err.Error())
    }

    log.Printf("Bot is running\n")

    // Stop logic
    sc := make(chan os.Signal, 1)
    signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
    <-sc

    bot.Close()
}


func message_create(s *discordgo.Session, m *discordgo.MessageCreate) {
    // make sure the message has text, and that the bot is not the author of the message
    if len(m.Content) == 0 {
         return
    }

    // make sure the message starts with the bot's cmd prefix
    if m.Content[0] != settings.cmd_prefix {
        return
    }

    guild_id, vc_id, err := vc_from_message(s, m)
    if err != nil {
        log.Printf("could not find vc: %s\n", err.Error())
    }

    // Run the appropriate command function based on command
    cmd_sections := strings.Split(m.Content[1:], " ")
    switch cmd_sections[0] {
    case "join":
        join_voice(s, guild_id, vc_id)
    case "dc":
        leave_voice(guild_id)
    case "play":
        if _, exists := calls[guild_id]; !exists {
            join_voice(s, guild_id, vc_id)
        }
        err := play_audio(guild_id, cmd_sections[1])
        if err != nil {
            log.Printf("error playing: %s\n", err.Error())
        }
    case "skip":
    case "pause":
    default:
        s.ChannelMessageSend(m.ChannelID, "Unknown command")
    }
}


func load_settings() (Settings, error) {
    var s Settings 

    // Read CMD Prefix setting
    prefix_s, set := os.LookupEnv("PREFIX")
    if !set {
        s.cmd_prefix = '!'
    } else if len(prefix_s) > 1 {
        return s, fmt.Errorf("invalid cmd prefix: must be a single character")
    } else {
        s.cmd_prefix = prefix_s[0]
    }
    log.Printf("Prefix set to: '%c'\n", s.cmd_prefix);

    // Read token secret path setting
    tok_path, set := os.LookupEnv("TOKEN_FILE")
    if !set {
        return s, fmt.Errorf("missing token file path, please specify with 'TOKEN_FILE' env variable")
    }
    s.token_secret_path = tok_path

    return s, nil
}
