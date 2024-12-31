package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)


func get_audio(argument string) {
    var url string

    if strings.HasPrefix(argument, "http://") || strings.HasPrefix(argument, "https://") {
        url = argument
    } else {
        // Search Logic Here
    }

    client := &http.Client {
        Timeout: 30 * time.Second,
    }

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
    

    resp, err := client.Do(req)
    if err != nil {
        log.Printf("Error performing request\n")
        return
    }
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Printf("Error reading body\n")
        return
    }
    resp.Body.Close()

    log.Printf("%#v\n", string(body))


}   
