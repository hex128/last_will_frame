package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"os"
	"os/exec"
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
var rtspUrl = getEnvDefault("RTSP_URL", "/Streaming/Channels/101")
var ffmpegBin = getEnvDefault("FFMPEG_BIN", "/usr/bin/ffmpeg")
var snapshotDir = getEnvDefault("SNAPSHOT_DIR", "/dev/shm/elevator_snapshots")

var numericChatId, _ = strconv.ParseInt(chatId, 10, 64)

var bot *tgbotapi.BotAPI

func capture(rtspUrl string, snapshotPath string, streamName string) {
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
			SendSnap(snapshotPath)
		}
		initial = false
	}
}

func SendSnap(snapshotPath string) {
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

func main() {
	var err error
	err = os.MkdirAll(snapshotDir, 0o0755)
	if err != nil {
		log.Fatal(err)
	}
	addresses := strings.Split(cameraAddresses, ",")
	for i := 0; i < len(addresses); i++ {
		go capture(
			fmt.Sprintf("rtsp://%s:%s@%s%s", rtspUsername, rtspPassword, addresses[i], rtspUrl),
			fmt.Sprintf("%s/snap%d.jpg", snapshotDir, i),
			fmt.Sprintf("cam%d", i),
		)
	}
	bot, err = tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

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
				SendSnap(fmt.Sprintf("%s/%s.jpg", snapshotDir, update.Message.Command()))
			} else {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "🚫 Unsupported command")
				if _, err := bot.Send(msg); err != nil {
					log.Println(err)
				}
			}
		}
	}
}
