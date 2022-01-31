package main

import (
	"encoding/json"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"os"
)

var Token = os.Getenv("Token")
var running = true

const BotToken = os.Getenv("BotToken")
const ChannelID = os.Getenv("ChannelId")

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func getAdmins() []string {
	return []string{}
}

// List of departments from https://my.edu.sharif.edu/server/common/data/departments.js
// Get the list with print(str(c['id']) + ':"' + c['name'].replace(' ', '_').replace('(','').replace(')','') + '",')
var departments = map[int]string{
	21: "مهندسی_صنایع",
	22: "علوم_ریاضی",
	24: "فیزیک",
	25: "مهندسی_برق",
	30: "مرکز_تربیت_بدنی",
	31: "مرکز_زبان‌ها_و_زبان‌شناسی",
	33: "مرکز_آموزش_مهارت‌های_مهندسی",
	35: "مرکز_گرافيک_مرکز_آموزش_مهارت‌های_مهندسی",
	37: "مرکز_معارف_اسلامی_و_علوم_انسانی",
	40: "مهندسی_کامپیوتر",
	42: "گروه_فلسفه_علم",
	44: "مدیریت_و_اقتصاد",
}

type CapacityType int

func (c CapacityType) String() string {
	if c == -1 {
		return "-"
	} else if c == 0 {
		return "∞"
	} else {
		return strconv.Itoa(int(c))
	}
}

type Course struct {
	ID               string       `json:"id"`
	Name             string       `json:"title"`
	Lecturer         string       `json:"instructors"`
	Capacity         CapacityType `json:"capacity"`
	ReservedCapacity int          `json:"reservedCapacity"`
	Registered       int          `json:"count"`
	Units            int          `json:"units"`
	Department       int          `json:"department"`
	Reserve          bool         `json:"reserve"`
}

func (c *Course) String() string {
	return c.Name + " - " + c.Lecturer + "\n#" + departments[c.Department] + "\nID: " + c.ID + "\nUnits: " + strconv.Itoa(c.Units) + "\nRegistered: " + strconv.Itoa(c.Registered) + "\nCapacity: " + c.Capacity.String()
}

func (c *Course) StringDiffCapacity(old CapacityType) string {
	return c.Name + " - " + c.Lecturer + "\n#" + departments[c.Department] + "\nID: " + c.ID + "\nUnits: " + strconv.Itoa(c.Units) + "\nRegistered: " + strconv.Itoa(c.Registered) + "\nCapacity: " + old.String() + " → " + c.Capacity.String()
}

func (c *Course) StringDiffRegistered(old int) string {
	return c.Name + " - " + c.Lecturer + "\n#" + departments[c.Department] + "\nID: " + c.ID + "\nUnits: " + strconv.Itoa(c.Units) + "\nRegistered: " + strconv.Itoa(old) + " → " + strconv.Itoa(c.Registered) + "\nCapacity: " + c.Capacity.String()
}

var courses = make(map[string]Course)

func sendMessage(text string, update tgbotapi.Update, bot *tgbotapi.BotAPI) error {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
	msg.ReplyToMessageID = update.Message.MessageID
	if _, err := bot.Send(msg); err != nil {
		return err
	}
	return nil
}

func sendFuckOff(update tgbotapi.Update, bot *tgbotapi.BotAPI) {
	if err := sendMessage("fuck off", update, bot); err != nil {
		log.Println(err)
	}
}

func readMessagesFromTelegram() {
	for {
		bot, err := tgbotapi.NewBotAPI(BotToken)
		if err != nil {
			panic(err)
		}

		updateConfig := tgbotapi.NewUpdate(0)
		updateConfig.Timeout = 30

		updates := bot.GetUpdatesChan(updateConfig)
		for update := range updates {
			if update.Message == nil {
				continue
			}
			if !contains(getAdmins(), strconv.FormatInt(update.Message.From.ID, 10)) {
				sendFuckOff(update, bot)
				continue
			}

			messageParts := strings.Split(update.Message.Text, " ")
			if len(messageParts) < 2 {
				sendFuckOff(update, bot)
				continue
			}

			if messageParts[0] != "Token" {
				sendFuckOff(update, bot)
				continue
			}

			Token = messageParts[1]
			running = false
			err = sendMessage("token is changed", update, bot)
			if err != nil {
				log.Println(err)
			}
		}
	}
}

func sendMessageToAdmins(message string) {
	urlTelegram := "https://api.telegram.org/bot" + BotToken + "/sendMessage"
	for _, adminId := range getAdmins() {
		_, err := http.PostForm(urlTelegram, url.Values{"chat_id": {adminId}, "text": {message}})
		if err != nil {
			log.Println(err)
		}
	}
}

func listenEdu(token string) {
	ws, _, err := websocket.DefaultDialer.Dial("wss://my.edu.sharif.edu/api/ws?token="+token, nil)
	if err != nil {
		log.Fatalln(err)
	}
	defer sendMessageToAdmins("socket is closed")
	defer ws.Close()
	firstScrap := true
	for running {
		var body struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		}
		err = ws.ReadJSON(&body)
		if err != nil {
			log.Println(err)
			return
		}
		if body.Type != "listUpdate" {
			continue
		}
		// Parse the json
		var lessons []Course
		err = json.Unmarshal(body.Message, &lessons)
		if err != nil {
			log.Println(err)
			return
		}
		// Check all lessons
		var message strings.Builder
		for _, course := range lessons {
			// Check the department
			_, exists := departments[course.Department]
			if !exists {
				continue
			}
			// Get the diff
			oldCourse, exists := courses[course.ID]
			if exists {
				// Diff!
				capacityChange := false
				if oldCourse.Registered > course.Registered {
					message.WriteString("change in registered:\n")
					message.WriteString(course.StringDiffRegistered(oldCourse.Registered))
					message.WriteString("\n\n")
					log.Println("registered", oldCourse, course)
				}
				if oldCourse.Capacity != course.Capacity {
					message.WriteString("change in capacity:\n")
					message.WriteString(course.StringDiffCapacity(oldCourse.Capacity))
					message.WriteString("\n\n")
					capacityChange = true
					log.Println("cap", oldCourse, course)
				}
				if !capacityChange && (oldCourse.Reserve == false && course.Reserve == true) {
					message.WriteString("now you can pick this lesson:\n")
					message.WriteString(course.String())
					message.WriteString("\n\n")
					log.Println("pick", oldCourse, course)
				}
			} else if !firstScrap {
				message.WriteString("new course added:\n")
				message.WriteString(course.String())
				message.WriteString("\n\n")
			}
			// Just replace the old one
			courses[course.ID] = course
		}
		// Send message
		if message.Len() != 0 {
			_, _ = http.PostForm("https://api.telegram.org/bot"+BotToken+"/sendMessage", url.Values{"chat_id": {ChannelID}, "text": {message.String()}})
		}
		firstScrap = false
	}
}

func main() {
	defer sendMessageToAdmins("bot exited")
	sendMessageToAdmins("bot starts")
	go readMessagesFromTelegram()
	for {
		listenEdu(Token)
		running = true
	}
}
