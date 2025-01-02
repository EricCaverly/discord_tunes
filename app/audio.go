package main

import (
	"bytes"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)


func read_m4a(data []byte) (*audio.IntBuffer, error) {
    decoder := wav.NewDecoder(bytes.NewReader(data))
    buf, err := decoder.FullPCMBuffer()
    if err != nil {
        return nil, err
    }
    return buf, nil
}


func convert_to_opus(pcm *audio.IntBuffer, sampleRate int) ([]byte, error) {

    

    return nil, nil
}



    /* Download the video - save for later download command 
    file, err := os.OpenFile("output/out.m4a", os.O_CREATE | os.O_WRONLY, 0664)
    if err != nil {
        return err
    }
    
    log.Printf("writing to file\n")

    _, err = file.Write(audio_contents)
    if err != nil {
        return err
    }

    file.Close()

    log.Printf("done writing to file\n")
    */
