package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/bwmarrin/discordgo"
)

var (
    calls = map[string]*discordgo.VoiceConnection{}
    calls_mutx sync.Mutex
)

const (
    audio_chan int = 2
    audio_frame_size int = 960
    sample_rate int = 48000
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
    audio_stream, err := get_audio_stream(arg)    
    if err != nil {
        return err
    }

    // Open a file
    var fn string = "output/testfile"
    fout, err := os.OpenFile(fn+".m4a", os.O_CREATE | os.O_WRONLY, 0664)
    if err != nil {
        return fmt.Errorf("unable to open m4a file: %s", err.Error())
    }

    // Write to the file
    io.Copy(fout, audio_stream)

    // Convert using FFMPEG
    cmd := fmt.Sprintf("ffmpeg -i %s.m4a -f s16le -ar 48000 -ac 2 pipe:1 | dca > %s.dca", fn, fn)
    c := exec.Command("bash", "-c", cmd)
    err = c.Run()
    if err != nil {
        return fmt.Errorf("ffmpeg: %s\n", err.Error())
    }
    

    dca_file, err := os.Open(fn+".dca")
    if err != nil {
        return fmt.Errorf("unable to open dca file: %s", err.Error())
    }

    var reading bool = true
    var opuslen int16
    for reading {
 
        err = binary.Read(dca_file, binary.LittleEndian, &opuslen)
        if err != nil {
            if err == io.EOF {
                break
            } else if err == io.ErrUnexpectedEOF {
                reading = false
            } else {
                return fmt.Errorf("error while reading dca file: %s", err.Error())
            }
        }

        opus_data := make([]byte, opuslen)
        err = binary.Read(dca_file, binary.LittleEndian, &opus_data)
        if err != nil {
            return fmt.Errorf("error while reading data from dca file: %s\n", err.Error())
        }

        call.OpusSend <- opus_data

    }

    err = call.Speaking(false)
    if err != nil {
        log.Printf("error stopping speaking\n")
    }

    return nil
}

func convert_m4a_opus() {
    
}
/*

    err = call.Speaking(true)
    if err != nil {
        return err
    }
    
    mpeg_buf, err := mpeg.NewBuffer(audio_stream)
    if err != nil {
        return fmt.Errorf("unable to create mpeg buffer: %s", err.Error())
    }
    mpeg_buf.SetLoadCallback(mpeg_buf.LoadReaderCallback)
    
    mpeg_audio := mpeg.NewAudio(mpeg_buf)
    mpeg_audio.Rewind()

    log.Printf("samplerate: %v\n", mpeg_audio.Samplerate())

    pcm_data := []int16{}
    for !mpeg_audio.HasEnded() {
        samples := mpeg_audio.Decode()
        if samples == nil {
            log.Printf("samples is nil\n")
            break
        }

        pcm_data = append(pcm_data, samples.S16...)
    }

*/
