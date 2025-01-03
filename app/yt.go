package main

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
)


func download_cmd(s *discordgo.Session, m *discordgo.MessageCreate) {
    cmd_sections := strings.Split(m.Content[1:], " ")

    // Make sure command is formatted correctly
    if len(cmd_sections) != 2 {
        s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("invalid syntax: use `%cdl [link]`", settings.cmd_prefix))
        return
    }

    // Get the file and upload it to discord
    get_file(s, m.ChannelID, cmd_sections[1]) 
    
}


func get_file(s *discordgo.Session, channel_id string, argument string) {
    // Get the video based on the argument
    video, err := get_video(argument) 
    if err != nil {
        log.Printf("failed to get video: %s\n", err.Error())
        return 
    }

    // Get the stream based on the argument
    stream, err := get_audio_stream(video)
    if err != nil {
        log.Printf("failed to get audio stream: %s\n", err.Error())
        return
    }

    // Send the raw m4a data to discord as a file upload
    s.ChannelMessageSend(channel_id, fmt.Sprintf("Here is the audio for %s", video.Title))
    s.ChannelFileSend(channel_id, "song.m4a", stream)
}


func get_video(argument string) (*youtube.Video, error) {
    client := youtube.Client{}

    var id string
    var err error

    // If the provided argument is a URL, just use that
    if strings.HasPrefix(argument, "http://") || strings.HasPrefix(argument, "https://") {
        id, err = youtube.ExtractVideoID(argument)
        if err != nil {
            return nil, err
        }
    } else {
        // otherwise, assume the user wants to search

        // TODO : Implement youtube search
        return nil, fmt.Errorf("search not currently implemented")
    }
    
    // Obtain a video object based on the video ID
    video, err := client.GetVideo(id)
    if err != nil {
        return nil, err
    }

    return video, nil
    
}


func get_audio_stream(video *youtube.Video) (io.ReadCloser, error) {
    client := youtube.Client{}

    formats := video.Formats.WithAudioChannels()
    if len(formats) < 1 {
        return nil, fmt.Errorf("no formats returned")
    }

    stream, _, err := client.GetStream(video, &formats[0])
    return stream, err

}   
