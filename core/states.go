package core

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/star-39/moe-sticker-bot/pkg/msbimport"
	tele "gopkg.in/telebot.v3"
)

// Handle conversation state during a command.
func handleMessage(c tele.Context) error {
	var err error
	command, state := getState(c)
	if command == "" {
		return handleNoSession(c)
	}
	switch command {
	case "import":
		switch state {
		case "waitSTitle":
			err = waitSTitle(c)
		case "waitEmojiChoice":
			err = waitEmojiChoice(c)
		case "process":
			err = stateProcessing(c)
		case "waitSEmojiAssign":
			err = waitSEmojiAssign(c)
		}
	case "create":
		switch state {
		case "waitSType":
			err = waitSType(c)
		case "waitSTitle":
			err = waitSTitle(c)
		case "waitSID":
			err = waitSID(c)
		case "waitSFile":
			err = waitSFile(c)
		case "waitEmojiChoice":
			err = waitEmojiChoice(c)
		case "waitSEmojiAssign":
			err = waitSEmojiAssign(c)
		case "process":
			err = stateProcessing(c)
		}
	case "manage":
		switch state {
		case "waitSManage":
			err = statePrepareSManage(c)
		case "waitCbEditChoice":
			err = waitCbEditChoice(c)
		case "waitSFile":
			err = waitSFile(c)
		case "waitEmojiChoice":
			err = waitEmojiChoice(c)
		case "waitSEmojiAssign":
			err = waitSEmojiAssign(c)
		case "waitSDel":
			err = waitSDel(c)
		case "waitCbDelset":
			err = waitCbDelset(c)
		case "process":
			err = stateProcessing(c)
		}

	// case "register":
	// 	switch state {
	// 	case "waitRegLineLink":
	// 		err = waitRegLineLink(c)
	// 	case "waitRegS":
	// 		err = waitRegS(c)
	// 	}
	case "search":
		switch state {
		case "waitSearchKW":
			err = waitSearchKeyword(c)
		}
	}
	return err
}

// Received bare message without using a command.
func handleNoSession(c tele.Context) error {
	log.Debugf("user %d entered no session with message: %s", c.Sender().ID, c.Message().Text)

	//During previous stage, bot will reply to a message with callback buttons.
	//Now we react to user's choice.
	if c.Callback() != nil && c.Message().ReplyTo != nil {
		switch c.Callback().Data {
		case CB_DN_SINGLE:
			return downloadStickersAndSend(c.Message().ReplyTo.Sticker, "", c)
		case CB_DN_WHOLE:
			id := getSIDFromMessage(c.Message().ReplyTo)
			return downloadStickersAndSend(nil, id, c)
		case CB_MANAGE:
			return statePrepareSManage(c)
		case CB_OK_IMPORT:
			return confirmImport(c)
		case CB_OK_DN:
			ud := initUserData(c, "download", "process")
			c.Send("Please wait...")
			msbimport.ParseImportLink(findLink(c.Message().ReplyTo.Text), ud.lineData)
			return downloadLineSToZip(c, ud)
		case CB_EXPORT_WA:
			hex := secHex(6)
			id := getSIDFromMessage(c.Message().ReplyTo)
			ss, _ := c.Bot().StickerSet(id)
			go prepareWebAppExportStickers(ss, hex)
			return sendConfirmExportToWA(c, id, hex)
		case CB_BYE:
			return c.Send("Bye. /start")
		}
	}

	// bare sticker, ask user's choice.
	if c.Message().Sticker != nil {
		sn := c.Message().Sticker.SetName
		if matchUserS(c.Sender().ID, c.Message().Sticker.SetName) {
			return sendAskSChoice(c, sn)
		} else {
			return sendAskSDownloadChoice(c, sn)
		}
	}

	if c.Message().Animation != nil {
		return downloadGifToZip(c)
	}

	// bare text message, expect a link, if no link, search keyword.
	link, tp := findLinkWithType(c.Message().Text)

	switch tp {
	case LINK_TG:
		if matchUserS(c.Sender().ID, path.Base(link)) {
			return sendAskTGLinkChoice(c)
		} else {
			return sendAskWantSDown(c)
		}
	case LINK_IMPORT:
		ld := &msbimport.LineData{}
		warn, err := msbimport.ParseImportLink(link, ld)
		if err != nil {
			return sendBadImportLinkWarn(c)
		}
		if warn != "" {
			sendNeedKakaoAnimatedShareLinkWarning(c)
		}

		sendNotifySExist(c, ld.Id)
		return sendAskWantImportOrDownload(c)

	default:
		// User sent plain text, attempt to search.
		if trySearchKeyword(c) {
			return sendNotifyNoSessionSearch(c)
		} else {
			return sendNoSessionWarning(c)
		}
	}
}

func confirmImport(c tele.Context) error {
	ud := initUserData(c, "import", "waitSTitle")
	_, err := msbimport.ParseImportLink(findLink(c.Message().ReplyTo.Text), ud.lineData)
	if err != nil {
		return err
	}
	ud.stickerData.id = sanitizeLineID(ud.lineData.Category + ud.lineData.Id + secHex(2) + "_by_" + botName)
	workDir := filepath.Join(ud.workDir, ud.lineData.Id)
	sendAskTitle_Import(c)
	ud.wg.Add(1)
	err = msbimport.PrepareImportStickers(ud.ctx, ud.lineData, workDir, true)
	ud.wg.Done()
	if err != nil {
		return err
	}
	ud.stickerData.lAmount = ud.lineData.Amount
	ud.stickerData.isVideo = ud.lineData.IsAnimated

	//After PrepareImportStickers returns, individual LineFile might not be ready yet.
	//When transfering data to ud.stickerData.stickers, make sure transfer finished data only.
	for range ud.lineData.Files {
		sf := &StickerFile{}
		sf.wg.Add(1)
		ud.stickerData.stickers = append(ud.stickerData.stickers, sf)
	}
	for i, lf := range ud.lineData.Files {
		lf.Wg.Wait()
		ud.stickerData.stickers[i].wg.Done()
		ud.stickerData.stickers[i].oPath = lf.OriginalFile
		ud.stickerData.stickers[i].cPath = lf.ConvertedFile
	}
	return nil
}

func trySearchKeyword(c tele.Context) bool {
	keywords := strings.Split(c.Text(), " ")
	lines := searchLineS(keywords)
	if len(lines) == 0 {
		return false
	}
	sendSearchResult(20, lines, c)
	return true
}

func stateProcessing(c tele.Context) error {
	if c.Callback() != nil {
		if c.Callback().Data == "bye" {
			return cmdQuit(c)
		}
	}
	return c.Send("processing, please wait... 作業中, 請稍後... /quit")
}

func statePrepareSManage(c tele.Context) error {
	var ud *UserData
	if c.Callback() != nil && c.Message().ReplyTo != nil {
		if c.Callback().Data == CB_MANAGE {
			ud = initUserData(c, "manage", "waitCbEditChoice")
			id := getSIDFromMessage(c.Message().ReplyTo)
			ud.stickerData.id = id
		}
	} else {
		ud = users.data[c.Sender().ID]
		if c.Message().Sticker != nil {
			ud.stickerData.sticker = c.Message().Sticker
			ud.stickerData.id = c.Message().Sticker.SetName
		} else {
			link, tp := findLinkWithType(c.Message().Text)
			if tp != LINK_TG {
				return c.Send("Send telegram sticker or link or /quit")
			}
			ud.stickerData.id = path.Base(link)
		}
	}
	ud.lastContext = c
	// Allow admin to manage all sticker sets.
	if c.Sender().ID == Config.AdminUid {
		goto NEXT
	}
	if !matchUserS(c.Sender().ID, ud.stickerData.id) {
		return c.Send("Sorry, this sticker set cannot be edited. try another or /quit")
	}

NEXT:
	err := retrieveSSDetails(c, ud.stickerData.id, ud.stickerData)
	if err != nil {
		return c.Send("bad sticker set! try again or /quit")
	}
	err = prepareWebAppEditStickers(users.data[c.Sender().ID], false)
	if err != nil {
		return c.Send("error preparing stickers for webapp /quit")
	}
	if (ud.stickerData.isVideo && ud.stickerData.cAmount == 50) ||
		(ud.stickerData.cAmount == 120) {
		sendStickerSetFullWarning(c)
	}
	setState(c, "waitCbEditChoice")
	return sendAskEditChoice(c)
}

func waitCbEditChoice(c tele.Context) error {
	if c.Callback() == nil {
		return sendNoCbWarn(c)
	}

	switch c.Callback().Data {
	case "add":
		setState(c, "waitSFile")
		return sendAskStickerFile(c)
	case "del":
		setState(c, "waitSDel")
		return sendAskSDel(c)
	case "delset":
		setState(c, "waitCbDelset")
		return sendConfirmDelset(c)
	case "bye":
		endManageSession(c)
		terminateSession(c)
	}
	return nil
}

func waitSDel(c tele.Context) error {
	ud := users.data[c.Sender().ID]
	if c.Message().Sticker == nil {
		return c.Send("send sticker! try again or /quit")
	}
	if c.Message().Sticker.SetName != ud.stickerData.id {
		return c.Send("wrong sticker! try again or /quit")
	}

	err := c.Bot().DeleteSticker(c.Message().Sticker.FileID)
	if err != nil {
		c.Send("error deleting sticker! try another one or /quit")
		return err
	}
	c.Send("Delete OK. 成功刪除一張貼圖。")
	ud.stickerData.cAmount--
	if ud.stickerData.cAmount == 0 {
		deleteUserS(ud.stickerData.id)
		deleteLineS(ud.stickerData.id)
		terminateSession(c)
		return nil
	} else {
		setState(c, "waitCbEditChoice")
		return sendAskEditChoice(c)
	}
}

func waitCbDelset(c tele.Context) error {
	if c.Callback() == nil {
		setState(c, "waitCbEditChoice")
		return sendAskEditChoice(c)
	}
	if c.Callback().Data != CB_YES {
		setState(c, "waitCbEditChoice")
		return sendAskEditChoice(c)
	}
	ud := users.data[c.Sender().ID]
	setState(c, "process")
	c.Send("please wait...")

	ss, _ := c.Bot().StickerSet(ud.stickerData.id)
	for _, s := range ss.Stickers {
		c.Bot().DeleteSticker(s.FileID)
	}
	deleteUserS(ud.stickerData.id)
	deleteLineS(ud.stickerData.id)
	c.Send("Delete set OK. bye")
	endManageSession(c)
	terminateSession(c)
	return nil
}

func waitSType(c tele.Context) error {
	if c.Callback() == nil {
		return c.Send("Please press a button.")
	}

	if strings.Contains(c.Callback().Data, "video") {
		users.data[c.Sender().ID].stickerData.isVideo = true
	}

	sendAskTitle(c)
	setState(c, "waitSTitle")
	return nil
}

func waitSFile(c tele.Context) error {
	if c.Callback() != nil {
		switch c.Callback().Data {
		case CB_DONE_ADDING:
			goto NEXT
		case CB_BYE:
			terminateSession(c)
			return nil
		default:
			return sendPromptStopAdding(c)
		}
	}
	if c.Message().Media() != nil {
		err := appendMedia(c)
		if err != nil {
			c.Reply("Failed processing this file. 處理此檔案時錯誤:\n" + err.Error())
		}
		return nil
	} else {
		return sendPromptStopAdding(c)
	}
NEXT:
	if len(users.data[c.Sender().ID].stickerData.stickers) == 0 {
		return c.Send("No image received. try again or /quit")
	}

	setState(c, "waitEmojiChoice")
	sendAskEmoji(c)

	return nil
}

func waitSTitle(c tele.Context) error {
	ud := users.data[c.Sender().ID]
	command := ud.command

	if c.Callback() == nil {
		ud.stickerData.title = c.Message().Text
	} else {
		// do not expect callback in /create
		if command == "create" {
			return nil
		}
		titleIndex, atoiErr := strconv.Atoi(c.Callback().Data)
		if atoiErr == nil && titleIndex != -1 {
			ud.stickerData.title = ud.lineData.I18nTitles[titleIndex] + " @" + botName
		} else {
			ud.stickerData.title = ud.lineData.Title + " @" + botName
		}
	}

	if !checkTitle(ud.stickerData.title) {
		return c.Send("bad title! try again or /quit")
	}

	switch command {
	case "import":
		setState(c, "waitEmojiChoice")
		return sendAskEmoji(c)
	case "create":
		setState(c, "waitSID")
		sendAskID(c)
	}

	return nil
}

func waitSID(c tele.Context) error {
	var id string
	if c.Callback() != nil {
		if c.Callback().Data == "auto" {
			users.data[c.Sender().ID].stickerData.id = "sticker_" + secHex(4) + "_by_" + botName
			goto NEXT
		}
	}

	id = regexAlphanum.FindString(c.Message().Text)
	if !checkID(id) {
		return sendBadIDWarn(c)
	}
	id = id + "_by_" + botName
	if _, err := c.Bot().StickerSet(id); err == nil {
		return sendIDOccupiedWarn(c)
	}
	users.data[c.Sender().ID].stickerData.id = id

NEXT:
	setState(c, "waitSFile")
	return sendAskStickerFile(c)
}

func waitEmojiChoice(c tele.Context) error {
	ud := users.data[c.Sender().ID]
	if c.Callback() != nil {
		switch c.Callback().Data {
		case "random":
			users.data[c.Sender().ID].stickerData.emojis = []string{"⭐"}
		case "manual":
			sendProcessStarted(ud, c, "preparing...")
			setState(c, "waitSEmojiAssign")
			return sendAskEmojiAssign(c)
		default:
			return nil
		}
	} else {
		emojis := findEmojis(c.Message().Text)
		if emojis == "" {
			return c.Send("Send emoji or press button!")
		}
		users.data[c.Sender().ID].stickerData.emojis = []string{emojis}
	}

	setState(c, "process")

	err := execAutoCommit(!(ud.command == "manage"), c)
	endSession(c)
	if err != nil {
		return err
	}
	return nil
}

func waitSEmojiAssign(c tele.Context) error {
	emojis := findEmojis(c.Message().Text)
	if emojis == "" {
		return c.Send("send emoji! try again or /quit")
	}
	return execEmojiAssign(!(users.data[c.Sender().ID].command == "manage"), emojis, c)
}

func waitSearchKeyword(c tele.Context) error {
	keywords := strings.Split(c.Text(), " ")
	lines := searchLineS(keywords)
	if len(lines) == 0 {
		return sendSearchNoResult(c)
	}
	sendSearchResult(-1, lines, c)
	terminateSession(c)
	return nil
}