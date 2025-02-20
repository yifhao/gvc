package vctrl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	color "github.com/TwiN/go-color"
	"github.com/mholt/archiver/v3"
	config "github.com/moqsien/gvc/pkgs/confs"
	"github.com/moqsien/gvc/pkgs/query"
	"github.com/moqsien/gvc/pkgs/utils"
	"github.com/moqsien/gvc/pkgs/utils/sorts"
)

var AllowedSuffixes = []string{
	".zip",
	".tar.gz",
	".tar.bz2",
	".tar.xz",
}

type JDKPackage struct {
	Url      string
	FileName string
	OS       string
	Arch     string
	Size     string
	Checksum string
}

type JDKVersion struct {
	Versions map[string][]*JDKPackage
	Doc      *goquery.Document
	Conf     *config.GVConfig
	fetcher  *query.Fetcher
	dir      string
	env      *utils.EnvsHandler
}

func NewJDKVersion() (jv *JDKVersion) {
	jv = &JDKVersion{
		Versions: make(map[string][]*JDKPackage, 100),
		Conf:     config.New(),
		fetcher:  query.NewFetcher(),
		env:      utils.NewEnvsHandler(),
	}
	jv.initeDirs()
	jv.env.SetWinWorkDir(config.GVCWorkDir)
	return
}

func (that *JDKVersion) initeDirs() {
	if ok, _ := utils.PathIsExist(config.DefaultJavaRoot); !ok {
		os.RemoveAll(config.DefaultJavaRoot)
		if err := os.MkdirAll(config.DefaultJavaRoot, os.ModePerm); err != nil {
			fmt.Println("[mkdir Failed] ", err)
		}
	}
	if ok, _ := utils.PathIsExist(config.JavaTarFilesPath); !ok {
		if err := os.MkdirAll(config.JavaTarFilesPath, os.ModePerm); err != nil {
			fmt.Println("[mkdir Failed] ", err)
		}
	}
	if ok, _ := utils.PathIsExist(config.JavaUnTarFilesPath); !ok {
		if err := os.MkdirAll(config.JavaUnTarFilesPath, os.ModePerm); err != nil {
			fmt.Println("[mkdir Failed] ", err)
		}
	}
}

func (that *JDKVersion) getDoc(isOfficial bool) {
	jUrl := that.Conf.Java.JDKUrl
	if isOfficial {
		jUrl = that.Conf.Java.CompilerUrl
	}
	if !utils.VerifyUrls(jUrl) {
		return
	}
	that.fetcher.Url = jUrl
	if resp := that.fetcher.Get(); resp != nil {
		that.Doc, _ = goquery.NewDocumentFromReader(resp.RawBody())
	}
	if that.Doc == nil {
		panic(fmt.Sprintf("Cannot parse html for %s", that.fetcher.Url))
	}
}

func (that *JDKVersion) GetSha(sUrl string) (res string) {
	that.fetcher.Url = sUrl
	resp := that.fetcher.Get()
	if resp != nil {
		res = string(resp.Body())
	}
	return
}

func (that *JDKVersion) GetFileSuffix(fName string) string {
	for _, k := range AllowedSuffixes {
		if strings.HasSuffix(fName, k) {
			return k
		}
	}
	return ""
}

func (that *JDKVersion) GetVersions(isOfficial bool) {
	if that.Doc == nil {
		that.getDoc(isOfficial)
	}
	if isOfficial {
		that.Doc.Find("ul.rw-inpagetabs").First().Find("li").Each(func(i int, s *goquery.Selection) {
			v, _ := s.Find("a").Attr("href")
			sList := strings.Split(v, "java")
			vn := sList[len(sList)-1]
			that.Doc.Find(fmt.Sprintf("div#java%s", vn)).After("nav").Find("table").Find("tbody").Find("tr").Each(func(i int, s *goquery.Selection) {
				if i == 0 {
					return
				}
				tArchive := strings.ToLower(s.Find("td").Eq(0).Text())
				tArchive = strings.ReplaceAll(tArchive, " ", "")
				tSize := s.Find("td").Eq(1).Text()
				tUrl, _ := s.Find("td").Eq(2).Find("a").Eq(0).Attr("href")
				tSha, _ := s.Find("td").Eq(2).Find("a").Eq(1).Attr("href")
				if !strings.Contains(tArchive, "archive") {
					return
				}
				p := &JDKPackage{}
				p.Arch = utils.ParseArch(tUrl)
				p.OS = utils.ParsePlatform(tUrl)
				if p.Arch == "" || p.OS == "" || tUrl == "" {
					return
				}
				p.Size = tSize
				p.Url = tUrl
				if suffix := that.GetFileSuffix(p.Url); suffix != "" {
					p.FileName = fmt.Sprintf("jdk%s-%s_%s%s", vn, p.OS, p.Arch, suffix)
				} else {
					return
				}
				p.Checksum = that.GetSha(tSha)
				key := fmt.Sprintf("jdk%s", vn)
				that.Versions[key] = append(that.Versions[key], p)
			})
		})
	} else {
		that.Doc.Find("div#oracle-jdk").Find("div.col-sm-3").Each(func(i int, s *goquery.Selection) {
			vName := strings.ToLower(s.Find("span").Text())
			vName = strings.ReplaceAll(vName, "\n", "")
			vName = strings.ReplaceAll(vName, "\r", "")
			vName = strings.ReplaceAll(vName, " ", "")
			vName = strings.ReplaceAll(vName, "(lts)", "-lts")
			s.Find("li").Each(func(i int, ss *goquery.Selection) {
				if strings.Contains(vName, "jdk8") {
					return
				}
				p := &JDKPackage{}
				fileName := strings.ReplaceAll(strings.ToLower(ss.Find("a").Text()), " ", "")
				p.Arch = utils.ParseArch(fileName)
				p.OS = utils.ParsePlatform(fileName)
				if p.Arch == "" || p.OS == "" {
					return
				}
				if suffix := that.GetFileSuffix(fileName); suffix != "" {
					p.FileName = fmt.Sprintf("%s-%s_%s%s", vName, p.OS, p.Arch, suffix)
				} else {
					return
				}
				p.Url = strings.ReplaceAll(ss.Find("a").AttrOr("href", ""), " ", "")
				if p.Url == "" {
					return
				}
				that.Versions[vName] = append(that.Versions[vName], p)
			})
		})

		that.Doc.Find("#Kona").Find("div.col-sm-3").Each(func(i int, s *goquery.Selection) {
			vName := strings.ToLower(s.Find("span").Text())
			vName = strings.ReplaceAll(vName, "\n", "")
			vName = strings.ReplaceAll(vName, "\r", "")
			vName = strings.ReplaceAll(vName, " ", "")
			vName = strings.ReplaceAll(vName, "(lts)", "-lts")
			s.Find("li").Each(func(i int, ss *goquery.Selection) {
				if !strings.Contains(vName, "jdk8") {
					return
				}
				if !strings.Contains(vName, "lts") {
					vName = fmt.Sprintf("%s-%s", vName, "lts")
				}
				p := &JDKPackage{}
				fileName := strings.ReplaceAll(strings.ToLower(ss.Find("a").Text()), " ", "")
				p.Arch = utils.ParseArch(fileName)
				p.OS = utils.ParsePlatform(fileName)
				if p.Arch == "" || p.OS == "" {
					return
				}
				if suffix := that.GetFileSuffix(fileName); suffix != "" {
					p.FileName = fmt.Sprintf("%s-%s_%s%s", vName, p.OS, p.Arch, suffix)
				} else {
					return
				}
				p.Url = strings.ReplaceAll(ss.Find("a").AttrOr("href", ""), " ", "")
				if p.Url == "" {
					return
				}
				that.Versions[vName] = append(that.Versions[vName], p)
			})
		})
	}
}

func (that *JDKVersion) ShowVersions(isOfficial bool) {
	that.GetVersions(isOfficial)
	vList := []string{}
	for k := range that.Versions {
		vList = append(vList, k)
	}
	vList = sorts.SortJDKVersion(vList)
	fmt.Println(color.InGreen(strings.Join(vList, " ")))
}

func (that *JDKVersion) findVersion(version string) (p *JDKPackage) {
	var pList []*JDKPackage
	for k, v := range that.Versions {
		if strings.Contains(k, version) {
			pList = v
			break
		}
	}
	if len(pList) > 0 {
		for _, p := range pList {
			if p.Arch == runtime.GOARCH && p.OS == runtime.GOOS {
				return p
			}
		}
	}
	return
}

func (that *JDKVersion) download(version string, isOfficial bool) (r string) {
	that.GetVersions(isOfficial)

	if p := that.findVersion(version); p != nil {
		that.fetcher.Url = p.Url
		that.fetcher.Timeout = 100 * time.Minute
		fpath := filepath.Join(config.JavaTarFilesPath, p.FileName)
		if size := that.fetcher.GetAndSaveFile(fpath); size > 0 {
			if p.Checksum != "" {
				if ok := utils.CheckFile(fpath, "sha256", p.Checksum); ok {
					return fpath
				} else {
					os.RemoveAll(fpath)
				}
			} else {
				return fpath
			}
		} else {
			os.RemoveAll(fpath)
		}
	} else {
		fmt.Println(color.InRed("Invalid jdk version: "), color.InYellow(version))
		fmt.Println(color.InCyan("Versions available: "))
		that.ShowVersions(isOfficial)
	}
	return
}

func (that *JDKVersion) CheckAndInitEnv() {
	if runtime.GOOS != utils.Windows {
		javaEnv := fmt.Sprintf(utils.JavaEnv, config.DefaultJavaRoot)
		that.env.UpdateSub(utils.SUB_JDK, javaEnv)
	} else {
		classPath := filepath.Join(config.DefaultJavaRoot, "lib")
		envList := map[string]string{
			"JAVA_HOME":  config.DefaultJavaRoot,
			"CLASS_PATH": filepath.Join(config.DefaultJavaRoot, "lib"),
			"PATH": fmt.Sprintf("%s;%s;%s", filepath.Join(config.DefaultJavaRoot, "bin"),
				filepath.Join(classPath, "tools.jar"), filepath.Join(classPath, "dt.jar")),
		}
		that.env.SetEnvForWin(envList)
	}
}

func (that *JDKVersion) findDir(untarfile string) {
	if rd, err := os.ReadDir(untarfile); err == nil {
		for _, d := range rd {
			if d.IsDir() && d.Name() == "bin" {
				that.dir = untarfile
			} else if d.IsDir() {
				that.findDir(filepath.Join(untarfile, d.Name()))
			}
		}
	}
}

func (that *JDKVersion) UseVersion(version string, isOfficial bool) {
	untarfile := filepath.Join(config.JavaUnTarFilesPath, version)
	if ok, _ := utils.PathIsExist(untarfile); !ok {
		if tarfile := that.download(version, isOfficial); tarfile != "" {
			if err := archiver.Unarchive(tarfile, untarfile); err != nil {
				os.RemoveAll(untarfile)
				fmt.Println(color.InRed("[Unarchive failed] "), err)
				return
			}
		}
	}
	if ok, _ := utils.PathIsExist(config.DefaultJavaRoot); ok {
		os.RemoveAll(config.DefaultJavaRoot)
	}
	that.findDir(untarfile)
	if that.dir == "" {
		fmt.Println(color.InRed("[Can not find binaries] "), untarfile)
		return
	}

	if err := utils.MkSymLink(that.dir, config.DefaultJavaRoot); err != nil {
		fmt.Println(color.InRed("[Create link failed] "), err)
		return
	}
	if !that.env.DoesEnvExist(utils.SUB_JDK) {
		that.CheckAndInitEnv()
	}
	fmt.Println(color.InGreen(fmt.Sprintf("Use %s succeeded!", version)))
}

func (that *JDKVersion) getCurrent() (version string) {
	fpath := filepath.Join(config.DefaultJavaRoot, "release")
	content, _ := os.ReadFile(fpath)
	if len(content) == 0 {
		return
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.Contains(line, "JAVA_VERSION=") {
			version = strings.ReplaceAll(strings.Split(line, "=")[1], `"`, "")
			version = strings.Split(version, ".")[0]
			version = fmt.Sprintf("jdk%s", version)
		}
	}
	version = strings.TrimSpace(version)
	return
}

func (that *JDKVersion) ShowInstalled() {
	current := that.getCurrent()
	dList, _ := os.ReadDir(config.JavaUnTarFilesPath)
	for _, d := range dList {
		if !strings.Contains(d.Name(), "jdk") {
			continue
		}
		if current == strings.TrimSpace(d.Name()) {
			fmt.Println(color.InYellow(fmt.Sprintf("%s <Current>", d.Name())))
		} else {
			fmt.Println(color.InCyan(d.Name()))
		}
	}
}

func (that *JDKVersion) removeTarFile(version string) {
	fNameStr := fmt.Sprintf("%s-%s_%s", version, runtime.GOOS, runtime.GOARCH)
	fNameStr1 := fmt.Sprintf("%s-lts-%s_%s", version, runtime.GOOS, runtime.GOARCH)
	dList, _ := os.ReadDir(config.JavaTarFilesPath)
	for _, d := range dList {
		if strings.Contains(d.Name(), fNameStr) || strings.Contains(d.Name(), fNameStr1) {
			os.RemoveAll(filepath.Join(config.JavaTarFilesPath, d.Name()))
		}
	}
}

func (that *JDKVersion) RemoveVersion(version string) {
	if !strings.HasPrefix(version, "jdk") {
		version = fmt.Sprintf("jdk%s", version)
	}
	current := that.getCurrent()
	if version != current {
		os.RemoveAll(filepath.Join(config.JavaUnTarFilesPath, version))
		that.removeTarFile(version)
	}
}

func (that *JDKVersion) RemoveUnused() {
	current := that.getCurrent()
	dList, _ := os.ReadDir(config.JavaUnTarFilesPath)
	for _, d := range dList {
		fmt.Println(d.Name())
		if current != d.Name() && strings.Contains(d.Name(), "jdk") {
			os.RemoveAll(filepath.Join(config.JavaUnTarFilesPath, d.Name()))
			that.removeTarFile(d.Name())
		}
	}
}
