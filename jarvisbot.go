package jarvisbot

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/tucnak/telebot"
)

var exchange_rate_bucket_name = []byte("rates")

// JarvisBot is the main struct. All response funcs bind to this.
type JarvisBot struct {
	bot  *telebot.Bot
	log  *log.Logger
	fmap FuncMap
	db   *bolt.DB
}

// Wrapper struct for a message
type message struct {
	Cmd  string
	Args []string
	*telebot.Message
}

// A FuncMap is a map of command strings to response functions.
// It is use for routing comamnds to responses.
type FuncMap map[string]ResponseFunc

type ResponseFunc func(m *message)

// Initialise a JarvisBot.
// lg and fmap are optional. If no FuncMap is provided, JarvisBot will
// be initialised with a default FuncMap
func InitJarvis(bot *telebot.Bot, lg *log.Logger, fmap FuncMap) *JarvisBot {
	if lg == nil {
		lg = log.New(os.Stdout, "[jarvis] ", 0)
	}
	j := &JarvisBot{bot: bot, log: lg, fmap: fmap}

	if fmap == nil {
		j.fmap = j.GetDefaultFuncMap()
	}

	// Setup database
	db, err := bolt.Open("jarvis.db", 0600, nil)
	if err != nil {
		lg.Fatal(err)
	}
	j.db = db
	createAllBuckets(db)

	return j
}

// Get the built-in, default FuncMap.
func (j *JarvisBot) GetDefaultFuncMap() FuncMap {
	return FuncMap{
		"/hello": j.SayHello,
		"/echo":  j.Echo,
		"/xchg":  j.Exchange,
		"/rxr":   j.Retrieve,
	}
}

// Test function to explore db.
func (j *JarvisBot) Retrieve(msg *message) {
	if j.ratesAreEmpty() {
		j.RetrieveAndSaveExchangeRates()
	}

	err := j.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(exchange_rate_bucket_name)
		b.ForEach(func(k, v []byte) error {
			f, err := strconv.ParseFloat(string(v), 64)
			if err != nil {
				return err
			}
			fmt.Printf("key=%s, value=%s\n", string(k), f)
			return nil
		})
		return nil
	})

	if err != nil {
		j.log.Println(err)
	}
}

// Set a custom FuncMap.
func (j *JarvisBot) SetFuncMap(fmap FuncMap) {
	j.fmap = fmap
}

// Route received Telegram messages to the appropriate response functions.
func (j *JarvisBot) Router(msg *telebot.Message) {
	jmsg := parseMessage(msg)
	execFn := j.fmap[jmsg.Cmd]

	if execFn != nil {
		j.GoSafely(func() { execFn(jmsg) })
	}
}

func (j *JarvisBot) CloseDB() {
	j.db.Close()
}

// GoSafely is a utility wrapper to recover and log panics in goroutines.
// If we use naked goroutines, a panic in any one of them crashes
// the whole program. Using GoSafely prevents this.
func (j *JarvisBot) GoSafely(fn func()) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				stack := make([]byte, 1024*8)
				stack = stack[:runtime.Stack(stack, false)]

				j.log.Printf("PANIC: %s\n%s", err, stack)
			}
		}()

		fn()
	}()
}

// Ensure all buckets needed by jarvisbot are created.
func createAllBuckets(db *bolt.DB) error {
	// Check all buckets have been created
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(exchange_rate_bucket_name)
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
	return err
}

// Helper to parse incoming messages and return JarvisBot messages
func parseMessage(msg *telebot.Message) *message {
	msgTokens := strings.Split(msg.Text, " ")
	cmd, args := msgTokens[0], msgTokens[1:]
	return &message{Cmd: cmd, Args: args, Message: msg}
}