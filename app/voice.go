package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
    calls = map[string]*discordgo.VoiceConnection{}
    calls_mutx sync.Mutex
)

func vc_from_message(s *discordgo.Session, m *discordgo.MessageCreate) (string, string, error)   {
    // Text Channel
    c, err := s.State.Channel(m.ChannelID)
    if err != nil {
    return "", "", fmt.Errorf("could not find text channel message was sent in")
    }

    // Find guild
    g, err := s.State.Guild(c.GuildID)
    if err != nil {
        return "", "", fmt.Errorf("could not find guild")
    }

    // Find voice state the author of txt message is in
    for _, vs := range g.VoiceStates {
        if vs.UserID == m.Author.ID {
            return g.ID, vs.ChannelID, nil
        }
    }
    
    return "", "", fmt.Errorf("author is not in an accessible voice channel")
}

func join_voice(s *discordgo.Session, guild_id string, vc_id string) error {
    _, exists := calls[guild_id]
    if exists {
        log.Printf("already in a voice call, leaving old one\n")
        leave_voice(guild_id)        
    }
    calls_mutx.Lock()

    call, err := s.ChannelVoiceJoin(guild_id, vc_id, false, true) 
    if err != nil {
        calls_mutx.Unlock()
        return fmt.Errorf("unable to join voice call: %s\n", err.Error())
    }
    calls[guild_id] = call
    calls_mutx.Unlock()

    log.Printf("Joined a voice call in %s\n", guild_id)
    return nil
}


func leave_voice(guild_id string) {
    calls_mutx.Lock()

    // Make sure the call actually exists
    _, exists := calls[guild_id]
    if !exists {
        calls_mutx.Unlock()
        return
    }

    // Disconnect, close channel, and close web socket
    calls[guild_id].Disconnect()
    close(calls[guild_id].OpusSend)
    calls[guild_id].Close()

    // Delete the entry from the hashmap
    delete(calls, guild_id)
    calls_mutx.Unlock()

    log.Printf("Left a voice call in %s\n", guild_id)
}


func play_audio(guild_id string, arg string) error {
    // Get current call, make sure in a call
    var call *discordgo.VoiceConnection
    var exists bool
    call, exists = calls[guild_id]
    if !exists {
        return fmt.Errorf("not in a voice call")
    }

    // Obtain youtube stream
    get_audio(arg)

    call.Speaking(true)

    time.Sleep(5 * time.Second)

    call.Speaking(false)

    return nil
}
