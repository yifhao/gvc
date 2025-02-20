package vctrl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TwiN/go-color"
	config "github.com/moqsien/gvc/pkgs/confs"
	"github.com/moqsien/gvc/pkgs/utils"
	"github.com/moqsien/gvc/pkgs/utils/bkm"
	"github.com/moqsien/hackbrowser/browser"
	"github.com/moqsien/hackbrowser/item"
	"github.com/moqsien/hackbrowser/log"
)

type Browser struct {
	Conf *config.GVConfig
}

func NewBrowser() *Browser {
	return &Browser{
		Conf: config.New(),
	}
}

func (that *Browser) ShowSupportedBrowser() {
	bList := browser.ListBrowsers()
	fmt.Println(color.InYellow("Supported Browsers: "), color.InCyan(strings.Join(bList, "  ")))
}

func (that *Browser) ShowBackupPath() {
	fmt.Println(color.InYellow("Browser data restore dir: "), color.InCyan(config.GVCBackupDir))
}

func (that *Browser) isBrowserSupported(name string) bool {
	bList := browser.ListBrowsers()
	for _, bName := range bList {
		if bName == name {
			return true
		}
	}
	return false
}

func (that *Browser) getBrowser(browserName string) browser.Browser {
	log.SetVerbose()
	browsers, err := browser.PickBrowsers(browserName, "")
	if err != nil {
		log.Error(err)
		return nil
	}
	return browsers[0]
}

func (that *Browser) clearTempFiles() {
	fPathList := []string{
		item.TempChromiumKey,
		item.TempChromiumPassword,
		item.TempChromiumCookie,
		item.TempChromiumBookmark,
		item.TempChromiumHistory,
		item.TempChromiumDownload,
		item.TempChromiumCreditCard,
		item.TempChromiumLocalStorage,
		item.TempChromiumSessionStorage,
		item.TempChromiumExtension,
		item.TempYandexPassword,
		item.TempYandexCreditCard,
		item.TempFirefoxKey4,
		item.TempFirefoxPassword,
		item.TempFirefoxCookie,
		item.TempFirefoxBookmark,
		item.TempFirefoxHistory,
		item.TempFirefoxDownload,
		item.TempFirefoxLocalStorage,
		item.TempFirefoxSessionStorage,
		item.TempFirefoxCreditCard,
		item.TempFirefoxExtension,
	}
	for _, f := range fPathList {
		if ok, _ := utils.PathIsExist(f); ok && f != "" {
			os.RemoveAll(f)
		}
	}
}

func (that *Browser) Save(browserName string, toPush bool) {
	if !that.isBrowserSupported(browserName) {
		fmt.Println(color.InRed("unsupported browser!"))
		return
	}

	itemsToSave := []item.Item{
		item.FirefoxPassword,
		item.ChromiumPassword,
		item.YandexPassword,
		item.FirefoxExtension,
		item.ChromiumExtension,
	}

	b := that.getBrowser(browserName)
	if b == nil {
		return
	}
	b.OnlyToSave(itemsToSave)
	data, err := b.BrowsingData(true)
	if err != nil {
		log.Error(err)
	}
	data.Output(config.GVCBackupDir, b.Name(), "json")

	b.CopyBookmark()

	bType := bkm.Chrome
	copyPath := item.TempChromiumBookmark
	if browserName == "firefox" {
		bType = bkm.Firefox
		copyPath = item.TempFirefoxBookmark
	}
	toSavePath := filepath.Join(config.GVCBackupDir, fmt.Sprintf("%s_bookmarks.html", browserName))
	n := bkm.NewBkmTree(bType, copyPath, toSavePath)
	n.SaveHtml()
	if toPush {
		vconf := NewGVCWebdav()
		vconf.Push()
	}
	that.clearTempFiles()
}

func (that *Browser) PullData() {
	vconf := NewGVCWebdav()
	vconf.Pull()
}
