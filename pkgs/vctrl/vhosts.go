package vctrl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/TwiN/go-color"
	"github.com/go-resty/resty/v2"
	config "github.com/moqsien/gvc/pkgs/confs"
	"github.com/moqsien/gvc/pkgs/query"
	"github.com/moqsien/gvc/pkgs/utils"
)

const (
	HEAD        = "# FromGhosts Start"
	TAIL        = "# FromGhosts End"
	TIME        = "# UpdateTime: %s"
	LinePattern = "%s\t\t\t%s # %s"
)

type host struct {
	IP     string        // ip address
	AvgRTT time.Duration // average RTT
}

type hostList map[string]host // key: host name, value: host

type Hosts struct {
	Conf     *config.GVConfig
	filePath string
	rawList  map[string]string
	hList    hostList
	lineReg  *regexp.Regexp
	hostReg  *regexp.Regexp
	lock     *sync.Mutex
	wg       sync.WaitGroup
	fetcher  *query.Fetcher
}

func NewHosts() *Hosts {
	conf := config.New()
	lineReg := `((2(5[0-5]|[0-4]\d))|[0-1]?\d{1,2})(\.((2(5[0-5]|[0-4]\d))|[0-1]?\d{1,2})){3}`
	hostsReg := fmt.Sprintf(`%s[\s\S]*%s`, HEAD, TAIL)
	return &Hosts{
		Conf:     conf,
		filePath: config.GetHostsFilePath(),
		rawList:  make(map[string]string, 200),
		hList:    make(hostList, 200),
		lineReg:  regexp.MustCompile(lineReg),
		hostReg:  regexp.MustCompile(hostsReg),
		lock:     &sync.Mutex{},
		wg:       sync.WaitGroup{},
		fetcher:  query.NewFetcher(),
	}
}

func (that *Hosts) extractHostUrl(text, ip string) string {
	raw := strings.Replace(text, ip, "", -1)
	return strings.TrimSpace(raw)
}

func (that *Hosts) ParseHosts(content []byte) {
	sc := bufio.NewScanner(strings.NewReader(string(content)))
	for sc.Scan() {
		text := sc.Text()
		ipList := that.lineReg.FindAllString(text, -1)
		if len(ipList) == 1 {
			ip_ := ipList[0]
			url := that.extractHostUrl(text, ip_)
			if url == "" {
				continue
			}
			if _, ok := that.rawList[ip_]; !ok {
				that.rawList[ip_] = url
			}
		}
	}
}

func (that *Hosts) GetHosts() {
	resps := make(chan *resty.Response, 10)
	that.fetcher.Timeout = time.Duration(that.Conf.Hosts.ReqTimeout) * time.Second
	for _, url := range that.Conf.Hosts.SourceUrls {
		that.wg.Add(1)
		var url_ string = url
		go func() {
			defer that.wg.Done()
			that.fetcher.Url = url_
			resp := that.fetcher.Get()
			resps <- resp
		}()
	}
	that.wg.Wait()
	close(resps)
	for r := range resps {
		if r != nil {
			content, err := io.ReadAll(r.RawBody())
			r.RawBody().Close()
			if err != nil {
				fmt.Println(color.InRed("[Read Body Errored] "), err)
				return
			}
			that.ParseHosts(content)
		}
	}
}

func (that *Hosts) ReadAndBackupHosts(hPath, hBackupPath string) (content []byte) {
	var (
		err  error
		file *os.File
	)
	file, err = os.Open(hPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()
	content, err = io.ReadAll(file)
	if err != nil {
		fmt.Println(err)
		return
	}
	if utils.GetShell() == "win" {
		err = os.WriteFile(hBackupPath, content, 0644)
	} else {
		err = utils.CopyFileOnUnixSudo(hPath, hBackupPath)
	}
	if err != nil {
		fmt.Println(color.InRed("Hosts file backup failed: "), err)
		return
	}
	return
}

func (that *Hosts) replace(oldContent []byte, newHostStr string) string {
	old := string(oldContent)
	if strings.Contains(old, HEAD) {
		return that.hostReg.ReplaceAllString(old, newHostStr)
	} else {
		if newHostStr != "" {
			return fmt.Sprintf("%s\n%s", oldContent, newHostStr)
		}
		return old
	}
}

func (that *Hosts) FormatAndSaveHosts(oldContent []byte) {
	if len(that.hList) > 0 {
		lineList := []string{}
		for url, h := range that.hList {
			lineList = append(lineList, fmt.Sprintf(LinePattern, h.IP, url, h.AvgRTT))
		}
		if len(oldContent) < 1 {
			return
		}
		newHostStr := fmt.Sprintf("%s\n%s\n%s\n%s",
			HEAD,
			fmt.Sprintf(TIME, time.Now().Format("2006-01-02")),
			strings.Join(lineList, "\n"),
			TAIL)
		newStr := that.replace(oldContent, newHostStr)
		if newStr == "" {
			return
		}
		var err error
		if runtime.GOOS == utils.Windows {
			err = os.WriteFile(config.GetHostsFilePath(), []byte(newStr), 0666)
		} else {
			err = os.WriteFile(config.TempHostsFilePath, []byte(newStr), 0666)
			if err == nil {
				err = utils.CopyFileOnUnixSudo(config.TempHostsFilePath, config.GetHostsFilePath())
			}
		}
		if err != nil {
			fmt.Println(color.InRed("Write file errored: "), err)
			return
		}
		fmt.Println(color.InGreen("Succeeded!"))
	}
}

func (that *Hosts) Run() {
	that.GetHosts()
	hostFilePath := config.GetHostsFilePath()
	hostBackupFilePath := filepath.Join(filepath.Dir(hostFilePath), "hosts.backup")
	oldContent := that.ReadAndBackupHosts(hostFilePath, hostBackupFilePath)
	for ip, hUrl := range that.rawList {
		that.hList[hUrl] = host{IP: ip, AvgRTT: 0}
	}
	that.FormatAndSaveHosts(oldContent)
}

const (
	HostsCmd          = "hosts"
	HostsFileFetchCmd = "fetch"
	HostsFlagName     = "previlege"
)

var (
	HostsFetchBatPath = filepath.Join(config.GVCWorkDir, "hosts.bat")
)

func (that *Hosts) WinRunAsAdmin() {
	if ok, _ := utils.PathIsExist(HostsFetchBatPath); !ok {
		exePath, _ := os.Executable()
		content := fmt.Sprintf("%s %s %s --%s", exePath, HostsCmd, HostsFileFetchCmd, HostsFlagName)
		os.WriteFile(HostsFetchBatPath, []byte(content), 0777)
	}
	cmd := exec.Command("powershell", "Start-Process", "-verb", "runas", HostsFetchBatPath)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		fmt.Println(color.InRed("[update hosts file failed] "), err)
	}
	fmt.Println(color.InGreen("Succeeded!"))
}

func (that *Hosts) ShowFilePath() {
	fmt.Println(color.InGreen(fmt.Sprintf("HostsFile: %s", config.GetHostsFilePath())))
}
