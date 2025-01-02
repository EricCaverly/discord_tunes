package main

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
)

func get_file(s *discordgo.Session, channel_id string, argument string) {

    stream, err := get_audio_stream(argument)
    if err != nil {
        log.Printf("failed to get audio stream: %s\n", err)
        return
    }

    s.ChannelMessageSend(channel_id, "Here is the song:")
    s.ChannelFileSend(channel_id, "song.m4a", stream)
}

func get_audio_stream(argument string) (io.ReadCloser, error) {
    var id string
    var err error

    if strings.HasPrefix(argument, "http://") || strings.HasPrefix(argument, "https://") {
        id, err = youtube.ExtractVideoID(argument)
        if err != nil {
            return nil, err
        }
    } else {
    }
    
    client := youtube.Client{}
    video, err := client.GetVideo(id)
    if err != nil {
        return nil, err
    }

    formats := video.Formats.WithAudioChannels()
    if len(formats) < 1 {
        return nil, fmt.Errorf("no formats returned")
    }

    log.Printf("The format chosen is %#v\n", formats[0])

    stream, _, err := client.GetStream(video, &formats[0])
    return stream, err

}   
