package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
	"layeh.com/gopus"
)

type Call struct {
    vc *discordgo.VoiceConnection
    playing bool
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
    audio_bitrate int = 64
    audio_max_bytes int = (audio_frame_size * audio_chan) * 2
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
    // Check if the bot is already in a call
    _, exists := calls[guild_id]
    if exists {
        // If it is, leave the old call
        log.Printf("already in a voice call, leaving old one\n")
        leave_voice(guild_id)        
    }
    calls_mutx.Lock()

    // Join the new voice channel
    call, err := s.ChannelVoiceJoin(guild_id, vc_id, false, true) 
    if err != nil {
        calls_mutx.Unlock()
        return fmt.Errorf("unable to join voice call: %s\n", err.Error())
    }

    // Create a new call object in the global calls map
    calls[guild_id] = Call{
        vc: call,
        playing: false,
        queue: []*youtube.Video{},
    }
    calls_mutx.Unlock()

    log.Printf("Joined a voice call in %s\n", guild_id)
    return nil
}


func leave_voice(guild_id string) {

    // Make sure the call actually exists
    call, exists := calls[guild_id]
    if !exists {
        return
    }

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
        time.Sleep(100 * time.Millisecond) 
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


func play_audio(s *discordgo.Session, txt_chan string, guild_id string, arg string) error {
    // Get current call, make sure in a call
    calls_mutx.Lock()
    var call Call
    var exists bool
    call, exists = calls[guild_id]
    if !exists {
        calls_mutx.Unlock()
        return fmt.Errorf("not in a voice call")
    }

    // Try to find the video specified in the command
    vid, err := get_video(arg)
    if err != nil {
        calls_mutx.Unlock()
        s.ChannelMessageSend(txt_chan, "could not find video")
        return err
    }
    
    // Add the found video into the video queue
    call.queue = append(call.queue, vid) 
    calls[guild_id] = call
    log.Printf("added song to queue: '%s'\n", vid.ID)
    s.ChannelMessageSend(txt_chan, fmt.Sprintf("Added '%s' to the queue", vid.Title))

    // If this sessions is already playing music, stop here, we dont want the bot to play over itself
    if call.playing {
        calls_mutx.Unlock()
        return nil
    }

    // For each song in the queue
    for {

        // Make sure the call still exists and has not been disconnected
        if call, exists = calls[guild_id]; !exists {
            calls_mutx.Unlock()
            return nil
        }

        // Only continue if there are more songs in the queue
        if len(calls[guild_id].queue) == 0 {
            calls_mutx.Unlock()
            return nil
        }
        // Create a new opus encoder that econdes pcm data into opus packets for discord
        opus_enc, err := gopus.NewEncoder(audio_sample_rate, audio_chan, gopus.Voip)
        if err != nil {
            calls_mutx.Unlock()
            return fmt.Errorf("unable to make encoder: %s", err.Error())
        }
        opus_enc.SetBitrate(audio_bitrate * 1000)
        
        // Set playing to true
        call.playing = true

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

        // Obtain youtube stream
        audio_stream, err := get_audio_stream(calls[guild_id].queue[0])    
        if err != nil {
            return err
        }
        
        // Inform the users what is up next
        title := calls[guild_id].queue[0].Title
        s.ChannelMessageSend(txt_chan, fmt.Sprintf("Now Playing: %s [%v]", title, calls[guild_id].queue[0].Duration)) 

        // Use FFMpeg to convert the M4A AAC encoded file into raw PCM data
        pcm_data_bytes, err := convert_m4a_pcm(audio_stream, calls[guild_id].ffm_ctx)
        if err != nil {
            return fmt.Errorf("converting m4a -> pcm: %s", err.Error())
        }

        // Get bytes from output of command, turn into int16 slices and send to encoding thread
        short_chan := make(chan []int16, 20) 
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

                        // Send to discord if the thread has not been cancelled
                        select {
                        case <- calls[guild_id].eas_ctx.Done():
                            log.Printf("eas cancelled\n")
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

        // Lockcalls
        calls_mutx.Lock()
        
        // Check if this call still exists
        call, exists := calls[guild_id]
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
        
        // We are no longer playing
        call.playing = false

        // Update calls map with new settings
        calls[guild_id] = call

        calls_mutx.Unlock()
        s.ChannelMessageSend(txt_chan, fmt.Sprintf("Stopped playing '%s' - %s", title, context.Cause(call.eas_ctx)))

        log.Printf("song complete / skipped / dc called - removed song from queue")

        calls_mutx.Lock()
    }
}

