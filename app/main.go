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

type Command struct {
    help string
    act func(*discordgo.Session, *discordgo.MessageCreate)
}

var (
    settings Settings
    cmds map[string]Command = map[string]Command{
        "join":{
            help: "Joins the voice call of whoever sent the command",
        }, 
    }
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


func show_help(s *discordgo.Session, m *discordgo.MessageCreate) {
    help_msg := `Help:
- +join -> Joins the voice call of whoever sent the command
- +dc -> Leaves the current voice call of the server if there is one
- +play [link] -> Plays the specified youtube link
- +skip -> Skips the currently playing song, moves onto the next in queue
- +q -> Displays the current song queue
- +dl -> Fetches the raw audio and sends to discord as a file upload. Returned format is a .m4a file
- +pause -> Pauses the currently playing song
- +resume -> Resumes the currently paused song
`
    s.ChannelMessageSend(m.ChannelID, help_msg)
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
    case "help":
        show_help(s, m)
    case "h":
        show_help(s, m)
    case "join":
        err := join_voice(s, guild_id, vc_id)
        if err != nil {
            s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Unable to join voice channel: %s", err.Error()))
            log.Printf("joining vc: %s", err.Error())
            return
        }   
    case "dc":
        leave_voice(guild_id)
    case "play":
        play_cmd(s, m, vc_id, cmd_sections)
    case "dl":
        download_cmd(s, m, cmd_sections)
    case "skip":
        skip_cmd(s, m)
    case "q":
        queue_cmd(s, m)
    case "pause":
        set_paused(s, m, true)
    case "resume":
        set_paused(s, m, false)
    default:
        s.ChannelMessageSend(m.ChannelID, "Unknown command")
    }
}


func load_settings() (Settings, error) {
    var s Settings 

    // Read CMD Prefix setting
    prefix_s, set := os.LookupEnv("PREFIX")
    if !set {
        s.cmd_prefix = '+'
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
