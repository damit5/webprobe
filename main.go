package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/EDDYCJY/gsema"
)

// http客户端
var HttpClient http.Client

// 参数
var Threads int
var URL string
var FILE string
var Proxy string
var Timeout int
var Debug bool

// 多线程信号量
var Semaphore *gsema.Semaphore

/*
用法
*/
func usage() {
	flag.IntVar(&Threads, "thread", 100, "线程")
	flag.StringVar(&URL, "url", "", "要检查的URL")
	flag.StringVar(&FILE, "file", "", "要检查的URL文件列表")
	flag.StringVar(&Proxy, "proxy", "", "代理，如socks5://127.0.0.1:1080")
	flag.IntVar(&Timeout, "timeout", 3, "超时时间")
	flag.BoolVar(&Debug, "debug", false, "显示错误信息")
	flag.Parse()

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n\n", os.Args[0])
		fmt.Printf("\nexamples:\n")
		fmt.Printf("\t %s -url baidu.com\n", os.Args[0])
		fmt.Printf("\t %s -file targets.txt\n", os.Args[0])
		fmt.Printf("\t %s -file targets.txt -thread 100 \n", os.Args[0])
		fmt.Println()
		flag.PrintDefaults()
	}

	if URL == "" && FILE == "" {
		flag.Usage()
		os.Exit(0)
	}
}

/*
初始化http客户端
*/
func initClient() {
	var tr *http.Transport
	if Proxy == "" {
		tr = &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // 忽略SSL证书
			DisableKeepAlives: true,
		}
	} else {
		proxy, _ := url.Parse(Proxy)
		tr = &http.Transport{
			Proxy:                 http.ProxyURL(proxy),
			MaxIdleConnsPerHost:   20,
			ResponseHeaderTimeout: time.Second * time.Duration(Timeout),
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true}, // 忽略SSL证书
			DisableKeepAlives:     true,
		}
	}

	HttpClient = http.Client{
		Timeout:   time.Second * time.Duration(Timeout),
		Transport: tr,
	}
}

/*
解析传入的目标，补全http，内部scanTarget()调用
*/
func parseTarget(uri string) []string {
	if strings.HasPrefix(uri, "http") {
		return []string{uri}
	} else {
		target1 := "http://" + uri
		target2 := "https://" + uri
		return []string{target1, target2}
	}
}

/*
发起GET请求，获取title，内部调用
*/
func doReq(uri string) {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		if Debug {
			fmt.Println(err)
		}
	} else {
		// header 设置
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.61 Safari/537.36")
		// 发起请求
		response, err := HttpClient.Do(req)

		if err != nil {
			if Debug {
				fmt.Println(err)
			}
		} else {
			// close body
			defer response.Body.Close()
			body := response.Body
			res, err := ioutil.ReadAll(body)
			if err != nil {
				if Debug {
					fmt.Println(err)
				}
			} else {
				// 匹配title
				htmlSource := string(res)
				bodyLen := len(htmlSource)
				titleRegex := regexp.MustCompile("(?i)<title>((?s).*?)</title>")
				titleList := titleRegex.FindStringSubmatch(htmlSource)
				var title string
				if len(titleList) == 2 {
					title = strings.Trim(titleList[1], " \r\n\t")
				} else {
					title = ""
				}
				fmt.Printf("%s %d %s\n", uri, bodyLen, title)
			}
		}
	}
	defer Semaphore.Done()
}

/*
扫描目标
*/
func scanTarget() error {
	if URL != "" { // 单目标
		for _, uri := range parseTarget(URL) {
			Semaphore.Add(1)
			doReq(uri)
		}
		Semaphore.Wait()
	} else if FILE != "" { // 多目标
		// 流式读取文件，避免大文件全部加载到内存
		f, err := os.Open(FILE)
		defer f.Close()
		if err != nil {
			return err
		}

		buf := bufio.NewReader(f)
		for {
			line, _, err := buf.ReadLine()

			if err != nil {
				if err == io.EOF {
					Semaphore.Wait()
					return nil
				}
				return err
			}

			target := string(line) // 每一个域名
			if strings.TrimSpace(target) != "" {
				// 开始探测
				for _, uri := range parseTarget(target) {
					if Debug {
						fmt.Println("testing url: ", uri)
					}
					Semaphore.Add(1)
					go doReq(uri) // 启动
				}
			}
		}
	}
	return nil
}

func main() {
	usage()
	initClient()
	Semaphore = gsema.NewSemaphore(Threads)
	scanTarget()
}
