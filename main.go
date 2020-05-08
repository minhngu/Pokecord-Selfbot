package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/devedge/imagehash"
	"github.com/sirupsen/logrus"

	discord "github.com/bwmarrin/discordgo"
)

type Config struct {
	Token     string   `json:"token"`
	WhiteList []string `json:"white_list"`
	LimitIV   float64  `json:"limit_iv"`
}

var (
	config              Config
	hashMap             map[string]string
	pkmNameMap          map[string]string
	recentlyCaught      bool
	recentlyCaughtName  string
	recentlyCaughtID    string
	recentlyCaughtPrice string
	spamChan            chan bool
	isSpamming          bool
)

func getArt(m map[string]string) {
	baseURL := "https://www.serebii.net/pokemon/art/"
	for i, v := range m {
		url := baseURL + i + ".png"
		// don't worry about errors
		response, err := http.Get(url)
		if err != nil {
			log.Fatal(err)
		}
		defer response.Body.Close()

		// open a file for writing
		filePath := "sprite/" + v + ".png"
		file, err := os.Create(filePath)
		if err != nil {
			log.Fatal(err)
		}

		// Use io.Copy to just dump the response body to the file. This supports huge files
		_, err = io.Copy(file, response.Body)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
	}
}
func sleep() {
	sleepTime := rand.Intn(5) + 2
	time.Sleep(time.Duration(sleepTime) * time.Second)
	return
}
func main() {
	// Get user's config
	config := getConfig()
	// Get a map of all pokemon
	pkmNameMap = getNameMap()
	// Generate a map of pokemon hash
	hashMap = getHashMap(pkmNameMap)

	// Create a new Discord session using the provided bot token.
	dg, err := discord.New(config.Token)
	if err != nil {
		logrus.Fatalf("Error creating Discord session: ", err)
		return
	}

	// Register messageCreate as a callback for the messageCreate events.
	dg.AddHandler(messageCreate)

	// Open the websocket and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

// Get user's config by parsing config file
func getConfig() *Config {
	configFile, err := ioutil.ReadFile("config.json")
	if err != nil {
		logrus.Errorf("Failed to read config file: #%v ", err)
	}
	err = json.Unmarshal(configFile, &config)
	if err != nil {
		logrus.Errorf("Failed to unmarshal: %v", err)
	}
	logrus.Infof("Sorting white list...")
	sort.Strings(config.WhiteList)
	logrus.Infof("Sorting white list complete...")
	return &config
}

func getNameMap() map[string]string {
	m := make(map[string]string)
	file, err := os.Open("pokemon.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		split := strings.Split(line, " ")
		m[split[0]] = split[1]
	}
	return m
}

func getHashMap(m map[string]string) map[string]string {
	hashMap = make(map[string]string)
	logrus.Infof("Generating hash map")
	for _, v := range m {
		filePath := "sprite/" + v + ".png"
		src, err := imagehash.OpenImg(filePath)
		if err != nil {
			logrus.Fatal(err)
		}
		hash, err := imagehash.Ahash(src, 8)
		if err != nil {
			logrus.Fatal(err)
		}
		hashMap[string(hash)] = v
	}
	logrus.Infof("Generating hash map complete...")
	return hashMap
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discord.Session, m *discord.MessageCreate) {
	if m.Author.Username == "Pokécord" {
		var pkmName string
		var err error
		for _, v := range m.Embeds {
			if strings.Contains(v.Description, "Guess") {
				pkmName, err = getPokemonString(m, v)
				if err != nil {
					logrus.Error(err)
				}
				// send catch command
				if pkmName != "" {
					sleep()
					s.ChannelMessageSend(m.ChannelID, "p!catch "+pkmName)
					recentlyCaught = true
					recentlyCaughtName = pkmName
					if isWhiteList(config.WhiteList, recentlyCaughtName) {
						return
					}
					getLatest := "p!info latest"
					sleep()
					s.ChannelMessageSend(m.ChannelID, getLatest)
				}
				break
			}
			if recentlyCaught && (v.Footer != nil) && strings.Contains(v.Footer.Text, "Displaying") {
				posFirst := strings.Index(v.Footer.Text, "Displaying Pokémon: ")
				posLast := strings.Index(v.Footer.Text, " - ")
				test := v.Footer.Text[posFirst+len("Displaying Pokémon: ") : posLast]
				strArry := strings.Split(test, "/")
				recentlyCaughtID = strArry[0]
				strArr := strings.SplitAfter(v.Description, "Total IV %:** ")
				pkmIV := strArr[1]
				iv, _ := strconv.ParseFloat(pkmIV[:len(pkmIV)-1], 64)
				if math.Floor(iv) >= config.LimitIV {
					return
				}
				marketSearchQuery := "p!market search --name " + recentlyCaughtName + " --iv > " + fmt.Sprintf("%f", math.Floor(iv)) + " --order price a"
				sleep()
				s.ChannelMessageSend(m.ChannelID, marketSearchQuery)
				break
			}
			if recentlyCaught && strings.Contains(v.Title, "Pokécord Market") {
				market := strings.SplitAfter(v.Description, "\n**")
				posPrice := strings.Index(market[1], "Price: ")
				posCreds := strings.Index(market[1], " Credits")
				recentlyCaughtPrice = market[1][posPrice+len("Price: ") : posCreds]
				listQuery := "p!market list " + recentlyCaughtID + " " + recentlyCaughtPrice
				sleep()
				s.ChannelMessageSend(m.ChannelID, listQuery)
				sleep()
				s.ChannelMessageSend(m.ChannelID, "p!confirmlist")
				recentlyCaught = false
			}
		}
		return
	}
	if strings.HasPrefix(m.Content, "/spam") {
		suffix := strings.TrimPrefix(m.Content, "/spam ")
		if !isSpamming && suffix == "on" {
			spamChan = spam(s, m)
			return
		}
		if isSpamming && suffix == "off" {
			spamChan <- true
			close(spamChan)
			return
		}

		_, err := s.ChannelMessageSend(m.ChannelID, "I don't understand your command")
		if err != nil {
			logrus.Errorf("failed to send message")
		}
	}
	if strings.HasPrefix(m.Content, "generate_data") {
		getArt(pkmNameMap)
	}
}

func getPokemonString(m *discord.MessageCreate, v *discord.MessageEmbed) (string, error) {
	url := v.Image.URL
	response, err := http.Get(url)
	if err != nil {
	}
	defer response.Body.Close()

	//open a file for writing
	file, err := os.Create("pokemon.png")
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Use io.Copy to just dump the response body to the file. This supports huge files
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return "", err
	}
	src, err := imagehash.OpenImg("pokemon.png")
	if err != nil {
		return "", err
	}
	hash, err := imagehash.Ahash(src, 8)
	if err != nil {
		return "", err
	}
	pokemon := hashMap[string(hash)]
	return pokemon, nil
}

func spam(s *discord.Session, m *discord.MessageCreate) chan bool {
	isSpamming = true
	stop := make(chan bool, 1)
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				sleep()
				_, err := s.ChannelMessageSend(m.ChannelID, "spam")
				if err != nil {
					logrus.Errorf("failed to send spam")
				}
			case <-stop:
				ticker.Stop()
				logrus.Infof("Ticker Stopped")
				return
			}
		}
	}()
	return stop
}

func isWhiteList(whiteList []string, pkmName string) bool {
	for _, a := range whiteList {
		if a == pkmName {
			return true
		}
	}
	return false
}
