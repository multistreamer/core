package main

import (
	"bufio"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	inputAudio = "res/audio.mp3"
	hlsDir     = "tmp/hls"
	textFile   = "res/text.txt"
	chunkSec   = 2.0
)

func main() {
	err := os.MkdirAll("tmp/hls", 0755)
	if err != nil {
		log.Fatalf("failed to create tmp/hls dir: %s", err)
	}

	err = runFFmpegHLS(inputAudio, hlsDir)
	if err != nil {
		log.Fatalf("failed to generate HLS: %s", err)
	}
	log.Println("HLS segments generated.")

	vttFile := filepath.Join(hlsDir, "subtitles.vtt")
	err = createWebVTT(textFile, vttFile, chunkSec)
	if err != nil {
		log.Fatalf("failed to create WebVTT: %s", err)
	}
	log.Println("Subtitles generated.")

	fs := http.FileServer(http.Dir(hlsDir))
	http.Handle("/hls/", http.StripPrefix("/hls/", fs))

	http.HandleFunc("/", serveIndex)

	addr := ":8080"
	log.Printf("Server listening on %s ...", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func runFFmpegHLS(audioPath, outputDir string) error {
	args := []string{
		"-y", // overwrite existing files
		"-i", audioPath,
		"-c", "copy", // no re-encoding
		"-hls_time", fmt.Sprintf("%.0f", math.Round(chunkSec)),
		"-hls_list_size", "0",
		"-f", "hls",
		filepath.Join(outputDir, "playlist.m3u8"),
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

func createWebVTT(textFile, vttFile string, chunkSec float64) error {
	f, err := os.Open(textFile)
	if err != nil {
		return fmt.Errorf("open text file: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan text file: %w", err)
	}

	out, err := os.Create(vttFile)
	if err != nil {
		return fmt.Errorf("create vtt file: %w", err)
	}
	defer out.Close()

	_, _ = out.WriteString("WEBVTT\n\n")

	for i, text := range lines {
		startTime := chunkSec * float64(i)
		endTime := chunkSec * float64(i+1)

		_, err = fmt.Fprintf(out, "%d\n%s --> %s\n%s\n\n",
			i+1,
			formatVTTTime(startTime),
			formatVTTTime(endTime),
			text)
		if err != nil {
			return fmt.Errorf("write vtt file: %w", err)
		}
	}

	return nil
}

func formatVTTTime(sec float64) string {
	d := time.Duration(sec * float64(time.Second))
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	millis := d.Milliseconds() % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

func serveIndex(w http.ResponseWriter, _ *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := tmpl.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
