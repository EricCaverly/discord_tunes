package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)


func convert_m4a_pcm(audio_stream io.ReadCloser, ctx context.Context) (io.ReadCloser, error) {
    // Build the FFmpeg command using pipes for stdin and stdout
    // This removes the need to write any data to a file on the disk, as all audio data gets sent / recevied directrly between this program and ffmpeg
    cmd := fmt.Sprintf("ffmpeg -i pipe:.m4a -f s16le -ar 48000 -ac 2 pipe:1")

    // Build the command with a cancellable context
    c := exec.CommandContext(ctx, "bash", "-c", cmd)

    // Stderr for ffmpeg error messages
    c.Stderr = os.Stderr
    
    // Obtain the pipes that this program will communicate with
    cstdin, err := c.StdinPipe()
    if err != nil {
        return nil, fmt.Errorf("getting stdin: %s", err.Error())
    }
    cstdout, err := c.StdoutPipe()
    if err != nil {
        return nil, fmt.Errorf("getting stdout: %s", err.Error())
    }
    log.Printf("got pipes\n")

    // Start the command without waiting for it to complete
    err = c.Start()
    if err != nil {
        return nil, fmt.Errorf("ffmpeg: %s\n", err.Error())
    }
    log.Printf("started command\n")

    // In another thread, copy audio stream from youtube into ffmpeg's input directly
    go func() {
        io.Copy(cstdin, audio_stream)
    }()
    
    // Return the stdout reader
    return cstdout, nil

}


func pcm_bts(byte_stream io.ReadCloser, short_chan chan []int16, guild_id string) (error) {
    var reading bool = true

    // While we should still be reading from ffmpeg
    for reading {
        // Make sure this function has not been cancelled
        select {
        case <- calls[guild_id].bts_ctx.Done():
            log.Printf("pcm cancelled\n")
            return nil
        default:
            // Make a buffer for audio data that is just the right amount of size for the amount of PCM data that can be encoded into an OPUS frame
            buf := make([]int16, audio_frame_size*audio_chan)

            // Read output from ffmpeg into the buffer
            err := binary.Read(byte_stream, binary.LittleEndian, &buf)

            // if we got to EOF, break out of the loop
            if err == io.EOF {
                log.Printf("EOF reached in FFMPEG\n")
                return nil

            // otherwise, there was an actual problem
            } else if err != nil {
                return err
            }

            // Since the short_chan buffer can become full, another select is needed here so that this thread
            // does not get stuck and the skip / dc commands can work properly
            select {
            // Pass the buffer data into the short_chan channel
            case short_chan <- buf:
                continue
            // Check if this thread was cancelled via context
            case <- calls[guild_id].bts_ctx.Done():
                log.Printf("cancelled in second select\n")
                return nil
            }
        }
    }

    return nil
}

