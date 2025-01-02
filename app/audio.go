package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)


func convert_m4a_pcm(audio_stream io.ReadCloser) (io.ReadCloser, error) {
    cmd := fmt.Sprintf("ffmpeg -i pipe:.m4a -f s16le -ar 48000 -ac 2 pipe:1")
    c := exec.Command("bash", "-c", cmd)
    c.Stderr = os.Stderr
    cstdin, err := c.StdinPipe()
    if err != nil {
        return nil, fmt.Errorf("getting stdin: %s", err.Error())
    }
    cstdout, err := c.StdoutPipe()
    if err != nil {
        return nil, fmt.Errorf("getting stdout: %s", err.Error())
    }
    log.Printf("got pipes\n")

    err = c.Start()
    if err != nil {
        return nil, fmt.Errorf("ffmpeg: %s\n", err.Error())
    }
    log.Printf("started command\n")

    go func() {
        io.Copy(cstdin, audio_stream)
    }()

    
    return cstdout, nil

}


func pcm_bts(byte_stream io.ReadCloser, short_chan chan []int16) (error) {
    var reading bool = true

    defer close(short_chan)
    defer byte_stream.Close()

    for reading {
        buf := make([]int16, audio_frame_size*audio_chan)

        err := binary.Read(byte_stream, binary.LittleEndian, &buf)

        if err == io.EOF {
            break
        } else if err == io.ErrUnexpectedEOF {
            reading = false
        } else if err != nil {
            return err
        }

        short_chan <- buf
    }

    return nil
}

