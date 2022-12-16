package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"
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
var cam1Address = os.Getenv("E1_ADDRESS")
var cam2Address = os.Getenv("E2_ADDRESS")
var rtspUsername = os.Getenv("RTSP_USERNAME")
var rtspPassword = os.Getenv("RTSP_PASSWORD")
var rtspUrl = getEnvDefault("RTSP_URL", "/Streaming/Channels/101")
var ffmpegBin = getEnvDefault("FFMPEG_BIN", "/usr/bin/ffmpeg")
var snapshotDir = getEnvDefault("SNAPSHOT_DIR", "/dev/shm/elevator_snapshots")

var numericChatId, _ = strconv.ParseInt(chatId, 10, 64)

var bot *tgbotapi.BotAPI

func capture(rtspUrl string, snapshotPath string, streamName string) {
	for {
		prevSt, prevStErr := os.Stat(snapshotPath)
		ctimePrev := time.Unix(0, 0)
		if prevStErr == nil {
			prevStat := prevSt.Sys().(*syscall.Stat_t)
			ctimePrev = time.Unix(int64(prevStat.Ctimespec.Sec), int64(prevStat.Ctimespec.Nsec))
		}

		cmd := exec.Command(
			ffmpegBin,
			"-y", "-timeout", "1000000", "-re", "-rtsp_transport", "tcp", "-i",
			rtspUrl, "-an", "-vf", "select='eq(pict_type,PICT_TYPE_I)'",
			"-vsync", "vfr", "-q:v", "23", "-update", "1", snapshotPath,
		)
		_ = cmd.Run()

		lastSt, lastStErr := os.Stat(snapshotPath)
		ctimeLast := time.Unix(0, 0)
		if lastStErr == nil {
			lastStat := lastSt.Sys().(*syscall.Stat_t)
			ctimeLast = time.Unix(int64(lastStat.Ctimespec.Sec), int64(lastStat.Ctimespec.Nsec))
		}
		log.Println(fmt.Sprintf("FFmpeg for %s has failed", streamName))
		if ctimeLast != ctimePrev {
			SendSnap(snapshotPath)
		}
	}
}

func SendSnap(snapshotPath string) {
	photoFileBytes := tgbotapi.FilePath(snapshotPath)
	msg := tgbotapi.NewPhoto(numericChatId, photoFileBytes)
	if _, err := bot.Send(msg); err != nil {
		log.Println(err)
	}
}

func main() {
	var err error
	err = os.MkdirAll(snapshotDir, 0o0755)
	if err != nil {
		log.Fatal(err)
	}
	go capture(
		fmt.Sprintf("rtsp://%s:%s@%s%s", rtspUsername, rtspPassword, cam1Address, rtspUrl),
		fmt.Sprintf("%s/%s", snapshotDir, "snap1.jpg"),
		"cam1",
	)
	go capture(
		fmt.Sprintf("rtsp://%s:%s@%s%s", rtspUsername, rtspPassword, cam2Address, rtspUrl),
		fmt.Sprintf("%s/%s", snapshotDir, "snap2.jpg"),
		"cam2",
	)
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

			switch update.Message.Command() {
			case "snap1":
				SendSnap(fmt.Sprintf("%s/%s", snapshotDir, "snap1.jpg"))
			case "snap2":
				SendSnap(fmt.Sprintf("%s/%s", snapshotDir, "snap2.jpg"))
			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "🚫 Unsupported command")
				if _, err := bot.Send(msg); err != nil {
					log.Println(err)
				}
			}
		}
	}
}
