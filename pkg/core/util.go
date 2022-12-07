package core

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	tele "gopkg.in/telebot.v3"
	"mvdan.cc/xurls/v2"
)

var regexAlphanum = regexp.MustCompile(`[a-zA-Z0-9_]+`)

func checkTitle(t string) bool {
	if len(t) > 128 || len(t) == 0 {
		return false
	} else {
		return true
	}
}

func checkID(s string) bool {
	maxL := 60 - len(botName)
	if len(s) < 1 || len(s) > maxL {
		return false
	}
	if _, err := strconv.Atoi(s[:1]); err == nil {
		return false
	}
	if strings.Contains(s, "__") {
		return false
	}
	if strings.Contains(s, " ") {
		return false
	}

	return true
}

func secHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func secNum(n int) string {
	numbers := ""
	for i := 0; i < n; i++ {
		randInt, _ := rand.Int(rand.Reader, big.NewInt(10))
		numbers += randInt.String()
	}
	return numbers
}

func findLink(s string) string {
	rx := xurls.Strict()
	return rx.FindString(s)
}

func findLinkWithType(s string) (string, string) {
	rx := xurls.Strict()
	link := rx.FindString(s)
	if link == "" {
		return "", ""
	}

	u, _ := url.Parse(link)
	host := u.Host

	if host == "t.me" {
		host = LINK_TG
	} else if strings.HasSuffix(host, "line.me") {
		host = LINK_IMPORT
	} else if strings.HasSuffix(host, "e.kakao.com") {
		host = LINK_IMPORT
	}

	log.Debugf("link parsed: link=%s, host=%s", link, host)
	return link, host
}

func httpDownload(link string, f string) error {
	res, err := http.Get(link)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	fp, _ := os.Create(f)
	defer fp.Close()
	_, err = io.Copy(fp, res.Body)
	return err
}

func httpGet(link string) (string, error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", link, nil)
	req.Header.Set("User-Agent", "curl/7.61.1")
	req.Header.Set("Accept-Language", "zh-Hant;q=0.9, ja;q=0.8, en;q=0.7")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	content, _ := io.ReadAll(res.Body)

	return string(content), nil
}

func httpPost(link string, data string) (string, error) {
	bdata := []byte(data)
	req, err := http.Post(link, "Content-Type: text/plain",
		bytes.NewBuffer(bdata))
	if err != nil {
		return "", err
	}

	resbody := req.Body
	res, err := io.ReadAll(resbody)
	if err != nil {
		return "", err
	}
	return string(res), nil
}

func findEmojis(s string) string {
	r, _ := httpPost("http://127.0.0.1:5000", s)
	return r
}

// func findEmojis(s string) string {
// 	var eString string
// 	gomojis := gomoji.FindAll(s)
// 	for _, e := range gomojis {
// 		eString += e.Character
// 	}
// 	return eString
// 	// r := ""
// 	// emoji.ReplaceAllEmojiFunc(s, func(emoji string) string {
// 	// 	r += emoji
// 	// 	return ""
// 	// })
// 	// return r
// }

func sanitizeCallback(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		log.Debug("Sanitizing callback data...")
		c.Callback().Data = regexAlphanum.FindString(c.Callback().Data)

		log.Debugln("now:", c.Callback().Data)
		return next(c)
	}
}
func autoRespond(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		if c.Callback() != nil {
			defer c.Respond()
		}
		return next(c)
	}
}

func escapeTagMark(s string) string {
	s = strings.ReplaceAll(s, "<", "＜")
	s = strings.ReplaceAll(s, ">", "＞")
	return s
}

func getSIDFromMessage(m *tele.Message) string {
	if m.Sticker != nil {
		return m.Sticker.SetName
	}

	link := findLink(m.Text)
	return path.Base(link)
}

func retrieveSSDetails(c tele.Context, id string, sd *StickerData) error {
	ss, err := c.Bot().StickerSet(id)
	if err != nil {
		return err
	}
	sd.stickerSet = ss
	sd.title = ss.Title
	sd.id = ss.Name
	sd.cAmount = len(ss.Stickers)
	sd.isVideo = ss.Video
	return nil
}

func GetUd(uidS string) (*UserData, error) {
	uid, err := strconv.ParseInt(uidS, 10, 64)
	if err != nil {
		return nil, err
	}
	ud, ok := users.data[uid]
	if ok {
		return ud, nil
	} else {
		return nil, errors.New("no such user in state")
	}
}

func sliceMove[T any](oldIndex int, newIndex int, s []T) []T {
	originalS := s
	element := s[oldIndex]

	if oldIndex > newIndex {
		if len(s)-1 == oldIndex {
			s = s[0 : len(s)-1]
		} else {
			s = append(s[0:oldIndex], s[oldIndex+1:]...)
		}
		s = append(s[:newIndex], append([]T{element}, s[newIndex:]...)...)
	} else if oldIndex < newIndex {
		s = append(s[0:oldIndex], s[oldIndex+1:]...)
		newIndex = newIndex + 1
		s = append(s[:newIndex], append([]T{element}, s[newIndex:]...)...)
	} else {
		return originalS
	}
	return s
}

func chunkSlice(slice []string, chunkSize int) [][]string {
	var chunks [][]string
	for {
		if len(slice) == 0 {
			break
		}

		if len(slice) < chunkSize {
			chunkSize = len(slice)
		}

		chunks = append(chunks, slice[0:chunkSize])
		slice = slice[chunkSize:]
	}
	return chunks
}