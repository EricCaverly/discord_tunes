package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
	"layeh.com/gopus"
)

type Call struct {
    vc *discordgo.VoiceConnection
    playing bool
    should_exit bool
    paused *bool
    bts_ctx context.Context
    bts_cancel context.CancelFunc
    eas_ctx context.Context
    eas_cancel context.CancelCauseFunc
    ffm_ctx context.Context
    ffm_cancel context.CancelFunc
    queue []*youtube.Video
}

var (
    calls = map[string]Call{}
    calls_mutx sync.Mutex
)

const (
    audio_chan int = 2
    audio_frame_size int = 960
    audio_sample_rate int = 48000
    //audio_bitrate int = 64
    audio_bitrate int = 128
    audio_max_bytes int = (audio_frame_size * audio_chan) * 2
)


func vc_from_message(s *discordgo.Session, m *discordgo.MessageCreate) (string, error)   {
    // Text Channel
    c, err := s.State.Channel(m.ChannelID)
    if err != nil {
        return "", fmt.Errorf("could not find text channel message was sent in")
    }

    // Find guild
    g, err := s.State.Guild(c.GuildID)
    if err != nil {
        return "", fmt.Errorf("could not find guild")
    }

    // Find voice state the author of txt message is in
    for _, vs := range g.VoiceStates {
        if vs.UserID == m.Author.ID {
            return vs.ChannelID, nil
        }
    }
    
    return "", fmt.Errorf("author is not in an accessible voice channel")
}


func set_paused(s *discordgo.Session, m *discordgo.MessageCreate, val bool) {
    calls_mutx.Lock()
    call, exists := calls[m.GuildID]
    if !exists {
        s.ChannelMessageSend(m.ChannelID, "I'm not in a call, you can't tell me what to do with my life!")
    }

    if !call.playing {
        s.ChannelMessageSend(m.ChannelID, "Nothing is currently playing")
    }

    *call.paused = val
    switch val {
    case false:
        s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Resumed %s", call.queue[0].Title))
    case true:
        s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Paused %s, type `%cresume` to resume playing", call.queue[0].Title, settings.cmd_prefix))
    }
    calls[m.GuildID] = call
    calls_mutx.Unlock()
}


func queue_cmd(s *discordgo.Session, m *discordgo.MessageCreate) {
    // Make sure the bot is in a call
    _, exists := calls[m.GuildID]
    if !exists {
        s.ChannelMessageSend(m.ChannelID, "I'm not in a call, you don't live rent free in my head")
        return
    }

    var list string
    for _, vid := range calls[m.GuildID].queue {
        list+=fmt.Sprintf("\n(%s - [%v])", vid.Title, vid.Duration)
    }
    s.ChannelMessageSend(m.ChannelID, "Queue (including currently playing): "+list)
}


func skip_cmd(s *discordgo.Session, m *discordgo.MessageCreate) {
    // Make sure the bot is in a call
    _, exists := calls[m.GuildID]
    if !exists {
        s.ChannelMessageSend(m.ChannelID, "I'm not in a call, don't get ahead of yourself")
        return
    }

    // Cancel all child threads that are being used to play the current song
    calls[m.GuildID].ffm_cancel()
    calls[m.GuildID].eas_cancel(fmt.Errorf("Skipped"))
    calls[m.GuildID].bts_cancel()
    // From here, the existing play_audio thread will do the rest

    log.Printf("skip command executed\n")
}


func join_voice(s *discordgo.Session, guild_id string, vc_id string) error {
    // Check if the bot is already in a call
    calls_mutx.Lock()
    _, exists := calls[guild_id]
    if exists {
        // If it is, leave the old call
        log.Printf("already in a voice call\n")
        calls_mutx.Unlock()
        return fmt.Errorf("already in a voice call")
    }

    // Join the new voice channel
    call, err := s.ChannelVoiceJoin(guild_id, vc_id, false, true) 
    if err != nil {
        calls_mutx.Unlock()
        return fmt.Errorf("unable to join voice call: %s\n", err.Error())
    }

    // Create a new call object in the global calls map
    paused := false

    calls[guild_id] = Call{
        vc: call,
        playing: false,
        should_exit: false,
        paused: &paused,
        queue: []*youtube.Video{},
    }
    calls_mutx.Unlock()

    log.Printf("Joined a voice call in %s\n", guild_id)
    return nil
}


func leave_voice(guild_id string) {
    // Make sure the call actually exists
    calls_mutx.Lock()
    call, exists := calls[guild_id]
    if !exists {
        return
    }

    // Set the should exit flag high so the current go routine playing audio knows to stop
    call.should_exit = true
    calls[guild_id] = call
    calls_mutx.Unlock()

    // Cancel child threads of call
    call.ffm_cancel()
    call.bts_cancel()
    call.eas_cancel(fmt.Errorf("Disconnected"))

    var wait bool = true
    for wait {
        // Wait until play_audio is done
        calls_mutx.Lock()
        wait = calls[guild_id].playing
        calls_mutx.Unlock()
        time.Sleep(10 * time.Millisecond) 
    }

    // Disconnect, close channel, and close web socket
    calls_mutx.Lock()
    calls[guild_id].vc.Disconnect()
    close(calls[guild_id].vc.OpusSend)
    calls[guild_id].vc.Close()
    
    // Delete the entry from the hashmap
    delete(calls, guild_id)
    calls_mutx.Unlock()

    log.Printf("Left a voice call in %s\n", guild_id)
}


func play_cmd(s *discordgo.Session, m *discordgo.MessageCreate) {
    cmd_sections := strings.Split(m.Content[1:], " ")

    // Make sure the user has only put the play command and one argument
    if len(cmd_sections) != 2 {
        s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Invalid syntax: use `%cplay [link]`", settings.cmd_prefix))
        return
    }

    // Try to find the video specified in the command
    vid, err := get_video(cmd_sections[1])
    if err != nil {
        s.ChannelMessageSend(m.ChannelID, "Could not find video, please try again (Node: Search is not currently implemented)")
        return 
    }
    log.Printf("found youtube video: [%s] - [%s]\n", vid.Title, vid.ID)


    // Make sure the call exists, if it doesn't, try to join the voice channel
    if _, exists := calls[m.GuildID]; !exists {
        log.Printf("not currently in a voice call, attempting to join\n")
        vc_id, err := vc_from_message(s, m)
        if err != nil {
            s.ChannelMessageSend(m.ChannelID, "You are not currently within a voice call")
            log.Printf("could not find vc: %s\n", err.Error())
            return
        }
        err = join_voice(s, m.GuildID, vc_id)
        if err != nil {
            s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Unable to join voice channel: %s", err.Error()))
            log.Printf("joining vc: %s", err.Error())
            return
        }
    }

    // Lock the calls mutx, for a fleeting feeling of thread safety
    calls_mutx.Lock()

    // Add the found video into the video queue
    call := calls[m.GuildID]
    call.queue = append(call.queue, vid) 
    log.Printf("added song to queue: '%s'\n", vid.ID)
    s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Added '%s' to the queue", vid.Title))

    // If the voice connection is not currently playing, start playing
    if !call.playing { 
        call.playing = true
        calls[m.GuildID] = call
        calls_mutx.Unlock()

        // Turn this thread into the play_audio thread
        log.Printf("no existing audio thread - creating new one\n")
        err = play_audio(s, m.ChannelID, m.GuildID)
        if err != nil {
            log.Printf("error playing: %s\n", err.Error())
            s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Error while playing: %s", err.Error()))
        }
    } else {
        calls[m.GuildID] = call
        calls_mutx.Unlock()
    }
}


func play_audio(s *discordgo.Session, txt_chan string, guild_id string) error {
    // Set playing to false upon the return of this function
    defer func() {
        calls_mutx.Lock()
        call, exists := calls[guild_id]
        if !exists {
            return
        }   
        call.playing = false
        calls[guild_id] = call
        calls_mutx.Unlock()
    }()

    // For each song in the queue
    for {
        calls_mutx.Lock()

        var call Call
        var exists bool

        // Get the current call state, and make sure it still exists
        call, exists = calls[guild_id]
        if !exists {
            calls_mutx.Unlock()
            return fmt.Errorf("call missing")
        }

        // Only continue if there is at least one song in the queue
        if len(calls[guild_id].queue) == 0 {
            calls_mutx.Unlock()
            return nil
        }

        // Check if this go-routine is instructed to exit
        if call.should_exit {
            calls_mutx.Unlock()
            return nil
        }

        // Create a new opus encoder that econdes pcm data into opus packets for discord
        opus_enc, err := gopus.NewEncoder(audio_sample_rate, audio_chan, gopus.Audio)
        if err != nil {
            calls_mutx.Unlock()
            return fmt.Errorf("unable to make encoder: %s", err.Error())
        }
        opus_enc.SetBitrate(audio_bitrate * 1000)

        // Tell discord we want to start speaking
        call.vc.Speaking(true)

        // Set contexts to new contexts with cancel
        call.bts_ctx, call.bts_cancel = context.WithCancel(context.Background())
        call.eas_ctx, call.eas_cancel = context.WithCancelCause(context.Background())
        call.ffm_ctx, call.ffm_cancel = context.WithCancel(context.Background())
        
        // Update map with new call settings
        calls[guild_id] = call
        calls_mutx.Unlock()
        
        // Control variables for multi threading
        var wg sync.WaitGroup 

        // Obtain youtube audio only stream
        audio_stream, err := get_audio_stream(calls[guild_id].queue[0])    
        if err != nil {
            return err
        }
        
        // Inform the users what will now be playing
        title := calls[guild_id].queue[0].Title
        s.ChannelMessageSend(txt_chan, fmt.Sprintf("Now Playing: %s [%v]", title, calls[guild_id].queue[0].Duration)) 

        // Use FFMpeg to convert the M4A AAC encoded file into raw PCM data
        pcm_data_bytes, err := convert_m4a_pcm(audio_stream, calls[guild_id].ffm_ctx)
        if err != nil {
            return fmt.Errorf("converting m4a -> pcm: %s", err.Error())
        }

        // Get bytes from output of command, turn into int16 slices and send to encoding thread
        short_chan := make(chan []int16, 30) 
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := pcm_bts(pcm_data_bytes, short_chan, guild_id)
            if err != nil {
                log.Printf("error while doing bts: %s\n", err.Error())
            }   
            log.Printf("exited bts loop")
        }()

        // Encode PCM to Opus Thread
        wg.Add(1)
        go func() {
            defer wg.Done()
            for calls[guild_id].playing {
                // Get PCM data from FFMPEG in appropriately sized chunks to be converted to OPUS
                select {
                case <- calls[guild_id].eas_ctx.Done():
                    log.Printf("eas cancelled check 1\n")
                    return
                case pcm, ok := <- short_chan:
                    if !ok {
                        return
                    }

                    // Encode into opus
                    opus, err := opus_enc.Encode(pcm, audio_frame_size, audio_max_bytes)
                    if err != nil {
                        log.Printf("error while doing eas: %s\n", err.Error())
                        return
                    }
                    
                    var paused *bool = calls[guild_id].paused
                    for *paused {
                        time.Sleep(20 * time.Millisecond)
                    }

                    // Send to discord if the thread has not been cancelled
                    select {
                    case <- calls[guild_id].eas_ctx.Done():
                        log.Printf("eas cancelled check 2\n")
                        return
                    case call.vc.OpusSend <- opus:
                        continue
                    }
                }
            }

            log.Printf("exited eas loop\n")
        }()

        // Wait for byte-to-short and encode-and-send threads to exit, either gracefully or failure
        wg.Wait()
        log.Printf("wait group complete\n")

        // Close readers and channels for getting / encoding audio
        // Leaves discord send chan open for next song
        audio_stream.Close()
        pcm_data_bytes.Close()
        close(short_chan)

        calls_mutx.Lock()
        
        // Check if this call still exists
        call, exists = calls[guild_id]
        if !exists {
            calls_mutx.Unlock()
            return nil
        }

        // Tell discord we are done speaking
        err = call.vc.Speaking(false)
        if err != nil {
            calls_mutx.Unlock()
            return fmt.Errorf("problem stopping speaking: %s", err.Error())
        }

        // Remove from the queue
        call = calls[guild_id]
        if len(call.queue) > 0 {
            call.queue = call.queue[1:]
        } else {
            call.queue = []*youtube.Video{}
        }

        // Update calls map with new settings
        calls[guild_id] = call
        calls_mutx.Unlock()

        // Inform users we are done playing the song and why
        s.ChannelMessageSend(txt_chan, fmt.Sprintf("Stopped playing '%s' - %s", title, context.Cause(call.eas_ctx)))

        log.Printf("stopped playing: %s", context.Cause(call.eas_ctx))
    }
}

