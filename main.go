package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func getEnvDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

var telegramToken = os.Getenv("TELEGRAM_TOKEN")
var chatId = os.Getenv("CHAT_ID")
var cameraAddresses = os.Getenv("RTSP_ADDRESSES")
var rtspUsername = os.Getenv("RTSP_USERNAME")
var rtspPassword = os.Getenv("RTSP_PASSWORD")
var rtspUrl = getEnvDefault("RTSP_URL", "/")
var ffmpegBin = getEnvDefault("FFMPEG_BIN", "/usr/bin/ffmpeg")
var snapshotDir = getEnvDefault("SNAPSHOT_DIR", "/dev/shm/elevator_snapshots")
var snapshotSequence, _ = strconv.ParseInt(getEnvDefault("SNAPSHOT_SEQUENCE", "3"), 10, 8)
var snapshotSequenceInterval, _ = strconv.ParseInt(getEnvDefault("SNAPSHOT_SEQUENCE_INTERVAL", "2000"), 10, 64)

var numericChatId, _ = strconv.ParseInt(chatId, 10, 64)

var bot *tgbotapi.BotAPI

func Capture(rtspUrl string, streamName string) {
	snapshotPath := fmt.Sprintf("%s/%s.jpg", snapshotDir, streamName)
	var initial = true
	for {
		prevSt, prevStErr := os.Stat(snapshotPath)
		prevMtime := time.Unix(0, 0)
		if prevStErr == nil {
			prevMtime = prevSt.ModTime()
		}
		lastMtime := time.Unix(0, 0)

		for range [3]struct{}{} {
			cmd := exec.Command(
				ffmpegBin,
				"-y", "-timeout", "1000000", "-re", "-rtsp_transport", "tcp", "-i",
				rtspUrl, "-an", "-vf", "select='eq(pict_type,PICT_TYPE_I)'",
				"-vsync", "vfr", "-q:v", "23", "-update", "1", snapshotPath,
			)
			_ = cmd.Run()
			lastSt, lastStErr := os.Stat(snapshotPath)
			if lastStErr == nil {
				lastMtime = lastSt.ModTime()
			}
			if prevMtime != lastMtime || initial {
				log.Println(fmt.Sprintf("FFmpeg for %s has failed", streamName))
			}
		}

		if prevMtime != lastMtime {
			SendVideo(streamName)
		}
		initial = false
	}
}

func MaintainHistory(streamName string, interval int64) {
	snapshotPath := fmt.Sprintf("%s/%s.jpg", snapshotDir, streamName)
	nameTemplate := fmt.Sprintf("%s/%s-%%d.jpg", snapshotDir, streamName)
	snapshotPaths := make([]string, snapshotSequence)
	for i := range snapshotPaths {
		snapshotPaths[i] = fmt.Sprintf(nameTemplate, i+1)
	}
	for {
		_, err := os.Stat(snapshotPath)
		if os.IsNotExist(err) {
			//log.Println("File does not exist:", snapshotPath)
			time.Sleep(time.Duration(interval) * time.Millisecond)
			continue
		} else if err != nil {
			log.Println("Unable to obtain file info", err)
			time.Sleep(time.Duration(interval) * time.Millisecond)
			continue
		}

		if _, err := os.Stat(snapshotPaths[len(snapshotPaths)-1]); !os.IsNotExist(err) {
			_ = os.Remove(snapshotPaths[len(snapshotPaths)-1])
		}

		for i := len(snapshotPaths) - 1; i > 0; i-- {
			if _, err := os.Stat(snapshotPaths[i-1]); !os.IsNotExist(err) {
				_ = os.Rename(snapshotPaths[i-1], snapshotPaths[i])
			}
		}

		input, err := ioutil.ReadFile(snapshotPath)
		if err != nil {
			log.Println("Unable to read file", err)
			time.Sleep(time.Duration(interval) * time.Millisecond)
			continue
		}

		err = ioutil.WriteFile(snapshotPaths[0], input, 0644)
		if err != nil {
			log.Println("Unable to write file", err)
			time.Sleep(time.Duration(interval) * time.Millisecond)
			continue
		}

		files, err := ioutil.ReadDir(snapshotDir)
		if err != nil {
			log.Println("Unable to read directory", err)
			time.Sleep(time.Duration(interval) * time.Millisecond)
			continue
		}

		var unwantedFiles []string
		prefix := fmt.Sprintf("%s-", streamName)
		for _, file := range files {
			if strings.HasPrefix(file.Name(), prefix) && strings.HasSuffix(file.Name(), filepath.Ext(snapshotPath)) {
				match := false
				for _, snapshotPath := range snapshotPaths {
					if file.Name() == filepath.Base(snapshotPath) {
						match = true
						break
					}
				}
				if !match {
					unwantedFiles = append(unwantedFiles, file.Name())
				}
			}
		}

		for _, fileName := range unwantedFiles {
			err = os.Remove(filepath.Join(snapshotDir, fileName))
			if err != nil {
				log.Println("Unable to remove unwanted file", err)
				time.Sleep(time.Duration(interval) * time.Millisecond)
				continue
			}
		}

		time.Sleep(time.Duration(interval) * time.Millisecond)
	}
}

func SendSnap(streamName string) {
	snapshotPath := fmt.Sprintf("%s/%s.jpg", snapshotDir, streamName)
	if _, err := os.Stat(snapshotPath); err == nil {
		photoFileBytes := tgbotapi.FilePath(snapshotPath)
		msg := tgbotapi.NewPhoto(numericChatId, photoFileBytes)
		if _, err := bot.Send(msg); err != nil {
			log.Println(err)
		}
	} else {
		msg := tgbotapi.NewMessage(numericChatId, "🚫 Snapshot doesn't exist")
		if _, err := bot.Send(msg); err != nil {
			log.Println(err)
		}
	}
}

func SendVideo(streamName string) {
	mp4Path := fmt.Sprintf("%s/%s.mp4", snapshotDir, streamName)
	cmd := exec.Command(
		ffmpegBin,
		"-y", "-r", "1", "-i",
		fmt.Sprintf("%s/%s-%%d.jpg", snapshotDir, streamName),
		"-an", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-vf", "tpad=stop_mode=clone:stop=1", mp4Path,
	)
	log.Println(cmd.String())
	_ = cmd.Run()
	if _, err := os.Stat(mp4Path); err == nil {
		videoFileBytes := tgbotapi.FilePath(mp4Path)
		msg := tgbotapi.NewVideo(numericChatId, videoFileBytes)
		if _, err := bot.Send(msg); err != nil {
			log.Println(err)
		}
	} else {
		msg := tgbotapi.NewMessage(numericChatId, "🚫 Failed to create video")
		if _, err := bot.Send(msg); err != nil {
			log.Println(err)
		}
	}
	_ = os.Remove(mp4Path)
}

func main() {
	var err error
	err = os.MkdirAll(snapshotDir, 0o0755)
	if err != nil {
		log.Fatal(err)
	}
	addresses := strings.Split(cameraAddresses, ",")
	for i := 0; i < len(addresses); i++ {
		streamName := fmt.Sprintf("snap%d", i)
		go Capture(
			fmt.Sprintf("rtsp://%s:%s@%s%s", rtspUsername, rtspPassword, addresses[i], rtspUrl),
			streamName,
		)
		go MaintainHistory(streamName, snapshotSequenceInterval)
	}
	bot, err = tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Panic(err)
	}

	log.Println("Authorized on account:", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message != nil {
			log.Println(
				fmt.Sprintf("Got message from chat %d from %s", update.Message.Chat.ID, update.Message.From),
			)

			if update.Message.Chat.ID != numericChatId {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "🚫 Access denied")
				if _, err := bot.Send(msg); err != nil {
					log.Println(err)
				}
				continue
			}

			if !update.Message.IsCommand() {
				continue
			}

			if match, err := regexp.MatchString("^snap\\d$", update.Message.Command()); err == nil && match {
				snapNum := strings.TrimPrefix(update.Message.Command(), "snap")
				SendSnap("snap" + snapNum)
			} else if match, err := regexp.MatchString("^vid\\d$", update.Message.Command()); err == nil && match {
				snapNum := strings.TrimPrefix(update.Message.Command(), "vid")
				SendVideo("snap" + snapNum)
			} else {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "🚫 Unsupported command")
				if _, err := bot.Send(msg); err != nil {
					log.Println(err)
				}
			}
		}
	}
}
